package entity

import (
	"testing"
	"time"
)

// ledgeGround is a terrace at height 10 west of x 0 over a field at 0 east of
// it, its edge a cliff but where a ramp runs down from it (z rampZ0 to rampZ1,
// x 0 to 20, falling 1 in 2): what the Elephant Graveyard's Xrags stand on.
type ledgeGround struct{ rampZ0, rampZ1 float32 }

func (g ledgeGround) surface(x, z float32) float32 {
	switch {
	case x < 0:
		return 10
	case z >= g.rampZ0 && z <= g.rampZ1 && x <= 20:
		return 10 - x/2
	}
	return 0
}

// Floor is the one surface at (x, z) where it is no more than 2 over y (the
// ground's StepUp), as a heightfield's.
func (g ledgeGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	s := g.surface(x, z)
	if s > y+2 {
		return y, false
	}
	return s, true
}

// Wall is the cliff's face under the terrace, and the ramp's two sides.
func (g ledgeGround) Wall(_ uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	if (x0 < 0) != (x1 < 0) {
		t := -x0 / (x1 - x0)
		z, y := z0+(z1-z0)*t, y0+(y1-y0)*t
		if y < 10 && (z < g.rampZ0 || z > g.rampZ1) {
			return true
		}
	}
	for _, side := range []float32{g.rampZ0, g.rampZ1} {
		if (z0 < side) != (z1 < side) {
			t := (side - z0) / (z1 - z0)
			x, y := x0+(x1-x0)*t, y0+(y1-y0)*t
			if x > 0 && x < 20 && y < 10-x/2 {
				return true
			}
		}
	}
	return false
}

// chaseOver runs npc's chase of an enemy standing at (x, y, z) a tick at a time
// (a tenth of a second) until it is in reach or ticks pass, and reports whether
// it got there and the heights it stood at, a tick apart.
func chaseOver(w *World, npc *NPC, x, y, z float32, ticks int) (bool, [][3]float32) {
	now := time.Unix(1_800_000_000, 0)
	at := [][3]float32{{npc.X, npc.Y, npc.Z}}
	for i := 0; i < ticks; i++ {
		now = now.Add(100 * time.Millisecond)
		w.TickCount++
		w.mu.Lock()
		in, giveUp := w.chaseLocked(npc, x, y, z, 0, 0, now)
		w.mu.Unlock()
		at = append(at, [3]float32{npc.X, npc.Y, npc.Z})
		if giveUp {
			return false, at
		}
		if in && absf(npc.Y-y) <= routeLevel {
			return true, at
		}
	}
	return false, at
}

// fallsOrClimbs is the first tick a chase fell past legDrop or rose past the
// ground's step (2) between two places: off a ledge, or up through a cliff.
func fallsOrClimbs(at [][3]float32) (int, bool) {
	for i := 1; i < len(at); i++ {
		if dy := at[i][1] - at[i-1][1]; dy < -legDrop-0.5 || dy > 2.5 {
			return i, true
		}
	}
	return 0, false
}

// A creature on the terrace chasing an enemy in the field below (the user,
// 2026-11-08: "the xrags when they are aggroed will run through the ledge
// instead of using the ramp pathing") goes round by the ramp: it reaches its
// enemy on the field's floor, never falling or climbing past a step; one in the
// field chasing an enemy on the terrace comes up the ramp the same way. The
// control: with the routes off, the chase goes off the ledge in one step.
func TestAChaseGoesRoundALedgeByTheRamp(t *testing.T) {
	run := func(routes bool, fromX, fromY, toX, toY float32) (bool, [][3]float32) {
		was := chaseRoutes
		chaseRoutes = routes
		defer func() { chaseRoutes = was }()
		w := roamWorld(t)
		w.SetGround(ledgeGround{rampZ0: 16, rampZ1: 24})
		npc := addNPC(w, 100, "pariah of Xrag", fromX, 0, creatureModel, 40)
		npc.Y, npc.SpawnY = fromY, fromY
		return chaseOver(w, npc, toX, toY, 0, 400)
	}
	for _, c := range []struct {
		what                   string
		fromX, fromY, toX, toY float32
	}{{"down", -6, 10, 8, 0}, {"up", 8, 0, -6, 10}} {
		ok, at := run(true, c.fromX, c.fromY, c.toX, c.toY)
		if !ok {
			last := at[len(at)-1]
			t.Fatalf("%s: never reached its enemy: at (%.1f, %.1f, %.1f) after %d ticks", c.what, last[0], last[1], last[2], len(at)-1)
		}
		if i, bad := fallsOrClimbs(at); bad {
			t.Errorf("%s: tick %d went from %v to %v", c.what, i, at[i-1], at[i])
		}
		ramp := false
		for _, p := range at {
			ramp = ramp || (p[0] > 1 && p[0] < 19 && p[2] >= 16 && p[2] <= 24 && p[1] > 0.5 && p[1] < 9.5)
		}
		if !ramp {
			t.Errorf("%s: never on the ramp", c.what)
		}
	}
	_, at := run(false, -6, 10, 8, 0)
	if _, bad := fallsOrClimbs(at); !bad {
		t.Error("the control: with the routes off the chase never fell off the ledge")
	}
}

// An enemy with no way to it (the terrace with no ramp) is chased straight, as
// before, after a search that finds nothing; and the next search waits
// routeAgain. The control: the same chase on flat ground never searches.
func TestAChaseWithNoWayRunsStraight(t *testing.T) {
	w := roamWorld(t)
	w.SetGround(ledgeGround{rampZ0: 1e6, rampZ1: 1e6 + 1})
	npc := addNPC(w, 100, "pariah of Xrag", -6, 0, creatureModel, 40)
	npc.Y, npc.SpawnY = 10, 10
	_, at := chaseOver(w, npc, 8, 0, 0, 40)
	if _, bad := fallsOrClimbs(at); !bad {
		t.Error("with no way round it did not run straight")
	}
	if npc.chase.route.failedUntil.IsZero() {
		t.Error("no search was made")
	}

	flat := roamWorld(t)
	flat.SetGround(ledgeGround{rampZ0: 1e6, rampZ1: 1e6 + 1})
	f := addNPC(flat, 101, "beetle", 30, 0, creatureModel, 40)
	f.Y, f.SpawnY = 0, 0
	if ok, _ := chaseOver(flat, f, 60, 0, 5, 100); !ok {
		t.Fatal("the control did not reach its enemy on the field")
	}
	if !f.chase.route.failedUntil.IsZero() || len(f.chase.route.pts) != 0 {
		t.Error("the control searched on flat ground")
	}
}
