package entity

import "testing"

// Hate is never below 0 (hatefloor.go, petition 3254).

func TestHateTakenFromOneNotListedDoesNothing(t *testing.T) {
	r := newAreaRig(t)
	npc := r.creature(2001, "a gnoll", 1010, 1000)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.updateAggro(npc, 1001, -144)
	if len(npc.Threats) != 0 || npc.CurrentEnemy != 0 {
		t.Fatalf("a hate taken from no one left threats %v, enemy %d", npc.Threats, npc.CurrentEnemy)
	}
	// a later nuke makes the caster its enemy as any first hit does
	r.w.updateAggro(npc, 1001, 50)
	if npc.Threats[1001] != 50 || npc.CurrentEnemy != 1001 {
		t.Fatalf("after a nuke: threats %v, enemy %d, want 50 and 1001", npc.Threats, npc.CurrentEnemy)
	}
}

func TestHateTakenStopsAtZero(t *testing.T) {
	r := newAreaRig(t)
	npc := r.creature(2001, "a gnoll", 1010, 1000)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.updateAggro(npc, 1001, 100)
	r.w.updateAggro(npc, 1001, -144)
	if got := npc.Threats[1001]; got != 0 {
		t.Fatalf("100 less 144: %d, want 0", got)
	}
	r.w.updateAggro(npc, 1001, -144) // again: still 0, not -144
	r.w.updateAggro(npc, 1001, 30)
	if got := npc.Threats[1001]; got != 30 {
		t.Fatalf("a nuke of 30 after two evades: %d, want 30", got)
	}
}

func TestAddHate(t *testing.T) {
	for _, c := range []struct {
		held  int64
		delta int
		want  int64
	}{{0, 5, 5}, {10, -4, 6}, {10, -10, 0}, {10, -99, 0}, {0, -1, 0}} {
		if got := addHate(c.held, c.delta); got != c.want {
			t.Errorf("addHate(%d, %d) = %d, want %d", c.held, c.delta, got, c.want)
		}
	}
}
