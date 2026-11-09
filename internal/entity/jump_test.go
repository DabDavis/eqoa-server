package entity

import (
	"math"
	"net"
	"strings"
	"testing"
	"time"
)

// The movement check's jump (jump.go): honest jumps taken, fake ones and double
// jumps refused, and the same honest jump from a client that does not jump
// refused as any rise in the air is (the control: without the jump it is a fault).

// jumpSpeed is vanilla's start speed (walker.rs JUMP_SPEED, u/s up).
const jumpSpeed = 7.1

// jumpGround is mcGround with a box 0.7 high over x 300-306 and z 0-10, a step
// 0.3 high over x 320-330, and, over z 100-200, the ground rising at 20 degrees
// east of x 400.
type jumpGround struct{ mcGround }

func (jumpGround) floors(x, z float32) []float32 {
	fs := mcGround{}.floors(x, z)
	if x >= 300 && x <= 306 && z >= 0 && z <= 10 {
		fs = append(fs, 0.7)
	}
	if x >= 320 && x <= 330 && z >= 0 && z <= 10 {
		fs = []float32{0.3}
	}
	if z >= 100 && z <= 200 && x > 400 {
		fs = []float32{(x - 400) * float32(math.Tan(20*math.Pi/180))}
	}
	return fs
}

func (g jumpGround) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	best, found := float32(0), false
	for _, f := range g.floors(x, z) {
		if f <= y+up && f >= y-down && (!found || f > best) {
			best, found = f, true
		}
	}
	return best, found
}

func (g jumpGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	return g.Support(world, x, y, z, 2, 1000)
}

// newJumpRig is a player in the made world (jumpGround) on a server with jumps,
// whose client jumps when jumps is set.
func newJumpRig(t *testing.T, mode MoveCheckMode, x, y, z float32, jumps bool) *mcRig {
	t.Helper()
	r := newMCRig(t, mode, x, y, z)
	r.w.SetGround(jumpGround{})
	r.w.SetJumps(true)
	r.p.Jumps = jumps
	return r
}

// arcReport is a 0x40 of a jump: the body's height over the floor it left, how
// far across from where it left it, and the ms since the report before.
type arcReport struct {
	dy, dx float32
	ms     int
	landed bool
}

// consoleStep is VITimer's count into console frame n (walker.rs console_step):
// 32 or 34.
func consoleStep(n int) int {
	c := func(n int) int { return int(float64(n)*2*1001/60*0.49152) * 2 }
	return c(n) - c(n-1)
}

// jumpArc is a jump's reports as vanilla's walk makes them: from a floor the
// body leaves at jumpSpeed upward, its first 32 ms tick at the ground's damping
// (it leaves from one), then CLIENT's air, Gravity (-19.6) and the damping of
// 0.8 a second, over console frames of 32 or 34 VITimer units (a 32 tick, then
// a 2 rest), a 0x40 every 100 of them; across along +x at vh, which the air
// slows as the walk's does. lead is the units since the last report as it
// leaves. It ends at the report that lands it on floorAt (relative to the floor
// it left), or after 4 s.
func jumpArc(vh float64, lead int, floorAt func(dx float32) float32) []arcReport {
	y, vy, x := 0.0, jumpSpeed, 0.0
	since, ms := lead, 0.0
	var out []arcReport
	first := true
	for n := 1; n < 120; n++ {
		step := consoleStep(n)
		ticks := []int{32}
		if step == 34 {
			ticks = append(ticks, 2)
		}
		landed := false
		for _, tk := range ticks {
			dt := float64(tk) * 0.001
			d := math.Pow(0.8, dt)
			if first {
				d = math.Pow(0.02, dt) // the run's thrust keeps vh on this one
			}
			y += (-19.6*dt*dt + vy*dt) * d
			vy = (vy - 19.6*dt) * d
			x += vh * dt * d
			if !first {
				vh *= d
			}
			first = false
			if f := float64(floorAt(float32(x))); vy < 0 && y <= f {
				y, landed = f, true
			}
		}
		since += step
		ms += float64(step) / 0.98304
		if since >= 100 || landed {
			out = append(out, arcReport{dy: float32(y), dx: float32(x), ms: int(math.Round(ms)), landed: landed})
			since, ms = 0, 0
		}
		if landed {
			break
		}
	}
	return out
}

func level(float32) float32 { return 0 }

