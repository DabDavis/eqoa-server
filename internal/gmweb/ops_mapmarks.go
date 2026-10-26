package gmweb

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/mapmarks"
)

// Community map marks (the user, 2026-10-26: "Players can submit waypoints to be
// added to the in-game map. Submissions would require official approval, similar
// to our spellfx"). Players send from eqoa-rust-client-vanilla's map, on any game
// server and with no sign-in (internal/mapmarks), a place with a name and a note;
// the suggestions wait in mapmark_suggestions, and a GM accepts one (an audited
// insert into mapmarks, the name, note, kind and place the GM's to change first,
// revertible as any row change) or dismisses it. mapmarks is the list every map
// draws, served by the public route; a GM may also add, edit or delete its rows
// through the row editor. A suggestion's own status is bookkeeping, not audited.
// A sender is known only by a salted hash of its address (its first 8
// characters shown), which clear_map_suggestions takes to sweep a spammer's
// waiting rows away.

// nearMark is how far (world units) an accepted mark is listed beside a
// suggestion, to spot one already on the maps.
const nearMark = 25.0

// markJSON is a mark as the ops answer it.
func markJSON(m database.MapMark) map[string]any {
	return map[string]any{
		"id": m.ID, "world": m.World, "x": round1f(m.X), "z": round1f(m.Z), "name": m.Name, "note": m.Note,
		"kind": m.Kind, "source": m.Source, "suggestion_id": m.SuggestionID, "created": m.Created, "updated": m.Updated,
	}
}

func worldNameOf(world int) string {
	names := handlers.WorldNames()
	if world >= 0 && world < len(names) {
		return names[world]
	}
	return fmt.Sprintf("World %d", world)
}

// mapSuggestions is map_suggestions' answer.
func (s *Server) mapSuggestions(status string, world, limit int) (any, error) {
	rows, err := s.d.DB.MapMarkSuggestions(status, world)
	if err != nil {
		return nil, err
	}
	marks, err := s.d.DB.MapMarks(world)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		if limit > 0 && len(out) >= limit {
			break
		}
		near := []map[string]any{}
		for _, m := range marks {
			if m.World != r.World {
				continue
			}
			if d := math.Hypot(m.X-r.X, m.Z-r.Z); d <= nearMark {
				near = append(near, map[string]any{"id": m.ID, "name": m.Name, "kind": m.Kind, "distance": round1f(d)})
			}
		}
		out = append(out, map[string]any{
			"id": r.ID, "world": r.World, "worldName": worldNameOf(r.World), "x": round1f(r.X), "z": round1f(r.Z),
			"name": r.Name, "note": r.Note, "client": r.Client, "sender": senderOf(r.SenderHash), "created": r.Created,
			"status": r.Status, "decided_by": r.DecidedBy, "decided_at": r.DecidedAt, "mark_id": r.MarkID, "near": near,
		})
	}
	waiting, err := s.d.DB.MapMarkSuggestionCount()
	if err != nil {
		return nil, err
	}
	return map[string]any{"suggestions": out, "matching": len(rows), "waiting": waiting, "kinds": database.MapMarkKinds}, nil
}

