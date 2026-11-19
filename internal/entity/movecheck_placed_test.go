package entity

import (
	"os"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// mcRockGround is the made world with a rock on it (the owner's report of
// 2026-11-18, a player in the rock by Noroxus): a block over x 300-306 and
// z 0-6, its top at 3. A body is in it when its ball (r round the middle,
// mid over the feet) meets the block; the free place is on its top from
// its middle, else beside it, west of it.
type mcRockGround struct{ mcGround }

func (mcRockGround) Embedded(world uint8, x, y, z, mid, r float32) bool {
	m := y + mid
	return x+r > 300 && x-r < 306 && z+r > 0 && z-r < 6 && m+r > 0 && m-r < 3
}

func (g mcRockGround) Free(world uint8, x, y, z, mid, r float32) (fx, fy, fz float32, top, ok bool) {
	if x > 302 && x < 304 && z > 2 && z < 4 {
		return x, 3, z, true, true
	}
	return 300 - r - 0.25, 0, z, false, true
}

func newRockRig(t *testing.T, mode MoveCheckMode, x, y, z float32) *mcRig {
	t.Helper()
	was := MoveCheck()
	SetMoveCheck(mode)
	t.Cleanup(func() { SetMoveCheck(was) })
	w := NewWorld()
	w.SetGround(mcRockGround{})
	w.SetEnv(mcEnv{})
	p := w.CreatePlayer(nil, &database.CharacterRecord{CharName: "Loneowls", Level: 60, X: x, Y: y, Z: z})
	if p == nil {
		t.Fatal("no player")
	}
	w.UpdatePosition(p.ServerID, x, y, z, 0)
	r := &mcRig{t: t, w: w, p: p, now: time.Date(2026, 11, 18, 2, 50, 0, 0, time.UTC)}
	r.report(x, y, z, 0) // the arrival
	return r
}

// A login (or a teleport) into the rock: put out at once, beside it, in
// either mode; the client's report from there is taken.
func TestMoveCheckPutsOutOfTheRockAtArrival(t *testing.T) {
	for _, mode := range []MoveCheckMode{MoveCheckLog, MoveCheckEnforce} {
		r := newRockRig(t, mode, 300.5, 0, 3)
		if r.p.X != 299 || r.p.Y != 0 || !r.p.mv.pending || len(r.p.mv.fixes) != 1 {
			t.Fatalf("%s: not put out at the arrival: at (%.2f, %.2f, %.2f), pending %v, fixes %d", mode, r.p.X, r.p.Y, r.p.Z, r.p.mv.pending, len(r.p.mv.fixes))
		}
		if !r.report(299, 0, 3, 300) {
			t.Fatalf("%s: the report from where it was put not taken (%s)", mode, r.p.mv.why)
		}
		if n := r.walk(280, 3, RunSpeed, flat); n != 0 {
			t.Errorf("%s: walking off: %d not taken (%s)", mode, n, r.p.mv.why)
		}
	}
	// from its middle: on its top
	r := newRockRig(t, MoveCheckEnforce, 303, 0, 3)
	if r.p.X != 303 || r.p.Y != 3 || r.p.Z != 3 {
		t.Errorf("from the rock's middle: put at (%.2f, %.2f, %.2f), want its top (303, 3, 3)", r.p.X, r.p.Y, r.p.Z)
	}
}

// A walk into the rock: enforce refuses it and moves the client back to the
// last place outside; log mode takes it, and a second on puts it out.
func TestMoveCheckWalkIntoTheRock(t *testing.T) {
	r := newRockRig(t, MoveCheckEnforce, 290, 0, 3)
	r.walk(305, 3, RunSpeed, flat)
	if r.p.X > 299.6 || len(r.p.mv.fixes) == 0 {
		t.Fatalf("enforce: into the rock: the server has it at x %.2f, fixes %d", r.p.X, len(r.p.mv.fixes))
	}
	if r.w.inPlacedLocked(0, r.p.mv.pendX, r.p.mv.pendY, r.p.mv.pendZ) {
		t.Error("enforce: moved back into the rock")
	}

	r = newRockRig(t, MoveCheckLog, 290, 0, 3)
	r.walk(299, 3, RunSpeed, flat)
	if !r.report(301, 0, 3, 100) || r.p.mv.pending { // in by 1.45
		t.Fatalf("log: the step in not taken: x %.2f, pending %v", r.p.X, r.p.mv.pending)
	}
	for i := 0; i < 9; i++ {
		r.report(301, 0, 3, 100)
	}
	if r.p.mv.pending {
		t.Fatal("log: put out within the second")
	}
	r.report(301, 0, 3, 100)
	if !r.p.mv.pending || r.p.X != 299 {
		t.Fatalf("log: in the rock a second and more, not put out: at x %.2f, pending %v", r.p.X, r.p.mv.pending)
	}
	if !r.report(299, 0, 3, 300) {
		t.Fatalf("log: the report from where it was put not taken (%s)", r.p.mv.why)
	}
}

// The controls: a walk past the rock, its ball clear of it, and one over
// open ground, every report taken, nobody put out.
func TestMoveCheckWalkPastTheRock(t *testing.T) {
	for _, mode := range []MoveCheckMode{MoveCheckLog, MoveCheckEnforce} {
		r := newRockRig(t, mode, 290, 0, -1)
		if n := r.walk(320, -1, RunSpeed, flat); n != 0 {
			t.Errorf("%s: past the rock: %d not taken (%s)", mode, n, r.p.mv.why)
		}
		if n := r.walk(320, 40, RunSpeed, flat); n != 0 {
			t.Errorf("%s: over open ground: %d not taken (%s)", mode, n, r.p.mv.why)
		}
		if len(r.p.mv.fixes) != 0 || r.p.mv.pending {
			t.Errorf("%s: moved %d times", mode, len(r.p.mv.fixes))
		}
	}
}

// A player's place on the real ground (EQOA_GROUND_DIR): Secrets, under the
// edge of the rock east of Noroxus (4238.5, -21.2, 2164), as a login there
// and as a walk in from the open ground south of it; and a walk past it
// along the open ground, judged right. Skipped without a baked ground.
func TestRealMoveCheckLonewulfInTheRock(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	rig := func(mode MoveCheckMode, x, z float32) (*mcRig, func(x, z float32) float32) {
		was := MoveCheck()
		SetMoveCheck(mode)
		t.Cleanup(func() { SetMoveCheck(was) })
		w := NewWorld()
		w.SetGround(g)
		w.SetEnv(env)
		floor := func(x, z float32) float32 {
			f, ok := g.Floor(5, x, 0, z)
			if !ok {
				t.Fatalf("no floor at (%.1f, %.1f)", x, z)
			}
			return f
		}
		y := floor(x, z)
		p := w.CreatePlayer(nil, &database.CharacterRecord{CharName: "Loneowls", Level: 60, World: 5, X: x, Y: y, Z: z})
		w.UpdatePosition(p.ServerID, x, y, z, 0)
		p.World = 5
		r := &mcRig{t: t, w: w, p: p, now: time.Date(2026, 11, 18, 2, 50, 0, 0, time.UTC)}
		r.report(x, y, z, 0)
		return r, floor
	}

	// a login (or the portal's summons) there: out at once
	r, floor := rig(MoveCheckLog, 4238.5, 2164)
	if !r.p.mv.pending || len(r.p.mv.fixes) != 1 {
		t.Fatalf("login in the rock: not put out (at %.2f, %.2f, %.2f)", r.p.X, r.p.Y, r.p.Z)
	}
	if g.Embedded(5, r.p.X, r.p.Y, r.p.Z, mcMiddle, mcClear) {
		t.Fatalf("put out into the rock: (%.2f, %.2f, %.2f)", r.p.X, r.p.Y, r.p.Z)
	}
	t.Logf("login: put out to (%.2f, %.2f, %.2f)", r.p.X, r.p.Y, r.p.Z)

	// a walk from the open ground south of it (smaller z is north) in under
	// its edge, in log mode, the shards' mode: out within a second or so
	r, floor = rig(MoveCheckLog, 4238.5, 2156)
	r.walk(4238.5, 2164, RunSpeed, floor)
	for i := 0; i < 12 && !r.p.mv.pending; i++ {
		r.report(4238.5, floor(4238.5, 2164), 2164, 100)
	}
	if !r.p.mv.pending || g.Embedded(5, r.p.X, r.p.Y, r.p.Z, mcMiddle, mcBall) {
		t.Fatalf("log: standing in the rock, not put out (at %.2f, %.2f, %.2f)", r.p.X, r.p.Y, r.p.Z)
	}
	t.Logf("walk in (log): put out to (%.2f, %.2f, %.2f)", r.p.X, r.p.Y, r.p.Z)

	// and in enforce mode the walk in is refused, the client sent back out
	r, floor = rig(MoveCheckEnforce, 4238.5, 2156)
	r.walk(4238.5, 2164, RunSpeed, floor)
	for i := 0; i < 3; i++ {
		r.report(4238.5, floor(4238.5, 2164), 2164, 100)
	}
	if g.Embedded(5, r.p.X, r.p.Y, r.p.Z, mcMiddle, mcBall) || len(r.p.mv.fixes) == 0 {
		t.Fatalf("enforce: the walk into the rock taken (at %.2f, %.2f, %.2f, fixes %d)", r.p.X, r.p.Y, r.p.Z, len(r.p.mv.fixes))
	}

	// the controls: a walk past the rock along the open ground north of it
	// (z 2156, west to east under Noroxus's rock), and round its west end,
	// every report taken
	for _, mode := range []MoveCheckMode{MoveCheckLog, MoveCheckEnforce} {
		r, floor = rig(mode, 4215, 2156)
		if n := r.walk(4260, 2156, RunSpeed, floor); n != 0 {
			t.Errorf("%s: past the rock: %d not taken (%s)", mode, n, r.p.mv.why)
		}
		if n := r.walk(4260, 2140, RunSpeed, floor); n != 0 {
			t.Errorf("%s: on over open ground: %d not taken (%s)", mode, n, r.p.mv.why)
		}
		if len(r.p.mv.fixes) != 0 || r.p.mv.pending {
			t.Errorf("%s: moved %d times (%s)", mode, len(r.p.mv.fixes), r.p.mv.why)
		}
	}
}
