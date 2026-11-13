package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A hazard hurts a player standing in it (petition 1937, hazards.go): at once on stepping in, then
// every every_ms while it stays (not between), not off its capsule nor on the floor above it, and a
// hit that takes the last hit points is a death. The controls: a player beside the vine, and one
// flying (#fly).
func TestAHazardHurtsWhoStandsInIt(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP, f.p.MaxHP = 2500, 2500
	vine := database.Hazard{ID: 7, Name: "Spiky vines", World: 0, X1: 90, Y1: 10, Z1: 100, X2: 110, Y2: 10, Z2: 100,
		Radius: 1.5, Damage: 1000, EveryMS: 2000, Words: "The spiky vines tear at you for %d."}
	f.w.SetHazards([]database.Hazard{vine})
	tick := func(at time.Duration) {
		f.w.mu.Lock()
		f.w.TickCount = 10
		f.w.hazardSystem(f.t0.Add(at))
		f.w.mu.Unlock()
	}
	// beside it (2 off the segment), and above it on a floor 6 up: nothing
	f.p.X, f.p.Y, f.p.Z = 100, 10, 102
	tick(0)
	f.p.X, f.p.Y, f.p.Z = 100, 16, 100
	tick(0)
	if f.p.HP != 2500 {
		t.Fatalf("beside the vine and above it: HP %d", f.p.HP)
	}
	f.p.X, f.p.Y, f.p.Z = 95, 10, 100.5
	tick(time.Second)
	if f.p.HP != 1500 {
		t.Fatalf("stepping in: HP %d, want 1500", f.p.HP)
	}
	tick(2500 * time.Millisecond) // 1.5 s after: not yet
	if f.p.HP != 1500 {
		t.Fatalf("1.5 s after the hit: HP %d", f.p.HP)
	}
	f.p.Flying = true
	tick(3 * time.Second)
	if f.p.HP != 1500 {
		t.Fatalf("flying: HP %d", f.p.HP)
	}
	f.p.Flying, f.p.flyGrace = false, time.Time{}
	tick(3 * time.Second)
	if f.p.HP != 500 {
		t.Fatalf("2 s after the hit: HP %d, want 500", f.p.HP)
	}
	tick(5 * time.Second)
	if f.p.HP != 0 || f.died() != 1 {
		t.Fatalf("the third hit: HP %d, %d deaths", f.p.HP, f.died())
	}
	if got := hazardWords(&vine, 1000); got != "The spiky vines tear at you for 1000." {
		t.Errorf("the words: %q", got)
	}
	vine.Words = ""
	if got := hazardWords(&vine, 7); got != "Spiky vines hurts you for 7." {
		t.Errorf("the default words: %q", got)
	}
}

// A round area (one point), the server's figures where the row gives 0, and the checks.
func TestHazardRows(t *testing.T) {
	r := database.Hazard{X1: 5, Y1: 0, Z1: 5, X2: 5, Y2: 0, Z2: 5, Damage: 10}
	if !database.HazardHolds(r, 6, 0, 5) || database.HazardHolds(r, 7, 0, 5) || database.HazardHolds(r, 5, -3, 5) {
		t.Error("a round area of the default radius 1.5 and 2 below")
	}
	for _, bad := range []database.Hazard{{Damage: 0}, {Damage: 5, EveryMS: 100}, {Damage: 5, X2: 600}, {Damage: 5, Radius: 101}, {Damage: 5, Words: "100% %d"}} {
		if database.CheckHazard(bad, 0) == nil {
			t.Errorf("%+v taken", bad)
		}
	}
	if database.CheckHazard(r, 30) == nil || database.CheckHazard(r, 2) != nil {
		t.Error("worlds")
	}
}