// jump reports a jump from where the player stands, along +x (the rest of the
// way on floorAt as jumpArc has it); it returns how many reports were not
// taken, and the why of the first.
func (r *mcRig) jump(vh float64, lead int, floorAt func(dx float32) float32) (int, string) {
	x0, y0, z := r.p.X, r.p.Y, r.p.Z
	missed, why := 0, ""
	for _, a := range jumpArc(vh, lead, floorAt) {
		if !r.report(x0+a.dx, y0+a.dy, z, a.ms) {
			if missed == 0 {
				why = r.p.mv.why
			}
			missed++
		}
	}
	return missed, why
}

// stand reports the player standing where it is after ms.
func (r *mcRig) stand(ms int) bool { return r.report(r.p.X, r.p.Y, r.p.Z, ms) }

// The jump's arc as the client makes it tops out at JumpTop, and lands about
// 0.6 s after it left on flat ground: the two sides agree on the number.
func TestJumpArcTopsAtJumpTop(t *testing.T) {
	top, end := float32(0), 0
	for _, a := range jumpArc(0, 100, level) {
		top = max(top, a.dy)
		end += a.ms
	}
	// the reports sample it: the arc's own top is 0.896 (walker.rs's test)
	if top > JumpTop+0.01 || top < JumpTop-0.15 {
		t.Fatalf("the reports' top %.3f, JumpTop %.2f", top, JumpTop)
	}
	if end < 550 || end > 750 {
		t.Fatalf("in the air %d ms", end)
	}
}

// Honest jumps from a client that jumps are taken in enforce mode: in place,
// at a run, one after another (a report standing between, as vanilla sends),
// onto a box 0.7 high, off the platform's edge into a 10 fall, and up a 20-degree
// slope leaving 90 ms after its last report (higher than it, which the floor
// under its first report in the air says). Never moved back. The controls: the
// same jump in place from a client that does not jump, and from one that does on
// a server without jumps, is refused as any rise in the air (the check before
// the jump).
func TestMoveCheckTakesAJump(t *testing.T) {
	r := newJumpRig(t, MoveCheckEnforce, 0, 0, 5, true)
	if n, why := r.jump(0, 100, level); n != 0 {
		t.Fatalf("a jump in place: %d not taken (%s)", n, why)
	}
	if r.p.Y != 0 || r.p.mv.air || r.p.mv.jump.on {
		t.Fatalf("landed: y %.2f, air %v, jump %+v", r.p.Y, r.p.mv.air, r.p.mv.jump)
	}
	for i := 0; i < 4; i++ {
		if !r.stand(120) {
			t.Fatalf("standing after jump %d not taken (%s)", i, r.p.mv.why)
		}
		if n, why := r.jump(float64(RunSpeed), 100+i*30, level); n != 0 {
			t.Fatalf("running jump %d: %d not taken (%s)", i, n, why)
		}
		if n := r.walk(r.p.X+4, 5, RunSpeed, flat); n != 0 {
			t.Fatalf("running on after jump %d: %d not taken (%s)", i, n, r.p.mv.why)
		}
	}

	// onto the box: it lands on its top
	r.w.UpdatePosition(r.p.ServerID, 297, 0, 5, 0)
	r.report(297, 0, 5, 300)
	box := func(dx float32) float32 {
		if x := 297 + dx; x >= 300 && x <= 306 {
			return 0.7
		}
		return 0
	}
	if n, why := r.jump(float64(RunSpeed), 100, box); n != 0 {
		t.Fatalf("onto the box: %d not taken (%s)", n, why)
	}
	if r.p.Y != 0.7 || r.p.mv.air {
		t.Fatalf("on the box: y %.2f, air %v", r.p.Y, r.p.mv.air)
	}

	// off the platform's edge: up, then 10.9 down to the ground
	r.w.UpdatePosition(r.p.ServerID, 209, 10, 5, 0)
	r.report(209, 10, 5, 300)
	edge := func(dx float32) float32 {
		if 209+dx <= 210 {
			return 0
		}
		return -10
	}
	if n, why := r.jump(float64(RunSpeed), 100, edge); n != 0 {
		t.Fatalf("off the platform: %d not taken (%s)", n, why)
	}
	if r.p.Y != 0 {
		t.Fatalf("off the platform: landed at %.2f", r.p.Y)
	}

	// up the slope: it ran on 90 ms past its last report there (0.79 across,
	// 0.29 up), then jumped
	slope := float32(math.Tan(20 * math.Pi / 180))
	start := float32(410)
	sy := (start - 400) * slope
	r.w.UpdatePosition(r.p.ServerID, start, sy, 150, 0)
	r.report(start, sy, 150, 300)
	run := float32(RunSpeed) * 0.09
	x0, y0 := start+run, sy+run*slope
	for i, a := range jumpArc(float64(RunSpeed), 88, func(dx float32) float32 { return dx * slope }) {
		ms := a.ms
		if i == 0 {
			ms += 90
		}
		if !r.report(x0+a.dx, y0+a.dy, 150, ms) {
			t.Fatalf("up the slope at %.2f over its last report: not taken (%s)", y0+a.dy-sy, r.p.mv.why)
		}
	}
	if r.p.mv.air {
		t.Fatal("up the slope: not landed")
	}

	// up a step between its last report and the jump: its last report stood at
	// 0 by the step, it ran on onto it (0.3 up) and jumped there, and its first
	// report in the air came four frames after (0.73 over the step): the floor
	// it left is the step, which the floor under that report says
	r.w.UpdatePosition(r.p.ServerID, 319.5, 0, 5, 0)
	r.report(319.5, 0, 5, 300)
	for i, a := range jumpArc(float64(RunSpeed), 0, func(dx float32) float32 { return 0 }) {
		ms := a.ms
		if i == 0 {
			ms += 80 // the run onto the step
		}
		if !r.report(320.2+a.dx, 0.3+a.dy, 5, ms) {
			t.Fatalf("a jump off the step at %.2f over it: not taken (%s)", a.dy, r.p.mv.why)
		}
	}
	if len(r.p.mv.fixes) != 0 {
		t.Errorf("moved back %d times", len(r.p.mv.fixes))
	}

	// the controls: no jump from this client, or none on this server
	for _, c := range []struct {
		name          string
		client, world bool
	}{{"a client that does not jump", false, true}, {"a server without jumps", true, false}} {
		r := newJumpRig(t, MoveCheckEnforce, 0, 0, 5, c.client)
		r.w.SetJumps(c.world)
		n, why := r.jump(0, 100, level)
		if n == 0 {
			t.Fatalf("the control, %s: the jump was taken", c.name)
		}
		if !strings.Contains(why, "rose") {
			t.Fatalf("the control, %s: refused for %q", c.name, why)
		}
	}
}

