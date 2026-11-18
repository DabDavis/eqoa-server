package entity

import (
	"testing"
	"time"
)

// Zaryn Boome, the Siliskor quest's giver, "spawns rarely" (the owner,
// 2026-11-18): on his clock he stands his up time, then is gone (out of every
// view) for a time drawn between his gone_min and gone_max, then stands again.
// The control: an NPC with no clock stands throughout.
func TestARareCycleComesAndGoes(t *testing.T) {
	w := NewWorld()
	zaryn := &NPC{ServerID: 101355, Name: "Zaryn Boome", NPCType: 0x80, HP: 10, MaxHP: 10}
	other := &NPC{ServerID: 101075, Name: "Corliss Magemell", NPCType: 0x80, HP: 10, MaxHP: 10}
	w.NPCs[zaryn.ServerID], w.NPCs[other.ServerID] = zaryn, other
	back := rareCycleRand
	t.Cleanup(func() { rareCycleRand = back })
	rareCycleRand = func(lo, hi time.Duration) time.Duration { return hi } // the longest gone
	rc := &rareCycle{up: 30 * time.Minute, goneMin: 60 * time.Minute, goneMax: 180 * time.Minute}
	w.rareCycles = map[EntityID]*rareCycle{zaryn.ServerID: rc}
	now := time.Now()

	step := func(at time.Duration) {
		w.mu.Lock()
		w.rareCycleSystem(now.Add(at))
		w.mu.Unlock()
	}
	step(0)
	if zaryn.hiddenAt(now) {
		t.Fatal("standing at first, he is gone")
	}
	step(29 * time.Minute)
	if zaryn.hiddenAt(now.Add(29 * time.Minute)) {
		t.Fatal("gone before his up time is over")
	}
	step(31 * time.Minute)
	if !zaryn.hiddenAt(now.Add(31*time.Minute)) || !zaryn.hiddenAt(now.Add(210*time.Minute)) {
		t.Fatal("not gone after his up time, for his gone time")
	}
	if other.hiddenAt(now.Add(31 * time.Minute)) {
		t.Fatal("an NPC with no clock went (the control)")
	}
	step(212 * time.Minute)
	if zaryn.hiddenAt(now.Add(212 * time.Minute)) {
		t.Fatal("not back after his gone time")
	}
	step(213 * time.Minute)
	step(241 * time.Minute)
	if zaryn.hiddenAt(now.Add(241 * time.Minute)) {
		t.Fatal("back, he went before his up time")
	}
	step(244 * time.Minute)
	if !zaryn.hiddenAt(now.Add(244 * time.Minute)) {
		t.Fatal("back, he did not go after his up time")
	}
}
