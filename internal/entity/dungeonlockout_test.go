package entity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// lockRig is the dungeons' rig with a lockout of 10 minutes and its clock on the rig's.
func lockRig(t *testing.T, n int, lockout time.Duration) *dungeonsRig {
	r := newDungeons(t, n)
	r.w.mu.Lock()
	r.w.dungeons.set.Lockout = lockout
	r.w.dungeons.clock = func() time.Time { return r.now }
	r.w.mu.Unlock()
	return r
}

// Entry 1261: /dungeon reset then /dungeon enter within 10 minutes is refused, the minutes rounded up; another
// dungeon is not; after 10 minutes a new instance opens.
func TestDungeonLockoutAfterReset(t *testing.T) {
	r := lockRig(t, 1, 10*time.Minute)
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("open")
	}
	r.land()
	r.w.DungeonLeave(r.ss[0])
	r.land()
	if line := r.w.DungeonReset(r.ss[0], "guk"); !strings.Contains(line, "is closed") {
		t.Fatalf("reset: %q", line)
	}
	r.now = r.now.Add(90 * time.Second)
	why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now)
	if ok || why != "You may enter a new instance of Test Guk in 9 minutes." {
		t.Fatalf("reopened at once: %v %q", ok, why)
	}
	if st := r.w.DungeonStatus(r.ss[0]); !strings.Contains(st, "Locked out of new instances of Test Guk for 9 more minutes.") {
		t.Fatalf("/dungeon: %q", st)
	}
	if _, ok := r.w.DungeonEnter(r.ss[0], "bb", r.now); !ok {
		t.Fatal("a different dungeon refused")
	}
	r.land()
	r.w.DungeonLeave(r.ss[0])
	r.land()
	r.now = r.now.Add(8*time.Minute + 30*time.Second) // 10 minutes since the reset
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatalf("still locked out after 10 minutes: %q", why)
	}
}

// Everyone who stood in an instance closed by close_after is locked out; one who never stood in it is not; a
// locked-out member may go back into its group's instance opened before the lockout, but not one opened after.
func TestDungeonLockoutStoodInIt(t *testing.T) {
	r := lockRig(t, 3, 10*time.Minute)
	a, b, c := r.ps[0], r.ps[1], r.ps[2]
	r.group(a, b)
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("open")
	}
	if _, ok := r.w.DungeonEnter(r.ss[1], "guk", r.now); !ok {
		t.Fatal("member")
	}
	r.land()
	first := a.InstanceKey()
	r.w.DungeonLeave(r.ss[0])
	r.w.DungeonLeave(r.ss[1])
	r.land()
	r.tick(11 * time.Minute) // close_after 10m, no one inside
	if _, open := r.w.dungeons.insts[first]; open {
		t.Fatal("not closed")
	}
	for i, want := range []bool{true, true, false} {
		if _, ok := r.w.DungeonEnter(r.ss[i], "guk", r.now); ok == want {
			t.Fatalf("player %d (%s) refused %v, want %v", i, r.ps[i].Name, !ok, want)
		}
	}
	_ = c
	// the lockout over, a opens a new instance
	r.now = r.now.Add(11 * time.Minute)
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("lockout over, a opens")
	}
	r.land()
	opened := r.now
	// b locked out (an instance it stood in elsewhere closed) AFTER a's instance opened: b may go back into it
	r.w.mu.Lock()
	r.w.dungeons.lockouts[dungeonLock{b.CharServerID, "guk"}] = opened.Add(time.Second)
	r.w.mu.Unlock()
	if why, ok := r.w.DungeonEnter(r.ss[1], "guk", opened.Add(2*time.Second)); !ok {
		t.Fatalf("refused an instance opened before its lockout: %q", why)
	}
	r.land()
	r.w.DungeonLeave(r.ss[1])
	r.land()
	// b locked out BEFORE a's instance opened: refused, with its minutes
	r.w.mu.Lock()
	r.w.dungeons.lockouts[dungeonLock{b.CharServerID, "guk"}] = opened.Add(-time.Minute)
	r.w.mu.Unlock()
	if why, ok := r.w.DungeonEnter(r.ss[1], "guk", opened.Add(2*time.Second)); ok || !strings.Contains(why, "in 9 minutes") {
		t.Fatalf("entered an instance opened after its lockout began: %v %q", ok, why)
	}
}

// Negative control: lockout 0 (world.dungeons.lockout: 0) is off; a reset and a new instance at once work as before.
func TestDungeonLockoutOff(t *testing.T) {
	r := lockRig(t, 1, 0)
	r.w.DungeonEnter(r.ss[0], "guk", r.now)
	r.land()
	r.w.DungeonLeave(r.ss[0])
	r.land()
	r.w.DungeonReset(r.ss[0], "guk")
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatalf("lockout off refused: %q", why)
	}
}

// A keep restart carries the lockouts and who stood in each instance (JSON through, as udp/keep.go writes it).
func TestDungeonLockoutKeptThroughKeep(t *testing.T) {
	r := lockRig(t, 2, 10*time.Minute)
	r.w.DungeonEnter(r.ss[0], "guk", r.now) // instance 1, a stands in it
	r.land()
	r.w.DungeonEnter(r.ss[1], "bb", r.now)
	r.land()
	r.w.DungeonLeave(r.ss[1])
	r.land()
	r.w.DungeonReset(r.ss[1], "bb") // b locked out of bb
	k := r.w.KeepDungeons(r.now)
	raw, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	var back KeptDungeons
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Lockouts) != 1 || back.Lockouts[0].Key != "bb" || len(back.Insts) != 1 || len(back.Insts[0].Stood) != 1 {
		t.Fatalf("kept: %s", raw)
	}
	r2 := lockRig(t, 2, 10*time.Minute)
	r2.now = r.now
	r2.w.RestoreDungeons(&back)
	if why, ok := r2.w.DungeonEnter(r2.ss[1], "bb", r2.now.Add(time.Minute)); ok || !strings.Contains(why, "in 9 minutes") {
		t.Fatalf("lockout lost through the keep: %v %q", ok, why)
	}
	r2.w.mu.Lock()
	in := r2.w.dungeons.insts[back.Insts[0].ID]
	r2.w.mu.Unlock()
	if in == nil || !in.stood[r.ps[0].CharServerID] {
		t.Fatal("who stood in the instance lost through the keep")
	}
}
