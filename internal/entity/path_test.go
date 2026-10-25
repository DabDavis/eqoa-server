package entity

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A GM's path (path.go): its points walked in order at the path's pace, in
// its mode, with its pauses; a fight takes the creature off it and it takes
// it up again at the nearest leg's end; taken away, the creature walks home.

// pathRun ticks the wander and the paths every 100 ms from start for d, as
// the tick does (roamSystem, then pathSystem), calling each after each tick.
func pathRun(w *World, start time.Time, d time.Duration, each func(now time.Time)) time.Time {
	now := start
	for end := start.Add(d); now.Before(end); now = now.Add(100 * time.Millisecond) {
		w.TickCount++
		w.roamSystem(now)
		w.pathSystem(now)
		if each != nil {
			each(now)
		}
	}
	return now
}

// square is a 40-unit square of points round (1000, 1000).
func square(mode PathMode) *NPCPath {
	return &NPCPath{Mode: mode, Points: []PathPoint{
		{X: 980, Z: 980, Pause: -1}, {X: 1020, Z: 980, Pause: -1}, {X: 1020, Z: 1020, Pause: -1}, {X: 980, Z: 1020, Pause: -1},
	}}
}

// visits records the points a creature reaches, in order (a point again
// only after another).
func visits(p *NPCPath, npc *NPC, seen *[]int) func(time.Time) {
	return func(time.Time) {
		for i, pt := range p.Points {
			if distXZ(npc.X, npc.Z, pt.X, pt.Z) < 1e-3 && (len(*seen) == 0 || (*seen)[len(*seen)-1] != i) {
				*seen = append(*seen, i)
			}
		}
	}
}

// onPath is how far (x, z) is from the nearest of the path's legs.
func onPath(p *NPCPath, x, z float32) float32 {
	best := float32(math.MaxFloat32)
	n := len(p.Points)
	for i := 0; i < n; i++ {
		best = min(best, distToLeg(x, z, p.Points[i], p.Points[(i+1)%n]))
	}
	return best
}

func TestALoopIsWalkedInOrderAtAWalkAndTheCreatureNeverWanders(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathLoop)
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	var seen []int
	px, pz := npc.X, npc.Z
	each := visits(p, npc, &seen)
	pathRun(w, time.Now(), 2*time.Minute, func(now time.Time) {
		each(now)
		if d := onPath(p, npc.X, npc.Z); d > 0.01 {
			t.Fatalf("%.2f off its path at (%.1f, %.1f)", d, npc.X, npc.Z)
		}
		if step := distXZ(npc.X, npc.Z, px, pz); step > NPCWalkSpeed*0.1+1e-3 {
			t.Fatalf("moved %.2f in a tick: faster than a walk", step)
		}
		if npc.moving() {
			if npc.Loco != LocoWalk {
				t.Fatalf("walking with animation %d", npc.Loco)
			}
			// facing along the leg: its heading is its velocity's
			if h := headingTo(npc.VelX, npc.VelZ); math.Abs(float64(h-npc.Facing)) > 1e-3 {
				t.Fatalf("facing %.3f, walking toward %.3f", npc.Facing, h)
			}
		}
		px, pz = npc.X, npc.Z
	})
	// it starts on point 0; 2 minutes at 5 a second is 600 units: three and
	// more times round 160
	want := []int{0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3}
	if len(seen) < len(want) {
		t.Fatalf("visited %v, want at least %v", seen, want)
	}
	for i, v := range want {
		if seen[i] != v {
			t.Fatalf("visited %v, want %v first", seen, want)
		}
	}
}

// The control for the test above: the same creature with no path wanders
// off the square (were the path ignored, that test's on-path check fails).
func TestWithoutAPathTheSameCreatureWandersOffIt(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathLoop)
	off := float32(0)
	pathRun(w, time.Now(), 5*time.Minute, func(time.Time) { off = max(off, onPath(p, npc.X, npc.Z)) })
	if off < 1 {
		t.Fatalf("with no path it kept within %.2f of the square: the wander did not run", off)
	}
}