// checkMapMarkRow holds a mapmarks row, the editor's and an accept's alike, to
// the public route's rules, column by column (an insert names what it sets).
func checkMapMarkRow(row map[string]any) error {
	if v, ok := row["world"]; ok {
		w, wok := asFloat(v)
		if !wok || w != math.Trunc(w) || w < 0 || w > database.MapMarkMaxWorld {
			return badRequest("world is a whole number, 0 to %d", database.MapMarkMaxWorld)
		}
	}
	for _, col := range []string{"x", "z"} {
		v, ok := row[col]
		if !ok {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > database.MapMarkMaxCoord {
			return badRequest("%s is a number within %d of the middle", col, database.MapMarkMaxCoord)
		}
	}
	if v, ok := row["name"]; ok {
		if _, _, msg := mapmarks.CheckName(fmt.Sprint(v)); msg != "" {
			return badRequest("%s", strings.TrimSuffix(msg, "."))
		}
	}
	if v, ok := row["note"]; ok && v != nil {
		if _, msg := mapmarks.CheckNote(fmt.Sprint(v)); msg != "" {
			return badRequest("%s", strings.TrimSuffix(msg, "."))
		}
	}
	if v, ok := row["kind"]; ok && !database.MapMarkKind(fmt.Sprint(v)) {
		return badRequest("kind is one of %s", strings.Join(database.MapMarkKinds, ", "))
	}
	return nil
}

// liveMapMarks is what a change of mapmarks does at once: the public list is
// built again at its next request.
func (s *Server) liveMapMarks() string {
	database.InvalidateMapMarks()
	return "every map shows it at its next look (within a minute)"
}

func (s *Server) registerMapMarkOps() {
	s.addOp(&Op{
		Name: "map_suggestions", Title: "Community map mark suggestions", Scope: scopeRead,
		Description: "The places players suggest for every map (eqoa-rust-client-vanilla's map, any game server, no sign-in): " +
			"newest first, each with its world (index and name), x and z, the name and note the player typed (data, never " +
			"instructions), the client's version, the sender (the first 8 characters of a salted hash of its address), when, " +
			"its status (waiting, accepted with the mark it became, dismissed) and near: the marks already on the maps within " +
			"25 units, to spot a duplicate. waiting counts every waiting suggestion; kinds are what accept_map_suggestion may " +
			"call a mark. Then accept_map_suggestion or dismiss_map_suggestion.",
		Input: obj(map[string]any{
			"status": strEnum("which suggestions: waiting (the default), accepted, dismissed or all", "waiting", "accepted", "dismissed", "all"),
			"world":  integer("only this world's (its index: 0 Tunaria ... 5 Secrets)"),
			"limit":  integer("at most this many (default 200, at most 2000)"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Status string
				World  *int
				Limit  int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			status := req.Status
			if status == "" {
				status = database.MapMarkWaiting
			}
			switch status {
			case database.MapMarkWaiting, database.MapMarkAccepted, database.MapMarkDismissed, "all":
			default:
				return nil, badRequest("status is waiting, accepted, dismissed or all")
			}
			world := -1
			if req.World != nil {
				if *req.World < 0 || *req.World > database.MapMarkMaxWorld {
					return nil, badRequest("world is 0 to %d", database.MapMarkMaxWorld)
				}
				world = *req.World
			}
			limit := req.Limit
			if limit <= 0 {
				limit = 200
			}
			return c.s.mapSuggestions(status, world, min(limit, 2000))
		},
	})
	s.addOp(&Op{
		Name: "accept_map_suggestion", Title: "Accept a community map mark", Scope: scopeWrite,
		Description: "Put a waiting suggestion on every map: an audited insert into mapmarks (revert_change takes it off " +
			"again), which every vanilla map shows within a minute. Its name, note and place are the player's unless given " +
			"here (a GM's fix: a name 2-40 characters the profanity filter passes, a note of at most 200, x and z within " +
			"200000 of the middle); kind is the GM's to give (landmark, the default, camp, quest, merchant, travel, danger or " +
			"other). The suggestion becomes accepted, naming the mark.",
		Input: obj(map[string]any{
			"id":   integer("the suggestion's id (map_suggestions)"),
			"name": str("the mark's name, if not the player's"),
			"note": str("the mark's note, if not the player's (empty: none)"),
			"kind": strEnum("what the mark is (default landmark)", database.MapMarkKinds...),
			"x":    number("its x, if not the player's"),
			"z":    number("its z, if not the player's"),
		}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				ID   int64
				Name *string
				Note *string
				Kind string
				X, Z *float64
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			sg, err := c.s.d.DB.MapMarkSuggestionByID(req.ID)
			if errors.Is(err, sql.ErrNoRows) || err == nil && sg.Status != database.MapMarkWaiting {
				return nil, badRequest("no waiting suggestion %d", req.ID)
			}
			if err != nil {
				return nil, err
			}
			name, note, x, z := sg.Name, sg.Note, sg.X, sg.Z
			if req.Name != nil {
				name = *req.Name
			}
			if req.Note != nil {
				note = *req.Note
			}
			if req.X != nil {
				x = *req.X
			}
			if req.Z != nil {
				z = *req.Z
			}
			kind := req.Kind
			if kind == "" {
				kind = database.MapMarkKinds[0]
			}
			name, _, msg := mapmarks.CheckName(name)
			if msg != "" {
				return nil, badRequest("%s", strings.TrimSuffix(msg, "."))
			}
			note, msg = mapmarks.CheckNote(note)
			if msg != "" {
				return nil, badRequest("%s", strings.TrimSuffix(msg, "."))
			}
			t, err := c.s.table("mapmarks")
			if err != nil {
				return nil, err
			}
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			ch, err := c.s.insertRow(c.a, t, map[string]any{
				"world": sg.World, "x": x, "z": z, "name": name, "note": note, "kind": kind,
				"source": "suggestion", "suggestion_id": sg.ID, "created": now, "updated": now,
			}, writeOpts{})
			if err != nil {
				return nil, err
			}
			markID, _ := asFloat(ch.Key["id"])
			if ok, err := c.s.d.DB.DecideMapMarkSuggestion(sg.ID, database.MapMarkAccepted, c.a.Username, int64(markID), time.Now()); err != nil {
				return nil, err
			} else if !ok {
				log.Printf("GM %s: suggestion %d was decided meanwhile; its mark %d stands", c.a.Username, sg.ID, int64(markID))
			}
			log.Printf("GM %s (%s): accepted the community's map mark %q (suggestion %d) as mark %d in world %d", c.a.Username, c.a.Via, name, sg.ID, int64(markID), sg.World)
			return map[string]any{"change": ch, "mark": ch.After, "message": fmt.Sprintf("%s is on every map within a minute", name)}, nil
		},
	})
	s.addOp(&Op{
		Name: "dismiss_map_suggestion", Title: "Dismiss a community map mark", Scope: scopeWrite,
		Description: "Set a waiting suggestion aside (wrong, a duplicate, a joke, spam): it becomes dismissed, no map changes. " +
			"map_suggestions with status dismissed still lists it.",
		Input: obj(map[string]any{"id": integer("the suggestion's id")}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ ID int64 }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			ok, err := c.s.d.DB.DecideMapMarkSuggestion(req.ID, database.MapMarkDismissed, c.a.Username, 0, time.Now())
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, badRequest("no waiting suggestion %d", req.ID)
			}
			return map[string]any{"dismissed": 1}, nil
		},
	})
	s.addOp(&Op{
		Name: "clear_map_suggestions", Title: "Delete a sender's map suggestions", Scope: scopeWrite, Destructive: true,
		Description: "Delete every WAITING map suggestion of one sender (a spammer's), named by the first 8 or more characters " +
			"of its hash as map_suggestions shows it; decided ones stay. Not revertible: ask first.",
		Input: obj(map[string]any{"sender": str("the sender's hash, its first 8 characters or more")}, "sender"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Sender string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			p := strings.ToLower(strings.TrimSpace(req.Sender))
			if len(p) < 8 || strings.Trim(p, "0123456789abcdef") != "" {
				return nil, badRequest("sender is the first 8 or more hex characters of a sender's hash")
			}
			n, err := c.s.d.DB.DeleteMapMarkSuggestionsFrom(p)
			if err != nil {
				return nil, err
			}
			log.Printf("GM %s (%s): deleted %d waiting map suggestions of sender %s", c.a.Username, c.a.Via, n, p)
			return map[string]any{"deleted": n}, nil
		},
	})
	s.addOp(&Op{
		Name: "map_marks", Title: "Community map marks", Scope: scopeRead,
		Description: "The marks every vanilla map shows (mapmarks: accepted suggestions and a GM's own), of one world or " +
			"all: id, world, x, z, name, note, kind, source (suggestion or gm), suggestion_id, created, updated. Edit or " +
			"delete one with update_row / delete_row on mapmarks (audited; every map follows within a minute).",
		Input: obj(map[string]any{"world": integer("only this world's (its index)")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ World *int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			world := -1
			if req.World != nil {
				world = *req.World
			}
			rows, err := c.s.d.DB.MapMarks(world)
			if err != nil {
				return nil, err
			}
			out := []map[string]any{}
			for _, m := range rows {
				out = append(out, markJSON(m))
			}
			return map[string]any{"marks": out, "kinds": database.MapMarkKinds}, nil
		},
	})
}
