package gmweb

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Paths a GM gives NPCs (entity/path.go, table npcpaths): the map's path
// editor (static/views/paths.js) and the same for an agent. A path is one
// row an NPC, its waypoints one JSON list, so setting or clearing one is one
// row change: audited, revertible, and applied to the running world at once
// (livePath).

// pathRecordOf is a npcpaths row as the database package reads one; a
// column missing is the table's default.
func pathRecordOf(row map[string]any) (database.NPCPathRecord, error) {
	rec := database.NPCPathRecord{Mode: database.PathModeLoop, Speed: database.PathSpeedWalk}
	id, ok := asFloat(row["npcid"])
	if !ok {
		return rec, errors.New("npcid names the NPC (its npcs id)")
	}
	rec.NPCID = int(id)
	if v, ok := asFloat(row["world"]); ok {
		rec.World = int(v)
	}
	if v, ok := row["mode"].(string); ok {
		rec.Mode = v
	}
	if v, ok := row["speed"].(string); ok {
		rec.Speed = v
	}
	if v, ok := asFloat(row["pause"]); ok {
		rec.Pause = v
	} else if s, ok := row["pause"].(string); ok && strings.TrimSpace(s) != "" {
		return rec, fmt.Errorf("pause %q is not a number of seconds", s)
	}
	points := "[]"
	if v, ok := row["points"].(string); ok {
		points = v
	}
	pts, err := database.ParsePathPoints(points)
	if err != nil {
		return rec, err
	}
	rec.Points = pts
	return rec, nil
}

// checkRow refuses a row a table cannot hold although SQLite would: a path
// the world could not walk, a map mark the public route would refuse
// (checkMapMarkRow). It reads nothing (it runs inside the change's
// transaction, and the database has one connection).
func checkRow(table string, row map[string]any) error {
	if (table == "workboard" || table == "workboard_overview") && row != nil {
		return checkWorkRow(table, row) // ops_work.go
	}
	if table == "mapmarks" && row != nil {
		return checkMapMarkRow(row)
	}
	if table == "petitions" && row != nil {
		return checkPetitionRow(row) // ops_petitions.go
	}
	if table == "spellgates" && row != nil {
		return checkSpellGateRow(row) // live_gates.go
	}
	if table == "racesize" && row != nil {
		return checkRaceSizeRow(row) // live_racesize.go
	}
	if table == "chardyes" && row != nil {
		return checkDyeRow(row) // live_dyes.go
	}
	if table == "charcoaches" && row != nil {
		return checkCoachRow(row) // live_coaches.go
	}
	if table == "charresets" && row != nil {
		return checkCharResetRow(row) // live_rehab.go
	}
	if table == "rehabsettings" && row != nil {
		return checkRehabSettingRow(row) // live_rehab.go
	}
	if table == "gmtriggers" && row != nil {
		return checkTriggerRow(row) // ops_triggers.go
	}
	if table == "craftrecipes" && row != nil {
		return checkCraftRecipeRow(row) // live_craft.go
	}
	if table == "charcraftskills" && row != nil {
		return checkCraftSkillRow(row) // live_craft.go
	}
	if (table == "balancelevel" || table == "balancecon" || table == "balanceclass" || table == "balanceclasslevel") && row != nil {
		return checkBalanceRow(table, row) // live_balance.go
	}
	if table == "classtemplates" && row != nil {
		return checkClassTemplateRow(row) // live_templates.go
	}
	if table == "npctemplate" && row != nil {
		return checkNPCTemplateRow(row) // live_templates.go
	}
	if table == "pettemplate" && row != nil {
		return checkPetTemplateRow(row) // live_templates.go
	}
	if table == "npcspawnonly" && row != nil {
		return checkSpawnOnlyRow(row) // live.go
	}
	if table != "npcpaths" || row == nil {
		return nil
	}
	rec, err := pathRecordOf(row)
	if err == nil {
		err = rec.Check()
	}
	if err != nil {
		return badRequest("not a path: %v", err)
	}
	return nil
}

