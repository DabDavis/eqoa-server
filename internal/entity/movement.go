package entity

import (
	"fmt"

	"github.com/eqoa/server/internal/database"
)

// How the creatures of a name move: table npcmove (database/npcmove.go; the
// GM panel's "Creature movement", applied at once). roam 0 stands a
// creature on its spawn until a fight takes it (F3 20261026-021730, jib:
// "hexbones in zentaurs keep do NOT roam, they are static spawn until
// agrod"); radius is its own wander radius; water what it keeps to
// (water.go). With no row a creature wanders the server's radius and keeps
// to where it spawned.
//
// Wherever it wanders, a creature must be able to stand all along the leg
// (legOKLocked): one on the ground finds a floor at every two units and no
// drop past legDrop between two of them (a cliff it would fall off), one
// keeping to the land meets no water, one keeping to the water leaves none
// and passes under no rising sea floor. And one that stands off the ground
// (on what the ground leaves out: a keep's floor, a bridge, a dock; or with
// no floor under it) does not wander at all, but for one swimming where it
// spawned: walked at its spawn's height it would float wherever the floor
// beneath it fell away (F3 20261026-021425).

// legSample is the spacing of the points a wander leg is tried at, and
// legDrop how far the floor may fall between two of them (51 degrees).
const (
	legSample float32 = 2
	legDrop   float32 = 2.5
)

// SetMoveRules sets table npcmove's rules, keyed as database.NPCClassKey
// keys a name.
func (w *World) SetMoveRules(rules map[string]database.NPCMoveRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.moveRules = rules
}

// LoadMoveRules reads table npcmove into the world: at the start and after a
// GM's edit. Call it without the database held. It reports how many names it
// read.
func (w *World) LoadMoveRules(db *database.SQLiteDB) (int, error) {
	rules, err := db.NPCMoveRules()
	if err != nil {
		return 0, fmt.Errorf("npcmove: %w", err)
	}
	w.SetMoveRules(rules)
	return len(rules), nil
}

// MoveSummary says how creatures move, for the log at the start.
func (w *World) MoveSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	stand, both, water := 0, 0, 0
	for _, r := range w.moveRules {
		if !r.Roam {
			stand++
		}
		switch r.Water {
		case keepsToBoth:
			both++
		case keepsToWater:
			water++
		}
	}
	where := "no water known (no <world>.env)"
	if w.env != nil {
		where = "the water known"
	}
	return fmt.Sprintf("movement: %d names in npcmove (%d stand, %d go in and out of the water, %d keep to it), %s", len(w.moveRules), stand, both, water, where)
}

// moveRuleLocked is a creature's row of npcmove, false with none. Must be
// called with the lock held.
func (w *World) moveRuleLocked(npc *NPC) (database.NPCMoveRule, bool) {
	r, ok := w.moveRules[database.NPCClassKey(npc.Name)]
	return r, ok
}

// standsLocked reports a creature that does not wander: its row says roam
// 0, or it stands off the ground and is not swimming where it spawned. Must
// be called with the lock held.
func (w *World) standsLocked(npc *NPC) bool {
	if r, ok := w.moveRuleLocked(npc); ok && !r.Roam {
		return true
	}
	return w.ground != nil && !w.spawnOnGroundLocked(npc) && !w.spawnInWaterLocked(npc)
}

// roamRadiusLocked is how far from its spawn a creature wanders: its row's
// radius, else its own. Must be called with the lock held.
func (w *World) roamRadiusLocked(npc *NPC) float32 {
	if r, ok := w.moveRuleLocked(npc); ok && r.Radius > 0 {
		return r.Radius
	}
	return npc.RoamRadius
}

// legOKLocked reports a wander leg from where a creature stands to (x, z)
// that it can walk (the file's comment). Must be called with the lock held.
func (w *World) legOKLocked(npc *NPC, x, z float32) bool {
	d := distXZ(npc.X, npc.Z, x, z)
	n := int(d/legSample) + 1
	onGround := w.spawnOnGroundLocked(npc)
	keeps := keepsToBoth
	if w.env != nil {
		keeps = w.keepsToLocked(npc)
	}
	y := npc.Y
	for i := 1; i <= n; i++ {
		t := float32(i) / float32(n)
		sx, sz := npc.X+(x-npc.X)*t, npc.Z+(z-npc.Z)*t
		if onGround {
			f, ok := w.ground.Floor(npc.World, sx, y, sz)
			if !ok || f < y-legDrop {
				return false // no floor, or a fall
			}
			y = f
		}
		switch keeps {
		case keepsToLand:
			if w.waterAtLocked(npc.World, sx, y+bodyAbove, sz) {
				return false
			}
		case keepsToWater:
			if !w.waterAtLocked(npc.World, sx, y+bodyAbove, sz) {
				return false
			}
			if !onGround && w.ground != nil {
				if f, ok := w.ground.Floor(npc.World, sx, y, sz); ok && f > y {
					return false // the sea floor rises above it
				}
			}
		}
	}
	return true
}
