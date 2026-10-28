package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// gmtriggers: a GM's scripted events (handlers/gmtriggers.go), a row a
// trigger. The GM's TEST menu has eight lines under Programming > TRIGGER,
// each sending 0x0702 {100, its index}, and nothing records what SOE's
// server did with one (claude-notes/gm-trigger-0702-20261028.md): here slot
// 1 to 8 is the line, top to bottom (the index plus one), and a slot past 8
// is fired by #trigger or the GM panel alone.
//
// A firing is a run: its steps in order, a wait putting the rest off, at a
// place (world -1: where the firing GM stands; else world, x, y, z) for the
// players its scope takes there (gm the firing GM alone, near those within
// radius of the place (0: TriggerNearRadius), world everyone in its world,
// all everyone in the game), looked at again at each step. steps is a JSON
// array of TriggerStep. enabled 0 keeps a trigger from firing.
const gmTriggersSchema = `
	CREATE TABLE IF NOT EXISTS gmtriggers (
		slot    INTEGER PRIMARY KEY,
		name    TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		scope   TEXT NOT NULL DEFAULT 'near',
		radius  REAL NOT NULL DEFAULT 0,
		world   INTEGER NOT NULL DEFAULT -1,
		x       REAL NOT NULL DEFAULT 0,
		y       REAL NOT NULL DEFAULT 0,
		z       REAL NOT NULL DEFAULT 0,
		steps   TEXT NOT NULL DEFAULT '',
		notes   TEXT NOT NULL DEFAULT ''
	);`

// The scopes of gmtriggers.scope: whom a step that reaches players reaches.
const (
	TriggerScopeGM    = "gm"
	TriggerScopeNear  = "near"
	TriggerScopeWorld = "world"
	TriggerScopeAll   = "all"
)

// TriggerScopes is every scope, in the order the panel offers them.
var TriggerScopes = []string{TriggerScopeNear, TriggerScopeGM, TriggerScopeWorld, TriggerScopeAll}

// TriggerNearRadius is a near trigger's reach when its row says 0: a camp's
// size, past a town square's.
const TriggerNearRadius float32 = 100

// TriggerMenuSlots is how many slots the TEST menu's TRIGGER lines reach.
const TriggerMenuSlots = 8

// The bounds of a trigger: steps, a spawn's count, spread and stay, a wait,
// a name for the TEST menu (vanilla's 0x7E08 sends at most this many bytes).
const (
	TriggerMaxSteps      = 100
	TriggerMaxSpawn      = 50
	TriggerMaxSpread     = 200
	TriggerMaxMinutes    = 240
	TriggerDefaultSpread = 5
	TriggerDefaultStay   = 30 // minutes
	TriggerMaxWait       = 3600
	TriggerMaxName       = 32
)

// What a trigger's step does.
const (
	TriggerBroadcast = "broadcast" // a line in each player's chat, in the GMs' yellow
	TriggerPopup     = "popup"     // a window each player closes
	TriggerSay       = "say"       // an NPC's words to each player, a box a line
	TriggerGive      = "give"      // items into each player's inventory
	TriggerVisual    = "visual"    // a spell visual played on each player
	TriggerTeleport  = "teleport"  // each player moved to a place
	TriggerSpawn     = "spawn"     // copies of an NPC round the place, for a while
	TriggerDespawn   = "despawn"   // this run's spawns taken away (of one NPC's name)
	TriggerStop      = "stop"      // another trigger's runs ended (slot 0: every other)
	TriggerWait      = "wait"      // the rest put off
)

// TriggerActions is every action, in the order the panel offers them.
var TriggerActions = []string{TriggerBroadcast, TriggerPopup, TriggerSay, TriggerGive, TriggerVisual,
	TriggerTeleport, TriggerSpawn, TriggerDespawn, TriggerStop, TriggerWait}

