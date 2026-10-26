package entity

import "testing"

// slopeGround is a hill rising 1 in 2 toward +x from height 100 at x 1000.
type slopeGround struct{}

func (slopeGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	return 100 + 0.5*(x-1000), true
}

// A creature that spawned on the ground walks down the hill on it (its
// height each tick the floor's, so CLIENT never shows it on air); one that
// spawned 10 over the ground (a bridge the ground leaves out) keeps its
// height near its spawn and stands on the terrain once it has left it
// (platformReach: a chase or its way home; it does not wander, movement.go).
// The control: a world with no ground keeps every height, the floating of F3
// 20261025-165427.
func TestACreatureWalksDownAHillOnTheGround(t *testing.T) {
	run := func(ground bool) (walker, bridge *NPC) {
		w := roamWorld(t)
		if ground {
			w.SetGround(slopeGround{})
		}
		walker = addNPC(w, 100, "beetle", 1030, 1000, creatureModel, 0)
		walker.Y, walker.SpawnY = 115, 115 // on the slope at x 1030
		bridge = addNPC(w, 101, "bat", 1020, 1010, creatureModel, 0)
		bridge.Y, bridge.SpawnY = 120, 120 // 10 over the slope at x 1020
		w.mu.Lock()
		for i := 0; i < 40; i++ {
			w.stepNPCToward(walker, walker.SpawnX-20, walker.SpawnZ, NPCWalkSpeed, 0.1)
		}
		for i := 0; i < 16; i++ { // 8 off its spawn: still on what it stood on
			w.stepNPCToward(bridge, bridge.SpawnX-8, bridge.SpawnZ, NPCWalkSpeed, 0.1)
		}
		w.mu.Unlock()
		return walker, bridge
	}
	walker, bridge := run(true)
	if want := 100 + 0.5*(walker.X-1000); !near(walker.Y, want) || walker.X != 1010 {
		t.Errorf("walked to x %.2f at height %.2f, want 1010 at the floor's %.2f", walker.X, walker.Y, want)
	}
	if bridge.Y != 120 || bridge.X != 1012 {
		t.Errorf("the bridge's creature 8 off its spawn at x %.2f height %.2f, want its 120 kept at 1012", bridge.X, bridge.Y)
	}
	walker, _ = run(false)
	if walker.Y != 115 {
		t.Errorf("with no ground the height changed to %.2f", walker.Y)
	}
}
