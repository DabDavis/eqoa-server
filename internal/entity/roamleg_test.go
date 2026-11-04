package entity

import (
	"math"
	"testing"
	"time"
)

// waveGround is a floor that waves gently (0.3 over 7 units across, 0.2 over
// 5 along) and counts how often it is asked.
type waveGround struct{ asked *int }

func (g waveGround) Floor(_ uint8, x, _, z float32) (float32, bool) {
	*g.asked++
	return waveAt(x, z), true
}

func waveAt(x, z float32) float32 {
	return 100 + 0.3*float32(math.Sin(float64(x)/7)) + 0.2*float32(math.Cos(float64(z)/5))
}

// A wandering creature walks a leg on the floor its check found: the ground
// is asked when a leg is picked (the start and each point of its check) and
// at the leg's end, never at a step between, and at every step the creature
// stands within a hair of the ground's own floor (straight between the
// check's points, legSample apart). The control: the same steps by
// stepNPCToward (a chase's, a pet's) ask the ground at every one.
func TestAWanderLegIsWalkedOnItsOwnFloor(t *testing.T) {
	asked := 0
	w := roamWorld(t)
	w.SetGround(waveGround{&asked})
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	npc.Y, npc.SpawnY = waveAt(1005, 1000), waveAt(1005, 1000)
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Unix(1_800_000_000, 0)
	legs, between, worst := 0, 0, float32(0)
	for tick := 0; tick < 900; tick++ {
		before, walking := asked, npc.roam.walking
		w.TickCount++
		w.roamSystem(now)
		now = now.Add(100 * time.Millisecond)
		if !walking && npc.roam.walking {
			legs++ // a leg picked, and its first step
			if !npc.roam.legUsable {
				t.Fatalf("a leg on the ground of a creature of the land cannot say its floor")
			}
		} else if walking && npc.roam.walking {
			between++
			if asked != before {
				t.Fatalf("tick %d: a step between a leg's ends asked the ground %d times", tick, asked-before)
			}
		}
		if d := float32(math.Abs(float64(npc.Y - waveAt(npc.X, npc.Z)))); d > worst {
			worst = d
		}
	}
	if legs < 3 || between < 30 {
		t.Fatalf("%d legs and %d steps between their ends in 90 s: want a creature wandering", legs, between)
	}
	if worst > 0.02 {
		t.Errorf("the creature stood %.4f off the ground's floor", worst)
	}

	// the control: the same kind of step with no leg asks at every one
	before := asked
	for i := 0; i < 10; i++ {
		w.stepNPCToward(npc, npc.X+20, npc.Z, NPCWalkSpeed, 0.1)
	}
	if asked-before != 10 {
		t.Errorf("ten steps with no leg asked the ground %d times, want 10", asked-before)
	}
}

// A swimmer, and a creature moved off its leg, ask the ground: the leg's
// floor is not theirs.
func TestALegCannotSayOffItsLine(t *testing.T) {
	r := &roamState{legX: 0, legZ: 0, toX: 10, toZ: 0, legLen: 10, heights: []float32{1, 2, 3, 4, 5, 6}, legUsable: true}
	npc := &NPC{X: 5, Z: 0}
	if y, ok := r.legY(npc); !ok || y != 3.5 {
		t.Fatalf("halfway along a leg rising 1 a sample: %v %v, want 3.5", y, ok)
	}
	npc.Z = 1
	if _, ok := r.legY(npc); ok {
		t.Error("a creature a unit off its leg took the leg's floor")
	}
	npc.Z, npc.swimming = 0, true
	if _, ok := r.legY(npc); ok {
		t.Error("a swimmer took the leg's floor")
	}
	if _, ok := (*roamState)(nil).legY(npc); ok {
		t.Error("no leg said a floor")
	}
}
