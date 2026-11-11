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
	TriggerDefaultStay   = 30    // minutes
	TriggerMaxWait       = 14400 // 4 h: a portal shut 45 minutes to 3 hours (the Plane of Disease's Bloody Portal)
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
	// A run that is a place's whole event (the guides' raid encounters, fired by a
	// quest's event or a creature's death: npcdeath): what its players and creatures
	// must do before it goes on, and how long they have.
	TriggerLock      = "lock"      // while this run lives no trigger of the same lock fires
	TriggerPlace     = "place"     // the run's place moved: world/x/y/z, or dead: where its last spawn died; radius its reach
	TriggerClock     = "clock"     // the time the run has (seconds): at its end it goes on from its end step
	TriggerEnd       = "end"       // where a clock's end goes on from
	TriggerAwaitTalk = "awaittalk" // the rest waits for a player to talk to npc
	TriggerAwaitDead = "awaitdead" // the rest waits for the run's spawns (of npc; none: all) to be dead, or any: for one to die
	TriggerWalk      = "walk"      // the NPCs named npc near the place walk (run) to x/y/z
	TriggerHome      = "home"      // and walk back to their spawns
	TriggerHide      = "hide"      // the NPCs named npc near the place out of every view for minutes (0: back)
	TriggerRouse     = "rouse"     // the run's spawns named npc attack on sight from now on
	TriggerSic       = "sic"       // the run's spawns named npc (none: all) attack the NPC named target nearest each
	// The run's spawn named npc called back while its spawn named by lives: seconds after the
	// last one fell, one more beside it, set on target (the user, 2026-11-09: "cause tunare to
	// resummon her pets"), from this step to the run's end
	TriggerResummon = "resummon"
	// The NPC named npc nearest the run's place guarded from this step to the run's end:
	// its death (or its going) fails the run, text said to its players, its spawns taken
	// away and the rest left undone (petition 1617: "If the herbman dies while you are
	// making potions, you will have to wait until someone else triggers the event")
	TriggerProtect = "protect"
	// A quest's step (quest, step) done for each player the scope takes who is on it and
	// holds what it needs, as at its NPC: what a held trigger event (database.QuestEvent's
	// Hold) left undone ("Since the popups have an Area of Effect, your quest will still be
	// advanced if you have the necessary items")
	TriggerQuestStep = "queststep"
)

// TriggerActions is every action, in the order the panel offers them.
var TriggerActions = []string{TriggerBroadcast, TriggerPopup, TriggerSay, TriggerGive, TriggerVisual,
	TriggerTeleport, TriggerSpawn, TriggerDespawn, TriggerStop, TriggerWait,
	TriggerLock, TriggerPlace, TriggerClock, TriggerEnd, TriggerAwaitTalk, TriggerAwaitDead, TriggerWalk, TriggerHome, TriggerHide, TriggerRouse, TriggerSic,
	TriggerResummon, TriggerProtect, TriggerQuestStep}

