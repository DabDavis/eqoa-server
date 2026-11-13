package entity

import (
	"strings"
	"testing"
)

// Ticket 2010 (Waldorf, 2026-11-13): "Guk Instance - I've killed 306 of 307. Is the last npc the teleport? If so
// that shouldn't be counted." An instance counts the creatures a player can kill alone: a teleporter with the NPC
// bit standing in Test Guk is copied in but is in neither the total nor the alive count; with the three others
// killed it is "3 of 3". An instance kept before the rule (Killable unset, its count 4) is counted again as it is
// made again (3); one kept after keeps its own.
func TestADungeonCountsOnlyWhatCanBeKilled(t *testing.T) {
	addPorter := func(r *dungeonsRig) {
		r.w.mu.Lock()
		b := &NPC{ServerID: 920005, Name: "froglok teleporter", X: 1005, Y: 50, Z: 1080, World: 14, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1}
		b.SpawnX, b.SpawnY, b.SpawnZ = b.X, b.Y, b.Z
		r.w.putNPCLocked(b)
		r.w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
		r.w.mu.Unlock()
	}
	r := newDungeons(t, 1)
	addPorter(r)
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal(why)
	}
	r.land()
	c := r.copies(r.ps[0].InstanceKey())
	if c["froglok teleporter"] == nil {
		t.Fatal("the teleporter was not copied in (the control)")
	}
	r.w.mu.Lock()
	c["Lord Hoptor"].Dead = true
	r.w.mu.Unlock()
	if got := r.w.DungeonStatus(r.ss[0]); !strings.Contains(got, "1 of 3 creatures killed") {
		t.Fatalf("status %q, want 1 of 3", got)
	}
	r.w.mu.Lock()
	c["froglok vis knight"].Dead = true
	c["froglok tal wizard"].Dead = true
	r.w.mu.Unlock()
	if got := r.w.DungeonStatus(r.ss[0]); !strings.Contains(got, "3 of 3 creatures killed") {
		t.Fatalf("status %q, want 3 of 3", got)
	}

	// kept and made again: before the rule, and after it
	for _, old := range []bool{true, false} {
		k := r.w.KeepDungeons(r.now)
		if k == nil || len(k.Insts) != 1 || !k.Insts[0].Killable || k.Insts[0].Creatures != 3 {
			t.Fatalf("kept: %+v", k)
		}
		if old {
			k.Insts[0].Killable, k.Insts[0].Creatures = false, 4
		}
		r2 := newDungeons(t, 1)
		addPorter(r2)
		r2.w.RestoreDungeons(k)
		r2.w.mu.Lock()
		var in *dungeonInst
		for _, i := range r2.w.dungeons.insts {
			in = i
		}
		n := in.creatures
		r2.w.mu.Unlock()
		if n != 3 {
			t.Errorf("made again (kept before the rule %v): %d creatures, want 3", old, n)
		}
	}
}
