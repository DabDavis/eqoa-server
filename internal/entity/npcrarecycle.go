package entity

// npcrarecycle.go — an NPC only sometimes there (table npcrarecycle,
// database/npcrarecycle.go; the owner, 2026-11-18, the forum's Siliskor quest:
// its giver Zaryn Boome "spawns rarely" in Blackwater). Its row stands for its
// up time, then is gone for a time drawn between its gone_min and gone_max, out
// of every view as a GM's refresh hold or a trigger's hide puts one (HiddenUntil:
// no client sees, targets or talks to it), and put back on its spawn; then it
// stands again for its up time. At a start (or a GM's edit of the table) it is
// gone for a time drawn between 0 and its gone_max, so a restart does not bring
// every such NPC at once. A row held out of the world otherwise (spawn-only, a
// GM's hold) is left to that.

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/database"
)

// rareCycle is one NPC's clock.
type rareCycle struct {
	up, goneMin, goneMax time.Duration
	// upUntil is when it goes (zero: not standing yet by this clock)
	upUntil time.Time
}

// rareCycleRand draws a duration in [lo, hi] (a test's own).
var rareCycleRand = func(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(rand.Int63n(int64(hi-lo)+1))
}

// LoadRareCycles reads table npcrarecycle into the world: at the start and after
// a GM's edit of it. Each NPC it names is gone at once for a time drawn between 0
// and its gone_max; an NPC that a rule no longer names is let back. A rule whose
// row is not loaded is logged and left out. It reports how many rules it read.
func (w *World) LoadRareCycles(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCRareCycles()
	if err != nil {
		return 0, fmt.Errorf("npcrarecycle: %w", err)
	}
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	old := w.rareCycles
	rules := map[EntityID]*rareCycle{}
	for _, r := range rows {
		id := EntityID(r.NPCRow)
		npc := w.NPCs[id]
		if npc == nil || npc.Temporary {
			log.Printf("Rare cycles: row %d is not loaded: left out", r.NPCRow)
			continue
		}
		rc := &rareCycle{up: minutes(r.UpMinutes), goneMin: minutes(r.GoneMin), goneMax: minutes(r.GoneMax)}
		rules[id] = rc
		if !npc.SpawnOnly {
			npc.HiddenUntil = now.Add(rareCycleRand(0, rc.goneMax))
			w.respawnNPCLocked(npc)
		}
	}
	for id := range old {
		if rules[id] == nil {
			if npc := w.NPCs[id]; npc != nil {
				npc.HiddenUntil = time.Time{} // no clock now: it stands
			}
		}
	}
	w.rareCycles = rules
	return len(rules), nil
}

// minutes is m minutes as a duration.
func minutes(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }

// RareCycleSummary says how many NPCs keep such a clock, for the log.
func (w *World) RareCycleSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return fmt.Sprintf("%d NPCs only sometimes there (npcrarecycle)", len(w.rareCycles))
}

// rareCycleSystem runs the clocks (the file's doc). Must be called with the lock
// held.
func (w *World) rareCycleSystem(now time.Time) {
	for id, rc := range w.rareCycles {
		npc := w.NPCs[id]
		if npc == nil || npc.SpawnOnly {
			continue
		}
		if now.Before(npc.HiddenUntil) {
			rc.upUntil = time.Time{} // gone: its time up begins as it comes back
			continue
		}
		if rc.upUntil.IsZero() {
			rc.upUntil = now.Add(rc.up)
			continue
		}
		if now.Before(rc.upUntil) || npc.CurrentEnemy != 0 && !npc.Dead {
			continue // its time, or a fight it is in, not over
		}
		gone := rareCycleRand(rc.goneMin, rc.goneMax)
		npc.HiddenUntil = now.Add(gone)
		rc.upUntil = time.Time{}
		w.respawnNPCLocked(npc)
		log.Printf("Rare cycles: %q (%d) goes for %v", npc.Name, id, gone.Round(time.Minute))
	}
}