// TriggerStep is one step of a trigger, kept in the steps column's array.
type TriggerStep struct {
	Do      string  `json:"do"`
	Text    string  `json:"text,omitempty"`    // broadcast, popup, say: the words (%p the player's name)
	Answer  string  `json:"answer,omitempty"`  // popup: its one answer ("OK" when none)
	NPC     string  `json:"npc,omitempty"`     // say: who says it; spawn: whom; despawn: whom (none: all)
	Items   string  `json:"items,omitempty"`   // give: patternid*count, comma separated
	FX      string  `json:"fx,omitempty"`      // visual: its DictID (0x1DAEB5B6)
	Seconds float64 `json:"seconds,omitempty"` // visual: its casting part (0 to 30); wait: how long
	Upto    float64 `json:"upto,omitempty"`    // wait: the longest it may be, a random time from seconds to upto (0: seconds)
	Count   int     `json:"count,omitempty"`   // spawn: how many (0: one)
	Spread  float32 `json:"spread,omitempty"`  // spawn: how far round its place (0: TriggerDefaultSpread)
	Minutes float64 `json:"minutes,omitempty"` // spawn: how long they stay alive (0: TriggerDefaultStay)
	Hostile bool    `json:"hostile,omitempty"` // spawn: they may be attacked, and attack players on sight
	World   *int    `json:"world,omitempty"`   // teleport: the world; spawn: a place of its own (none: the run's)
	X       float32 `json:"x,omitempty"`
	Y       float32 `json:"y,omitempty"`
	Z       float32 `json:"z,omitempty"`
	Slot    int     `json:"slot,omitempty"`    // stop: the trigger whose runs end (0: every other)
	Lock    string  `json:"lock,omitempty"`    // lock: its name
	Radius  float32 `json:"radius,omitempty"`  // place: the run's reach from now on; walk, home, hide: how far from the place (0: its reach)
	Dead    bool    `json:"dead,omitempty"`    // place: where the run's last spawn died (where it fell back on world/x/y/z)
	Max     int     `json:"max,omitempty"`     // teleport: how many it takes at most, the nearest first, a pet counting as one (0: all)
	Any     bool    `json:"any,omitempty"`     // awaitdead: the first death, not the last
	Classes []int   `json:"classes,omitempty"` // awaittalk: the classes (WAR=0..ALC=14) it answers (none: any)
	Refuse  string  `json:"refuse,omitempty"`  // awaittalk: said to another class; teleport: said to those it cannot take
	// Attackable (spawn): it may be attacked, and does not attack on sight (until a rouse)
	Attackable bool `json:"attackable,omitempty"`
	// Target (sic): the NPC they attack, by its name in the game: of that name the one nearest
	// each spawn within the step's radius (0: the run's reach), in its world (the user,
	// 2026-11-08: "spawn nagafen attacking mr moogie")
	Target string `json:"target,omitempty"`
	// Casts (sic): they fight the target by their whole class template, its casts and
	// specials too (entity.SetNPCOn; the user, 2026-11-09: Tunare, "a strong druid god",
	// fights Mr. Moogie with a druid's spells); without it a fight between two NPCs is their
	// melee alone
	Casts bool `json:"casts,omitempty"`
	// Empty (clock): its seconds with no player within the run's reach end the clock at once
	Empty float64 `json:"empty,omitempty"`
	// By (resummon): the run's spawn, by name, that calls npc back while it lives; seconds the
	// time after npc's last one fell, target, radius and casts the sic it is given, attackable,
	// hostile and minutes its spawn's, text a line in its players' chat as it comes
	By string `json:"by,omitempty"`
	// Quest and QuestStep (queststep): the quest's id and the step done
	Quest     int `json:"quest,omitempty"`
	QuestStep int `json:"step,omitempty"`
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
	if n := t.ListName(); n != "" {
		return n
	}
	return fmt.Sprintf("Trigger %d", t.Slot)
}

