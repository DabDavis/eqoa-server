package gmweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The GMs' triggers (handlers/gmtriggers.go, table gmtriggers): scripted
// events the TEST menu's TRIGGER lines, #trigger and this page fire. A
// trigger is one row, saved as a row change (logged and revertible); firing
// and stopping go through the running engine and are logged as actions.

// triggerCols are a trigger row's columns the editor sends.
var triggerCols = []string{"slot", "name", "enabled", "scope", "radius", "world", "x", "y", "z", "steps", "notes"}

// triggerStepsHelp is what the steps column holds, for the schemas.
const triggerStepsHelp = `the steps in order, each {"do", ...}: broadcast {text} a line in the chat of each player ` +
	`the scope takes, in the GMs' yellow; popup {text, answer} a window each closes (one answer, "OK" when none); ` +
	`say {npc, text} an NPC's words, a box a line; give {items: "patternid*count, ..."} into each one's inventory; ` +
	`visual {fx: "0x1DAEB5B6", seconds} a spell visual on each living one; teleport {world, x, y, z} each living ` +
	`one moved there; spawn {npc, count (1 to 50), spread (how far round the place, 5), minutes (how long they ` +
	`stay alive, 30, at most 240), hostile (they may be attacked and attack any player on sight, in a town too), ` +
	`world/x/y/z (a place of their own; none: the trigger's)} copies of that NPC, as it is named in the game; ` +
	`despawn {npc} this run's spawns alive (of that name; none: all) taken away; stop {slot} the runs of that ` +
	`trigger ended (0: every other run); wait {seconds} the rest put off (at most 3600). text may hold %p, each ` +
	`player's name. The players a step reaches are looked at as it runs`

func (s *Server) registerTriggerOps() {
	triggerSchema := obj(map[string]any{
		"slot":    integer("1 to 8: the line of the GM's TEST > Programming > TRIGGER menu, top to bottom; past 8: fired by #trigger or this page alone"),
		"name":    str("what GMs call it; vanilla's TEST menu shows it on its line (32 bytes)"),
		"enabled": integer("1: may be fired; 0: kept from firing"),
		"scope":   strEnum("whom a step that reaches players reaches: near those within radius of the place, gm the GM who fired it, world everyone in the place's world, all everyone in the game", database.TriggerScopes...),
		"radius":  number("near: how far from the place (0: 100)"),
		"world":   integer("-1: where the GM who fires it stands; else the world of its own place (0 Tunaria, 1 Rathe, 2 Odus...)"),
		"x":       number("its own place"),
		"y":       number("its own place's height"),
		"z":       number("its own place"),
		"steps":   map[string]any{"description": triggerStepsHelp + `. A JSON list, or that list as a string`},
		"notes":   str("for GMs"),
	}, "slot")
	s.addOp(&Op{
		Name: "trigger_list", Title: "List the triggers", Scope: scopeRead,
		Description: "Every GM trigger (scripted events): its slot (1 to 8 are the TEST menu's TRIGGER lines), name, " +
			"whether it is on, whom it reaches and where, how many steps, how many problems keep it from running, " +
			"and what runs now.",
		Handler: func(c *opCtx) (any, error) { return c.s.triggerList() },
	})
	s.addOp(&Op{
		Name: "trigger_get", Title: "Get a trigger", Scope: scopeRead,
		Description: "One trigger with its steps, what keeps it from running as written (problems: a step missing " +
			"what it needs, an NPC or an item this server lacks), and its runs.",
		Input: obj(map[string]any{"slot": integer("the trigger's slot")}, "slot"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Slot int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.triggerGet(req.Slot)
		},
	})
	s.addOp(&Op{
		Name: "trigger_save", Title: "Save a trigger", Scope: scopeWrite,
		Description: "Make or change a trigger (its row by slot), logged and revertible. The next firing runs it, " +
			"and vanilla's TEST menu shows its name at once. The answer lists its problems: a trigger with any " +
			"cannot fire.",
		Input: obj(map[string]any{"trigger": triggerSchema}, "trigger"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Trigger map[string]any }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.triggerSave(c.a, req.Trigger)
		},
	})
	s.addOp(&Op{
		Name: "trigger_delete", Title: "Delete a trigger", Scope: scopeWrite, Destructive: true,
		Description: "Delete a trigger's row (logged and revertible). Its runs go on: trigger_stop ends them.",
		Input:       obj(map[string]any{"slot": integer("the trigger's slot")}, "slot"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Slot int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.triggerDelete(c.a, req.Slot)
		},
	})
	s.addOp(&Op{
		Name: "trigger_fire", Title: "Fire a trigger", Scope: scopeWrite,
		Description: "Fire a trigger now, as its TEST menu line does in the game: its steps run in order (a wait " +
			"puts the rest off). One at the place where the GM stands is fired from your character in the world; " +
			"one with a place of its own fires without one. Logged. The answer is what it did.",
		Input: obj(map[string]any{"slot": integer("the trigger's slot")}, "slot"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Slot int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.triggerFire(c.a, req.Slot)
		},
	})
	s.addOp(&Op{
		Name: "trigger_stop", Title: "Stop a trigger's runs", Scope: scopeWrite, Destructive: true,
		Description: "End the runs of a trigger (slot 0: every run): their steps left undone, their spawns alive " +
			"taken away. Logged.",
		Input: obj(map[string]any{"slot": integer("the trigger's slot; 0: every run")}, "slot"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Slot int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.triggerStop(c.a, req.Slot)
		},
	})
	s.addOp(&Op{
		Name: "trigger_runs", Title: "What the triggers run now", Scope: scopeRead,
		Description: "Every run now: its trigger, who fired it and where, its next step, a wait's seconds left, " +
			"its spawns alive.",
		Handler: func(c *opCtx) (any, error) {
			e, err := triggers()
			if err != nil {
				return nil, err
			}
			return map[string]any{"runs": listOf(e.Runs())}, nil
		},
	})
}