func TestBackAndForthTurnsAtTheEnds(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := &NPCPath{Mode: PathPingPong, Points: []PathPoint{{X: 980, Z: 980, Pause: -1}, {X: 1000, Z: 980, Pause: -1}, {X: 1020, Z: 980, Pause: -1}}}
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	var seen []int
	pathRun(w, time.Now(), time.Minute, visits(p, npc, &seen))
	want := []int{0, 1, 2, 1, 0, 1, 2, 1, 0}
	if len(seen) < len(want) || stringOf(seen[:len(want)]) != stringOf(want) {
		t.Fatalf("visited %v, want %v first", seen, want)
	}
}

func stringOf(v []int) string {
	var b strings.Builder
	b.WriteString("[")
	for i, x := range v {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(string(rune('0' + x)))
	}
	b.WriteString("]")
	return b.String()
}

func TestOnceWalksFromItsFirstPointAndHoldsAtItsLast(t *testing.T) {
	w := roamWorld(t)
	// it stands near the last point: once is walked from the first all the same
	npc := addNPC(w, 100, "beetle", 1019, 1019, creatureModel, 0)
	p := square(PathOnce)
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	var seen []int
	now := pathRun(w, time.Now(), time.Minute, visits(p, npc, &seen))
	if stringOf(seen) != "[0 1 2 3]" {
		t.Fatalf("visited %v, want [0 1 2 3]", seen)
	}
	last := p.Points[3]
	if distXZ(npc.X, npc.Z, last.X, last.Z) > 1e-3 || npc.moving() {
		t.Fatalf("at (%.1f, %.1f) moving %v: not standing at its last point", npc.X, npc.Z, npc.moving())
	}
	if s, _ := w.PathStatusOf(100); s.State != "holding" {
		t.Errorf("state %q, want holding", s.State)
	}
	// and it stays
	pathRun(w, now, 30*time.Second, func(time.Time) {
		if distXZ(npc.X, npc.Z, last.X, last.Z) > 1e-3 {
			t.Fatalf("left its last point")
		}
	})
}

func TestItStandsThePathsPauseAndAPointsOwn(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := &NPCPath{Mode: PathLoop, Pause: 2 * time.Second, Points: []PathPoint{
		{X: 980, Z: 980, Pause: -1}, {X: 1000, Z: 980, Pause: 5 * time.Second},
	}}
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	// how long it stood at each point it came to
	var at []struct {
		i     int
		stood time.Duration
	}
	cur, since := -1, time.Time{}
	pathRun(w, time.Now(), time.Minute, func(now time.Time) {
		i := -1
		for k, pt := range p.Points {
			if distXZ(npc.X, npc.Z, pt.X, pt.Z) < 1e-3 {
				i = k
			}
		}
		if i != cur {
			if cur >= 0 {
				at = append(at, struct {
					i     int
					stood time.Duration
				}{cur, now.Sub(since)})
			}
			cur, since = i, now
		}
	})
	if len(at) < 4 {
		t.Fatalf("stops %v: too few", at)
	}
	for _, a := range at[1:] { // the first: it set off at once from where it stood
		want := 2 * time.Second
		if a.i == 1 {
			want = 5 * time.Second
		}
		if d := a.stood - want; d < 0 || d > 200*time.Millisecond {
			t.Errorf("stood %v at point %d, want %v", a.stood, a.i, want)
		}
	}
}

func TestARunIsTheChasesPace(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathLoop)
	p.Run = true
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	ran := false
	pathRun(w, time.Now(), 10*time.Second, func(time.Time) {
		if npc.moving() {
			if v := math.Hypot(float64(npc.VelX), float64(npc.VelZ)); math.Abs(v-float64(NPCRunSpeed)) > 1e-3 || npc.Loco != LocoRun {
				t.Fatalf("at %.2f with animation %d, want %v running", v, npc.Loco, NPCRunSpeed)
			}
			ran = true
		}
	})
	if !ran {
		t.Fatal("it never moved")
	}
}

