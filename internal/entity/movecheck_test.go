package entity

import (
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A made world for the judge: flat ground at 0 everywhere, a platform with
// its top at 10 over x 200-210 and z 0-10, a wall across x = 100 that faces
// the west (so a way from the west crosses its front), and from z 500 on, a
// sea: its floor at -20, its surface at 5.
type mcGround struct{}

func (mcGround) floors(x, z float32) []float32 {
	if z > 500 {
		return []float32{-20}
	}
	fs := []float32{0}
	if x >= 200 && x <= 210 && z >= 0 && z <= 10 {
		fs = append(fs, 10)
	}
	return fs
}

func (g mcGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	return g.Support(world, x, y, z, 2, 1000)
}

func (g mcGround) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	best, found := float32(0), false
	for _, f := range g.floors(x, z) {
		if f <= y+up && f >= y-down && (!found || f > best) {
			best, found = f, true
		}
	}
	return best, found
}

func (mcGround) Blocked(world uint8, x0, y0, z0, x1, y1, z1 float32) (string, bool) {
	switch {
	case x0 < 100 && x1 >= 100:
		return "placed", true
	case z1 <= 500 && y0 >= 0 && y1 < 0:
		return "terrain floor", true // the middle down through the ground
	}
	return "", false
}

func (mcGround) Knows(world uint8) bool { return world == 0 }

type mcEnv struct{}

func (mcEnv) Flags(world uint8, x, y, z float32) (uint32, bool) {
	if z > 500 && y < 5 {
		return 1, true
	}
	return 0, true
}

func (mcEnv) Knows(world uint8) bool { return world == 0 }

// mcRig is a player in the made world and a clock.
type mcRig struct {
	t   *testing.T
	w   *World
	p   *Player
	now time.Time
}

func newMCRig(t *testing.T, mode MoveCheckMode, x, y, z float32) *mcRig {
	t.Helper()
	was := MoveCheck()
	SetMoveCheck(mode)
	t.Cleanup(func() { SetMoveCheck(was) })
	w := NewWorld()
	w.SetGround(mcGround{})
	w.SetEnv(mcEnv{})
	p := w.CreatePlayer(nil, &database.CharacterRecord{CharName: "Mover", Level: 10, X: x, Y: y, Z: z})
	if p == nil {
		t.Fatal("no player")
	}
	w.UpdatePosition(p.ServerID, x, y, z, 0) // (a record at 0, 0 logs in at the default spawn)
	r := &mcRig{t: t, w: w, p: p, now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	r.report(x, y, z, 0) // the arrival
	return r
}

// report is a 0x40 from (x, y, z) after ms milliseconds; whether the server
// took its place.
func (r *mcRig) report(x, y, z float32, ms int) bool {
	r.now = r.now.Add(time.Duration(ms) * time.Millisecond)
	r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: x, Y: y, Z: z, World: r.p.World, HasWorld: true, At: r.now})
	return r.p.X == x && r.p.Y == y && r.p.Z == z
}

// walk reports a walk from where the player is to (x, y, z) at speed, a
// report every 100 ms, the height following floor(x, z); it returns how
// many reports were not taken.
func (r *mcRig) walk(x, z, speed float32, floor func(x, z float32) float32) int {
	fromX, fromZ := r.p.X, r.p.Z
	d := float32(math.Hypot(float64(x-fromX), float64(z-fromZ)))
	n := int(math.Ceil(float64(d / (speed * 0.1))))
	missed := 0
	for i := 1; i <= n; i++ {
		f := float32(i) / float32(n)
		px, pz := fromX+(x-fromX)*f, fromZ+(z-fromZ)*f
		if !r.report(px, floor(px, pz), pz, 100) {
			missed++
		}
	}
	return missed
}

func flat(x, z float32) float32 { return 0 }

func TestMoveCheckTakesAnHonestWalk(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	if n := r.walk(80, 40, RunSpeed, flat); n != 0 {
		t.Fatalf("an honest run at the run speed: %d reports not taken (%s)", n, r.p.mv.why)
	}
	// stand a minute (no reports), then run on: the first report comes after
	// a long silence
	r.now = r.now.Add(time.Minute)
	if n := r.walk(0, 0, RunSpeed, flat); n != 0 {
		t.Fatalf("running on after standing: %d not taken (%s)", n, r.p.mv.why)
	}
	if len(r.p.mv.fixes) != 0 {
		t.Errorf("moved back %d times", len(r.p.mv.fixes))
	}
}

