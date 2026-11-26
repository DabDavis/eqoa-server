package entity

import (
	"strings"
	"testing"
	"time"
)

// The scroll: read in the instance it keeps the exact place and sends its reader home
// (Return Home's travel, the scroll kept); read out of the dungeons within the window,
// the instance open and the reader still of its party, it sends it back to that place,
// facing as it stood, into that instance, and is spent. The controls: a read in town
// with no place kept, a read by a player not in the world, and the dungeons off do
// nothing.
func TestScrollOfReturningOutAndBack(t *testing.T) {
	r := newDungeons(t, 2)
	a := r.ps[0]
	if res, line := r.w.DungeonReturnRead(r.ss[0], r.now); res != ReturnRefused || line != returnLineNone {
		t.Fatalf("in town, nothing kept: %v %q", res, line)
	}
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal(why)
	}
	r.land()
	inst := a.Instance
	r.w.mu.Lock()
	a.X, a.Y, a.Z, a.Facing = 1012.5, 48, 990.25, 1.5
	r.w.mu.Unlock()
	res, line := r.w.DungeonReturnRead(r.ss[0], r.now)
	if res != ReturnOut || !strings.Contains(line, "Test Guk") {
		t.Fatalf("out: %v %q", res, line)
	}
	due := r.land()
	if len(due) != 1 || due[0].Home != a.ServerID || due[0].P != a {
		t.Fatalf("not sent home: %+v", due)
	}
	if in, x, y, z, ok := r.w.DungeonReturnOf(a.CharServerID); !ok || in != inst || x != 1012.5 || y != 48 || z != 990.25 {
		t.Fatalf("the place kept: %d %v %v %v %v", in, x, y, z, ok)
	}
	r.tick(5 * time.Minute) // home a while; the instance stays open (the rig's CloseAfter is 10 minutes with no one)
	res, line = r.w.DungeonReturnRead(r.ss[0], r.now)
	if res != ReturnBack || !strings.Contains(line, "crumbles") {
		t.Fatalf("back: %v %q", res, line)
	}
	due = r.land()
	if len(due) != 1 || due[0].To.World != 14 || due[0].To.X != 1012.5 || due[0].To.Z != 990.25 || !due[0].HasFacing || due[0].Facing != 1.5 {
		t.Fatalf("not sent back to the place: %+v", due)
	}
	if a.Instance != inst {
		t.Fatalf("back in instance %d, not %d", a.Instance, inst)
	}
	if _, _, _, _, ok := r.w.DungeonReturnOf(a.CharServerID); ok {
		t.Fatal("the place still kept after the way back")
	}
	r.tick(10 * time.Second)
	if len(r.w.gates.due) != 0 {
		t.Fatal("sent out of the instance it was returned to")
	}
	// and its way out of the dungeon is where it read the scroll at home
	r.w.mu.Lock()
	stay := r.w.dungeons.stays[a.CharServerID]
	r.w.mu.Unlock()
	if stay.From.World != 0 {
		t.Errorf("the way out: %+v", stay)
	}

	// no player, no dungeons
	r.w.mu.Lock()
	delete(r.w.Players, r.ps[1].ServerID)
	r.w.mu.Unlock()
	if res, _ := r.w.DungeonReturnRead(r.ss[1], r.now); res != ReturnRefused {
		t.Error("a session with no player read it")
	}
	r.w.SetDungeons(false, 14, nil, DungeonSettings{})
	if res, _ := r.w.DungeonReturnRead(r.ss[0], r.now); res != ReturnRefused {
		t.Error("read with the dungeons off")
	}
}

