package entity

import (
	"math"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The teleport orbs (orbs.go) in the move check's made world (movecheck_test.go:
// flat ground at 0, a platform with its top at 10 over x 200-210 and z 0-10, a
// wall across x = 100). Orb A stands at (50, 0, 50) and sends to beside orb B
// at (300, 0, 300), across the wall, facing away from B; B sends to beside A,
// facing away from A; orb P stands on the platform's top. The places are made
// up.

var (
	orbA = database.Orb{ID: 1, Name: "orb A", X: 50, Y: 0, Z: 50, ToX: 300, ToY: 0, ToZ: 296.5, ToFacing: math.Pi}
	orbB = database.Orb{ID: 2, Name: "orb B", X: 300, Y: 0, Z: 300, ToX: 50, ToY: 0, ToZ: 53.5, ToFacing: 0}
	orbP = database.Orb{ID: 3, Name: "orb P", X: 205, Y: 10, Z: 5, ToX: 50, ToY: 0, ToZ: 53.5}
)

// orbRig is a player in the made world with a session of its own, the orbs
// set, the travel hook recording what it is handed and, when land, landing
// it as handlers' travelPlayer does (PlaceFacing).
type orbRig struct {
	*mcRig
	s    *session.Session
	sent []GateTravel
	land bool
}

func newOrbRig(t *testing.T, mode MoveCheckMode, x, y, z float32) *orbRig {
	t.Helper()
	r := &orbRig{mcRig: newMCRig(t, mode, x, y, z)}
	mgr := session.NewManager()
	r.w.SessionManager = mgr
	r.s = mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, 1), Port: 7300}, 7300, 1, 1, 2, true)
	r.s.SetECSEntity(r.p)
	r.s.SetCharacterInWorld(true)
	r.p.HP, r.p.MaxHP = 500, 500
	r.w.SetOrbs([]database.Orb{orbA, orbB, orbP})
	r.w.OnTravel(func(tr GateTravel) {
		r.sent = append(r.sent, tr)
		if r.land {
			r.w.PlaceFacing(tr.P.ServerID, tr.To.X, tr.To.Y, tr.To.Z, tr.To.World, tr.Facing)
		}
	})
	return r
}

// trips hands the queued travellers to the hook (the tick's runTravels) and
// reports how many it was handed in all.
func (r *orbRig) trips() int {
	r.w.runTravels()
	return len(r.sent)
}

// stepTo walks the player in a straight line to (x, z) on the flat ground at
// the run speed, a report every 100 ms, and hands over the travellers after
// each: it stops at the first report that sent the player anywhere, and
// reports whether one did.
func (r *orbRig) stepTo(x, z float32) bool {
	fromX, fromZ := r.p.X, r.p.Z
	d := float32(math.Hypot(float64(x-fromX), float64(z-fromZ)))
	n := int(math.Ceil(float64(d / (RunSpeed * 0.1))))
	had := len(r.sent)
	for i := 1; i <= n; i++ {
		f := float32(i) / float32(n)
		r.report(fromX+(x-fromX)*f, 0, fromZ+(z-fromZ)*f, 100)
		if r.trips() > had {
			return true
		}
	}
	return false
}

// Stepping into an orb sends the player there and then, once, to the orb's
// destination with its facing; the control, a player who walks to 2 from the
// orb's middle (its radius is 1.5), stays.
func TestAnOrbSendsAPlayerWhoStepsIn(t *testing.T) {
	r := newOrbRig(t, MoveCheckLog, 40, 0, 50)
	if r.stepTo(48, 50) {
		t.Fatal("the control: a player 2 from the orb's middle was sent")
	}
	if r.stepTo(48, 50.5) || len(r.sent) != 0 {
		t.Fatal("the control: a player walking round the orb's edge was sent")
	}
	if !r.stepTo(50, 50) {
		t.Fatalf("a player who stepped into the orb (now at %.1f, %.1f) was not sent", r.p.X, r.p.Z)
	}
	if len(r.sent) != 1 {
		t.Fatalf("one step in sent the player %d times", len(r.sent))
	}
	tr := r.sent[0]
	if dx, dz := r.p.X-orbA.X, r.p.Z-orbA.Z; dx*dx+dz*dz > orbA.Radius*orbA.Radius+database.OrbDefaultRadius*database.OrbDefaultRadius {
		t.Errorf("sent from (%.2f, %.2f), outside the orb", r.p.X, r.p.Z)
	}
	if tr.P != r.p || tr.Sess != r.s || tr.To.World != 0 || tr.To.X != orbA.ToX || tr.To.Y != orbA.ToY || tr.To.Z != orbA.ToZ {
		t.Errorf("sent to %+v, want the row's (%.1f, %.1f, %.1f)", tr.To, orbA.ToX, orbA.ToY, orbA.ToZ)
	}
	if !tr.HasFacing || tr.Facing != orbA.ToFacing || tr.Ring != 0 {
		t.Errorf("facing %v %.2f ring %d, want the row's %.2f, ring 0", tr.HasFacing, tr.Facing, tr.Ring, orbA.ToFacing)
	}
	// standing in it, more reports: not sent again
	r.report(50.2, 0, 50.1, 100)
	r.report(49.9, 0, 50.2, 100)
	if r.trips() != 1 {
		t.Errorf("standing in the orb sent the player again: %d trips", len(r.sent))
	}
}