// TriggerStep is one step of a trigger, kept in the steps column's array.
type TriggerStep struct {
	Do      string  `json:"do"`
	Text    string  `json:"text,omitempty"`    // broadcast, popup, say: the words (%p the player's name)
	Answer  string  `json:"answer,omitempty"`  // popup: its one answer ("OK" when none)
	NPC     string  `json:"npc,omitempty"`     // say: who says it; spawn: whom; despawn: whom (none: all)
	Items   string  `json:"items,omitempty"`   // give: patternid*count, comma separated
	FX      string  `json:"fx,omitempty"`      // visual: its DictID (0x1DAEB5B6)
	Seconds float64 `json:"seconds,omitempty"` // visual: its casting part (0 to 30); wait: how long
	Count   int     `json:"count,omitempty"`   // spawn: how many (0: one)
	Spread  float32 `json:"spread,omitempty"`  // spawn: how far round its place (0: TriggerDefaultSpread)
	Minutes float64 `json:"minutes,omitempty"` // spawn: how long they stay alive (0: TriggerDefaultStay)
	Hostile bool    `json:"hostile,omitempty"` // spawn: they may be attacked, and attack players on sight
	World   *int    `json:"world,omitempty"`   // teleport: the world; spawn: a place of its own (none: the run's)
	X       float32 `json:"x,omitempty"`
	Y       float32 `json:"y,omitempty"`
	Z       float32 `json:"z,omitempty"`
	Slot    int     `json:"slot,omitempty"` // stop: the trigger whose runs end (0: every other)
}

// GMTrigger is one row of gmtriggers.
type GMTrigger struct {
	Slot    int
	Name    string
	Enabled bool
	Scope   string
	Radius  float32
	World   int // -1: where the firing GM stands
	X, Y, Z float32
	Steps   string
	Notes   string
}

// Fixed reports whether the trigger has a place of its own.
func (t GMTrigger) Fixed() bool { return t.World >= 0 }

// Reach is a near trigger's radius: its own, or TriggerNearRadius.
func (t GMTrigger) Reach() float32 {
	if t.Radius > 0 {
		return t.Radius
	}
	return TriggerNearRadius
}

// Title is the trigger as a GM reads it: "Trigger 3 (Elemental Invasion)".
func (t GMTrigger) Title() string {
	if n := strings.TrimSpace(t.Name); n != "" {
		return fmt.Sprintf("Trigger %d (%s)", t.Slot, n)
	}
	return fmt.Sprintf("Trigger %d", t.Slot)
}

// MenuName is the name the TEST menu shows (vanilla, 0x7E08): the trigger's
// name cut to TriggerMaxName bytes on a character's edge, else "Trigger n".
func (t GMTrigger) MenuName() string {
	n := strings.TrimSpace(t.Name)
	if n == "" {
		n = fmt.Sprintf("Trigger %d", t.Slot)
	}
	for len(n) > TriggerMaxName {
		r := []rune(n)
		n = string(r[:len(r)-1])
	}
	return n
}

// ensureGMTriggers makes table gmtriggers (initSchema).
func (db *SQLiteDB) ensureGMTriggers() error {
	if _, err := db.Exec(gmTriggersSchema); err != nil {
		return fmt.Errorf("gmtriggers: %w", err)
	}
	return nil
}

const gmTriggerCols = `slot, name, enabled, scope, radius, world, x, y, z, steps, notes`

func scanGMTrigger(sc interface{ Scan(...any) error }) (GMTrigger, error) {
	var t GMTrigger
	var enabled int
	err := sc.Scan(&t.Slot, &t.Name, &enabled, &t.Scope, &t.Radius, &t.World, &t.X, &t.Y, &t.Z, &t.Steps, &t.Notes)
	t.Enabled = enabled != 0
	t.Scope = strings.ToLower(strings.TrimSpace(t.Scope))
	return t, err
}