func TestAFightTakesItOffItsPathAndItTakesItUpAtTheNearestLegsEnd(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathLoop)
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// along the first leg, then a fight
	now = pathRun(w, now, 4*time.Second, nil)
	if npc.Z != 980 || npc.X <= 980 || npc.X >= 1020 {
		t.Fatalf("at (%.1f, %.1f), not on its first leg", npc.X, npc.Z)
	}
	npc.CurrentEnemy = 1
	npc.chase.on = true
	// its chase carries it off, beside the square's third leg
	npc.X, npc.Z = 1030, 1010
	now = pathRun(w, now, time.Second, func(time.Time) {
		if npc.X != 1030 || npc.Z != 1010 {
			t.Fatalf("its path moved it during the fight, to (%.1f, %.1f)", npc.X, npc.Z)
		}
	})
	if s, _ := w.PathStatusOf(100); s.State != "fighting" {
		t.Errorf("state %q in a fight", s.State)
	}
	npc.CurrentEnemy = 0
	npc.chase.on = false
	var seen []int
	pathRun(w, now, 30*time.Second, visits(p, npc, &seen))
	// the nearest leg is 1→2 (10 from it): it walks on to 2, not back to 0 or 1
	if len(seen) == 0 || seen[0] != 2 {
		t.Fatalf("after the fight it went to %v first, want 2", seen)
	}
}

func TestTakenAwayItWalksHomeAndWandersAgain(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	npc.SpawnFacing = 1.5
	if err := w.SetNPCPath(100, square(PathLoop)); err != nil {
		t.Fatal(err)
	}
	if w.wanders(npc) {
		t.Fatal("a creature with a path wanders")
	}
	now := pathRun(w, time.Now(), 10*time.Second, nil)
	if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) < 5 {
		t.Fatalf("still by its spawn after 10 s")
	}
	if !w.ClearNPCPath(100) || w.ClearNPCPath(100) {
		t.Fatal("ClearNPCPath: the first should report a path, the second none")
	}
	if s, _ := w.PathStatusOf(100); s.State != "home" {
		t.Errorf("state %q, want home", s.State)
	}
	home := false
	now = pathRun(w, now, 30*time.Second, func(time.Time) {
		if !home && !w.paths.home[100] {
			home = true
			if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) > 1e-3 || npc.Facing != npc.SpawnFacing {
				t.Errorf("home at (%.1f, %.1f) facing %.2f", npc.X, npc.Z, npc.Facing)
			}
		}
	})
	if !home {
		t.Fatal("never got home")
	}
	if !w.wanders(npc) {
		t.Error("home, it does not wander again")
	}
}

func TestDeadItStartsItsPathAgainWhenItComesBack(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathOnce)
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	now := pathRun(w, time.Now(), time.Minute, nil)
	if s, _ := w.PathStatusOf(100); s.State != "holding" {
		t.Fatalf("state %q before its death", s.State)
	}
	npc.Dead = true
	now = pathRun(w, now, time.Second, nil)
	w.respawnNPCLocked(npc)
	var seen []int
	pathRun(w, now, time.Minute, visits(p, npc, &seen))
	if stringOf(seen) != "[0 1 2 3]" {
		t.Fatalf("back from death it visited %v, want [0 1 2 3] again", seen)
	}
}

func TestAPathWalksWithNoPlayerNear(t *testing.T) {
	w := NewWorld()
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	if err := w.SetNPCPath(100, square(PathLoop)); err != nil {
		t.Fatal(err)
	}
	pathRun(w, time.Now(), 5*time.Second, nil)
	if distXZ(npc.X, npc.Z, 980, 980) < 10 {
		t.Fatal("it did not walk with no player in the world")
	}
}