func TestMoveCheckSpeed(t *testing.T) {
	// the control: at the speed a buff gives, nothing is refused
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	r.p.Effects = append(r.p.Effects, &ActiveEffect{Speed: true, Until: r.now.Add(time.Hour)})
	if n := r.walk(90, 0, r.p.SpeedNow(), flat); n != 0 {
		t.Fatalf("buffed (%.2f): %d not taken (%s)", r.p.SpeedNow(), n, r.p.mv.why)
	}

	// twice the speed it was given: refused within a few seconds, and
	// moved back
	r = newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	missed := r.walk(-90, 0, 2*RunSpeed, flat)
	if missed == 0 {
		t.Fatal("twice the run speed for five seconds: every report taken")
	}
	if len(r.p.mv.fixes) == 0 {
		t.Error("not moved back")
	}
	if r.p.X < -60 {
		t.Errorf("the server has it at x %.1f: it went faster than it may", r.p.X)
	}
}

func TestMoveCheckTeleportIsMovedBackAtOnce(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	r.walk(20, 0, RunSpeed, flat)
	if r.report(2000, 0, 2000, 100) {
		t.Fatal("a 2,800-unit jump in a tenth of a second was taken")
	}
	if !r.p.mv.pending || len(r.p.mv.fixes) != 1 {
		t.Fatalf("not moved back at once: pending %v, fixes %d", r.p.mv.pending, len(r.p.mv.fixes))
	}
	// the client, still where it jumped to, is not believed until it
	// reports from where it was put
	if r.report(2001, 0, 2000, 100) {
		t.Error("a report from the jump's end taken while the move back is pending")
	}
	if !r.report(r.p.mv.pendX, 0, r.p.mv.pendZ, 300) {
		t.Fatalf("the report from where it was put back not taken (%s)", r.p.mv.why)
	}
	if n := r.walk(0, 0, RunSpeed, flat); n != 0 {
		t.Errorf("walking on after the move back: %d not taken (%s)", n, r.p.mv.why)
	}
}

func TestMoveCheckHover(t *testing.T) {
	// up off the ground with nothing under it
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	if r.report(0.5, 5, 0, 100) {
		t.Fatal("rising 5 into the air was taken")
	}

	// held at a height with no floor under it (a fall that does not come
	// down), reached by walking off the platform
	r = newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	refused := 0
	for i := 1; i <= 30; i++ {
		if !r.report(205+float32(i)*0.6, 10, 5, 100) { // off the platform at x 210, and on at 10
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("three seconds in the air at 10 over the ground: every report taken")
	}
	if len(r.p.mv.fixes) == 0 {
		t.Fatal("not moved back")
	}
	// moved back onto the floor under its last place in the air, where
	// the fall would have put it, or the platform
	if r.p.Y != 0 && r.p.Y != 10 {
		t.Errorf("put back at y %.2f, not on a floor", r.p.Y)
	}
}

func TestMoveCheckTakesAnHonestFall(t *testing.T) {
	// off the platform's edge and down to the ground at the game's gravity,
	// a report every 100 ms, as CLIENT's walk falls (Gravity -19.6, the
	// air's damping 0.8 a second)
	r := newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	r.walk(209.5, 5, RunSpeed, func(x, z float32) float32 { return 10 })
	x, y, vy := r.p.X, float32(10), float32(0)
	for y > 0 {
		for step := 0; step < 3; step++ { // 32 ms ticks
			dt := float32(0.0333)
			vy = (vy - 19.6*dt) * float32(math.Pow(0.8, float64(dt)))
			y += vy * dt
		}
		x += 0.5
		if y < 0 {
			y = 0
		}
		if !r.report(x, y, 5, 100) {
			t.Fatalf("an honest fall at y %.2f not taken (%s)", y, r.p.mv.why)
		}
	}
	if r.p.mv.air {
		t.Error("landed, but still in the air to the judge")
	}
}

func TestMoveCheckUnderTheGround(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	if r.report(0.5, -6, 0, 100) {
		t.Fatal("down through the ground to y -6 was taken")
	}
	if len(r.p.mv.fixes) != 1 {
		t.Errorf("through the ground is moved back at once: %d", len(r.p.mv.fixes))
	}
}

func TestMoveCheckThroughAWall(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 90, 0, 0)
	if n := r.walk(99, 0, RunSpeed, flat); n != 0 {
		t.Fatalf("up to the wall: %d not taken", n)
	}
	if r.report(100.5, 0, 0, 100) {
		t.Fatal("through the wall was taken")
	}
	if len(r.p.mv.fixes) != 1 {
		t.Errorf("through a wall is moved back at once: %d", len(r.p.mv.fixes))
	}
	// the control: along the wall's face is no crossing
	r = newMCRig(t, MoveCheckEnforce, 99, 0, 0)
	if n := r.walk(99, 50, RunSpeed, flat); n != 0 {
		t.Errorf("along the wall: %d not taken (%s)", n, r.p.mv.why)
	}
}

