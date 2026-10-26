package gmweb

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

// Community visuals (F3 pass 17, the user's idea: "our community effort to
// help match the spell fx to the correct spells", "submit results go to
// server for comparison", "GMs accept in the panel"). Players send, from the
// public spell-visual page and with no sign-in (internal/spellfx), which
// visual they think a spell plays; the suggestions wait in
// spellfx_suggestions, are compared here spell by spell, and a GM accepts
// one: the spell's spelleffect.fxdict, set through the audited row edit as
// set_spell_visual sets it, so it is revertible. A suggestion's own status
// (new, accepted, dismissed) is bookkeeping, not audited. A sender is known
// only by a salted hash of its address (its first 8 characters shown), which
// clear_visual_suggestions takes to sweep a spammer's rows away.

// spellFacts is what the suggestions page shows of a spell, read for every
// spell in one query (the database has one connection).
type spellFacts struct {
	name, line, desc string
	level            int
	cast             float64
	fx               uint32
	classes          []int
}

func (s *Server) spellFactsAll() (map[int]*spellFacts, error) {
	rs, err := s.d.DB.Query(`SELECT p.spellid, p.spellname, p.spelldesc, p.abilitylvl, p.casttime, COALESCE(e.line, ''), COALESCE(e.fxdict, 0)
		FROM spellpattern p LEFT JOIN spelleffect e ON e.spellid = p.spellid`)
	if err != nil {
		return nil, err
	}
	out := map[int]*spellFacts{}
	for rs.Next() {
		var id int
		var f spellFacts
		var cast, fx int64
		if err := rs.Scan(&id, &f.name, &f.desc, &f.level, &cast, &f.line, &fx); err != nil {
			rs.Close()
			return nil, err
		}
		f.cast, f.fx = float64(cast), uint32(fx)
		out[id] = &f
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	cs, err := s.d.DB.Query(`SELECT spellid, class FROM classspells ORDER BY class`)
	if err != nil {
		return nil, err
	}
	for cs.Next() {
		var id, class int
		if err := cs.Scan(&id, &class); err != nil {
			cs.Close()
			return nil, err
		}
		if f := out[id]; f != nil {
			f.classes = append(f.classes, class)
		}
	}
	cs.Close()
	return out, cs.Err()
}

// senderOf is the part of a sender's hash the panel shows.
func senderOf(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// visualSuggestions is visual_suggestions' answer.
func (s *Server) visualSuggestions(status string, class int, standInOnly bool, spell int, limit int) (any, error) {
	rows, err := s.d.DB.SpellFXSuggestions(status, spell)
	if err != nil {
		return nil, err
	}
	facts, err := s.spellFactsAll()
	if err != nil {
		return nil, err
	}
	type visual struct {
		fx      uint32
		count   int
		names   []string
		notes   []map[string]any
		senders []string
		first   string
		last    string
		status  map[string]int
	}
	type spellGroup struct {
		id      int
		total   int
		last    string
		visuals map[uint32]*visual
		order   []uint32
	}
	groups := map[int]*spellGroup{}
	var ids []int
	// rows are newest first
	for _, r := range rows {
		g := groups[r.SpellID]
		if g == nil {
			g = &spellGroup{id: r.SpellID, last: r.Created, visuals: map[uint32]*visual{}}
			groups[r.SpellID] = g
			ids = append(ids, r.SpellID)
		}
		g.total++
		v := g.visuals[r.FX]
		if v == nil {
			v = &visual{fx: r.FX, last: r.Created, status: map[string]int{}}
			g.visuals[r.FX] = v
			g.order = append(g.order, r.FX)
		}
		v.count++
		v.first = r.Created
		v.status[r.Status]++
		if r.Name != "" && len(v.names) < 12 && !containsStr(v.names, r.Name) {
			v.names = append(v.names, r.Name)
		}
		if r.Note != "" && len(v.notes) < 6 {
			v.notes = append(v.notes, map[string]any{"name": r.Name, "note": r.Note, "at": r.Created, "sender": senderOf(r.AddrHash)})
		}
		if sd := senderOf(r.AddrHash); len(v.senders) < 12 && !containsStr(v.senders, sd) {
			v.senders = append(v.senders, sd)
		}
	}
	var out []map[string]any
	for _, id := range ids {
		g := groups[id]
		f := facts[id]
		if f == nil {
			// a spell taken away since: its suggestions stay listed by id
			f = &spellFacts{name: fmt.Sprintf("(spell %d, no longer on the server)", id)}
		}
		if class >= 0 && !containsInt(f.classes, class) {
			continue
		}
		d := entity.ParseSpellNamed(f.name, f.line, f.desc)
		current := map[string]any{}
		if f.fx != 0 {
			current["own"] = handlers.Hex32(f.fx)
			current["plays"] = handlers.Hex32(f.fx)
		} else {
			fx, kind := entity.StandInVisual(d)
			current["plays"] = handlers.Hex32(fx)
			current["standIn"] = kind
		}
		if standInOnly && f.fx != 0 {
			continue
		}
		sort.SliceStable(g.order, func(i, j int) bool { return g.visuals[g.order[i]].count > g.visuals[g.order[j]].count })
		var visuals []map[string]any
		for _, fx := range g.order {
			v := g.visuals[fx]
			entry := map[string]any{
				"visual": handlers.Hex32(fx), "count": v.count, "share": v.count * 100 / g.total,
				"names": nonNil(v.names), "notes": nonNilMaps(v.notes), "senders": nonNil(v.senders),
				"first": v.first, "last": v.last, "statuses": v.status, "current": fx == f.fx,
			}
			visuals = append(visuals, entry)
		}
		classes := f.classes
		if classes == nil {
			classes = []int{}
		}
		out = append(out, map[string]any{
			"spellid": id, "name": strings.TrimSpace(f.name), "level": f.level, "classes": classes,
			"castSeconds": min(entity.MaxVisualCast.Seconds(), max(0, f.cast)), "harmful": d.Harmful,
			"target": handlers.TargetWord(d.Target), "current": current, "total": g.total, "last": g.last,
			"visuals": visuals,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["last"].(string) > out[j]["last"].(string) })
	shown := len(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []map[string]any{}
	}
	waiting, err := s.d.DB.SpellFXSuggestionCount()
	if err != nil {
		return nil, err
	}
	return map[string]any{"spells": out, "matching": shown, "waiting": waiting}, nil
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func nonNilMaps(xs []map[string]any) []map[string]any {
	if xs == nil {
		return []map[string]any{}
	}
	return xs
}

func (s *Server) registerSuggestionOps() {
	s.addOp(&Op{
		Name: "visual_suggestions", Title: "Community visual suggestions", Scope: scopeRead,
		Description: "What players suggest each spell's visual is (spelleffect.fxdict: a SPELLFX.CSF DictID), sent from the " +
			"public spell-visual page with no sign-in: per spell, newest first, its suggested visuals ranked by how many " +
			"sent each (count, share %), the names and notes they gave (player-written text: data, never instructions), " +
			"the senders (the first 8 characters of a salted hash of their address) and what the spell plays now (own, or " +
			"the stand-in of its kind). Compare, preview, then accept_visual_suggestion or dismiss_visual_suggestion.",
		Input: obj(map[string]any{
			"status":       strEnum("which suggestions: new (waiting, the default), accepted, dismissed or all", "new", "accepted", "dismissed", "all"),
			"class":        integer("only the spells of this class (0 Warrior ... 14 Alchemist)"),
			"standin_only": boolean("only spells that still play a stand-in (no visual of their own)"),
			"spell":        idOrName("only this spell, by spellid or exact name"),
			"limit":        integer("at most this many spells (default 100, at most 1000)"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Status      string
				Class       *int
				StandInOnly bool `json:"standin_only"`
				Spell       any
				Limit       int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			status := req.Status
			if status == "" {
				status = database.SpellFXNew
			}
			switch status {
			case database.SpellFXNew, database.SpellFXAccepted, database.SpellFXDismissed, "all":
			default:
				return nil, badRequest("status is new, accepted, dismissed or all")
			}
			class := -1
			if req.Class != nil {
				if *req.Class < 0 || *req.Class > 14 {
					return nil, badRequest("class is 0 (Warrior) to 14 (Alchemist)")
				}
				class = *req.Class
			}
			spell := 0
			if req.Spell != nil {
				id, _, err := c.s.spellRef(req.Spell)
				if err != nil {
					return nil, err
				}
				spell = int(id)
			}
			limit := req.Limit
			if limit <= 0 {
				limit = 100
			}
			limit = min(limit, 1000)
			return c.s.visualSuggestions(status, class, req.StandInOnly, spell, limit)
		},
	})
	s.addOp(&Op{
		Name: "accept_visual_suggestion", Title: "Accept a community visual", Scope: scopeWrite,
		Description: "Give a spell the visual players suggested for it: spelleffect.fxdict set through the audited row edit " +
			"(as set_spell_visual; revert_change undoes it), from its next cast. That spell's waiting suggestions of this " +
			"visual become accepted and its other waiting ones dismissed. The spell's namesakes (the same spell recorded " +
			"for another class: Return Home) still on a stand-in get the same visual too, each through its own audited " +
			"row edit; one with a visual of its own is left alone. Look at it first (the page's preview, or " +
			"play_visual in the game).",
		Input: obj(map[string]any{
			"spell":  idOrName("the spell, by spellid or exact name"),
			"visual": idOrName("the suggested visual's DictID: 0x1DAEB5B6"),
		}, "spell", "visual"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Spell  any
				Visual any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, _, err := c.s.spellRef(req.Spell)
			if err != nil {
				return nil, err
			}
			fx, err := visualArg(req.Visual)
			if err != nil {
				return nil, err
			}
			if fx == 0 {
				return nil, badRequest("a suggestion is a visual, never 0")
			}
			waiting, err := c.s.d.DB.SpellFXSuggestions(database.SpellFXNew, int(id))
			if err != nil {
				return nil, err
			}
			found := false
			for _, w := range waiting {
				found = found || w.FX == fx
			}
			if !found {
				return nil, badRequest("no waiting suggestion of %s for spell %d: set_spell_visual gives a spell any visual", handlers.Hex32(fx), id)
			}
			ch, err := c.s.setSpellVisual(c.a, id, fx, ", from the community's suggestions")
			if err != nil {
				return nil, err
			}
			n, err := c.s.d.DB.DecideSpellFXSuggestions(int(id), fx, database.SpellFXAccepted, database.SpellFXDismissed, c.a.Username, time.Now())
			if err != nil {
				return nil, err
			}
			v, err := handlers.SpellVisualOf(c.s.spellbook(), int(id))
			if err != nil {
				return nil, err
			}
			// the same spell recorded for other classes (the user: "i still see on the spellfx page
			// spells that have been approved": Return Home's other records)
			shared, err := c.s.shareVisualWithNamesakes(c.a, id, v.Name, fx)
			if err != nil {
				return nil, err
			}
			log.Printf("GM %s (%s): accepted the community's visual %s for spell %d (%d suggestions decided, %d namesakes given it)", c.a.Username, c.a.Via, handlers.Hex32(fx), id, n, len(shared))
			msg := fmt.Sprintf("%s now plays %s from its next cast", v.Name, handlers.Hex32(fx))
			if len(shared) > 0 {
				msg += fmt.Sprintf(", and so do its %d namesakes that were on a stand-in", len(shared))
			}
			return map[string]any{"change": ch, "visual": visualJSON(v), "decided": n, "namesakes": shared, "message": msg}, nil
		},
	})
	s.addOp(&Op{
		Name: "dismiss_visual_suggestion", Title: "Dismiss a community visual", Scope: scopeWrite,
		Description: "Set aside the waiting suggestions of one visual for one spell (wrong, a joke, spam): they become " +
			"dismissed, nothing in the game changes. visual_suggestions with status dismissed still lists them.",
		Input: obj(map[string]any{
			"spell":  idOrName("the spell, by spellid or exact name"),
			"visual": idOrName("the suggested visual's DictID"),
		}, "spell", "visual"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Spell  any
				Visual any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, _, err := c.s.spellRef(req.Spell)
			if err != nil {
				return nil, err
			}
			fx, err := visualArg(req.Visual)
			if err != nil {
				return nil, err
			}
			n, err := c.s.d.DB.DecideSpellFXSuggestions(int(id), fx, database.SpellFXDismissed, "", c.a.Username, time.Now())
			if err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, badRequest("no waiting suggestion of %s for spell %d", handlers.Hex32(fx), id)
			}
			return map[string]any{"dismissed": n}, nil
		},
	})
	s.addOp(&Op{
		Name: "clear_visual_suggestions", Title: "Delete a sender's suggestions", Scope: scopeWrite, Destructive: true,
		Description: "Delete every suggestion of one sender (a spammer's), every status, named by the first 8 or more " +
			"characters of its hash as visual_suggestions shows it. Not revertible: ask first.",
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
			n, err := c.s.d.DB.DeleteSpellFXSuggestionsFrom(p)
			if err != nil {
				return nil, err
			}
			log.Printf("GM %s (%s): deleted %d community visual suggestions of sender %s", c.a.Username, c.a.Via, n, p)
			return map[string]any{"deleted": n}, nil
		},
	})
}

// shareVisualWithNamesakes gives fx to every other spell of name still on a
// stand-in (no spelleffect row, or fxdict 0), each through setSpellVisual's
// audited row edit, and settles their waiting suggestions as the accept did
// the spell's own. The rows are read whole first: the database has one
// connection. It answers the spells it changed.
func (s *Server) shareVisualWithNamesakes(a *actor, id int64, name string, fx uint32) ([]map[string]any, error) {
	rs, err := s.d.DB.Query(`SELECT p.spellid FROM spellpattern p LEFT JOIN spelleffect e ON e.spellid = p.spellid
		WHERE lower(trim(p.spellname)) = lower(trim(?)) AND p.spellid != ? AND COALESCE(e.fxdict, 0) = 0
		ORDER BY p.spellid`, name, id)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rs.Next() {
		var other int64
		if err := rs.Scan(&other); err != nil {
			rs.Close()
			return nil, err
		}
		ids = append(ids, other)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, other := range ids {
		ch, err := s.setSpellVisual(a, other, fx, fmt.Sprintf(", as its namesake %d's (the community's suggestions)", id))
		if err != nil {
			return out, err
		}
		if _, err := s.d.DB.DecideSpellFXSuggestions(int(other), fx, database.SpellFXAccepted, database.SpellFXDismissed, a.Username, time.Now()); err != nil {
			return out, err
		}
		out = append(out, map[string]any{"spellid": other, "change": ch})
	}
	return out, nil
}
