package entity

import (
	"testing"
	"time"
)

// wall is an upright wall from (ax, az) to (bx, bz), from bottom to top.
type wall struct{ ax, az, bx, bz, bottom, top float32 }

// wallGround is a flat floor at 0 everywhere, with walls on it.
type wallGround struct{ walls []wall }

func (wallGround) Floor(_ uint8, _, _, _ float32) (float32, bool) { return 0, true }

func (g wallGround) Wall(_ uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	for _, w := range g.walls {
		// where the way and the wall cross, seen from above: s along the
		// way, u along the wall
		dx, dz, ex, ez := x1-x0, z1-z0, w.bx-w.ax, w.bz-w.az
		den := dx*ez - dz*ex
		if den == 0 {
			continue
		}
		s := ((w.ax-x0)*ez - (w.az-z0)*ex) / den
		u := ((w.ax-x0)*dz - (w.az-z0)*dx) / den
		if s < 0 || s > 1 || u < 0 || u > 1 {
			continue
		}
		if h := y0 + (y1-y0)*s; h >= w.bottom && h <= w.top {
			return true
		}
	}
	return false
}

// floorOnly is the same floor with no walls known: a Ground that is not
// Walls (as before F3 pass 24).
type floorOnly struct{}

func (floorOnly) Floor(_ uint8, _, _, _ float32) (float32, bool) { return 0, true }

// room is the four walls of a room from (x0, z0) to (x1, z1), 10 high.
func room(x0, z0, x1, z1 float32) []wall {
	return []wall{{x0, z0, x1, z0, 0, 10}, {x1, z0, x1, z1, 0, 10}, {x1, z1, x0, z1, 0, 10}, {x0, z1, x0, z0, 0, 10}}
}

