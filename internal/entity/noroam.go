package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// noroam.go — ground no wandering creature walks into or through (table noroam,
// database/noroam.go; the GM Jibwiz's petitions 1963-1965, 2026-11-13, three kobold camps in
// Odus: "This kobold camp beneath me needs to not have wildlife in this area pathing into it. It can
// get close, but never through the tented camp").
//
// A wander leg (roam.go's roamPointLocked and spotLegLocked) that ends in a circle of the
// creature's world, or passes through one, is not taken, unless the creature spawned in that
// circle (it is the camp's own) or stands in it already (it walks out). A fight's chase and the
// walk home after one are not wanders: a creature chasing a player may cross it. A GM's patrol path
// is its own (path.go).

// noRoamState is the no-roam areas by the world they lie in.
type noRoamState struct {
	byWorld map[uint8][]database.NoRoam
	n       int
}

// SetNoRoam sets the no-roam areas (table noroam).
func (w *World) SetNoRoam(as []database.NoRoam) {
	by := make(map[uint8][]database.NoRoam)
	for _, a := range as {
		by[a.World] = append(by[a.World], a)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.noRoam = noRoamState{byWorld: by, n: len(as)}
}

// LoadNoRoam reads table noroam into the world: at the start and after a GM's edit. Call it without
// the database held. A row the world cannot use is logged and left out. It reports how many it read.
func (w *World) LoadNoRoam(db *database.SQLiteDB) (int, error) {
	as, bad, err := db.NoRoamAreas()
	if err != nil {
		return 0, fmt.Errorf("noroam: %w", err)
	}
	for _, b := range bad {
		log.Printf("WARNING: noroam %s: left out", b)
	}
	w.SetNoRoam(as)
	return len(as), nil
}

// NoRoamSummary says what the no-roam areas are, for the log at the start.
func (w *World) NoRoamSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.noRoam.n == 0 {
		return "no-roam areas: none in table noroam"
	}
	var nums []int
	for wn := range w.noRoam.byWorld {
		nums = append(nums, int(wn))
	}
	sort.Ints(nums)
	worlds := make([]string, len(nums))
	for i, wn := range nums {
		worlds[i] = fmt.Sprintf("world %d: %d", wn, len(w.noRoam.byWorld[uint8(wn)]))
	}
	return fmt.Sprintf("no-roam areas: %d in table noroam (%s)", w.noRoam.n, strings.Join(worlds, ", "))
}

// legInNoRoamLocked reports a wander leg from where npc stands to (x, z) that ends in a no-roam
// area or passes through one, but for one npc spawned in or stands in (the file's doc). Must be
// called with the lock held.
func (w *World) legInNoRoamLocked(npc *NPC, x, z float32) bool {
	for _, a := range w.noRoam.byWorld[npc.World] {
		if distXZ(npc.SpawnX, npc.SpawnZ, a.X, a.Z) <= a.Radius || distXZ(npc.X, npc.Z, a.X, a.Z) <= a.Radius {
			continue // its own, or it walks out
		}
		if segmentDist(npc.X, npc.Z, x, z, a.X, a.Z) <= a.Radius {
			return true // into it or through it
		}
	}
	return false
}