// livePath gives the world a path set, changed or taken away.
func (s *Server) livePath(before, after map[string]any) string {
	if s.d.World == nil {
		return "no world is running: at the next start"
	}
	var msgs []string
	if before != nil && (after == nil || !sameValue(before["npcid"], after["npcid"])) {
		id, _ := asFloat(before["npcid"])
		if s.d.World.ClearNPCPath(entity.EntityID(id)) {
			msgs = append(msgs, fmt.Sprintf("%s walks back to its spawn point and is as before", s.npcLabel(int64(id))))
		}
	}
	if after != nil {
		rec, err := pathRecordOf(after)
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		p, err := entity.PathFromRecord(rec)
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		if err := s.d.World.SetNPCPath(entity.EntityID(rec.NPCID), p); err != nil {
			return "not applied to the world: " + err.Error()
		}
		speed, _ := database.PathSpeed(rec.Speed)
		msgs = append(msgs, fmt.Sprintf("%s walks its path now: %d point(s), %s, at a %s", s.npcLabel(int64(rec.NPCID)),
			len(rec.Points), modeWords(p.Mode), speed))
	}
	return strings.Join(msgs, "; ")
}

// npcLabel is an NPC's name as the world has it, else its id.
func (s *Server) npcLabel(id int64) string {
	if s.d.World != nil {
		if st, ok := s.d.World.NPCStateOf(entity.EntityID(id)); ok {
			return st.Name
		}
	}
	return fmt.Sprintf("NPC %d", id)
}

func modeWords(m entity.PathMode) string {
	switch m {
	case entity.PathPingPong:
		return "back and forth"
	case entity.PathOnce:
		return "once, then it stands at the last point"
	}
	return "in a loop"
}

// pathPointIn is a waypoint as the operations take it: y may be left out,
// and the ground finds it.
type pathPointIn struct {
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
	Z     *float64 `json:"z"`
	Pause *float64 `json:"pause"`
}

// npcRow is the NPC row a path is for: its name, world and spawn.
type npcRow struct {
	ID      int64
	Name    string
	World   int
	X, Y, Z float64
}

func (s *Server) npcRowOf(id int64) (npcRow, error) {
	r := npcRow{ID: id}
	err := s.d.DB.QueryRow(`SELECT npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?`, id).
		Scan(&r.Name, &r.World, &r.X, &r.Y, &r.Z)
	if errors.Is(err, sql.ErrNoRows) {
		return r, notFound("no NPC row %d (a GM's spawn has none: a path needs a row)", id)
	}
	return r, err
}

// fillHeights gives a point with no height the ground's: for an NPC that
// stands on the ground, the floor near the last point's height (a cave's
// floor rather than the hill above it), else the highest terrain there; for
// one off the ground (a bridge, a roof), or where the server has no ground,
// the last point's height (the first's, its spawn's).
func (s *Server) fillHeights(n npcRow, pts []pathPointIn) ([]database.PathPoint, error) {
	out := make([]database.PathPoint, 0, len(pts))
	onGround := s.d.World != nil && s.d.World.NPCOnGround(entity.EntityID(n.ID))
	ref := n.Y
	for i, p := range pts {
		if p.X == nil || p.Z == nil {
			return nil, badRequest("point %d needs x and z", i+1)
		}
		q := database.PathPoint{X: round1f(*p.X), Z: round1f(*p.Z), Pause: p.Pause}
		switch {
		case p.Y != nil:
			q.Y = *p.Y
		case onGround:
			y, ok := s.d.World.GroundFloor(uint8(n.World), float32(q.X), float32(ref), float32(q.Z))
			if !ok {
				y, ok = s.d.World.GroundFloor(uint8(n.World), float32(q.X), 1e6, float32(q.Z))
			}
			if ok {
				q.Y = float64(y)
			} else {
				q.Y = ref
			}
		default:
			q.Y = ref
		}
		q.Y = round1f(q.Y)
		ref = q.Y
		out = append(out, q)
	}
	return out, nil
}

func round1f(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return math.Round(v*10) / 10
}

// pathOut is a path as the operations give it.
func pathOut(rec database.NPCPathRecord) map[string]any {
	pts := make([]map[string]any, len(rec.Points))
	for i, p := range rec.Points {
		m := map[string]any{"x": p.X, "y": p.Y, "z": p.Z}
		if p.Pause != nil {
			m["pause"] = *p.Pause
		}
		pts[i] = m
	}
	return map[string]any{"world": rec.World, "mode": rec.Mode, "pause": rec.Pause, "speed": rec.Speed, "points": pts}
}

