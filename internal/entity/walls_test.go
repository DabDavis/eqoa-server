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
