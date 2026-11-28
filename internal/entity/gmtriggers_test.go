package entity

import (
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
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

// A spawn step's face (Heirloom petition 2253: Bertoxxulous "facing south") turns
// the copy that way whatever middle it was spawned round: south is +z, east +x;
// the control, a copy given no face, faces the middle.
func TestATriggersSpawnFacesItsCompassPoint(t *testing.T) {
	f := newAggroFix(t, 10)
	f.npc(2001, "Brown Bear", 5, bearBody, 500, 500)
	for _, c := range []struct {
		face   string
		dx, dz float64
	}{{"south", 0, 1}, {"east", 1, 0}, {"North", 0, -1}, {"west", -1, 0}, {"", 0, -1}} {
		h, ok := database.TriggerFaceHeading(c.face)
		if !ok {
			t.Fatalf("face %q refused", c.face)
		}
		// the middle is north of the copy (smaller z): a copy given no face looks north
		id, _, ok := f.w.SpawnEventNPCFacing("brown bear", 0, 1000, 42, 990, 1000, 1000, h, false)
		if !ok {
			t.Fatal("no copy")
		}
		n := f.w.NPCs[id]
		dx, dz := math.Sin(float64(n.Facing)), math.Cos(float64(n.Facing))
		if math.Abs(dx-c.dx) > 1e-6 || math.Abs(dz-c.dz) > 1e-6 {
			t.Errorf("face %q: facing (%.3f, %.3f), want (%.0f, %.0f)", c.face, dx, dz, c.dx, c.dz)
		}
	}
	if _, ok := database.TriggerFaceHeading("up"); ok {
		t.Error("a face that is no compass point was taken")
	}
}

// A spawn step's exact (Heirloom petition 3517): the Thulite Golem woken on his anvil in the
// hut (y 69.6) stood 15.7 under it in a tree, the floor the ground files hold there (53.9:
// the hut's is not among them). A plain spawn is still put down on that floor, within
// triggerFloorReach; an exact one keeps the place's height, in an instance too.
func TestATriggersExactSpawnKeepsThePlacesHeight(t *testing.T) {
	f := newAggroFix(t, 60)
	f.npc(2001, "Thulite Golem", 65, humanBody, 500, 500)
	f.w.SetGround(flatGround{y: 53.9})
	plain, _, ok := f.w.SpawnEventNPCFacingIn("thulite golem", 0, 0, 18467.5, 69.6, 25426.5, 18467.5, 25426.5, 0, true)
	if !ok {
		t.Fatal("no copy")
	}
	exact, _, ok := f.w.SpawnEventNPCExactIn("thulite golem", 0, 0, 18467.5, 69.6, 25426.5, 18467.5, 25426.5, 0, true)
	if !ok {
		t.Fatal("no exact copy")
	}
	p, e := f.w.NPCs[plain], f.w.NPCs[exact]
	if p.Y != 53.9 {
		t.Errorf("the control: a plain spawn stands at %.2f, want the floor's 53.9", p.Y)
	}
	if e.Y != 69.6 || e.X != 18467.5 || e.Z != 25426.5 || e.Facing != 0 {
		t.Errorf("the exact spawn at (%.1f, %.2f, %.1f) facing %.2f, want (18467.5, 69.60, 25426.5) facing 0", e.X, e.Y, e.Z, e.Facing)
	}
	if e.NPCType&0x80 != 0 || !e.EventAggro || !e.Temporary {
		t.Errorf("the exact spawn is a hostile trigger copy as any: type %#x, event aggro %v, temporary %v", e.NPCType, e.EventAggro, e.Temporary)
	}
}