// pathRows is the npcpaths rows of one world (all < 0) or one NPC (npc > 0),
// with their NPCs' names and spawns; read whole before anything else asks
// the database (it has one connection).
func (s *Server) pathRows(world int, npc int64) ([]map[string]any, error) {
	q := `SELECT p.npcid, p.world, p.mode, p.pause, p.speed, p.points, n.npc_name, n.x_coord, n.y_coord, n.z_coord
		FROM npcpaths p LEFT JOIN npcs n ON n.id = p.npcid`
	var args []any
	switch {
	case npc > 0:
		q += ` WHERE p.npcid = ?`
		args = append(args, npc)
	case world >= 0:
		q += ` WHERE p.world = ?`
		args = append(args, world)
	}
	rs, err := s.d.DB.Query(q+` ORDER BY p.npcid`, args...)
	if err != nil {
		return nil, err
	}
	type raw struct {
		rec        database.NPCPathRecord
		points     string
		name       sql.NullString
		sx, sy, sz sql.NullFloat64
	}
	var rows []raw
	for rs.Next() {
		var r raw
		if err := rs.Scan(&r.rec.NPCID, &r.rec.World, &r.rec.Mode, &r.rec.Pause, &r.rec.Speed, &r.points, &r.name, &r.sx, &r.sy, &r.sz); err != nil {
			rs.Close()
			return nil, err
		}
		rows = append(rows, r)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{"npcid": r.rec.NPCID}
		if r.name.Valid {
			m["name"] = r.name.String
			m["spawn"] = map[string]any{"x": r.sx.Float64, "y": r.sy.Float64, "z": r.sz.Float64}
		} else {
			m["name"] = nil
			m["note"] = "its NPC row is gone: clear_npc_path removes the path"
		}
		pts, err := database.ParsePathPoints(r.points)
		if err != nil {
			m["error"] = err.Error()
		} else {
			r.rec.Points = pts
			if err := r.rec.Check(); err != nil {
				m["error"] = err.Error()
			}
		}
		m["path"] = pathOut(r.rec)
		if s.d.World != nil {
			if st, ok := s.d.World.PathStatusOf(entity.EntityID(r.rec.NPCID)); ok {
				m["live"] = st
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Server) registerPathOps() {
	s.addOp(&Op{
		Name: "npc_paths", Title: "NPC paths", Scope: scopeRead,
		Description: "The paths GMs gave NPCs (table npcpaths): each NPC's waypoints, mode (loop, pingpong = back and " +
			"forth, once = to the last point and stand there), pause at each point (seconds; a point's own pause first) " +
			"and speed (walk, run), with where the NPC is on it now (live: x, z, the point it walks to, and its state: " +
			"walking, paused, holding, fighting, rooted, dead, away, home, starting). In one world, or every world.",
		Input: obj(map[string]any{
			"world": integer("the world index; leave out for every world"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ World *int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			w := -1
			if req.World != nil {
				w = *req.World
			}
			list, err := c.s.pathRows(w, 0)
			if err != nil {
				return nil, err
			}
			return map[string]any{"paths": list}, nil
		},
	})
	s.addOp(&Op{
		Name: "npc_path", Title: "One NPC's path", Scope: scopeRead,
		Description: "One NPC's path (none: path null) and where it is now: its name, world and spawn point, its " +
			"waypoints, and live (its place, the point it walks to, its state). Poll it to watch the NPC walk.",
		Input: obj(map[string]any{"id": integer("the NPC's id (its npcs row)")}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ ID int64 }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			n, err := c.s.npcRowOf(req.ID)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"npcid": n.ID, "name": n.Name, "world": n.World,
				"spawn": map[string]any{"x": n.X, "y": n.Y, "z": n.Z}, "path": nil}
			list, err := c.s.pathRows(-1, n.ID)
			if err != nil {
				return nil, err
			}
			if len(list) == 1 {
				out["path"] = list[0]["path"]
				if e, ok := list[0]["error"]; ok {
					out["error"] = e
				}
			}
			if c.s.d.World != nil {
				if st, ok := c.s.d.World.PathStatusOf(entity.EntityID(n.ID)); ok {
					out["live"] = st
				}
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "set_npc_path", Title: "Give an NPC a path", Scope: scopeWrite,
		Description: "Give an NPC a path to walk, or replace its path, at once: points are its waypoints in the NPC's own " +
			"world, in order, each {x, z} (and y, the height: leave it out and the ground finds it; pause, seconds it stands " +
			"there, overriding the path's). mode: loop (the last point back to the first; the default), pingpong (back and " +
			"forth) or once (to the last point, then it stands there). pause: seconds at each point (default 0). speed: " +
			"walk (5 a second, the default) or run (8.8). The NPC walks to its nearest leg (once: to its first point) and " +
			"on; it no longer wanders; a fight takes it off its path and it takes it up again at the nearest leg; it keeps " +
			"its path through its deaths and restarts. Up to 256 points. Audited; revert_change undoes it. dry_run shows " +
			"the path (heights filled in) and writes nothing.",
		Input: obj(map[string]any{
			"id": integer("the NPC's id (its npcs row)"),
			"points": arr(obj(map[string]any{
				"x": number("x"), "z": number("z"), "y": number("height; leave out for the ground's"),
				"pause": number("seconds it stands at this point; leave out for the path's pause"),
			}, "x", "z"), "the waypoints, in order"),
			"mode":    strEnum("how it is walked (default loop)", "loop", "pingpong", "once"),
			"pause":   number("seconds it stands at each point (default 0, at most 600)"),
			"speed":   strEnum("its pace (default walk)", "walk", "run"),
			"dry_run": boolean("check and show, write nothing"),
		}, "id", "points"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				ID     int64
				Points []pathPointIn
				Mode   string
				Pause  *float64
				Speed  string
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			n, err := c.s.npcRowOf(req.ID)
			if err != nil {
				return nil, err
			}
			mode, ok := database.PathMode(req.Mode)
			if !ok {
				return nil, badRequest("mode %q is not loop, pingpong or once", req.Mode)
			}
			speed, ok := database.PathSpeed(req.Speed)
			if !ok {
				return nil, badRequest("speed %q is not walk or run", req.Speed)
			}
			pts, err := c.s.fillHeights(n, req.Points)
			if err != nil {
				return nil, err
			}
			rec := database.NPCPathRecord{NPCID: int(n.ID), World: n.World, Mode: mode, Speed: speed, Points: pts}
			if req.Pause != nil {
				rec.Pause = *req.Pause
			}
			if err := rec.Check(); err != nil {
				return nil, badRequest("%v", err)
			}
			t, err := c.s.table("npcpaths")
			if err != nil {
				return nil, err
			}
			values := map[string]any{"world": rec.World, "mode": rec.Mode, "pause": rec.Pause, "speed": rec.Speed,
				"points": database.PathPointsJSON(rec.Points)}
			var exists int
			c.s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcpaths WHERE npcid = ?`, n.ID).Scan(&exists)
			var ch *change
			if exists > 0 {
				ch, err = c.s.updateRow(c.a, t, map[string]any{"npcid": n.ID}, values, nil, writeOpts{Dry: req.DryRun})
			} else {
				values["npcid"] = n.ID
				ch, err = c.s.insertRow(c.a, t, values, writeOpts{Dry: req.DryRun})
			}
			if err != nil {
				return nil, err
			}
			out := map[string]any{"npc": n.Name, "npcid": n.ID, "path": pathOut(rec), "change": ch}
			if ch.Live != "" {
				out["live"] = ch.Live
			}
			if ch.Action == "unchanged" {
				out["live"] = "the path is as it was"
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "clear_npc_path", Title: "Take an NPC's path away", Scope: scopeWrite, Destructive: true,
		Description: "Take an NPC's path away (its npcpaths row): it walks back to its spawn point and is as before " +
			"(it wanders, or stands facing as spawned). Audited; revert_change puts the path back.",
		Input: obj(map[string]any{"id": integer("the NPC's id (its npcs row)")}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ ID int64 }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table("npcpaths")
			if err != nil {
				return nil, err
			}
			var exists int
			c.s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcpaths WHERE npcid = ?`, req.ID).Scan(&exists)
			if exists == 0 {
				return nil, notFound("NPC %d has no path", req.ID)
			}
			ch, err := c.s.deleteRow(c.a, t, map[string]any{"npcid": req.ID}, writeOpts{})
			if err != nil {
				return nil, err
			}
			return map[string]any{"npcid": req.ID, "change": ch, "live": ch.Live}, nil
		},
	})
}