// A wander leg does not pass through a wall: through the wall it is
// refused, away from it, short of it and over a kerb lower than a knee it is
// walked. The control: the same leg on a ground that knows no walls is
// walked, as every leg was before.
func TestAWanderLegDoesNotPassThroughAWall(t *testing.T) {
	w := roamWorld(t)
	w.SetGround(wallGround{walls: []wall{
		{1010, 990, 1010, 1010, 0, 10},  // a wall, 5 east of the creature
		{995, 1020, 1015, 1020, 0, 0.5}, // a kerb, 20 south
	}})
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	w.mu.Lock()
	for _, c := range []struct {
		what string
		x, z float32
		want bool
	}{
		{"through the wall", 1015, 1000, false},
		{"slantwise through it", 1020, 1008, false},
		{"away from it", 990, 1000, true},
		{"short of it", 1008, 1000, true},
		{"over the kerb", 1005, 1030, true},
	} {
		if got := w.legOKLocked(npc, c.x, c.z); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	w.mu.Unlock()

	w.SetGround(floorOnly{})
	npc.groundKnown = false
	w.mu.Lock()
	if !w.legOKLocked(npc, 1015, 1000) {
		t.Error("with no walls known the leg through the wall was refused")
	}
	w.mu.Unlock()
}

// A creature that spawned in a closed room (the tavern's cellar) wanders it
// for ten minutes and never leaves it, though its radius reaches far past
// the walls. The control: with no walls known it walks out through them.
func TestACreatureInAClosedRoomStaysInIt(t *testing.T) {
	inside := func(walls bool) (in bool, walked float64) {
		w := roamWorld(t)
		if walls {
			w.SetGround(wallGround{walls: room(990, 985, 1020, 1015)})
		} else {
			w.SetGround(floorOnly{})
		}
		npc := addNPC(w, 100, "tavern rat", 1005, 1000, creatureModel, 0)
		in = true
		px, pz := npc.X, npc.Z
		run(w, time.Now(), 10*time.Minute, func(time.Time) {
			walked += float64(distXZ(px, pz, npc.X, npc.Z))
			px, pz = npc.X, npc.Z
			if npc.X <= 990 || npc.X >= 1020 || npc.Z <= 985 || npc.Z >= 1015 {
				in = false
			}
		})
		return in, walked
	}
	if in, walked := inside(true); !in || walked < 50 {
		t.Errorf("with the walls: inside %v after walking %.0f (want inside, and wandering)", in, walked)
	}
	if in, _ := inside(false); in {
		t.Error("with no walls known it never left the room: the control proves nothing")
	}
}

// One that spawned on a house's floor 1.2 over the terrain (what the ground
// holds) is tested at a knee above that floor, so a wall standing on the
// floor stops it; one that spawned on the terrain beside it passes under
// that wall, which stands above its knee (a bench, a table's top).
func TestAHouseFloorsCreatureIsTestedAboveTheFloor(t *testing.T) {
	w := roamWorld(t)
	w.SetGround(wallGround{walls: []wall{{1010, 990, 1010, 1010, 1.2, 10}}})
	housed := addNPC(w, 100, "tavern rat", 1005, 1000, creatureModel, 0)
	housed.Y, housed.SpawnY = 1.2, 1.2
	street := addNPC(w, 101, "city cat", 1005, 1004, creatureModel, 0)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.legOKLocked(housed, 1015, 1000) {
		t.Error("the house's creature walked through the house's wall")
	}
	if !w.legOKLocked(street, 1015, 1004) {
		t.Error("the street's creature was stopped by what stands above its knee")
	}
}

// slab is a flat top (a bench, a table, a stair tread) over a rectangle.
type slab struct{ x0, z0, x1, z1, top float32 }

// placedGround is wallGround's walls as what is placed on the terrain, and
// slabs: internal/ground's Placed as clearLocked asks it, a way down through
// a slab's top meeting it.
type placedGround struct {
	wallGround
	slabs []slab
}

func (g placedGround) Placed(world uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	if g.wallGround.Wall(world, x0, y0, z0, x1, y1, z1) {
		return true
	}
	for _, s := range g.slabs {
		if y0 > s.top && y1 <= s.top {
			t := (y0 - s.top) / (y0 - y1)
			x, z := x0+(x1-x0)*t, z0+(z1-z0)*t
			if x >= s.x0 && x <= s.x1 && z >= s.z0 && z <= s.z1 {
				return true
			}
		}
	}
	return false
}

// A wander is set down by CLIENT on its own floor all along (the user,
// 2026-10-31: "the tavern rats (and probably any mob) though they are
// confined by the walls can still somewhat walk up them"): a leg over a
// bench lower than its knee is refused (it would be drawn on the bench), and
// one that ends against a wall or runs along it (drawn up it), and one that
// stops where the client may carry it on into the wall; one that stops a
// unit off the wall, or goes away from both, is walked. The controls: on a
// ground that knows its walls but not what is placed (the Walls of before)
// the bench is stepped over and a leg up to the wall is walked; and a
// house's creature, whose own floor is the house's, is not stopped by that
// floor, only by a bench on it.
func TestAWanderIsSetDownOnItsOwnFloor(t *testing.T) {
	bench := slab{1004, 995, 1006, 1005, 0.5}
	walls := wallGround{walls: []wall{{1012, 980, 1012, 1030, 0, 10}}}
	w := roamWorld(t)
	w.SetGround(placedGround{wallGround: walls, slabs: []slab{bench}})
	npc := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0)
	along := addNPC(w, 101, "beetle", 1011.7, 1010, creatureModel, 0)
	cases := []struct {
		what string
		npc  *NPC
		x, z float32
		want bool
	}{
		{"over the bench", npc, 1010, 1000, false},
		{"ending 0.2 off the wall", npc, 1011.8, 1012, false},
		{"stopping 0.6 off it, carried on into it", npc, 1011.4, 1013, false},
		{"along the wall, 0.3 off it", along, 1011.7, 1025, false},
		{"stopping a unit off the wall", npc, 1011, 1013, true},
		{"away from both", npc, 990, 990, true},
	}
	w.mu.Lock()
	for _, c := range cases {
		if got := w.legOKLocked(c.npc, c.x, c.z); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	w.mu.Unlock()

	// the controls: what is placed not known
	w.SetGround(walls)
	npc.groundKnown, along.groundKnown = false, false
	w.mu.Lock()
	for _, c := range cases[:4] {
		if c.npc == along {
			continue // its start stands where the old rule never looked
		}
		if !w.legOKLocked(c.npc, c.x, c.z) {
			t.Errorf("the control, %s: refused with only the walls known", c.what)
		}
	}
	w.mu.Unlock()

	// a house's creature: its own floor (the house's, 1.2 over the terrain)
	// is no bench; a bench on it is, where the client's probe (from 1.6, the
	// height it is sent, the terrain's, and 1.5 over it) reaches its top
	w.SetGround(placedGround{wallGround: walls, slabs: []slab{{990, 990, 1010, 1010, 1.2}, {1004, 995, 1006, 1005, 1.5}}})
	housed := addNPC(w, 102, "tavern rat", 1000, 1000, creatureModel, 0)
	housed.Y, housed.SpawnY = 1.2, 1.2
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.legOKLocked(housed, 1000, 994) {
		t.Error("a house's creature was kept off its own floor")
	}
	if w.legOKLocked(housed, 1010, 1000) {
		t.Error("a house's creature was walked over the bench on its floor")
	}
}
