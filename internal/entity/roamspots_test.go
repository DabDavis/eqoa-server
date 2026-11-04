package entity

import (
	"sort"
	"testing"
	"time"
)

// countedGround is a flat floor at 0 that knows its walls and what is placed
// on it (nothing of either), counting how often the walls are asked: a
// leg's check whole asks at every point, its floor alone never
// (movement.go's legFloorsLocked).
type countedGround struct{ walls *int }

func (countedGround) Floor(_ uint8, _, _, _ float32) (float32, bool) { return 0, true }

func (g countedGround) Wall(_ uint8, _, _, _, _, _, _ float32) bool {
	*g.walls++
	return false
}

func (countedGround) Placed(_ uint8, _, _, _, _, _, _ float32) bool { return false }

// A creature of the land on the ground wanders an hour: its first legs are
// picked as before and each end becomes a spot, until it has
// wanderSpotsMax; from then on every leg ends on one of them, its legs are
// still Sandstorm's lengths and it is still on its feet about half the time,
// and a leg it walked before is not checked whole again: in the last twenty
// minutes few legs were. The control: every leg before its spots were all
// made was checked whole.
func TestAWandererWalksBetweenItsSpots(t *testing.T) {
	walls := 0
	w := roamWorld(t)
	w.SetGround(countedGround{&walls})
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	start := time.Now()
	var legs []float64
	var walked, stood, early, earlyWhole, late, lateWhole, offSpot int
	wasWalking, asked := false, 0
	var fromX, fromZ float32
	run(w, start, time.Hour, func(now time.Time) {
		if d := distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ); d > DefaultRoamRadius+0.01 {
			t.Fatalf("%.1f from its spawn, past its radius %v", d, DefaultRoamRadius)
		}
		if npc.moving() {
			walked++
		} else if len(legs) > 0 || wasWalking {
			stood++
		}
		switch {
		case !wasWalking && npc.roam.walking: // a leg picked this tick
			full := npc.spots != nil && npc.spots.n == wanderSpotsMax
			onSpot := full && npc.spots.at(npc.roam.toX, 0, npc.roam.toZ) >= 0
			whole := walls != asked
			switch {
			case !full:
				early++
				if whole {
					earlyWhole++
				}
			case !onSpot:
				offSpot++
			case now.Sub(start) > 40*time.Minute:
				late++
				if whole {
					lateWhole++
				}
			}
			fromX, fromZ = npc.X, npc.Z
		case wasWalking && !npc.roam.walking: // a leg's end
			legs = append(legs, float64(distXZ(fromX, fromZ, npc.X, npc.Z)))
		}
		wasWalking, asked = npc.roam.walking, walls
	})
	if npc.spots == nil || npc.spots.n != wanderSpotsMax {
		t.Fatalf("after an hour its spots: %+v", npc.spots)
	}
	if early < wanderSpotsMax-2 || earlyWhole != early {
		t.Errorf("before its spots were made: %d legs, %d checked whole; want every one", early, earlyWhole)
	}
	if offSpot != 0 {
		t.Errorf("%d legs once its spots were made ended on none of them", offSpot)
	}
	if late < 100 || lateWhole*5 > late {
		t.Errorf("in its last twenty minutes %d of %d legs were checked whole: want a few", lateWhole, late)
	}
	// Sandstorm's legs: median 19, 90% under 40.4 (roam.go), as
	// TestACreatureWandersWithinItsRadiusAtAWalkAndRests holds a creature
	// that keeps no spots to
	sort.Float64s(legs)
	med, p90 := legs[len(legs)/2], legs[len(legs)*9/10]
	if med < 14 || med > 25 || p90 < 30 || p90 > 50 {
		t.Errorf("legs median %.1f, 90%% under %.1f: want Sandstorm's 19 and 40", med, p90)
	}
	if f := float64(walked) / float64(walked+stood); f < 0.38 || f > 0.62 {
		t.Errorf("on its feet %.0f%% of the time, want about half, as Sandstorm's", f*100)
	}
	t.Logf("%d legs: median %.1f, 90%% under %.1f; whole checks %d of %d before its spots, %d of %d in the last twenty minutes",
		len(legs), med, p90, earlyWhole, early, lateWhole, late)
}

