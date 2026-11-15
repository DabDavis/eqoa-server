package entity

import (
	"testing"
	"time"
)

// Heirloom petitions 2189/2190 (Zerkziee) and the owner, 2026-11-15: "the
// sting rays chased players out of water at isle of dread"; "Stingrays in IoD
// once you port in in the water chase you onto land when they attack you and
// follow you out of the water". The island's manta rays, regan piranhas and
// Megaladon swim off the sea's bed (not on the ground), and a chase lifted one
// onto the bed wherever it rose over it (groundNPCLocked), up the beach and out
// of the water, where nothing held it any more. On the beach a ray 5 over the
// sea floor chases a player standing on the shore: it stays in the water,
// stops at its edge, and breaks off at chaseHeld; a player in the water with it
// it fights on, never giving up.
func TestARayOffTheBedStopsAtTheWatersEdgeAndGoesHome(t *testing.T) {
	chase := func(px, py float32, secs int) (ray *NPC, out int, gaveUp time.Duration) {
		w := beachWorld(t, true)
		ray = addNPC(w, 100, "manta rays", 2030, 1000, creatureModel, 0)
		ray.Y, ray.SpawnY = 45, 45 // 5 over the sea floor at 40: a swimmer, off the ground
		p := w.Players[1]
		p.X, p.Y = px, py
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.spawnOnGroundLocked(ray) || w.keepsToLocked(ray) != keepsToWater {
			t.Fatalf("the ray: on the ground %v, keeps to %q; want off it, of the water", w.spawnOnGroundLocked(ray), w.keepsToLocked(ray))
		}
		start := time.Unix(1_800_000_000, 0)
		now := start
		ray.chase.on, ray.chase.fromX, ray.chase.fromY, ray.chase.fromZ = true, ray.X, ray.Y, ray.Z
		for i := 0; i < secs*10; i++ {
			now = now.Add(100 * time.Millisecond)
			ray.chase.last = now.Add(-100 * time.Millisecond)
			_, give := w.chaseLocked(ray, p.X, p.Y, p.Z, 0, 0, now)
			if !w.keepsInLocked(ray, ray.X, ray.Y, ray.Z) {
				out++
			}
			if give {
				return ray, out, now.Sub(start)
			}
		}
		return ray, out, 0
	}

	// its enemy on the shore, dry
	ray, out, gaveUp := chase(1990, 60, 30)
	t.Logf("enemy on the shore: the ray at (%.2f, %.2f), the beach's floor %.2f there; %d ticks out of the water; gave up after %v",
		ray.X, ray.Y, beachFloor(ray.X), out, gaveUp)
	if out > 0 || ray.Y+bodyAbove >= 50 {
		t.Errorf("the ray left the water: %d ticks out of it, at (%.2f, %.2f)", out, ray.X, ray.Y)
	}
	if ray.X < 1999 {
		t.Errorf("the ray stood at x %.2f: past the water's edge (x 2000.5 at the surface)", ray.X)
	}
	if gaveUp < chaseHeld || gaveUp > chaseHeld+5*time.Second {
		t.Errorf("held at the edge with its enemy dry, the ray gave up after %v; want chaseHeld (%v) after its swim there (3-4 s)", gaveUp, chaseHeld)
	}

	// the control: its enemy in the water (at x 2020, 46: under the surface)
	// is reached and fought, never given up on
	ray, out, gaveUp = chase(2020, 46, 15)
	if gaveUp != 0 || out > 0 || distXZ(ray.X, ray.Z, 2020, 1000) > npcReach(ray)+0.5 {
		t.Errorf("an enemy in the water: the ray at x %.2f, %d ticks out, gave up after %v; want it at its enemy, fighting", ray.X, out, gaveUp)
	}
}

// edgeSea is water east of x 1005, below 20: the aggro fixture's player at
// (1000, 10, 1000) stands on dry land, a creature at x 1015 in the sea.
type edgeSea struct{}

func (edgeSea) Flags(_ uint8, x, y, _ float32) (uint32, bool) {
	if x > 1005 && y < 20 {
		return envWater, true
	}
	return 0, true
}

// A creature of the water takes up no one out on the land on sight (it could
// only hold at the edge, then break off, and take him up again from home).
// The controls: the same player in the water is attacked; a creature of the
// land beside him on the shore attacks him.
func TestACreatureOfTheWaterTakesUpNoOneOnTheLand(t *testing.T) {
	f := newAggroFix(t, 50)
	f.w.SetEnv(edgeSea{})
	ray := f.npc(2001, "manta rays", 53, bearBody, 1015, 1000)
	bear := f.npc(2002, "Brown Bear", 53, bearBody, 995, 1000)
	f.look()
	if f.attacks(ray) {
		t.Error("the ray in the sea took up the player on the shore")
	}
	if !f.attacks(bear) {
		t.Error("the bear on the shore did not attack (the control)")
	}
	f.p.X = 1010 // into the sea
	f.look()
	if !f.attacks(ray) {
		t.Error("the ray did not attack the player in the water with it")
	}
}