// ListName is the name vanilla's whole list carries (0x7E36, which carries the
// slot beside it): the trigger's name cut to TriggerMaxName bytes on a
// character's edge, "" when it has none.
func (t GMTrigger) ListName() string {
	n := strings.TrimSpace(t.Name)
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
	if _, err := db.Exec(npcDeathSchema); err != nil {
		return fmt.Errorf("npcdeath: %w", err)
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
			if s.Upto != 0 && (s.Upto <= s.Seconds || s.Upto > TriggerMaxWait) {
				add(i, "wait's upto %.1f s: more than its %.1f, at most %d", s.Upto, s.Seconds, TriggerMaxWait)
			}
		case TriggerLock:
			if strings.TrimSpace(s.Lock) == "" {
				add(i, "lock names no lock")
			}
		case TriggerPlace:
			if s.World == nil && !s.Dead && s.Radius <= 0 {
				add(i, "place names no place (world, x, y, z; dead; radius)")
			}
			if s.World != nil && (*s.World < 0 || *s.World > 29) {
				add(i, "place's world %d: 0 to 29", *s.World)
			}
		case TriggerClock:
			if s.Seconds <= 0 || s.Seconds > TriggerMaxMinutes*60 {
				add(i, "clock %.1f s: more than 0, at most %d", s.Seconds, TriggerMaxMinutes*60)
			}
		case TriggerEnd:
		case TriggerAwaitTalk, TriggerWalk, TriggerHome, TriggerHide, TriggerRouse:
			if strings.TrimSpace(s.NPC) == "" {
				add(i, "%s names no NPC", s.Do)
			}
			if s.Do == TriggerWalk && s.X == 0 && s.Z == 0 {
				add(i, "walk's place is 0, 0")
			}
		case TriggerAwaitDead:
		case TriggerProtect:
			if strings.TrimSpace(s.NPC) == "" {
				add(i, "protect names no NPC")
			}
		case TriggerQuestStep:
			if s.Quest <= 0 || s.QuestStep <= 0 {
				add(i, "queststep names no quest and step (quest, step)")
			}
		case TriggerSic:
			if strings.TrimSpace(s.Target) == "" {
				add(i, "sic names no target")
			}
		case TriggerResummon:
			if strings.TrimSpace(s.NPC) == "" {
				add(i, "resummon names no NPC to call back")
			}
			if strings.TrimSpace(s.By) == "" {
				add(i, "resummon names no NPC who calls it back (by)")
			}
			if s.Seconds < 0 || s.Seconds > TriggerMaxWait {
				add(i, "resummon's %.1f s after a death: 0 to %d", s.Seconds, TriggerMaxWait)
			}
			if s.Minutes < 0 || s.Minutes > TriggerMaxMinutes {
				add(i, "resummon's stay %.1f minutes: 0 (%d) to %d", s.Minutes, TriggerDefaultStay, TriggerMaxMinutes)
			}
		}
		switch s.Do {
		case TriggerAwaitTalk, TriggerAwaitDead:
			if s.Seconds < 0 || s.Seconds > TriggerMaxMinutes*60 {
				add(i, "%s's %.1f s: 0 (for good, or the clock's) to %d", s.Do, s.Seconds, TriggerMaxMinutes*60)
			}
		case TriggerBroadcast, TriggerPopup, TriggerSay, TriggerGive, TriggerVisual, TriggerTeleport, TriggerSpawn,
			TriggerDespawn, TriggerStop, TriggerWait, TriggerLock, TriggerPlace, TriggerClock, TriggerEnd,
			TriggerWalk, TriggerHome, TriggerHide, TriggerRouse, TriggerSic, TriggerResummon, TriggerProtect, TriggerQuestStep:
		default:
			add(i, "do %q is not one of %s", s.Do, strings.Join(TriggerActions, ", "))
		}
	}
	return out
}

// npcdeath: a creature's death fires a trigger where it fell (handlers/gmtriggers.go),
// as the guides' spawn cycles have it ("Kill the Captain and a Commander will spawn"):
// a row a name (as npcclass keys one) and a trigger, by the trigger's name, chance a kill
// in percent. Only a kill a player is credited with counts, and a trigger of a lock
// already running does not fire again.
const npcDeathSchema = `
	CREATE TABLE IF NOT EXISTS npcdeath (
		name    TEXT NOT NULL,
		trigger TEXT NOT NULL,
		chance  REAL NOT NULL DEFAULT 100,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (name, trigger)
	);`

// NPCDeathTrigger is one row of npcdeath.
type NPCDeathTrigger struct {
	Trigger string
	Chance  float64
}

// NPCDeathTriggers is table npcdeath by name (NPCClassKey).
func (db *SQLiteDB) NPCDeathTriggers() (map[string][]NPCDeathTrigger, error) {
	rows, err := db.Query(`SELECT name, trigger, chance FROM npcdeath ORDER BY name, trigger`)
	if err != nil {
		return nil, fmt.Errorf("npcdeath: %w", err)
	}
	defer rows.Close()
	out := map[string][]NPCDeathTrigger{}
	for rows.Next() {
		var name string
		var t NPCDeathTrigger
		if err := rows.Scan(&name, &t.Trigger, &t.Chance); err != nil {
			return nil, fmt.Errorf("npcdeath: %w", err)
		}
		k := NPCClassKey(name)
		out[k] = append(out[k], t)
	}
	return out, rows.Err()
}

// TriggerLockName is the lock of a trigger's steps, its first lock step's, or "".
func TriggerLockName(steps []TriggerStep) string {
	for _, s := range steps {
		if s.Do == TriggerLock {
			return strings.ToLower(strings.TrimSpace(s.Lock))
		}
	}
	return ""
}