// GMTriggers is every row of gmtriggers, by slot.
func (db *SQLiteDB) GMTriggers() ([]GMTrigger, error) {
	rows, err := db.Query(`SELECT ` + gmTriggerCols + ` FROM gmtriggers ORDER BY slot`)
	if err != nil {
		return nil, fmt.Errorf("gmtriggers: %w", err)
	}
	defer rows.Close()
	var out []GMTrigger
	for rows.Next() {
		t, err := scanGMTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("gmtriggers: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GMTriggerBySlot is the trigger in a slot, false when there is none.
func (db *SQLiteDB) GMTriggerBySlot(slot int) (GMTrigger, bool, error) {
	t, err := scanGMTrigger(db.QueryRow(`SELECT `+gmTriggerCols+` FROM gmtriggers WHERE slot = ?`, slot))
	if errors.Is(err, sql.ErrNoRows) {
		return GMTrigger{}, false, nil
	}
	if err != nil {
		return GMTrigger{}, false, fmt.Errorf("gmtriggers: %w", err)
	}
	return t, true, nil
}

// ParseTriggerSteps reads a steps column: a JSON array of TriggerStep, ""
// none. What a step does is lowered and trimmed.
func ParseTriggerSteps(s string) ([]TriggerStep, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []TriggerStep
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Do = strings.ToLower(strings.TrimSpace(out[i].Do))
	}
	return out, nil
}

// FormatTriggerSteps writes steps as ParseTriggerSteps reads them ("" none).
func FormatTriggerSteps(v []TriggerStep) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// TriggerProblems is what keeps a trigger from running as written: a slot
// below 1, a scope or a place there is not, steps that are not a list, a step
// missing what it needs or past its bounds. It checks nothing in the database
// or the world (the GM panel's trigger_get adds that).
func TriggerProblems(t GMTrigger) []string {
	var out []string
	switch {
	case t.Slot < 1:
		out = append(out, fmt.Sprintf("slot %d: 1 or more (1 to 8 are the TEST menu's TRIGGER lines)", t.Slot))
	}
	if !containsString(TriggerScopes, t.Scope) {
		out = append(out, fmt.Sprintf("scope %q is not one of %s", t.Scope, strings.Join(TriggerScopes, ", ")))
	}
	if t.Radius < 0 {
		out = append(out, fmt.Sprintf("radius %.1f: 0 (%.0f) or more", t.Radius, TriggerNearRadius))
	}
	if t.World < -1 || t.World > 29 {
		out = append(out, fmt.Sprintf("world %d: -1 (where the GM stands) or 0 to 29", t.World))
	}
	steps, err := ParseTriggerSteps(t.Steps)
	if err != nil {
		return append(out, "its steps are not a JSON list of steps: "+err.Error())
	}
	return append(out, StepProblems(steps)...)
}

// StepProblems is what is wrong with a trigger's steps, each by its number.
func StepProblems(steps []TriggerStep) []string {
	var out []string
	add := func(i int, format string, a ...any) {
		out = append(out, fmt.Sprintf("step %d: ", i+1)+fmt.Sprintf(format, a...))
	}
	if len(steps) == 0 {
		out = append(out, "it has no steps: it would do nothing")
	}
	if len(steps) > TriggerMaxSteps {
		out = append(out, fmt.Sprintf("%d steps: %d at most", len(steps), TriggerMaxSteps))
	}
	for i, s := range steps {
		switch s.Do {
		case TriggerBroadcast, TriggerPopup, TriggerSay:
			if strings.TrimSpace(s.Text) == "" {
				add(i, "%s says nothing (text)", s.Do)
			}
		case TriggerGive:
			if len(ParseItemCounts(s.Items)) == 0 {
				add(i, "give names no items (patternid*count)")
			}
		case TriggerVisual:
			if _, ok := ParseVisualID(s.FX); !ok {
				add(i, "visual's fx %q is not a DictID (0x1DAEB5B6)", s.FX)
			}
			if s.Seconds < 0 || s.Seconds > 30 {
				add(i, "visual's casting part %.1f s: 0 to 30", s.Seconds)
			}
		case TriggerTeleport:
			switch {
			case s.World == nil:
				add(i, "teleport names no world")
			case *s.World < 0 || *s.World > 29:
				add(i, "teleport's world %d: 0 to 29", *s.World)
			}
			if s.X == 0 && s.Z == 0 {
				add(i, "teleport's place is 0, 0")
			}
		case TriggerSpawn:
			if strings.TrimSpace(s.NPC) == "" {
				add(i, "spawn names no NPC")
			}
			if s.Count < 0 || s.Count > TriggerMaxSpawn {
				add(i, "spawn's count %d: 1 to %d (0 is one)", s.Count, TriggerMaxSpawn)
			}
			if s.Spread < 0 || s.Spread > TriggerMaxSpread {
				add(i, "spawn's spread %.1f: 0 to %d", s.Spread, TriggerMaxSpread)
			}
			if s.Minutes < 0 || s.Minutes > TriggerMaxMinutes {
				add(i, "spawn's stay %.1f minutes: 0 (%d) to %d", s.Minutes, TriggerDefaultStay, TriggerMaxMinutes)
			}
			if s.World != nil {
				if *s.World < 0 || *s.World > 29 {
					add(i, "spawn's world %d: 0 to 29", *s.World)
				}
				if s.X == 0 && s.Z == 0 {
					add(i, "spawn's place is 0, 0")
				}
			}
		case TriggerDespawn:
		case TriggerStop:
			if s.Slot < 0 {
				add(i, "stop's slot %d: 0 (every other trigger) or a trigger's", s.Slot)
			}
		case TriggerWait:
			if s.Seconds <= 0 || s.Seconds > TriggerMaxWait {
				add(i, "wait %.1f s: more than 0, at most %d", s.Seconds, TriggerMaxWait)
			}
		default:
			add(i, "do %q is not one of %s", s.Do, strings.Join(TriggerActions, ", "))
		}
	}
	return out
}