// What is not a jump stays a fault for a client that jumps: a rise from the
// air (no floor under its start), a rise past the jump's top, a second rise once
// the jump came down (a double jump), a hover at the top, and a jump while held
// in place (no speed). Each is moved back in time, as the rest are.
func TestMoveCheckRefusesFakeJumps(t *testing.T) {
	// off the platform, falling, then up: no floor under the start
	r := newJumpRig(t, MoveCheckEnforce, 209, 10, 5, true)
	r.report(211, 9.9, 5, 100)
	r.report(212, 9.6, 5, 100)
	if r.report(213, 10.2, 5, 100) {
		t.Fatal("a rise in the air with no floor under its start was taken")
	}
	if !strings.Contains(r.p.mv.why, "rose 0.60 in the air") {
		t.Fatalf("refused for %q", r.p.mv.why)
	}

	// past the jump's top (0.5 still stands: a floor that near under the
	// feet holds them)
	r = newJumpRig(t, MoveCheckEnforce, 0, 0, 5, true)
	for _, y := range []float32{0.5, 0.95} {
		if !r.report(0, y, 5, 100) {
			t.Fatalf("a jump's %.2f not taken (%s)", y, r.p.mv.why)
		}
	}
	if r.report(0, 1.4, 5, 100) || !strings.Contains(r.p.mv.why, "jumped 1.40 over the floor it left") {
		t.Fatalf("past the top: taken, or refused for %q", r.p.mv.why)
	}

	// a double jump: the arc up and come down from its top, then a second
	// jump's first steps from there
	r = newJumpRig(t, MoveCheckEnforce, 0, 0, 5, true)
	arc := jumpArc(0, 100, level)
	again := -1
	for i, a := range arc {
		if !r.report(0, a.dy, 5, a.ms) {
			t.Fatalf("the first jump's report at %.2f not taken (%s)", a.dy, r.p.mv.why)
		}
		if i > 0 && a.dy < arc[i-1].dy-mcJumpDip {
			again = i
			break
		}
	}
	if again < 0 || arc[again].dy <= mcStandTol {
		t.Fatalf("the arc never came down in the air: %+v", arc)
	}
	from := arc[again].dy
	refused := false
	for _, b := range jumpArc(0, 100, level)[:2] {
		if !r.report(0, from+b.dy, 5, b.ms) {
			refused = true
			break
		}
	}
	if !refused || !strings.Contains(r.p.mv.why, "again in the air") {
		t.Fatalf("a double jump from %.2f: refused %v (%s)", from, refused, r.p.mv.why)
	}

	// a hover at the jump's top
	r = newJumpRig(t, MoveCheckEnforce, 0, 0, 5, true)
	for _, a := range arc[:3] {
		r.report(0, a.dy, 5, a.ms)
	}
	top := r.p.Y
	refusedN, backAt := 0, float32(-1)
	for i := 0; i < 20 && backAt < 0; i++ {
		if !r.report(0, top, 5, 100) {
			refusedN++
		}
		if len(r.p.mv.fixes) > 0 {
			backAt = r.p.Y // where the move back put it (the hover's next report is its arrival)
		}
	}
	if refusedN == 0 || backAt < 0 {
		t.Fatalf("two seconds at the jump's top: %d refused, %d moves back (%s)", refusedN, len(r.p.mv.fixes), r.p.mv.why)
	}
	if backAt != 0 || !strings.Contains(r.p.mv.why, "held up in the air") {
		t.Errorf("put back at %.2f, not on the ground (%s)", backAt, r.p.mv.why)
	}

	// held in place: a GM's freeze
	r = newJumpRig(t, MoveCheckEnforce, 0, 0, 5, true)
	r.p.Frozen = true
	if n, why := r.jump(0, 100, level); n == 0 || !strings.Contains(why, "rose") {
		t.Fatalf("a jump held in place: %d refused (%s)", n, why)
	}
}