// The trip lands the player beside the orb it pairs with, turned away from it,
// and nobody bounces: not off its landing, not on the reports its client sent
// from inside the first orb before it heard of the move (taken as they come in
// log mode), not by walking straight back in while the orbs rest. The control:
// after the rest, stepping off and into the pair sends it back, beside the
// first orb.
func TestAnOrbLandsBesideItsPairAndNobodyBounces(t *testing.T) {
	r := newOrbRig(t, MoveCheckLog, 40, 0, 50)
	r.land = true
	if !r.stepTo(50, 50) {
		t.Fatal("not sent")
	}
	if r.p.X != orbA.ToX || r.p.Y != orbA.ToY || r.p.Z != orbA.ToZ || r.p.Facing != orbA.ToFacing {
		t.Fatalf("landed at (%.1f, %.1f, %.1f) facing %.2f, want (%.1f, %.1f, %.1f) facing %.2f",
			r.p.X, r.p.Y, r.p.Z, r.p.Facing, orbA.ToX, orbA.ToY, orbA.ToZ, orbA.ToFacing)
	}
	if database.OrbHolds(orbB, r.p.X, r.p.Y, r.p.Z) {
		t.Fatal("landed inside the pair")
	}
	// two reports from inside A, sent before the client heard (log mode takes
	// them), then the client at the landing
	r.report(50.1, 0, 50, 100)
	r.report(50.2, 0, 50, 100)
	r.report(orbA.ToX, 0, orbA.ToZ, 100)
	r.report(orbA.ToX, 0, orbA.ToZ-0.5, 100)
	if r.trips() != 1 {
		t.Fatalf("the stale reports or the landing sent the player again: %d trips", len(r.sent))
	}
	// straight back into B while the orbs rest: not sent
	r.report(orbB.X, 0, orbB.Z-1, 100)
	r.report(orbB.X, 0, orbB.Z, 100)
	if r.trips() != 1 {
		t.Fatalf("walking straight back in while resting sent the player: %d trips", len(r.sent))
	}
	// standing in B after the rest, with no step off: still not sent
	r.now = r.now.Add(OrbRest)
	r.report(orbB.X+0.1, 0, orbB.Z, 100)
	if r.trips() != 1 {
		t.Fatalf("standing in B when the rest ended sent the player: %d trips", len(r.sent))
	}
	// the control: off B and back in, after the rest
	if r.stepTo(orbB.X, orbB.Z-4) {
		t.Fatal("stepping off B sent the player")
	}
	if !r.stepTo(orbB.X, orbB.Z) {
		t.Fatal("the control: stepping back into B after the rest did not send the player")
	}
	if r.p.X != orbB.ToX || r.p.Z != orbB.ToZ || r.p.Facing != orbB.ToFacing || database.OrbHolds(orbA, r.p.X, r.p.Y, r.p.Z) {
		t.Errorf("back at (%.1f, %.1f) facing %.2f, want beside A (%.1f, %.1f) facing %.2f",
			r.p.X, r.p.Z, r.p.Facing, orbB.ToX, orbB.ToZ, orbB.ToFacing)
	}
}

// An orb holds its own floor: a player under the platform's orb, within its
// radius across and 10 under it, stays; the control, the same steps on the
// platform's top, sends it.
func TestAnOrbIsNotTheFloorUnderIt(t *testing.T) {
	r := newOrbRig(t, MoveCheckLog, 195, 0, 5)
	r.report(200, 0, 5, 100)
	r.report(204, 0, 5, 100)
	r.report(205, 0, 5, 100)
	if r.trips() != 0 {
		t.Fatal("a player on the ground 10 under the orb was sent")
	}
	r.report(206, 10, 5, 2000) // up on the platform (a server's move, as far as the walk goes)
	r.report(205.5, 10, 5, 100)
	if r.trips() != 1 || r.sent[0].To.X != orbP.ToX {
		t.Fatalf("the control: on the platform's top in its orb: %d trips", len(r.sent))
	}
}

// A player the server puts in an orb (a login, a GM's goto) is not sent by
// its reports from there until it steps off and back on; the control is that
// step.
func TestAnOrbWaitsForAPlayerPutInIt(t *testing.T) {
	r := newOrbRig(t, MoveCheckLog, 40, 0, 50)
	r.w.UpdatePosition(r.p.ServerID, 50.3, 0, 50, 0) // the server puts it there
	r.report(50.3, 0, 50, 100)
	r.report(50.1, 0, 50.2, 100)
	if r.trips() != 0 {
		t.Fatal("a player put in the orb was sent by its reports from there")
	}
	if r.stepTo(45, 50) {
		t.Fatal("stepping off sent the player")
	}
	if !r.stepTo(50, 50) {
		t.Fatal("the control: stepping back on did not send the player")
	}
}

