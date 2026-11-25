package entity

import (
	"strings"
	"testing"
	"time"
)

// Petition 2972: a summon into a dungeon instance joins its player to the summoner's instance by the door's rules
// (dungeonsummon.go); the handlers' tests drive the Summoning Portal and #summon end to end.

// A player of the party joins (its instance, its way out where it stood); one locked out of the dungeon, or one not of
// the party, is refused with a line naming why, and nothing of it changes.
func TestDungeonSummonJoinsByTheDoorsRules(t *testing.T) {
	r := lockRig(t, 4, 10*time.Minute)
	a, b, c, d := r.ps[0], r.ps[1], r.ps[2], r.ps[3]
	r.group(a, b, d)
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("open")
	}
	r.land()
	in := a.Instance
	// d stood in an instance of Guk that closed: locked out of a new one
	r.w.mu.Lock()
	r.w.dungeons.lockouts[dungeonLock{d.CharServerID, "guk"}] = r.now.Add(-time.Minute)
	r.w.mu.Unlock()
	r.now = r.now.Add(time.Second)

	if why, ok := r.w.InstanceJoinCheck(c.ServerID, a.ServerID, false); ok || !strings.Contains(why, "not in its party") {
		t.Fatalf("c, of no group: %v %q", ok, why)
	}
	if why, ok := r.w.JoinInstanceOf(c.ServerID, a.ServerID, false); ok || c.Instance != 0 {
		t.Fatalf("c joined: %v %q instance %d", ok, why, c.Instance)
	}
	why, ok := r.w.InstanceJoinCheck(d.ServerID, a.ServerID, false)
	if ok || why != "Raider3 cannot be summoned into Raider0's instance of Test Guk: they are locked out of new instances of it for 9 more minutes." {
		t.Fatalf("d, locked out: %v %q", ok, why)
	}
	if _, ok := r.w.JoinInstanceOf(b.ServerID, a.ServerID, false); !ok || b.Instance != in {
		t.Fatalf("b, of the party: %v instance %d, want %d", ok, b.Instance, in)
	}
	r.w.mu.RLock()
	from := r.w.dungeons.stays[b.CharServerID].From
	r.w.mu.RUnlock()
	if from.World != 0 || from.X != b.X || from.Z != b.Z {
		t.Fatalf("b's way out: %+v", from)
	}
	// a summon by someone outside the dungeons is none of theirs
	if _, ok := r.w.InstanceJoinCheck(c.ServerID, d.ServerID, false); !ok {
		t.Fatal("a summon outside the dungeons refused")
	}
	// a GM's (force) brings c in, added to the instance
	if _, ok := r.w.JoinInstanceOf(c.ServerID, a.ServerID, true); !ok || c.Instance != in {
		t.Fatalf("a GM's summon of c: %v instance %d", ok, c.Instance)
	}
	r.w.mu.Lock()
	_, added := r.w.dungeons.insts[in].invited[c.CharServerID]
	r.w.mu.Unlock()
	if !added {
		t.Fatal("c not added to the instance")
	}
}

// The stray check leaves a player loading a world alone (its way out would land in the middle of the load: petition
// 2972's WORLD MISMATCH), and puts it out once it stands there past the grace. The control: the same player, its
// load done, goes out.
func TestDungeonStrayCheckWaitsForTheLoad(t *testing.T) {
	r := newDungeons(t, 1)
	p, s := r.ps[0], r.ss[0]
	r.w.mu.Lock()
	r.w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
	p.World, p.X, p.Z = 14, 1000, 1150 // summoned in with no instance, as before the fix
	r.w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	r.w.mu.Unlock()
	s.HoldStateChannels(time.Minute)
	for i := 0; i < 4; i++ {
		r.tick(10 * time.Second)
	}
	if due := r.land(); len(due) != 0 || p.World != 14 {
		t.Fatalf("sent out while loading: %d travels, world %d", len(due), p.World)
	}
	s.ConsumeWorldLoadPending()
	r.tick(10 * time.Second)
	r.tick(10 * time.Second)
	if due := r.land(); len(due) != 1 || p.World != 0 {
		t.Fatalf("not sent out once loaded: %d travels, world %d", len(due), p.World)
	}
}
