package entity

// npctime.go — NPCs that stand only by day, or only by night (table npctime,
// database/npctime.go; the owner, 2026-11-21, from a walkthrough of Southwatch
// Keep: "mercenaries by day, skeletal watchmen by night; the wildlife round it
// changes with the time of day too").
//
// Day is the sun over the horizon as every client draws it: Daylight
// (vampire.go), after 6:00 and before 18:00 of the game's clock, the sky's
// sunrise and sunset (VIAtmosphere::Update turns the sun a fifteenth of a half
// turn an hour; its light comes the same way, straight down at noon, and the
// stars are out only outside those hours). The clock is the server's own
// (session.ServerHour: the 0x0013 every client in the world is sent at entry
// and every two minutes, so a client's sky is within a game minute of it). A
// game day is 61.04 real minutes: each turn comes about every half hour.
//
// The turn: an NPC out of its time is held out of the world as a spawn-only one
// is (NPC.offHours, hiddenAt: in no view, attacking, helping and wandering for
// no one), put back on its spawn whole. One fighting (aiQuiet false: an enemy,
// a round, a chase) stays until its fight is over, then leaves. A corpse lies
// out its decay, and a creature whose respawn comes outside its time comes back
// held (corpseSystem): it is not seen again until its time. At the turn into
// its time every held one comes into every view at once. A GM's, a quest's or
// a trigger's copy is never held, nor a pet or a charmed creature.
//
// A keep restart carries nothing of it: the new image holds whatever the clock
// says is out of its time (a kept creature in a fight is out of it by then).
// The NPCs a row names are found again every minute (npcTimeRescan) and at a
// reload, so a row the GM panel adds or remakes is held within the minute.

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

const (
	// npcTimeTicks is how often the turn is looked at, in ticks: once a second
	npcTimeTicks = 10
	// npcTimeRescan is how often the NPCs of the rows are found again
	npcTimeRescan = time.Minute
)

// npcTimes is the World's state of the time of day's NPCs.
type npcTimes struct {
	// rules is npctime's rows: a key (NPCClassKey, or a RowKey) to day (true)
	// or night (false)
	rules map[string]bool
	// npcs is each NPC a row names, to its time (day true)
	npcs map[EntityID]bool
	// scanned is when npcs was last found
	scanned time.Time
	// day is the time at the last look: 0 not yet, 1 day, 2 night
	day uint8
	// hourAt is the game's hour at a time (a test's own); nil: the server's clock
	hourAt func(time.Time) float32
}

// gameHourLocked is the time of day the server's clock says at now, as a client set
// by it at once shows it.
func (w *World) gameHourLocked(now time.Time) float32 {
	if w.times.hourAt != nil {
		return w.times.hourAt(now)
	}
	return session.ServerHour(now)
}

// IsDay reports the sun up by the server's clock at now (the file's doc).
func (w *World) IsDay(now time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return Daylight(w.gameHourLocked(now))
}