// The ways back that fail: past the window, the instance closed (or another under its
// id), and the reader no longer of its party each forget the place and keep the scroll;
// a lockout keeps both. A second read in the dungeon keeps the new place.
func TestScrollOfReturningWayBackFails(t *testing.T) {
	r := newDungeons(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.group(a, b)
	out := func(p *Player, i int) uint32 {
		t.Helper()
		if why, ok := r.w.DungeonEnter(r.ss[i], "guk", r.now); !ok {
			t.Fatal(why)
		}
		r.land()
		if res, line := r.w.DungeonReturnRead(r.ss[i], r.now); res != ReturnOut {
			t.Fatalf("out: %v %q", res, line)
		}
		r.land()
		return p.Instance
	}
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok { // a, the leader, opens it
		t.Fatal(why)
	}
	r.land()
	inst := out(b, 1)
	// a second read in the dungeon keeps the new place
	r.w.DungeonEnter(r.ss[1], "guk", r.now)
	r.land()
	r.w.mu.Lock()
	b.X = 1050
	r.w.mu.Unlock()
	r.w.DungeonReturnRead(r.ss[1], r.now)
	r.land()
	if _, x, _, _, _ := r.w.DungeonReturnOf(b.CharServerID); x != 1050 {
		t.Fatalf("the new place not kept: %v", x)
	}
	// past the window
	r.now = r.now.Add(DungeonReturnWindow + time.Second)
	if res, line := r.w.DungeonReturnRead(r.ss[1], r.now); res != ReturnRefused || !strings.Contains(line, "minutes have passed") {
		t.Fatalf("past the window: %v %q", res, line)
	}
	if _, _, _, _, ok := r.w.DungeonReturnOf(b.CharServerID); ok {
		t.Fatal("a faded way kept")
	}

	// out of the party
	out(b, 1)
	r.w.mu.Lock()
	r.w.groups[a.GroupID].Members = r.w.groups[a.GroupID].Members[:1]
	b.GroupID = 0
	r.w.mu.Unlock()
	if res, line := r.w.DungeonReturnRead(r.ss[1], r.now); res != ReturnRefused || !strings.Contains(line, "no longer of") {
		t.Fatalf("out of the party: %v %q", res, line)
	}

	// the instance closed (reset by its owner, no one in it), and another opened under a new id
	a2 := out(a, 0)
	if a2 != inst {
		t.Fatalf("a's instance %d, b's was %d", a2, inst)
	}
	if line := r.w.DungeonReset(r.ss[0], "guk"); !strings.Contains(line, "closed") {
		t.Fatalf("reset: %q", line)
	}
	if res, line := r.w.DungeonReturnRead(r.ss[0], r.now); res != ReturnRefused || !strings.Contains(line, "instance is closed") {
		t.Fatalf("closed: %v %q", res, line)
	}

	// a lockout bars the way back for now, the place kept: a's lockout from Guk runs (the reset), and a new
	// instance, opened after it began, is no way back while it runs
	r.w.SetDungeons(true, 14, dungeonsTestDefs(), DungeonSettings{CloseAfter: 10 * time.Minute, Lockout: 10 * time.Minute})
	r.w.mu.Lock()
	r.w.dungeons.clock = func() time.Time { return r.now }
	r.w.mu.Unlock()
	if why, ok := r.w.DungeonEnter(r.ss[1], "guk", r.now); !ok { // b alone opens one
		t.Fatal(why)
	}
	r.land()
	r.w.DungeonReturnRead(r.ss[1], r.now)
	r.land()
	r.w.DungeonReset(r.ss[1], "guk") // b's own instance closed: b locked out ...
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now.Add(time.Second)); !ok { // ... and a opens another, b added
		t.Fatal(why)
	}
	r.land()
	r.w.mu.Lock()
	in := r.w.dungeons.insts[r.ps[0].Instance]
	in.invited = map[uint32]string{b.CharServerID: b.Name}
	r.w.dungeons.returns[b.CharServerID] = dungeonReturn{inst: in.id, opened: in.opened, x: 1000, z: 1000, at: r.now}
	r.w.mu.Unlock()
	if res, line := r.w.DungeonReturnRead(r.ss[1], r.now.Add(2*time.Second)); res != ReturnRefused || !strings.Contains(line, "in 10 minutes") {
		t.Fatalf("locked out: %v %q", res, line)
	}
	if _, _, _, _, ok := r.w.DungeonReturnOf(b.CharServerID); !ok {
		t.Fatal("a lockout forgot the way back")
	}
}

// A keep restart carries the ways back of open instances, and drops those past their
// window.
func TestScrollOfReturningKept(t *testing.T) {
	r := newDungeons(t, 1)
	a := r.ps[0]
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal(why)
	}
	r.land()
	r.w.DungeonReturnRead(r.ss[0], r.now)
	r.land()
	k := r.w.KeepDungeons(r.now)
	if k == nil || len(k.Returns) != 1 || k.Returns[0].Char != a.CharServerID {
		t.Fatalf("kept: %+v", k)
	}
	r.w.mu.Lock()
	r.w.dungeons.returns = nil
	r.w.mu.Unlock()
	r.w.RestoreDungeons(&KeptDungeons{Returns: k.Returns})
	if _, _, _, _, ok := r.w.DungeonReturnOf(a.CharServerID); !ok {
		t.Fatal("not restored")
	}
	if late := r.w.KeepDungeons(r.now.Add(DungeonReturnWindow + time.Minute)); late != nil && len(late.Returns) != 0 {
		t.Fatalf("a way past its window kept: %+v", late.Returns)
	}
}