func TestMoveCheckClimb(t *testing.T) {
	// onto the platform's top (10 up) in one step
	r := newMCRig(t, MoveCheckEnforce, 199, 0, 5)
	if r.report(200.5, 10, 5, 100) {
		t.Fatal("10 up in 1.5 across was taken")
	}
	// the control: a 45-degree ramp climbs
	r = newMCRig(t, MoveCheckEnforce, 0, 0, 100)
	ramp := func(x, z float32) float32 { return max(0, x) }
	r.w.SetGround(rampGround{})
	if n := r.walk(8, 100, RunSpeed, ramp); n != 0 {
		t.Errorf("a 45-degree ramp: %d not taken (%s)", n, r.p.mv.why)
	}
}

// rampGround is mcGround with the ground rising at 45 degrees east of 0.
type rampGround struct{ mcGround }

func (rampGround) Support(world uint8, x, y, z, up, down float32) (float32, bool) {
	f := max(0, x)
	return f, f <= y+up && f >= y-down
}

func TestMoveCheckSwimming(t *testing.T) {
	// in the sea: down from the surface, along, and up again at the swim's
	// speeds, with no floor under it: all taken
	r := newMCRig(t, MoveCheckEnforce, 0, 3, 600)
	y := float32(3)
	for i := 1; i <= 30; i++ {
		y -= 0.5 // 5 a second down
		if !r.report(float32(i)*0.5, y, 600, 100) {
			t.Fatalf("swimming down at y %.1f not taken (%s)", y, r.p.mv.why)
		}
	}
	for i := 1; i <= 30; i++ {
		y += 0.45
		if !r.report(15-float32(i)*0.3, y, 600, 100) {
			t.Fatalf("swimming up at y %.1f not taken (%s)", y, r.p.mv.why)
		}
	}
	// out of the water and into the air: no
	if r.report(6, 12, 600, 100) {
		t.Error("up out of the sea into the air was taken")
	}
}

func TestMoveCheckServerMoveDropsStaleReports(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	r.walk(10, 0, RunSpeed, flat)
	r.w.UpdatePosition(r.p.ServerID, 5000, 0, 5000, 0) // a coach
	// a report the client sent before it heard of it
	if r.report(11, 0, 0, 50) {
		t.Fatal("a report from before the teleport moved the player back")
	}
	if r.p.X != 5000 {
		t.Fatalf("the server lost the teleport: x %.1f", r.p.X)
	}
	if len(r.p.mv.fixes) != 0 {
		t.Errorf("a stale report counted as a fault: %d moves back", len(r.p.mv.fixes))
	}
	if !r.report(5000, 0, 5000, 500) {
		t.Fatalf("the arrival not taken (%s)", r.p.mv.why)
	}
	if n := r.walk(5010, 5000, RunSpeed, flat); n != 0 {
		t.Errorf("walking from the arrival: %d not taken (%s)", n, r.p.mv.why)
	}
}

func TestMoveCheckLogModeTakesEverything(t *testing.T) {
	r := newMCRig(t, MoveCheckLog, 0, 0, 0)
	if !r.report(2000, 50, 2000, 100) {
		t.Fatal("log mode refused a report")
	}
	if !r.report(2000, 55, 2000, 100) {
		t.Fatal("log mode refused a report")
	}
	if len(r.p.mv.fixes) != 0 || r.p.mv.pending {
		t.Error("log mode moved the client")
	}
}

func TestMoveCheckOffJudgesNothing(t *testing.T) {
	r := newMCRig(t, MoveCheckOff, 0, 0, 0)
	if !r.report(2000, 50, 2000, 100) || r.p.mv.init {
		t.Error("off judged a report")
	}
}