// stepGround is a floor at 2.2 west of x 1010 and at 0 east of it, found
// only within 2 above a height (as internal/ground's): a creature walks
// down the step (a fall of 2.2, under legDrop) and never up it.
type stepGround struct{ walls *int }

func (stepGround) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	f := float32(0)
	if x < 1010 {
		f = 2.2
	}
	return f, f <= y+2
}

func (g stepGround) Wall(_ uint8, _, _, _, _, _, _ float32) bool {
	*g.walls++
	return false
}

// A way from a spot that failed is not tried again, and one that was walked
// needs no whole check: a creature at a spot below the step, its spots on
// both sides of it, is asked for its next leg again and again where it
// stands; its first asks check the ways from there whole (those up the
// step fail), and once every way from there is known none is checked whole
// and none goes up the step.
func TestASpotsWaysAreCheckedOnce(t *testing.T) {
	walls := 0
	w := roamWorld(t)
	w.SetGround(stepGround{&walls})
	npc := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0)
	npc.Y, npc.SpawnY = 2.2, 2.2
	start := time.Now()
	for now := start; npc.spots == nil || npc.spots.n < wanderSpotsMax; {
		now = run(w, now, time.Minute, nil)
		if now.Sub(start) > 3*time.Hour {
			t.Fatal("its spots were not all made in three hours")
		}
	}
	s := npc.spots
	below := -1
	up := 0
	for i := 0; i < s.n; i++ {
		switch {
		case s.x[i] >= 1010 && below < 0:
			below = i
		case s.x[i] < 1010:
			up++
		}
	}
	if below < 0 || up == 0 {
		t.Fatalf("its spots are not on both sides of the step: %v", s.x[:s.n])
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	npc.stopMoving()
	npc.roam = roamState{}
	npc.X, npc.Y, npc.Z = s.x[below], s.y[below], s.z[below]
	asks, lastWhole := 0, -1
	for i := 0; i < 400; i++ {
		before := walls
		x, z := w.roamPointLocked(npc)
		if x < 1010 {
			t.Fatalf("ask %d: a leg up the step to (%.1f, %.1f)", i, x, z)
		}
		if walls != before {
			lastWhole = i
		}
		asks++
	}
	if s.tried[below]&^s.good[below] == 0 {
		t.Error("no way up the step was found to fail: the test proves nothing")
	}
	if lastWhole < 0 || lastWhole >= asks-100 {
		t.Errorf("the last ask that checked a way whole was ask %d of %d: want the first asks only", lastWhole, asks)
	}
	t.Logf("%d asks: the last whole check at ask %d; ways known from its spot %b, walkable %b", asks, lastWhole, s.tried[below], s.good[below])
}

// A creature's spots are made again when what they were found for changes
// (its radius here: none of the new ones past it), and a creature of the
// water, or with no ground known, keeps none.
func TestSpotsAreMadeAgainForANewRadius(t *testing.T) {
	walls := 0
	w := roamWorld(t)
	w.SetGround(countedGround{&walls})
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	now := run(w, time.Now(), 30*time.Minute, nil)
	if npc.spots == nil || npc.spots.n != wanderSpotsMax {
		t.Fatalf("after half an hour: %+v", npc.spots)
	}
	old := npc.spots
	w.SetRoamRadius(30)
	run(w, now, 30*time.Minute, nil)
	if npc.spots == old {
		t.Fatal("its radius changed and its spots did not")
	}
	for i := 0; i < npc.spots.n; i++ {
		if d := distXZ(npc.spots.x[i], npc.spots.z[i], npc.SpawnX, npc.SpawnZ); d > 30.01 {
			t.Errorf("spot %d %.1f from its spawn, past the new radius", i, d)
		}
	}

	w = beachWorld(t, true)
	shark := addNPC(w, 100, "mako shark", 2030, 1000, creatureModel, 0)
	shark.Y, shark.SpawnY = 40, 40
	legs(w, shark, 50)
	w = roamWorld(t)
	bare := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	run(w, time.Now(), 10*time.Minute, nil)
	if shark.spots != nil || bare.spots != nil {
		t.Errorf("spots for a creature of the water %v, for one with no ground known %v; want none", shark.spots != nil, bare.spots != nil)
	}
}
