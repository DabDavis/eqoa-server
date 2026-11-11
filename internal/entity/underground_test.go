package entity

import (
	"testing"
	"time"
)

// bankGround is a lake (x < 0, its bed at -10, its surface at 0) and the land
// beside it (x >= 0) at bank over the surface: a heightfield, its floor
// found no more than 2 over the height asked from (the ground's StepUp).
type bankGround struct{ bank float32 }

func (g bankGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	s := float32(-10)
	if x >= 0 {
		s = g.bank
	}
	if s > y+2 {
		return y, false
	}
	return s, true
}

// bankEnv is the lake's water: under its surface, and on under the bank to
// x = 6 (a zone leaf's box goes on under the terrain).
type bankEnv struct{}

func (bankEnv) Flags(_ uint8, x, y, z float32) (uint32, bool) {
	if x < 6 && y < 0 {
		return envWater, true
	}
	return 0, true
}

// bankWorld is a world with the lake and a creature of the land, spawned on
// the land at (20, bank), now swimming in the lake at (-10, z).
func bankWorld(t *testing.T, bank float32) (*World, *NPC) {
	t.Helper()
	w := roamWorld(t)
	w.SetGround(bankGround{bank: bank})
	w.SetEnv(bankEnv{})
	npc := addNPC(w, 100, "a sand giant", 20, 0, creatureModel, 40)
	npc.Y, npc.SpawnY = bank, bank
	w.mu.Lock()
	npc.X, npc.Y = -10, -10
	w.groundNPCLocked(npc)
	w.mu.Unlock()
	if !npc.swimming || npc.Y != -swimDepth {
		t.Fatalf("not swimming at the surface: y %.2f swimming %v", npc.Y, npc.swimming)
	}
	return w, npc
}

// Petition 1500 (Jibrish: "the mobs can end up underneath the world floor"): a
// swimmer chasing its enemy up a bank higher than it can climb out onto (3 over
// the water) does not go on under it, as it did; it is held at the water's
// edge, never in the ground, and gives up after chaseHeld as past its leash.
func TestSwimmerHeldUnderAHighBankGivesUp(t *testing.T) {
	w, npc := bankWorld(t, 3)
	now := time.Unix(1_800_000_000, 0)
	gaveUp := time.Duration(0)
	for i := 1; i <= 200; i++ {
		now = now.Add(100 * time.Millisecond)
		w.TickCount++
		w.mu.Lock()
		_, giveUp := w.chaseLocked(npc, 10, 3, 0, 0, 0, now)
		in := w.inGroundLocked(0, npc.X, npc.Y, npc.Z)
		w.mu.Unlock()
		if in || (npc.X >= 0 && npc.Y < 3) {
			t.Fatalf("tick %d: in the ground at (%.2f, %.2f)", i, npc.X, npc.Y)
		}
		if giveUp {
			gaveUp = time.Duration(i) * 100 * time.Millisecond
			break
		}
	}
	if npc.X >= 0 || npc.X < -1.5 {
		t.Errorf("held at x %.2f, want at the water's edge", npc.X)
	}
	if gaveUp == 0 || gaveUp > chaseHeld+2*time.Second {
		t.Errorf("gave up after %v, want about %v of being held", gaveUp, chaseHeld)
	}
}

// A bank it can climb out onto (2 over the water, inside bankClimb): the
// swimmer comes up onto the land and reaches its enemy there.
func TestSwimmerClimbsOutOntoALowBank(t *testing.T) {
	w, npc := bankWorld(t, 2)
	ok, at := chaseOver(w, npc, 10, 2, 0, 100)
	if !ok {
		t.Fatalf("never reached its enemy: ended at %v", at[len(at)-1])
	}
	for i, p := range at {
		if p[0] >= 0 && p[1] < 2 {
			t.Fatalf("tick %d: under the bank at (%.2f, %.2f)", i, p[0], p[1])
		}
	}
	if npc.swimming {
		t.Error("still swimming on the land")
	}
}

// A creature found in the ground already (put there before this rule) is set
// on the floor over it at its chase's next step, and its way home too.
func TestCreatureInTheGroundIsLiftedOut(t *testing.T) {
	w, npc := bankWorld(t, 3)
	npc.X, npc.Y, npc.swimming = 10, -1.5, false
	w.mu.Lock()
	w.chaseLocked(npc, 12, 3, 0, 0, 0, time.Unix(1_800_000_000, 0))
	w.mu.Unlock()
	if npc.Y != 3 {
		t.Errorf("chasing: y %.2f, want 3 (the bank's floor)", npc.Y)
	}
}

// A creature held by the ground on its way home (its home over the high bank,
// it in the water) is set home after chaseHeld, rather than held there for
// ever.
func TestHomeHeldByTheGroundIsSetHome(t *testing.T) {
	w, npc := bankWorld(t, 3)
	npc.chase = chaseState{on: true, fromX: 20, fromY: 3, fromZ: 0}
	w.mu.Lock()
	w.goHomeLocked(npc)
	w.mu.Unlock()
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < 200 && npc.chase.returning; i++ {
		now = now.Add(100 * time.Millisecond)
		w.TickCount++
		w.mu.Lock()
		w.homeSystem(now)
		in := w.inGroundLocked(0, npc.X, npc.Y, npc.Z)
		w.mu.Unlock()
		if in {
			t.Fatalf("tick %d: in the ground at (%.2f, %.2f)", i, npc.X, npc.Y)
		}
	}
	if npc.chase.returning || npc.X != 20 || npc.Y != 3 {
		t.Errorf("at (%.2f, %.2f) returning %v, want home at (20, 3)", npc.X, npc.Y, npc.chase.returning)
	}
}