func TestMoveCheckSilentInTheAir(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	r.report(211, 9.9, 5, 100) // off the edge, falling
	// then nothing: a client that stops in the air
	for i := 0; i < 50; i++ {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.mu.Lock()
		r.w.TickCount++
		r.w.silentAirSystem(r.now)
		r.w.mu.Unlock()
	}
	if r.p.Y != 0 {
		t.Errorf("silent in the air for five seconds: still at y %.2f", r.p.Y)
	}
	// the control: one that stands on the platform, silent, stays
	r = newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	for i := 0; i < 50; i++ {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.mu.Lock()
		r.w.TickCount++
		r.w.silentAirSystem(r.now)
		r.w.mu.Unlock()
	}
	if r.p.Y != 10 {
		t.Errorf("standing on the platform, silent: moved to y %.2f", r.p.Y)
	}
}

func TestMoveCheckBreathIsTheServers(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 0, -10, 600) // 15 under the sea's surface
	r.p.WalkFlags, r.p.HasWalkFlags = 0x0008, true  // the client: "at the surface"
	under, known := r.w.underLiquidLocked(r.p)
	if !known || !under {
		t.Fatalf("15 under the surface: under %v, known %v", under, known)
	}
	r.w.mu.Lock()
	r.w.TickCount = 6
	r.w.breathSystem(r.now)
	r.w.mu.Unlock()
	if !r.p.Underwater {
		t.Error("enforced: the client's word that it is at the surface was taken")
	}
	// the control: at the surface (the middle at it) it breathes
	r.p.Y = 5 - mcMiddle - 0.05
	if under, _ := r.w.underLiquidLocked(r.p); under {
		t.Error("at the surface: under")
	}
}

func TestMoveCheckAnimationPassedOnOnlyWhenTheWalksOwn(t *testing.T) {
	r := newMCRig(t, MoveCheckLog, 0, 0, 0)
	r.now = r.now.Add(100 * time.Millisecond)
	r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: 0.5, Y: 0, Z: 0, Animation: 3, At: r.now})
	if r.p.Animation != 3 {
		t.Errorf("a run's state: %d", r.p.Animation)
	}
	r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: 1, Y: 0, Z: 0, Animation: 0xEE, At: r.now.Add(100 * time.Millisecond)})
	if r.p.Animation != 0 {
		t.Errorf("0xEE passed on: %d", r.p.Animation)
	}
}

func TestFallMinIsUnderTheGamesFall(t *testing.T) {
	// the game's own fall, stepped as its walk steps it, is never shorter
	// than what the judge asks for
	y, vy := 0.0, 0.0
	for tick := 1; tick <= 300; tick++ {
		dt := 0.032
		vy = (vy - 19.6*dt) * math.Pow(0.8, dt)
		y += vy * dt
		if need := float64(fallMin(float64(tick) * dt)); need > -y+0.01 {
			t.Fatalf("at %.2f s the game has fallen %.2f and the judge asks %.2f", float64(tick)*dt, -y, need)
		}
	}
}

// A fall whose reports a stalled link brings in a bunch, two seconds late,
// is still a fall: their number counts its time, not their arrival.
func TestMoveCheckFallThroughAStall(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	r.walk(209.5, 5, RunSpeed, func(x, z float32) float32 { return 10 })
	x, y, vy := r.p.X, float32(10), float32(0)
	stalled := false
	for y > 0 {
		for step := 0; step < 3; step++ {
			dt := float32(0.0339)
			vy = (vy - 19.6*dt) * float32(math.Pow(0.8, float64(dt)))
			y += vy * dt
		}
		x += 0.5
		if y < 0 {
			y = 0
		}
		ms := 100
		if x > 211 && !stalled {
			ms, stalled = 2000, true // two seconds of nothing, then the rest at once
		} else if stalled {
			ms = 5
		}
		if !r.report(x, y, 5, ms) {
			t.Fatalf("a fall through a stall at y %.2f not taken (%s)", y, r.p.mv.why)
		}
	}
}

// One that hangs in the air and reports seldom, so its reports never count
// a fall's time, is caught by their arrival.
func TestMoveCheckSparseHover(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	refused := 0
	for i := 1; i <= 6; i++ {
		if !r.report(210.5+float32(i)*0.4, 10, 5, 1500) {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("nine seconds at 10 over the ground, a report every 1.5 s: every report taken")
	}
}
