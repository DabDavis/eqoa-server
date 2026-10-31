package entity

import (
	"math"
	"testing"
	"time"
)

// A creature running at a player that turns on the pet stands to fight it:
// no run left on its channel (the user, 2026-10-31, F3 20261031-061313:
// "mobs are rubberbanding but is only visual, the pet is standing still
// attacking the mob in it's real position"). npcFightPetLocked never moves
// a creature, and it kept the velocity and the run of the chase it was in
// when the pet's hate passed the player's, so every client carried it along
// that velocity (ExtrapolateMove) and each update put it back.
func TestACreatureThatTurnsOnThePetStandsToFightIt(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.pets[r.p.ServerID].stance = petPassive
	crab := r.enemy
	crab.Level = 60
	// the player 20 off: the crab runs at it
	r.p.X, r.p.Z = crab.X-20, crab.Z
	r.w.mu.Lock()
	r.w.updateAggro(crab, r.p.ServerID, 10)
	r.w.mu.Unlock()
	r.tick(10 * time.Second)
	r.tick(10*time.Second + 100*time.Millisecond)
	if !crab.moving() || crab.Loco != LocoRun {
		t.Fatalf("the crab does not run at the player: velocity %v %v, loco %d", crab.VelX, crab.VelZ, crab.Loco)
	}
	// the pet beside it, and hated more: it turns on the pet
	pet.X, pet.Z = crab.X, crab.Z+3
	r.w.mu.Lock()
	r.w.updateAggro(crab, pet.ServerID, 1000)
	r.w.mu.Unlock()
	if crab.CurrentEnemy != pet.ServerID {
		t.Fatalf("the crab's enemy %d, not the pet %d", crab.CurrentEnemy, pet.ServerID)
	}
	x, z := crab.X, crab.Z
	for ms := 10200; ms <= 11000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if crab.X != x || crab.Z != z {
		t.Fatalf("the crab moved fighting the pet: (%.2f, %.2f) -> (%.2f, %.2f)", x, z, crab.X, crab.Z)
	}
	if crab.moving() || crab.Loco != LocoStand {
		t.Errorf("the crab stands fighting the pet but its channel runs: velocity (%.2f, %.2f, %.2f), loco %d",
			crab.VelX, crab.VelY, crab.VelZ, crab.Loco)
	}
	// turned to the pet where it stands now (the pet's own step comes after
	// the creatures' in a tick)
	r.w.mu.Lock()
	r.w.aiSystem(r.at(11100 * time.Millisecond))
	r.w.mu.Unlock()
	want := headingTo(pet.X-crab.X, pet.Z-crab.Z)
	if d := math.Abs(float64(crab.Facing - want)); d > 1e-3 && math.Abs(d-2*math.Pi) > 1e-3 {
		t.Errorf("the crab faces %.3f, the pet is at %.3f", crab.Facing, want)
	}
}