func TestOffTheGroundItTakesTheLegsHeights(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	npc.Y, npc.SpawnY = 10, 10
	p := &NPCPath{Mode: PathOnce, Points: []PathPoint{{X: 980, Y: 10, Z: 980, Pause: -1}, {X: 1020, Y: 30, Z: 980, Pause: -1}}}
	if err := w.SetNPCPath(100, p); err != nil {
		t.Fatal(err)
	}
	pathRun(w, time.Now(), 30*time.Second, func(time.Time) {
		want := 10 + 20*(npc.X-980)/40
		if math.Abs(float64(npc.Y-want)) > 0.01 {
			t.Fatalf("at x %.1f height %.2f, want %.2f", npc.X, npc.Y, want)
		}
	})
	if npc.Y != 30 {
		t.Errorf("at its last point height %.2f, want 30", npc.Y)
	}
}

func TestSetNPCPathRefusesAnotherWorldAndAGMsSpawn(t *testing.T) {
	w := roamWorld(t)
	addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	p := square(PathLoop)
	p.World = 2
	if err := w.SetNPCPath(100, p); err == nil {
		t.Error("a path in world 2 for a creature of world 0 was taken")
	}
	spawn := addNPC(w, 101, "beetle", 980, 980, creatureModel, 0)
	spawn.Temporary = true
	if err := w.SetNPCPath(101, square(PathLoop)); err == nil {
		t.Error("a GM's spawn took a path")
	}
	// one not in the world takes it, for when it comes
	if err := w.SetNPCPath(999, square(PathLoop)); err != nil {
		t.Errorf("a creature not in the world: %v", err)
	}
}

func TestPathFromRecord(t *testing.T) {
	pause := 4.0
	rec := database.NPCPathRecord{NPCID: 5, World: 1, Mode: "back and forth", Pause: 1.5, Speed: "run",
		Points: []database.PathPoint{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6, Pause: &pause}}}
	p, err := PathFromRecord(rec)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != PathPingPong || !p.Run || p.World != 1 || p.Pause != 1500*time.Millisecond ||
		len(p.Points) != 2 || p.Points[0].Pause != -1 || p.Points[1].Pause != 4*time.Second || p.Points[1].Z != 6 {
		t.Fatalf("read as %+v", p)
	}
	bad := []func(r *database.NPCPathRecord){
		func(r *database.NPCPathRecord) { r.Mode = "zigzag" },
		func(r *database.NPCPathRecord) { r.Speed = "fly" },
		func(r *database.NPCPathRecord) { r.Points = nil },
		func(r *database.NPCPathRecord) { r.Pause = -1 },
		func(r *database.NPCPathRecord) { r.Pause = 601 },
		func(r *database.NPCPathRecord) { r.Points = []database.PathPoint{{X: math.NaN()}} },
		func(r *database.NPCPathRecord) { r.Points = make([]database.PathPoint, database.MaxPathPoints+1) },
		func(r *database.NPCPathRecord) { r.World = 300 },
	}
	for i, f := range bad {
		r := rec
		f(&r)
		if _, err := PathFromRecord(r); err == nil {
			t.Errorf("bad record %d was taken", i)
		}
	}
}

func TestLoadNPCPathsGivesTheGoodOnes(t *testing.T) {
	w := roamWorld(t)
	addNPC(w, 100, "beetle", 980, 980, creatureModel, 0)
	n := w.LoadNPCPaths([]database.NPCPathRecord{
		{NPCID: 100, Mode: "loop", Speed: "walk", Points: []database.PathPoint{{X: 990, Z: 990}}},
		{NPCID: 101, Mode: "zigzag", Speed: "walk", Points: []database.PathPoint{{X: 990, Z: 990}}},
	})
	if n != 1 {
		t.Fatalf("%d paths given, want 1", n)
	}
	if _, ok := w.NPCPathOf(100); !ok {
		t.Error("NPC 100 has no path")
	}
	if _, ok := w.NPCPathOf(101); ok {
		t.Error("a bad record became a path")
	}
}
