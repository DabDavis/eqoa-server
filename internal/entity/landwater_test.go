package entity

import (
	"testing"
	"time"
)

// pond is water a unit deep over the flat ground in x 995 to 1005 and, with
// zHalf > 0, z within zHalf of 1000; across every z (a river) with zHalf 0.
type pond struct{ zHalf float32 }

func (p pond) Flags(_ uint8, x, y, z float32) (uint32, bool) {
	if y < 1 && x > 995 && x < 1005 && (p.zHalf == 0 || (z > 1000-p.zHalf && z < 1000+p.zHalf)) {
		return envWater, true
	}
	return 0, true
}

// pondWorld is a roam world on flat ground at height 0 (camplanding_test.go) with the water given, and a
// creature of the land spawned at (985, 1000), west of it.
func pondWorld(t *testing.T, water pond) (*World, *NPC) {
	t.Helper()
	w := roamWorld(t)
	w.SetGround(flatGround{})
	w.SetEnv(water)
	npc := addNPC(w, 100, "orc pawn", 985, 1000, creatureModel, 0)
	return w, npc
}

// homeFrom runs npc's way home from (x, z) to where its chase began, its
// spawn, up to ticks ticks: the ticks it spent in the water, and whether it
// got home.
func homeFrom(w *World, npc *NPC, x, z float32, ticks int) (wet int, home bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc.X, npc.Y, npc.Z = x, 0, z
	npc.chase = chaseState{on: true, fromX: npc.SpawnX, fromY: npc.SpawnY, fromZ: npc.SpawnZ}
	w.goHomeLocked(npc)
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < ticks && npc.chase.returning; i++ {
		now = now.Add(100 * time.Millisecond)
		w.TickCount++
		w.homeSystem(now)
		if w.npcInWaterLocked(npc) {
			wet++
		}
	}
	return wet, !npc.chase.returning && distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) < 0.5
}

// A creature of the land given up on the far side of a pond comes home round
// it, never in the water; the control (no dry way home) walks straight back
// through it.
func TestALandCreatureComesHomeRoundTheWater(t *testing.T) {
	w, npc := pondWorld(t, pond{zHalf: 12})
	wet, home := homeFrom(w, npc, 1015, 1000, 600)
	if !home || wet != 0 {
		t.Errorf("home %v, %d ticks in the water: want home round the pond, none in it", home, wet)
	}
	was := homesDry
	homesDry = false
	defer func() { homesDry = was }()
	w, npc = pondWorld(t, pond{zHalf: 12})
	if wet, home := homeFrom(w, npc, 1015, 1000, 600); !home || wet == 0 {
		t.Errorf("the control: home %v, %d ticks in the water: the test proves nothing", home, wet)
	}
}

// Where the water runs across the whole way (a river with no way round), the
// creature still comes home, through it: it is not stranded. A creature of
// the water, or a chase, is as it was.
func TestALandCreatureCrossesTheWaterHomeWhenItMust(t *testing.T) {
	w, npc := pondWorld(t, pond{})
	wet, home := homeFrom(w, npc, 1015, 1000, 600)
	if !home || wet == 0 {
		t.Errorf("home %v, %d ticks in the water: want home through the river", home, wet)
	}
}

// A chase begun while a creature crosses the water on its way home keeps the
// home it had: given up again, it does not come back to stand in the water.
func TestAChaseOnTheWayHomeKeepsItsHome(t *testing.T) {
	w, npc := pondWorld(t, pond{})
	w.mu.Lock()
	defer w.mu.Unlock()
	npc.X, npc.Y, npc.Z = 1000, 0, 1000
	npc.chase = chaseState{on: true, fromX: npc.SpawnX, fromY: npc.SpawnY, fromZ: npc.SpawnZ}
	w.goHomeLocked(npc)
	if !npc.chase.returning {
		t.Fatal("not on its way home")
	}
	w.chaseLocked(npc, 1003, 0, 1000, 0, 0, time.Unix(1_800_000_000, 0))
	if npc.chase.fromX != npc.SpawnX || npc.chase.fromZ != npc.SpawnZ {
		t.Errorf("its home after a chase begun in the water: (%.1f, %.1f), want its spawn (%.1f, %.1f)",
			npc.chase.fromX, npc.chase.fromZ, npc.SpawnX, npc.SpawnZ)
	}
}

// A creature of the land left standing in the water walks out of it, and
// then keeps out of it.
func TestALandCreatureLeftInTheWaterWalksOut(t *testing.T) {
	w, npc := pondWorld(t, pond{zHalf: 12})
	npc.X, npc.Z = 1000, 1000
	w.mu.Lock()
	x, z := w.roamPointLocked(npc)
	if x == npc.X && z == npc.Z {
		w.mu.Unlock()
		t.Fatal("it stands in the water: no leg out of it")
	}
	if w.waterAtLocked(npc.World, x, bodyAbove, z) {
		t.Errorf("its leg out ends in the water at (%.1f, %.1f)", x, z)
	}
	npc.X, npc.Z = x, z
	defer w.mu.Unlock()
	for i := 0; i < 200; i++ {
		x, z := w.roamPointLocked(npc)
		for k := 0; k <= 20; k++ {
			f := float32(k) / 20
			if px, pz := npc.X+(x-npc.X)*f, npc.Z+(z-npc.Z)*f; w.waterAtLocked(npc.World, px, bodyAbove, pz) {
				t.Fatalf("out of the water, a leg back into it at (%.1f, %.1f)", px, pz)
			}
		}
		npc.X, npc.Z = x, z
	}
}