// Neither the dead nor a GM flying is sent; the control, the same steps alive
// and on foot, is.
func TestAnOrbSendsNeitherTheDeadNorAFlyingGM(t *testing.T) {
	r := newOrbRig(t, MoveCheckLog, 40, 0, 50)
	r.p.HP = 0
	if r.stepTo(50, 50) {
		t.Fatal("a dead player was sent")
	}
	r.p.HP = 500
	r.stepTo(40, 50)
	r.w.SetFlying(r.s, true, r.now)
	if r.stepTo(50, 50) {
		t.Fatal("a flying GM was sent")
	}
	r.w.SetFlying(r.s, false, r.now)
	r.now = r.now.Add(3 * time.Second) // past the fly's grace
	r.stepTo(40, 50)
	if !r.stepTo(50, 50) {
		t.Fatal("the control: alive and on foot, not sent")
	}
}

// The move check takes an orb's trip as the server's move, in enforce mode:
// the trip crosses the made world's wall to the far side, the client's last
// reports from inside the first orb (from before it heard) are not its place,
// and from the landing on every report is taken; nobody is moved back. The
// control: the same jump made by the client alone, with no orb (a client that
// just reports the landing), is refused and moved back.
func TestTheMoveCheckTakesAnOrbsTrip(t *testing.T) {
	r := newOrbRig(t, MoveCheckEnforce, 40, 0, 50)
	r.land = true
	if !r.stepTo(50, 50) {
		t.Fatal("not sent")
	}
	stale := r.report(50.1, 0, 50, 100) // before the client heard
	took := 0
	for i := 0; i < 10; i++ {
		if r.report(orbA.ToX, 0, orbA.ToZ-float32(i)*0.8, 100) {
			took++
		}
	}
	if stale || took != 10 {
		t.Fatalf("after the trip: a stale report taken %v, %d of 10 from the landing taken (%s)", stale, took, r.p.mv.why)
	}
	if len(r.p.mv.fixes) != 0 || r.trips() != 1 {
		t.Fatalf("moved back %d times, %d trips", len(r.p.mv.fixes), len(r.sent))
	}

	c := newOrbRig(t, MoveCheckEnforce, 40, 0, 40)
	c.w.SetOrbs(nil)
	c.stepTo(48, 40)
	if c.report(orbA.ToX, 0, orbA.ToZ, 100) || len(c.p.mv.fixes) != 1 {
		t.Fatalf("the control: the client's own jump across the wall taken (moved back %d times)", len(c.p.mv.fixes))
	}
}

// The orbs read from table orbs: a row that sends to a spot inside itself, or
// with no world the client has, is left out and named; the rest kept, by world.
func TestOrbRowsAreChecked(t *testing.T) {
	good := orbA
	if err := database.CheckOrb(good, 0, 0); err != nil {
		t.Fatalf("a good row: %v", err)
	}
	self := orbA
	self.ToX, self.ToZ = orbA.X+1, orbA.Z
	if database.CheckOrb(self, 0, 0) == nil {
		t.Error("a row sending to a spot inside itself was taken")
	}
	if database.CheckOrb(self, 0, 1) != nil {
		t.Error("the control: the same spot in another world was refused")
	}
	if database.CheckOrb(good, 30, 0) == nil || database.CheckOrb(good, 0, -1) == nil {
		t.Error("a world past the client's table was taken")
	}
	wide := orbA
	wide.Radius = -1
	if database.CheckOrb(wide, 0, 0) == nil {
		t.Error("a negative radius was taken")
	}
	// the band: the pad's top and a jump onto it, not the floor 19 over or under
	for _, c := range []struct {
		y    float32
		want bool
	}{{0, true}, {0.6, true}, {2.4, true}, {-0.9, true}, {2.6, false}, {-1.1, false}, {19, false}, {-19, false}} {
		if got := database.OrbHolds(orbA, orbA.X+1, c.y, orbA.Z); got != c.want {
			t.Errorf("feet at height %.1f over the orb: in it %v, want %v", c.y, got, c.want)
		}
	}
	if database.OrbHolds(orbA, orbA.X+1.6, 0, orbA.Z) {
		t.Error("1.6 across from the middle of an orb of the default radius is in it")
	}
	w := NewWorld()
	w.SetOrbs([]database.Orb{{ID: 9, World: 2, X: 1, Z: 1, ToWorld: 2, ToX: 9, ToZ: 9}, orbA, orbB})
	if got := w.OrbsSummary(); got != "orbs: 3 in table orbs (world 0: 2, world 2: 1)" {
		t.Errorf("orbs by world: %q", got)
	}
}
