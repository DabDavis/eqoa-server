package entity

import (
	"strings"
	"testing"
	"time"
)

// A creature that holds its post (npcaggro's holds; Tallon Zek's sentry, t218:
// "cannot be pulled out of position"): pulled from 28 away it keeps its enemy and
// turns to it but does not move. The control: the same crab without the rule runs
// at its puller (TestACreatureRunsAtItsEnemyAndFightsWhenItReaches).
func TestACreatureThatHoldsItsPostIsNotPulled(t *testing.T) {
	for _, holds := range []bool{true, false} {
		c := newCaster(t)
		c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Zek sentry", 64, 2000, 2000
		c.npc.LeashRadius = DefaultLeash
		c.w.SetAggroRules(map[string]AggroRule{"Zek sentry": {Aggressive: true, Holds: holds}})
		c.p.X, c.p.HP, c.p.MaxHP = 130, 5000, 5000 // 28 off it at 102
		c.npc.CurrentEnemy = c.p.ServerID
		c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}
		x0 := c.npc.X
		for d := time.Duration(0); d <= 2*time.Second; d += 100 * time.Millisecond {
			c.chaseTick(d)
		}
		moved := c.npc.X != x0
		if holds && moved {
			t.Errorf("holding its post it moved from %.2f to %.2f", x0, c.npc.X)
		}
		if !holds && !moved {
			t.Error("the control did not run at its puller")
		}
		if c.npc.CurrentEnemy != c.p.ServerID {
			t.Errorf("holds %v: dropped its enemy", holds)
		}
	}
}

// A blocker's box (segmentEntersBox): a move into it from outside meets it, one
// out of it from inside does not, one past it does not, and one over it does not.
func TestABlockerBoxStopsAMoveIntoIt(t *testing.T) {
	n := &NPC{X: 0, Y: 0, Z: 0, Size: 3}
	h := 3 * blockerHalf // 7.5
	for _, c := range []struct {
		what                   string
		x0, y0, z0, x1, y1, z1 float32
		want                   bool
	}{
		{"through it", -20, 1, 0, 20, 1, 0, true},
		{"into it", -20, 1, 0, 0, 1, 0, true},
		{"out of it", 0, 1, 0, 20, 1, 0, false},
		{"past its side", -20, 1, 8, 20, 1, 8, false},
		{"over it", -20, 9, 0, 20, 9, 0, false},
		{"short of it", -20, 1, 0, -8, 1, 0, false},
	} {
		if got := segmentEntersBox(n, h, c.x0, c.y0, c.z0, c.x1, c.y1, c.z1); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
}

// A door's blocker kept by the server's own check of moves (movecheck.go's
// blockerBetweenLocked): a run into a living ward's box is put back; with the ward
// gone (its sentry dead: the trigger's despawn) the same run is taken. The ward
// stands at (50, 0, 0), size 3: x 42.5-57.5.
func TestMoveCheckStopsAtADoorsBlockerWhileItStands(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 30, 0, 0)
	ward := &NPC{ServerID: 9100, Name: "Tallon's ward", ModelID: int64(BlockerBody), NPCType: 0x80, Size: 3,
		X: 50, Y: 0, Z: 0, HP: 1, MaxHP: 1, Temporary: true}
	r.w.mu.Lock()
	r.w.putNPCLocked(ward)
	r.w.SpatialIndex.Insert(ward.ServerID, ward.X, ward.Z)
	r.w.mu.Unlock()
	if n := r.walk(41, 0, RunSpeed, flat); n != 0 {
		t.Fatalf("up to the ward: %d not taken (%s)", n, r.p.mv.why)
	}
	if r.report(43.5, 0, 0, 300) {
		t.Fatal("a step into the ward's box was taken")
	}
	if !strings.Contains(r.p.mv.why, "blocker") {
		t.Fatalf("put back for %q, not the ward", r.p.mv.why)
	}
	// the ward gone: the way is open
	r.w.mu.Lock()
	r.w.removeNPCLocked(ward)
	r.w.mu.Unlock()
	r.now = r.now.Add(5 * time.Second)
	if n := r.walk(70, 0, RunSpeed, flat); n != 0 {
		t.Fatalf("with the ward gone: %d not taken (%s)", n, r.p.mv.why)
	}
}
