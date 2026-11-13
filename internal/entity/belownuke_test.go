package entity

import (
	"testing"
	"time"
)

// A cast held for its creature's hit points below a share (classtemplate.go's
// templateWantsLocked; the owner, 2026-11-13: Lord Koreshaa, "when he gets down
// to 1/3 hp 3500 ar nuke he fires once"): a fixed 3,500 arcane nuke below 34%,
// every hour (once a fight). Above its share it never casts, however long the
// fight (the control: the same template's nuke with no below casts at once);
// below it, it lands once, 3,500 on a player of no arcane resist, and not again
// in the next ten minutes.
func TestANukeHeldForAThirdOfItsHitPoints(t *testing.T) {
	const fx = 2943530132
	r := newTemplateRig(t)
	r.godTemplate("pride", ", class, melee_pct", ", 0, 10",
		`[{"kind":"nuke","name":"Pride's Fury","fx":2943530132,"damage":3500,"resist":"AR","below":34,"every":3600}]`, "gnoll guard")
	r.godTemplate("pride at once", ", class, melee_pct", ", 0, 10",
		`[{"kind":"nuke","name":"Pride's Fury","fx":2943530132,"damage":3500,"resist":"AR","every":3600}]`, "gnoll brute")
	guard, brute := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
	pool(guard, 100_000)
	pool(brute, 100_000)
	guard.HP = 34_000 // a third and more: held
	p, sess := r.player("Tank", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 50_000, 50_000
	q, qs := r.player("Other", brute.X+1, brute.Z)
	q.HP, q.MaxHP = 50_000, 50_000
	r.fight(guard, p.ServerID)
	r.fight(brute, q.ServerID)
	nukes := func(s [][]byte, by *NPC) int {
		n := 0
		for _, c := range castsIn(s) {
			if c.fx == fx && c.caster == r.objectID(by.ServerID) {
				n++
			}
		}
		return n
	}
	r.run(0, 30*time.Second)
	if n := nukes(drain(sess), guard); n != 0 {
		t.Fatalf("%d nukes at 34%% in 30 s, want none", n)
	}
	if n := nukes(drain(qs), brute); n != 1 {
		t.Fatalf("the control: %d nukes with no below in 30 s, want 1", n)
	}
	r.w.mu.Lock()
	guard.HP = 33_000
	r.w.mu.Unlock()
	ph := p.HP
	r.run(30*time.Second+100*time.Millisecond, 35*time.Second)
	s := drain(sess)
	if n := nukes(s, guard); n != 1 {
		t.Fatalf("%d nukes in 5 s at 33%%, want 1", n)
	}
	if !hasLine(linesOf(s), "Gnoll Guard's Pride's Fury hits you for 3500 (your resist took 0%).") || ph-p.HP < 3500 {
		t.Errorf("its landing: the tank lost %d, told %q", ph-p.HP, linesOf(s))
	}
	r.run(35*time.Second+100*time.Millisecond, 10*time.Minute)
	if n := nukes(drain(sess), guard); n != 0 {
		t.Fatalf("%d more nukes in the next ten minutes, want none: once a fight", n)
	}
}