// In log mode a jump logs nothing (it is no fault), where the same jump from a
// client that does not jump is logged and still taken.
func TestMoveCheckLogModeJump(t *testing.T) {
	r := newJumpRig(t, MoveCheckLog, 0, 0, 5, true)
	r.jump(0, 100, level)
	if r.p.mv.why != "" {
		t.Fatalf("an honest jump was faulted: %s", r.p.mv.why)
	}
	r = newJumpRig(t, MoveCheckLog, 0, 0, 5, false)
	if n, _ := r.jump(0, 100, level); n != 0 {
		t.Fatalf("log mode refused %d", n)
	}
	if !strings.Contains(r.p.mv.why, "rose") {
		t.Fatalf("the control: no fault logged (%q)", r.p.mv.why)
	}
}

// The client's answer is its player's on a server with jumps, not on one
// without; a restart keeps it, but not into a server without them; a takeover
// clears it (the new client answers for itself).
func TestJumpClientAnswerKeepAndTakeover(t *testing.T) {
	d := newDuelists(t)
	if d.w.SetJumpClient(d.sa, true) || d.a.Jumps {
		t.Fatal("taken on a server without jumps")
	}
	d.w.SetJumps(true)
	if !d.w.SetJumpClient(d.sa, true) || !d.a.Jumps {
		t.Fatal("not taken on a server with jumps")
	}
	if on, ok := d.w.JumpClient(d.sa); !on || !ok {
		t.Fatalf("JumpClient %v %v", on, ok)
	}
	now := time.Now()
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.Jumps {
		t.Fatalf("kept %+v", k)
	}
	if k2, _ := d.w.KeepPlayer(d.sb, now); k2.Jumps {
		t.Fatal("kept for a client that never said")
	}
	d.a.Jumps = false
	d.w.RestoreKeptPlayer(d.sa, k, now)
	if !d.a.Jumps {
		t.Fatal("not restored")
	}
	d.w.SetJumps(false)
	d.a.Jumps = false
	d.w.RestoreKeptPlayer(d.sa, k, now)
	if d.a.Jumps {
		t.Fatal("restored on a server since without jumps")
	}
	d.w.SetJumps(true)
	d.w.SetJumpClient(d.sa, true)
	if !d.w.SetJumpClient(d.sa, false) || d.a.Jumps {
		t.Fatal("its 0 not taken")
	}

	// a takeover: the same Player, a new client
	d.w.SetJumpClient(d.sa, true)
	d.a.CharServerID = 77
	s2 := d.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 21), Port: 7021}, 21, 21, 1, 2, true)
	s2.SetCharacterInWorld(true)
	s2.SetPlayerObjectID(0x1424A1E1)
	p, _ := d.w.TakeOverPlayer(s2, 77)
	if p != d.a {
		t.Fatal("not taken over")
	}
	if p.Jumps {
		t.Fatal("the new client inherited the old one's jumps")
	}
}