// triggers is the running engine, or an error saying it is not.
func triggers() (*handlers.TriggerEngine, error) {
	e := handlers.Triggers()
	if e == nil {
		return nil, errors.New("the triggers are not running in this server")
	}
	return e, nil
}

// listOf is v, or an empty list for none (a JSON [] rather than null).
func listOf[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// triggerRow is a trigger as the page lists it.
type triggerRow struct {
	Slot     int     `json:"slot"`
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Scope    string  `json:"scope"`
	Radius   float32 `json:"radius"`
	World    int     `json:"world"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Z        float32 `json:"z"`
	Steps    int     `json:"steps"`
	Problems int     `json:"problems"`
	Running  bool    `json:"running"`
	Menu     bool    `json:"menu"` // one of the TEST menu's eight lines
}

func (s *Server) triggerList() (any, error) {
	all, err := s.d.DB.GMTriggers()
	if err != nil {
		return nil, err
	}
	var running map[int]bool
	var runs []handlers.TriggerRunInfo
	if e := handlers.Triggers(); e != nil {
		running, runs = e.Running(), e.Runs()
	}
	out := make([]triggerRow, 0, len(all))
	for _, t := range all {
		steps, _ := database.ParseTriggerSteps(t.Steps)
		out = append(out, triggerRow{Slot: t.Slot, Name: t.Name, Enabled: t.Enabled, Scope: t.Scope, Radius: t.Radius,
			World: t.World, X: t.X, Y: t.Y, Z: t.Z, Steps: len(steps), Problems: len(s.triggerProblems(t)),
			Running: running[t.Slot], Menu: t.Slot >= 1 && t.Slot <= database.TriggerMenuSlots})
	}
	return map[string]any{"triggers": out, "runs": listOf(runs), "scopes": database.TriggerScopes,
		"actions": database.TriggerActions, "menuSlots": database.TriggerMenuSlots}, nil
}

// triggerProblems is database.TriggerProblems and what the world lacks: an
// NPC a say or spawn names (say's speaker is only words, so only a spawn's),
// an item a give names.
func (s *Server) triggerProblems(t database.GMTrigger) []string {
	out := database.TriggerProblems(t)
	steps, err := database.ParseTriggerSteps(t.Steps)
	if err != nil {
		return out
	}
	var items *database.ItemPatternStore
	if s.d.DB != nil {
		items = database.NewItemPatternStore(s.d.DB)
	}
	for i, st := range steps {
		switch st.Do {
		case database.TriggerSpawn:
			if n := strings.TrimSpace(st.NPC); n != "" && s.d.World != nil {
				if _, _, _, _, _, ok := s.d.World.NPCByName(n); !ok {
					out = append(out, fmt.Sprintf("step %d: no NPC named %q is in the world to copy", i+1, n))
				}
			}
		case database.TriggerGive:
			for _, it := range database.ParseItemCounts(st.Items) {
				if items == nil {
					break
				}
				if p, err := items.GetItemPattern(it.PatternID); err != nil || p == nil {
					out = append(out, fmt.Sprintf("step %d: no item %d", i+1, it.PatternID))
				}
			}
		}
	}
	return out
}

func (s *Server) triggerGet(slot int) (any, error) {
	t, ok, err := s.d.DB.GMTriggerBySlot(slot)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound("no trigger in slot %d", slot)
	}
	steps, perr := database.ParseTriggerSteps(t.Steps)
	var runs []handlers.TriggerRunInfo
	if e := handlers.Triggers(); e != nil {
		for _, r := range e.Runs() {
			if r.Slot == slot {
				runs = append(runs, r)
			}
		}
	}
	out := map[string]any{
		"trigger": map[string]any{"slot": t.Slot, "name": t.Name, "enabled": boolInt(t.Enabled), "scope": t.Scope,
			"radius": t.Radius, "world": t.World, "x": t.X, "y": t.Y, "z": t.Z, "steps": t.Steps, "notes": t.Notes},
		"problems": listOf(s.triggerProblems(t)),
		"runs":     listOf(runs),
		"menu":     slot >= 1 && slot <= database.TriggerMenuSlots,
	}
	if perr == nil {
		out["stepList"] = listOf(steps)
	}
	return out, nil
}

// triggerSave writes a trigger's row by its slot, an insert or the columns
// that differ, as a logged row change; the engine reads the table again
// (applyLive).
func (s *Server) triggerSave(a *actor, trig map[string]any) (any, error) {
	t, err := s.table("gmtriggers")
	if err != nil {
		return nil, err
	}
	vals := map[string]any{}
	for _, c := range triggerCols {
		if v, ok := trig[c]; ok {
			vals[c] = v
		}
	}
	// steps as a list (an agent's) or its JSON (the page's), kept in the
	// engine's own form: each step's fields in TriggerStep's order, a field
	// it has no use for dropped (a problem then says what is missing)
	if v, ok := vals["steps"]; ok && v != nil {
		text, isText := v.(string)
		if !isText {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, badRequest("steps: %v", err)
			}
			text = string(b)
		}
		if steps, err := database.ParseTriggerSteps(text); err == nil {
			text = database.FormatTriggerSteps(steps)
		}
		vals["steps"] = text
	}
	f, ok := asFloat(vals["slot"])
	if !ok || f < 1 || f != math.Trunc(f) {
		return nil, badRequest("slot is a whole number, 1 or more (1 to 8: the TEST menu's TRIGGER lines)")
	}
	slot := int64(f)
	vals["slot"] = slot
	key := map[string]any{"slot": slot}
	var ch *change
	if cur, err := getRow(s.d.DB, t, key); err != nil {
		if ch, err = s.insertRow(a, t, vals, writeOpts{}); err != nil {
			return nil, err
		}
	} else {
		diff := map[string]any{}
		for k, v := range vals {
			if k != "slot" && !sameValue(v, cur[k]) {
				diff[k] = v
			}
		}
		if len(diff) > 0 {
			if ch, err = s.updateRow(a, t, key, diff, nil, writeOpts{}); err != nil {
				return nil, err
			}
		}
	}
	got, err := s.triggerGet(int(slot))
	if err != nil {
		return nil, err
	}
	out := got.(map[string]any)
	if ch != nil {
		out["change"] = ch
	} else {
		out["message"] = "nothing changed"
	}
	return out, nil
}

func (s *Server) triggerDelete(a *actor, slot int) (any, error) {
	t, err := s.table("gmtriggers")
	if err != nil {
		return nil, err
	}
	ch, err := s.deleteRow(a, t, map[string]any{"slot": int64(slot)}, writeOpts{})
	if err != nil {
		return nil, err
	}
	return map[string]any{"change": ch}, nil
}

// triggerFire fires a trigger from the panel: from the GM's character in
// the world when it has one (a trigger at the GM's place needs it), and logs
// it.
func (s *Server) triggerFire(a *actor, slot int) (any, error) {
	e, err := triggers()
	if err != nil {
		return nil, err
	}
	by := handlers.TriggerFirer{Name: a.Username + " (GM panel)"}
	if sess := s.gmSession(a); sess != nil {
		by.Sess, by.Player = sess, s.d.World.SessionPlayer(sess)
	}
	lines, err := e.Fire(slot, by)
	if err != nil {
		return nil, badRequest("%s", strings.Join(lines, "; "))
	}
	s.audit(a, "live", "gmtriggers", map[string]any{"slot": int64(slot)}, nil, nil,
		fmt.Sprintf("fired trigger %d: %s", slot, strings.Join(lines, "; ")))
	return map[string]any{"lines": lines, "runs": listOf(e.Runs())}, nil
}

func (s *Server) triggerStop(a *actor, slot int) (any, error) {
	e, err := triggers()
	if err != nil {
		return nil, err
	}
	if slot < 0 {
		return nil, badRequest("slot is a trigger's, or 0 for every run")
	}
	runs, spawns := e.StopRuns(slot)
	what := fmt.Sprintf("trigger %d's runs", slot)
	if slot == 0 {
		what = "every run"
	}
	msg := fmt.Sprintf("stopped %s: %d runs, %d spawns taken away", what, runs, spawns)
	s.audit(a, "live", "gmtriggers", nil, nil, nil, msg)
	return map[string]any{"runs": runs, "spawns": spawns, "message": msg}, nil
}

// liveTriggers is a gmtriggers row change applied to the running engine.
func (s *Server) liveTriggers() string {
	e := handlers.Triggers()
	if e == nil {
		return "the triggers are not running in this server"
	}
	if err := e.Reload(); err != nil {
		return "not read again: " + err.Error()
	}
	return "at once: the next firing runs it, and vanilla's TEST menu shows its name"
}

// checkTriggerRow holds a gmtriggers row to what the table means: a slot of
// 1 or more, a scope of the four, a world of -1 to 29, no negative radius,
// steps a JSON list (or none). A step's own problems are saved and listed
// (trigger_get): such a trigger cannot fire.
func checkTriggerRow(row map[string]any) error {
	if v, ok := row["slot"]; ok && v != nil {
		f, fok := asFloat(v)
		if !fok || f < 1 || f != math.Trunc(f) {
			return badRequest("slot is a whole number, 1 or more")
		}
	}
	if v, ok := row["scope"]; ok && v != nil {
		sc := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
		if !containsStr(database.TriggerScopes, sc) {
			return badRequest("scope is one of %s", strings.Join(database.TriggerScopes, ", "))
		}
	}
	if v, ok := row["world"]; ok && v != nil {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f < -1 || f > 29 {
			return badRequest("world is -1 (where the GM stands) or 0 to 29")
		}
	}
	for _, col := range []string{"radius", "x", "y", "z"} {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
		if col == "radius" && f < 0 {
			return badRequest("radius is 0 (100) or more")
		}
	}
	if v, ok := row["steps"]; ok && v != nil {
		if _, err := database.ParseTriggerSteps(fmt.Sprint(v)); err != nil {
			return badRequest("steps are not a JSON list of steps: %v", err)
		}
	}
	return nil
}
