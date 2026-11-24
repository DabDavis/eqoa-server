package entity

import (
	"testing"
	"time"
)

// trenchGround is a floor at 10 with a trench to 0 across it (x 0 to 6), as
// beside the ramp in the Odus ruin of Heirloom 2821/2823 (a floor at 67.6, the
// trench by the ramp's side at 57.6), with nothing across it a search can find.
type trenchGround struct{}

func (trenchGround) surface(x, z float32) float32 {
	if x >= 0 && x <= 6 {
		return 0
	}
	return 10
}

func (g trenchGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	s := g.surface(x, z)
	if s > y+2 {
		return y, false
	}
	return s, true
}

// A creature whose search finds no way to its enemy on the floor across a
// trench (Heirloom 2821/2823: a phantom ran straight at a pet up the ramp and fell
// ten into the trench beside it, under the floor its players stood on) stops at
// the edge and gives up after chaseHeld, never falling in. The control: an enemy
// down in the trench is still chased into it, as TestAChaseWithNoWayRunsStraight.
func TestAChaseNeverFallsIntoATrenchUnderItsEnemy(t *testing.T) {
	run := func(toX, toY float32) (*NPC, bool, [][3]float32) {
		w := roamWorld(t)
		w.SetGround(trenchGround{})
		npc := addNPC(w, 100, "phantom", -6, 0, creatureModel, 40)
		npc.Y, npc.SpawnY = 10, 10
		ok, at := chaseOver(w, npc, toX, toY, 0, 300)
		return npc, ok, at
	}
	npc, ok, at := run(12, 10)
	if ok {
		t.Fatal("reached an enemy across the trench")
	}
	if i, bad := fallsOrClimbs(at); bad {
		t.Errorf("tick %d went from %v to %v: into the trench", i, at[i-1], at[i])
	}
	if npc.Y != 10 || npc.X > 0.5 {
		t.Errorf("stood at (%.2f, %.2f); want at the edge (x under 0) on the floor at 10", npc.X, npc.Y)
	}
	if n := len(at) - 1; n >= 300 || time.Duration(n)*100*time.Millisecond < chaseHeld {
		t.Errorf("gave up after %d ticks; want after chaseHeld (%v) at the edge", n, chaseHeld)
	}

	_, _, at = run(5, 0)
	if _, bad := fallsOrClimbs(at); !bad {
		t.Error("the control: an enemy down in the trench was not chased into it")
	}
}

// A creature home from a fight it gave up is whole again (Heirloom 2821: "it
// then reset but didnt go back to full health"), on its way home or set there
// at once; one whose fight ended any other way (its enemy dead) heals out of the
// fight as before, and a player's pet is left as it is.
func TestAnEvadedCreatureIsWholeAgainAtHome(t *testing.T) {
	home := func(giveUp bool, owner EntityID) *NPC {
		w := roamWorld(t)
		npc := addNPC(w, 100, "phantom", 0, 0, creatureModel, 40)
		npc.MaxHP, npc.HP, npc.Owner = 13260, 6940, owner
		w.mu.Lock()
		defer w.mu.Unlock()
		npc.chase = chaseState{on: true, fromX: 20, fromY: npc.Y, fromZ: 0}
		npc.CurrentEnemy = 1
		if giveUp {
			w.giveUpLocked(npc)
		} else {
			npc.CurrentEnemy = 0
			w.goHomeLocked(npc)
		}
		now := time.Unix(1_800_000_000, 0)
		for i := 0; i < 200 && npc.chase.returning; i++ {
			now = now.Add(100 * time.Millisecond)
			npc.chase.last = now.Add(-100 * time.Millisecond)
			w.homeSystem(now)
		}
		if npc.chase.returning || distXZ(npc.X, npc.Z, 20, 0) > 0.5 {
			t.Fatalf("never home: at (%.2f, %.2f)", npc.X, npc.Z)
		}
		return npc
	}
	if n := home(true, 0); n.HP != n.MaxHP {
		t.Errorf("home from a fight it gave up at %d of %d; want whole", n.HP, n.MaxHP)
	}
	if n := home(false, 0); n.HP != 6940 {
		t.Errorf("the control: home from a fight that ended at %d; want 6940, to heal out of it", n.HP)
	}
	if n := home(true, 7); n.HP != 6940 {
		t.Errorf("a player's pet home at %d; want 6940, as it was", n.HP)
	}

	// one that never moved is whole at once
	w := roamWorld(t)
	npc := addNPC(w, 101, "phantom", 0, 0, creatureModel, 40)
	npc.MaxHP, npc.HP, npc.CurrentEnemy = 13260, 100, 1
	w.mu.Lock()
	w.giveUpLocked(npc)
	w.mu.Unlock()
	if npc.HP != npc.MaxHP {
		t.Errorf("a creature that gave up where it stood: %d of %d; want whole", npc.HP, npc.MaxHP)
	}
}
