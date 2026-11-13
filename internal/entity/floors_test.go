package entity

import (
	"testing"

	"github.com/eqoa/server/internal/ground"
)

// slabGround is a terrain flat at 0 under a placed slab (Collonridge
// Cemetery's mausoleum: something placed, which the terrain's Floor leaves
// out): the slab's top at slabY over x and z 0 to 20, a step higher than a
// creature climbs, and a placed ramp from its east edge down to the terrain
// (z 8 to 12, x 20 to 24.6, falling 1 in 2). Its Floor is the terrain's alone;
// its Stand (internal/ground's) the slab's and the ramp's too.
type slabGround struct{}

const slabY float32 = 2.3

func overSlab(x, z float32) bool { return x >= 0 && x <= 20 && z >= 0 && z <= 20 }

func overSlabRamp(x, z float32) (float32, bool) {
	if x > 20 && x <= 20+2*slabY && z >= 8 && z <= 12 {
		return slabY - (x-20)/2, true
	}
	return 0, false
}

func (slabGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	if 0 > y+ground.StepUp {
		return y, false
	}
	return 0, true
}

func (slabGround) Stand(_ uint8, x, y, z float32) (float32, bool, bool, bool) {
	top := y + ground.StepUp
	best, placed := float32(0), false
	if overSlab(x, z) && slabY <= top {
		best, placed = slabY, true
	}
	if h, ok := overSlabRamp(x, z); ok && h <= top && h > best {
		best, placed = h, true
	}
	if !placed && 0 > top {
		return y, false, false, false
	}
	roofed := overSlab(x, z) && slabY > top && slabY-best <= ground.StandHead
	return best, placed, roofed, true
}

// Surfaces is every surface over (x, z) between lo and hi, for the test's
// look at where a creature stood (throughAFloor).
func (slabGround) Surfaces(_ uint8, x, z, lo, hi float32) []ground.Surface {
	var out []ground.Surface
	add := func(y float32, up, placed bool) {
		if y >= lo && y <= hi {
			out = append(out, ground.Surface{Y: y, Up: up, Placed: placed})
		}
	}
	add(0, true, false)
	if overSlab(x, z) {
		add(slabY, true, true)
	}
	if h, ok := overSlabRamp(x, z); ok {
		add(h, true, true)
	}
	return out
}

// terrainOfSlab is slabGround as a ground that is not Stands: the terrain's
// floor alone, as the creatures' moves had it before (the control).
type terrainOfSlab struct{}

func (terrainOfSlab) Floor(w uint8, x, y, z float32) (float32, bool) {
	return slabGround{}.Floor(w, x, y, z)
}

// Floors over floors, the owner's (2026-11-13: "mobs are going through the floor
// in the collinridge cemetary instance"): a creature on the terrain chasing an
// enemy on a placed slab goes round by its ramp and up onto it, and one spawned
// on the slab chasing an enemy on the terrain comes down the ramp, neither ever
// in the slab; the control, the terrain's floor alone, ran the first into the
// slab (on the terrain under its floor, where CLIENT shows it in it) and set the
// second down into it at its first step.
func TestAChaseKeepsToAPlacedFloor(t *testing.T) {
	run := func(g Ground, from, to [3]float32) (bool, [][3]float32) {
		w := roamWorld(t)
		w.SetGround(g)
		npc := addNPC(w, 100, "lesser ghoul", from[0], from[2], creatureModel, 40)
		npc.Y, npc.SpawnY = from[1], from[1]
		return chaseOver(w, npc, to[0], to[1], to[2], 600)
	}
	for _, c := range []struct {
		what     string
		from, to [3]float32
	}{
		{"onto the slab", [3]float32{10, 0, 40}, [3]float32{6, slabY, 6}},
		{"off the slab", [3]float32{4, slabY, 4}, [3]float32{10, 0, 45}},
	} {
		ok, at := run(slabGround{}, c.from, c.to)
		last := at[len(at)-1]
		if !ok {
			t.Errorf("%s: never reached its enemy: at (%.1f, %.1f, %.1f) after %d ticks", c.what, last[0], last[1], last[2], len(at)-1)
		}
		if i, why := throughAFloor(slabGround{}, 0, at); i >= 0 {
			t.Errorf("%s: tick %d at (%.1f, %.1f, %.1f): %s", c.what, i, at[i][0], at[i][1], at[i][2], why)
		}
		if i, bad := fallsOrClimbs(at); bad {
			t.Errorf("%s: tick %d went from %v to %v", c.what, i, at[i-1], at[i])
		}
		_, cat := run(terrainOfSlab{}, c.from, c.to)
		if i, _ := throughAFloor(slabGround{}, 0, cat); i < 0 {
			t.Errorf("the control, %s: never in the slab", c.what)
		}
	}
}

// A creature spawned on the slab whose fight took it down the ramp goes home by
// the ramp, never in the slab, and stands home at the height it left. Before,
// its way home asked the floor from the height it left (the slab's) at each
// step, and at the slab's edge it stood on it, up its side.
func TestAWayHomeKeepsToItsFloor(t *testing.T) {
	w := roamWorld(t)
	w.SetGround(slabGround{})
	npc := addNPC(w, 100, "lesser ghoul", 4, 4, creatureModel, 40)
	npc.Y, npc.SpawnY = slabY, slabY
	if ok, at := chaseOver(w, npc, 10, 0, 40, 600); !ok {
		last := at[len(at)-1]
		t.Fatalf("never reached its enemy off the slab: at (%.1f, %.1f, %.1f)", last[0], last[1], last[2])
	}
	home, at := homeOver(w, npc, 600)
	if i, why := throughAFloor(slabGround{}, 0, at); i >= 0 {
		t.Errorf("tick %d of its way home at (%.1f, %.1f, %.1f): %s", i, at[i][0], at[i][1], at[i][2], why)
	}
	if i, bad := fallsOrClimbs(at); bad {
		t.Errorf("tick %d of its way home went from %v to %v", i, at[i-1], at[i])
	}
	last := at[len(at)-1]
	if !home || last[0] != 4 || last[2] != 4 || last[1] != slabY {
		t.Errorf("home %v at (%.1f, %.1f, %.1f), want (4, %.1f, 4)", home, last[0], last[1], last[2], slabY)
	}
}
