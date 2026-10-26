package gmweb

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
)

// Spell visuals (F3 pass 16; the user: GMs "change the animation hash, so we
// can fix them manually by hand", with "a visual animation playback"). A
// visual is what 0x00B1 names: a 0xC200 record of SPELLFX.CSF, by its DictID.
// A spell's is spelleffect.fxdict, 0 for the stand-in of its kind, and most
// spells have none recorded. The page lists the visuals off the GM's own disc
// (static/disc/: nothing of it is sent here but DictIDs) and plays one in the
// game, from the GM's own character onto its target; the game's #fx and
// #spellfx (handlers/spellfx.go) do the same from the chat line.

func (s *Server) spellbook() *database.SpellbookStore { return database.NewSpellbookStore(s.d.DB) }

// (A spell is named as grant_spell names one: spellRef, by spellid or exact
// name.)

// visualArg reads a visual's DictID: hex as 0x1DAEB5B6 or the eight digits
// alone, or a number (0 is a spell's stand-in, for set_spell_visual).
func visualArg(v any) (uint32, error) {
	bad := badRequest("visual is a DictID: hex as 0x1DAEB5B6 (or the eight digits alone), or a number")
	switch x := v.(type) {
	case json.Number:
		n, err := strconv.ParseInt(string(x), 10, 64)
		if err != nil || n < 0 || n > math.MaxUint32 {
			return 0, bad
		}
		return uint32(n), nil
	case float64:
		if x < 0 || x > math.MaxUint32 || x != math.Trunc(x) {
			return 0, bad
		}
		return uint32(x), nil
	case string:
		if fx, ok := handlers.ParseVisual(x); ok {
			return fx, nil
		}
		if n, err := strconv.ParseUint(strings.TrimSpace(x), 10, 32); err == nil {
			return uint32(n), nil
		}
	}
	return 0, bad
}

// gameSession is the session of the actor's own character in the game, or
// nil (one login at a time per account: at most one).
func (s *Server) gameSession(a *actor) *session.Session {
	var found *session.Session
	if s.d.Sessions == nil {
		return nil
	}
	s.d.Sessions.ForEach(func(sess *session.Session) bool {
		if sess.AccountID() == a.AccountID && sess.CharacterInWorld() {
			found = sess
			return false
		}
		return true
	})
	return found
}

// visualJSON is a spell's visual as the operations answer it.
func visualJSON(v handlers.SpellVisual) map[string]any {
	out := map[string]any{
		"spellid": v.SpellID, "name": v.Name, "level": v.Level, "castSeconds": v.CastTime.Seconds(),
		"plays": handlers.Hex32(v.Plays), "summary": v.Describe(),
		// cast at an enemy (the preview's target: a training dummy), and whom
		// its line lands on
		"harmful": v.Harmful, "target": v.Target,
	}
	if v.Own != 0 {
		out["own"] = handlers.Hex32(v.Own)
	} else {
		out["standIn"] = v.StandIn
	}
	return out
}

