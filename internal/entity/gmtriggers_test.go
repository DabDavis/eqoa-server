package entity

import (
	"math"
	"testing"
)

// A GM trigger's hostile spawn (gmtriggers.go): a copy of a townsperson put
// down in a town attacks a level-60 player on sight, red-faced, with no NPC
// bit; the controls: the townsperson it copies, and a copy not hostile, leave
// the player alone.
func TestATriggersHostileSpawnAttacksAnyone(t *testing.T) {
	f := newAggroFix(t, 60)
	f.town(1000, 1020) // ten townsfolk 20 off the player: it stands in a town
	guard := f.npc(2001, "Town Guard", 5, humanBody, 1100, 1300)
	guard.NPCType = 0x80
	hostile, _, ok := f.w.SpawnEventNPC("Town Guard", 0, 1000, 10, 1000, 1010, 1000, true)
	if !ok {
		t.Fatal("no copy")
	}
	calm, _, _ := f.w.SpawnEventNPC("Town Guard", 0, 1000, 10, 1000, 990, 1000, false)
	f.look()
	h, c := f.w.NPCs[hostile], f.w.NPCs[calm]
	if !f.attacks(h) || f.face(h) != FaceHostile || h.NPCType&0x80 != 0 {
		t.Errorf("the hostile copy: attacked %v, face %d, npc type %#x", f.attacks(h), f.face(h), h.NPCType)
	}
	if f.attacks(c) || c.NPCType&0x80 == 0 || f.attacks(guard) || guard.EventAggro {
		t.Errorf("the controls: the calm copy attacked %v (type %#x), the guard %v (event %v)",
			f.attacks(c), c.NPCType, f.attacks(guard), guard.EventAggro)
	}
	if !h.Temporary || !c.Temporary {
		t.Error("a trigger's copies are a GM's spawns: temporary")
	}
}

// A copy faces the middle it was spawned round (the facing CLIENT reads:
// the direction (sin h, cos h)) and keeps the middle's height with no
// ground known.
func TestATriggersSpawnFacesTheMiddle(t *testing.T) {
	f := newAggroFix(t, 10)
	f.npc(2001, "Brown Bear", 5, bearBody, 500, 500)
	id, _, ok := f.w.SpawnEventNPC("brown bear", 0, 1000, 42, 1000, 1000, 990, false)
	if !ok {
		t.Fatal("no copy")
	}
	n := f.w.NPCs[id]
	dx, dz := math.Sin(float64(n.Facing)), math.Cos(float64(n.Facing))
	if math.Abs(dx) > 1e-6 || math.Abs(dz-1) > 1e-6 || n.Y != 42 || n.X != 1000 || n.Z != 990 {
		t.Errorf("at (%.1f, %.1f, %.1f) facing (%.3f, %.3f), want (1000, 42, 990) facing (0, 1)", n.X, n.Y, n.Z, dx, dz)
	}
	if _, _, ok := f.w.SpawnEventNPC("nobody here", 0, 1000, 42, 1000, 1000, 990, false); ok {
		t.Error("the control: a name no NPC has spawned something")
	}
}