// LoadNPCTimes reads table npctime into the world: at the start and after a
// GM's edit. An NPC a row no longer names is let back; the rest are held or let
// back by the clock at once. It reports how many NPCs keep a time and how many
// changed.
func (w *World) LoadNPCTimes(db *database.SQLiteDB) (ruled, changed int, err error) {
	rows, err := db.NPCTimes()
	if err != nil {
		return 0, 0, fmt.Errorf("npctime: %w", err)
	}
	rules := make(map[string]bool, len(rows))
	for k, t := range rows {
		rules[k] = t == database.NPCTimeDay
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	ruled, changed = w.setNPCTimesLocked(rules, time.Now())
	return ruled, changed, nil
}

// SetNPCTimes is LoadNPCTimes from rules already read: a key (NPCClassKey of a
// name, or a RowKey) to day (true) or night.
func (w *World) SetNPCTimes(rules map[string]bool, now time.Time) (ruled, changed int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.setNPCTimesLocked(rules, now)
}

func (w *World) setNPCTimesLocked(rules map[string]bool, now time.Time) (ruled, changed int) {
	w.times.rules = rules
	w.scanNPCTimesLocked(now)
	return len(w.times.npcs), w.applyNPCTimesLocked(now)
}

// npcTimeOfLocked is the time an NPC keeps: its row's key first, then its
// name's; ok false for none (and for any copy, pet or charmed creature). Must
// be called with the lock held.
func (w *World) npcTimeOfLocked(npc *NPC) (day, ok bool) {
	if len(w.times.rules) == 0 || npc.Temporary || npc.Owner != 0 || npc.Charm != nil {
		return false, false
	}
	if day, ok = w.times.rules[RowKey(npc.ServerID)]; ok {
		return day, true
	}
	day, ok = w.times.rules[database.NPCClassKey(npc.Name)]
	return day, ok
}

// scanNPCTimesLocked finds the NPCs the rows name again; one no longer named
// is let back. Must be called with the lock held.
func (w *World) scanNPCTimesLocked(now time.Time) {
	old := w.times.npcs
	found := make(map[EntityID]bool)
	if len(w.times.rules) > 0 {
		for id, npc := range w.NPCs {
			if day, ok := w.npcTimeOfLocked(npc); ok {
				found[id] = day
			}
		}
	}
	for id := range old {
		if _, ok := found[id]; !ok {
			if npc := w.NPCs[id]; npc != nil {
				npc.offHours = false
			}
		}
	}
	w.times.npcs = found
	w.times.scanned = now
}

// applyNPCTimesLocked holds or lets back every NPC of a time as the clock says
// at now, and logs a turn. It reports how many changed. Must be called with
// the lock held.
func (w *World) applyNPCTimesLocked(now time.Time) (changed int) {
	day := Daylight(w.gameHourLocked(now))
	state := uint8(2)
	if day {
		state = 1
	}
	turned := w.times.day != 0 && w.times.day != state
	w.times.day = state
	in, out, waiting := 0, 0, 0
	for id, npcDay := range w.times.npcs {
		npc := w.NPCs[id]
		if npc == nil {
			delete(w.times.npcs, id)
			continue
		}
		switch w.applyNPCTimeLocked(npc, npcDay == day) {
		case timeCameIn:
			in++
		case timeLeft:
			out++
		case timeWaits:
			waiting++
		}
	}
	if turned || in+out > 0 {
		when := "night"
		if day {
			when = "day"
		}
		log.Printf("Time of day: %s (%.2f): %d NPC(s) came in, %d left, %d wait on a fight or a corpse",
			when, w.gameHourLocked(now), in, out, waiting)
	}
	return in + out
}

// What applyNPCTimeLocked did with an NPC.
const (
	timeAsWas = iota
	timeCameIn
	timeLeft
	timeWaits
)

// applyNPCTimeLocked holds an NPC out of the world outside its time, or lets it
// back in it (the file's doc). Must be called with the lock held.
func (w *World) applyNPCTimeLocked(npc *NPC, inTime bool) int {
	if npc.Temporary || npc.Owner != 0 || npc.Charm != nil {
		return timeAsWas
	}
	if inTime {
		if !npc.offHours {
			return timeAsWas
		}
		npc.offHours = false
		return timeCameIn
	}
	if npc.offHours || npc.SpawnOnly {
		return timeAsWas
	}
	if npc.Dead || !aiQuiet(npc) {
		// a corpse lies out its decay (held at its respawn: corpseSystem), a
		// fight is fought to its end
		return timeWaits
	}
	npc.offHours = true
	// as a season's hold (spawnonly.go): back on its spawn, whole, nothing on it
	w.respawnNPCLocked(npc)
	npc.Effects = nil
	npc.endDummyFight()
	delete(w.affected, npc.ServerID)
	return timeLeft
}

// offHoursLocked reports an NPC of a time outside it at now: a respawn then
// comes back held (corpseSystem). Must be called with the lock held.
func (w *World) offHoursLocked(npc *NPC, now time.Time) bool {
	day, ok := w.npcTimeOfLocked(npc)
	return ok && day != Daylight(w.gameHourLocked(now))
}

// npcTimeSystem looks at the turn once a second (the file's doc). Must be
// called with the lock held.
func (w *World) npcTimeSystem(now time.Time) {
	if len(w.times.rules) == 0 && len(w.times.npcs) == 0 {
		return
	}
	if w.TickCount%npcTimeTicks != 0 {
		return
	}
	if now.Sub(w.times.scanned) >= npcTimeRescan {
		w.scanNPCTimesLocked(now)
	}
	w.applyNPCTimesLocked(now)
}

// NPCTimeSummary says how many NPCs keep a time of day, for the log.
func (w *World) NPCTimeSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	day, held := 0, 0
	for id, d := range w.times.npcs {
		if d {
			day++
		}
		if npc := w.NPCs[id]; npc != nil && npc.offHours {
			held++
		}
	}
	now := time.Now()
	return fmt.Sprintf("%d NPCs of %d rows keep a time of day (npctime: %d by day, %d by night), %d out of the world now (the game's hour %.2f)",
		len(w.times.npcs), len(w.times.rules), day, len(w.times.npcs)-day, held, w.gameHourLocked(now))
}

// npcTimeLineLocked is #stats's line of an NPC of a time of day, "" for one of
// none. Must be called with the lock held.
func (w *World) npcTimeLineLocked(npc *NPC, now time.Time) string {
	day, ok := w.npcTimeOfLocked(npc)
	if !ok {
		return ""
	}
	when, other := "day (6:00-18:00)", "night"
	if !day {
		when, other = "night (18:00-6:00)", "day"
	}
	h := w.gameHourLocked(now)
	state := "in the world now"
	switch {
	case npc.offHours:
		state = "out of the world now (" + other + ")"
	case day != Daylight(h):
		state = "its time is over: it leaves when its fight or corpse is done"
	}
	return fmt.Sprintf("Stands only by %s (npctime); the game's hour %02d:%02d: %s", when, int(h), int(h*60)%60, state)
}