func (s *Server) registerVisualOps() {
	s.addOp(&Op{
		Name: "play_visual", Title: "Play a spell visual in the game", Scope: scopeWrite,
		Description: "Play a spell visual now from your own character in the game (this account's, logged in) onto its target " +
			"(itself with none), seen by everyone near, as a cast begins one; nothing saved, nothing lands. visual: a " +
			"DictID (0x1DAEB5B6); or spell: what that spell plays, with its cast time. To see a visual before set_spell_visual.",
		Input: obj(map[string]any{
			"visual":  idOrName("the visual's DictID: 0x1DAEB5B6, 1DAEB5B6, or a number"),
			"spell":   idOrName("or a spell, by spellid or exact name: the visual it plays (its stand-in when it has none); with visual, only its cast time"),
			"seconds": number("the cast time it plays with, 0 to 30 (default 2, or the spell's own): the visual's casting part waits for it"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Visual  any
				Spell   any
				Seconds *float64
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			cast := 2 * time.Second
			var fx uint32
			what := ""
			switch {
			case req.Visual != nil:
				v, err := visualArg(req.Visual)
				if err != nil {
					return nil, err
				}
				if v == 0 {
					return nil, badRequest("visual 0 is none: give a DictID")
				}
				fx = v
				if req.Spell != nil {
					// with the spell's cast time, as it would cast with it
					id, _, err := c.s.spellRef(req.Spell)
					if err != nil {
						return nil, err
					}
					sv, err := handlers.SpellVisualOf(c.s.spellbook(), int(id))
					if err != nil {
						return nil, err
					}
					cast = sv.CastTime
				}
			case req.Spell != nil:
				id, _, err := c.s.spellRef(req.Spell)
				if err != nil {
					return nil, err
				}
				v, err := handlers.SpellVisualOf(c.s.spellbook(), int(id))
				if err != nil {
					return nil, err
				}
				fx, cast, what = v.Plays, v.CastTime, v.Describe()
			default:
				return nil, badRequest("give visual (a DictID) or spell")
			}
			if req.Seconds != nil {
				if *req.Seconds < 0 || *req.Seconds > entity.MaxVisualCast.Seconds() {
					return nil, badRequest("seconds is 0 to %g", entity.MaxVisualCast.Seconds())
				}
				cast = time.Duration(*req.Seconds * float64(time.Second))
			}
			sess := c.s.gameSession(c.a)
			if sess == nil {
				return nil, badRequest("your character is not in the game: log in with this account, target something, and play it again")
			}
			on, ok := c.s.d.World.PlayVisual(sess, fx, cast)
			if !ok {
				return nil, badRequest("your character is not in the world yet")
			}
			log.Printf("GM %s (%s): played visual %s on %s", c.a.Username, c.a.Via, handlers.Hex32(fx), on)
			out := map[string]any{"visual": handlers.Hex32(fx), "on": on, "seconds": cast.Seconds(),
				"message": fmt.Sprintf("%s played on %s, a %.1f s cast", handlers.Hex32(fx), on, cast.Seconds())}
			if what != "" {
				out["spell"] = what
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "spell_visual", Title: "A spell's visual", Scope: scopeRead,
		Description: "What a spell plays when cast (0x00B1's visual): its own (spelleffect.fxdict), or with none the stand-in " +
			"of its kind (heal, damage or buff). set_spell_visual gives it one; play_visual plays one in the game first.",
		Input: obj(map[string]any{"spell": idOrName("the spell, by spellid or exact name")}, "spell"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Spell any }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, _, err := c.s.spellRef(req.Spell)
			if err != nil {
				return nil, err
			}
			v, err := handlers.SpellVisualOf(c.s.spellbook(), int(id))
			if err != nil {
				return nil, err
			}
			return visualJSON(v), nil
		},
	})
	s.addOp(&Op{
		Name: "set_spell_visual", Title: "Give a spell a visual", Scope: scopeWrite,
		Description: "Set the visual a spell plays from its next cast (spelleffect.fxdict, a DictID; 0: back to the stand-in of " +
			"its kind). A spell with no effect row gets one. Audited and revertible as any row change. play_visual first to see one.",
		Input: obj(map[string]any{
			"spell":  idOrName("the spell, by spellid or exact name"),
			"visual": idOrName("the DictID: 0x1DAEB5B6, 1DAEB5B6, a number, or 0 for the stand-in"),
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
			ch, err := c.s.setSpellVisual(c.a, id, fx, "")
			if err != nil {
				return nil, err
			}
			v, err := handlers.SpellVisualOf(c.s.spellbook(), int(id))
			if err != nil {
				return nil, err
			}
			return map[string]any{"change": ch, "visual": visualJSON(v)}, nil
		},
	})
	s.addOp(&Op{
		Name: "visual_uses", Title: "Which spells use which visual", Scope: scopeRead,
		Description: "Every visual a spell has been given (spelleffect.fxdict not 0) with its spells, and the three stand-ins " +
			"(heal, damage, buff) with how many spells play each for want of their own.",
		Handler: func(c *opCtx) (any, error) {
			return c.s.visualUses()
		},
	})
}

// setSpellVisual gives a spell a visual through the audited row edit, a row
// inserted for a spell with none (its source naming the GM, and why): what
// set_spell_visual and accept_visual_suggestion do, so either is revertible.
func (s *Server) setSpellVisual(a *actor, id int64, fx uint32, why string) (*change, error) {
	t, err := s.table("spelleffect")
	if err != nil {
		return nil, err
	}
	key := map[string]any{"spellid": id}
	if _, gerr := getRow(s.d.DB, t, key); gerr == nil {
		return s.updateRow(a, t, key, map[string]any{"fxdict": int64(fx)}, nil, writeOpts{})
	}
	who := a.Username
	if who == "" {
		who = fmt.Sprintf("of account %d", a.AccountID)
	}
	return s.insertRow(a, t, map[string]any{"spellid": id, "line": "", "fxdict": int64(fx),
		"source": "visual set by GM " + who + why}, writeOpts{})
}

// visualUses is visual_uses' answer. Every row is read before anything is
// worked out from them: the database has one connection.
func (s *Server) visualUses() (any, error) {
	type spellRow struct {
		id, level        int
		name, line, desc string
		fx               int64
	}
	rs, err := s.d.DB.Query(`SELECT p.spellid, p.abilitylvl, p.spellname, p.spelldesc, COALESCE(e.line, ''), COALESCE(e.fxdict, 0)
		FROM spellpattern p LEFT JOIN spelleffect e ON e.spellid = p.spellid ORDER BY p.spellid`)
	if err != nil {
		return nil, err
	}
	var rows []spellRow
	for rs.Next() {
		var r spellRow
		if err := rs.Scan(&r.id, &r.level, &r.name, &r.desc, &r.line, &r.fx); err != nil {
			rs.Close()
			return nil, err
		}
		rows = append(rows, r)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	used := map[uint32][]map[string]any{}
	standIn := map[string]int{}
	for _, r := range rows {
		if r.fx != 0 {
			fx := uint32(r.fx)
			used[fx] = append(used[fx], map[string]any{"spellid": r.id, "name": r.name, "level": r.level})
			continue
		}
		_, kind := entity.StandInVisual(entity.ParseSpellNamed(r.name, r.line, r.desc))
		standIn[kind]++
	}
	var list []map[string]any
	for fx, spells := range used {
		list = append(list, map[string]any{"visual": handlers.Hex32(fx), "spells": spells})
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["visual"].(string) < list[j]["visual"].(string) })
	var ins []map[string]any
	for _, kind := range []string{"heal", "damage", "buff"} {
		ins = append(ins, map[string]any{"kind": kind, "visual": handlers.Hex32(entity.StandInVisuals()[kind]), "spells": standIn[kind]})
	}
	if list == nil {
		list = []map[string]any{}
	}
	return map[string]any{"used": list, "standIns": ins, "spells": len(rows)}, nil
}
