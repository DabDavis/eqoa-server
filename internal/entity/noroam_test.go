package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petitions 1963-1965: a wanderer whose ground holds a no-roam circle never steps into it over half
// an hour of wandering (it gets close: the control is how near it came), while a creature spawned
// in the circle (the camp's own) walks it as ever; a leg through the circle is refused even when
// both its ends are outside, and one standing inside may walk out.
func TestWanderersKeepOutOfANoRoamArea(t *testing.T) {
	w := roamWorld(t)
	camp := database.NoRoam{ID: 1, Name: "kobold camp", World: 0, X: 1020, Z: 1000, Radius: 12}
	w.SetNoRoam([]database.NoRoam{camp})
	beetle := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0) // 20 from the middle, 8 from its edge
	kobold := addNPC(w, 101, "beetle", 1020, 1002, creatureModel, 0) // its own
	w.mu.Lock()
	if !w.legInNoRoamLocked(beetle, 1040, 1000) || !w.legInNoRoamLocked(beetle, 1020, 1005) || w.legInNoRoamLocked(beetle, 1000, 1030) {
		t.Fatal("through, into, and the control away from it")
	}
	if w.legInNoRoamLocked(kobold, 1040, 1000) {
		t.Fatal("its own refused")
	}
	beetle.X, beetle.Z = 1020, 1000 // set inside by a fight: it walks out
	if w.legInNoRoamLocked(beetle, 990, 1000) {
		t.Fatal("walking out refused")
	}
	beetle.X, beetle.Z = 1000, 1000
	w.mu.Unlock()
	nearest, kin := float32(1e9), false
	run(w, time.Now(), 30*time.Minute, func(now time.Time) {
		if d := distXZ(beetle.X, beetle.Z, camp.X, camp.Z); d < nearest {
			nearest = d
		}
		if distXZ(kobold.X, kobold.Z, kobold.SpawnX, kobold.SpawnZ) > 1 {
			kin = true
		}
	})
	if nearest <= camp.Radius {
		t.Errorf("the beetle came %.1f from the camp's middle, inside its %.0f", nearest, camp.Radius)
	}
	if nearest > camp.Radius+10 {
		t.Errorf("the beetle came no nearer than %.1f: it should get close", nearest)
	}
	if !kin {
		t.Error("the camp's own never wandered")
	}
}
