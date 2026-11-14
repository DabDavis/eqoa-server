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

// An on_move area (petition 1960, the thorns): each player in it is hit at once, on its own clock
// (two at once both take it); one standing still is not hit again however long it stays; once its
// time is up a step hits it again, a step inside its time does not.
func TestAnOnMoveHazardSparesWhoStandsStill(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP, f.p.MaxHP = 5000, 5000
	thorns := database.Hazard{ID: 8, Name: "Spiky vines", World: 0, X1: 100, Y1: 10, Z1: 100, X2: 100, Y2: 10, Z2: 100,
		Radius: 3, Damage: 500, EveryMS: 6000, OnMove: true}
	f.w.SetHazards([]database.Hazard{thorns})
	other := &Player{ServerID: 4321, Name: "Other", World: 0, HP: 5000, MaxHP: 5000, X: 101, Y: 10, Z: 100}
	f.w.Players[other.ServerID] = other
	tick := func(at time.Duration) {
		f.w.mu.Lock()
		f.w.TickCount = 10
		f.w.hazardSystem(f.t0.Add(at))
		f.w.mu.Unlock()
	}
	f.p.X, f.p.Y, f.p.Z = 99, 10, 100
	tick(0)
	if f.p.HP != 4500 || other.HP != 4500 {
		t.Fatalf("two walking in: HP %d and %d, want 4500 each", f.p.HP, other.HP)
	}
	for s := 1; s <= 20; s++ { // still for 20 s
		tick(time.Duration(s) * time.Second)
	}
	if f.p.HP != 4500 || other.HP != 4500 {
		t.Fatalf("standing still 20 s: HP %d and %d", f.p.HP, other.HP)
	}
	f.p.X = 98 // a step, its time up long since
	tick(21 * time.Second)
	if f.p.HP != 4000 || other.HP != 4500 {
		t.Fatalf("one steps: HP %d and %d, want 4000 and 4500", f.p.HP, other.HP)
	}
	f.p.X = 99 // a step within its 6 s
	tick(23 * time.Second)
	if f.p.HP != 4000 {
		t.Fatalf("a step within the time: HP %d", f.p.HP)
	}
	tick(27 * time.Second) // the time up, and it stepped since the hit
	if f.p.HP != 3500 {
		t.Fatalf("its time up, moved since: HP %d, want 3500", f.p.HP)
	}
}

// A hazard of strikes (Desolation 4's unstable rocks: "3 warnings=death") warns, its words with
// the warning's number and no damage, at each hit but the last of its strikes, which takes its
// damage; a count is forgotten HazardStrikesForget after its last warning. The control: a plain
// hazard hits at once.
func TestAHazardOfStrikesWarnsThenHits(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP, f.p.MaxHP = 2500, 2500
	rock := database.Hazard{ID: 9, Name: "An unstable rock", World: 0, X1: 100, Y1: 10, Z1: 100, X2: 100, Y2: 10, Z2: 100,
		Radius: 5, Damage: 1000000, EveryMS: 3000, Words: "The rock shifts under you: it is unstable (%d of 3).", Strikes: 3}
	f.w.SetHazards([]database.Hazard{rock})
	tick := func(at time.Duration) {
		f.w.mu.Lock()
		f.w.TickCount = 10
		f.w.hazardSystem(f.t0.Add(at))
		f.w.mu.Unlock()
	}
	f.p.X, f.p.Y, f.p.Z = 101, 10, 100
	tick(0)
	tick(3 * time.Second)
	if f.p.HP != 2500 || f.p.hazardStrikes[9].n != 2 {
		t.Fatalf("two warnings: HP %d, count %d", f.p.HP, f.p.hazardStrikes[9].n)
	}
	// out and back after its count is forgotten: a first warning again
	f.p.X = 200
	tick(4 * time.Second)
	f.p.X = 101
	tick(4*time.Second + database.HazardStrikesForget*time.Second + time.Minute)
	if f.p.HP != 2500 || f.p.hazardStrikes[9].n != 1 {
		t.Fatalf("after its count was forgotten: HP %d, count %d", f.p.HP, f.p.hazardStrikes[9].n)
	}
	base := 4*time.Second + database.HazardStrikesForget*time.Second + time.Minute
	tick(base + 3*time.Second)
	tick(base + 6*time.Second)
	if f.p.HP != 0 || f.died() != 1 {
		t.Fatalf("the third warning: HP %d, %d deaths", f.p.HP, f.died())
	}
	if err := database.CheckHazard(rock, 0); err != nil {
		t.Fatal(err)
	}
	rock.Strikes = -1
	if database.CheckHazard(rock, 0) == nil {
		t.Fatal("strikes -1 passed")
	}
}
