package entity

import (
	"testing"
	"time"
)

// The cleric's damage marks (markhit.go; jib's PR #1, the GM Jibrish's ruling of
// 2026-10-29). The marks here are made up in the shape the workbook gives the
// live ten: a harmful spell at an enemy, one offensive proc of hp -N, for a
// minute, resisted by AR.

// testMark is a damage mark of n, its spell id id.
func testMark(id, n int) CastSpell {
	return wbSpell(id, 20, 1, "Mark of Testing", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,
		"effects":[{"id":"0001","type":"Offensive Proc","kind":"proc","procs":[{"id":"0002","type":"Dmg/Heal","kind":"hp",
		"amount":-`+itoa(n)+`,"resist":"ar","on":"offense","scope":"target","target_type":"Enemy"}]}]}`)
}

// hitTaken is what one of the caster's weapon hits took off crab at now, with a
// fixed roll.
func hitTaken(c *caster, crab *NPC) int {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	before := crab.HP
	c.w.playerHitLocked(c.sess, c.p, crab, c.npcO)
	return before - crab.HP
}

// A damage mark cast at a creature lands on it, and every weapon hit on it does
// the mark's figure more, told to the hitter. The controls: the same hit with no
// mark; a hit once the mark's minute is over; a friend's proc buff on a
// creature is no mark; of two marks the stronger answers alone.
func TestADamageMarkAnswersTheHitsOnItsCreature(t *testing.T) {
	noResist(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n - 1 }

	base, bcrab := casterAndCrab(t)
	plain := hitTaken(base, bcrab)
	if plain <= 0 {
		t.Fatalf("the control's hit took %d", plain)
	}

	c, crab := casterAndCrab(t)
	mark := testMark(9601, 10)
	if !c.w.StartCast(c.sess, mark, c.npcO, c.at(0)) {
		t.Fatalf("the mark at the crab did not begin: %x", c.sent())
	}
	c.tick(mark.CastTime + castLand)
	if ae := crab.damageMarkAt(time.Now()); ae == nil || ae.Proc != 10 {
		t.Fatalf("the crab's mark: %+v (effects %d)", ae, len(crab.Effects))
	}
	c.sent()
	if got := hitTaken(c, crab); got != plain+10 {
		t.Errorf("a hit on the marked crab took %d, want %d (the hit's %d and the mark's 10)", got, plain+10, plain)
	}
	if figs := figures(c.sess); len(figs) != 2 || figs[1] != -10 {
		t.Errorf("the hitter was told %v, want the hit's and the mark's -10", figs)
	}

	// the control: a mark whose time is over answers nothing
	for _, e := range crab.Effects {
		e.Until = time.Now().Add(-time.Second)
	}
	if got := hitTaken(c, crab); got != plain {
		t.Errorf("a hit after the mark ended took %d, want %d", got, plain)
	}

	// the control: a friend's proc on a creature (no harmful spell's) is no mark
	f, fcrab := casterAndCrab(t)
	fcrab.Effects = append(fcrab.Effects, &ActiveEffect{SpellID: 9602, Name: "Friendly Rune", Proc: 50, ProcMax: 50, Until: time.Now().Add(time.Minute)})
	if got := hitTaken(f, fcrab); got != plain {
		t.Errorf("a creature's own proc buff answered the hit: %d taken, want %d", got, plain)
	}

	// two marks: the stronger alone
	s, scrab := casterAndCrab(t)
	until := time.Now().Add(time.Minute)
	scrab.Effects = append(scrab.Effects,
		&ActiveEffect{SpellID: 9603, Name: "Mark of Testing", Proc: 10, ProcMax: 10, Until: until, Rules: EffectRules{Harm: true}},
		&ActiveEffect{SpellID: 9604, Name: "Brand of Testing", Proc: 40, ProcMax: 40, Until: until, Rules: EffectRules{Harm: true}})
	if got := hitTaken(s, scrab); got != plain+40 {
		t.Errorf("two marks: %d taken, want %d (the stronger's 40 alone)", got, plain+40)
	}
}

// A pet's hit on a marked creature is answered too, as its owner's.
func TestADamageMarkAnswersAPetsHit(t *testing.T) {
	defer func(f, g func(int) int) { meleeIntn, procIntn = f, g }(meleeIntn, procIntn)
	meleeIntn, procIntn = func(n int) int { return n - 1 }, func(int) int { return 99 }
	hit := func(marked bool) int {
		r := newTagRig(t)
		if marked {
			r.npc.Effects = append(r.npc.Effects, &ActiveEffect{SpellID: 9605, Name: "Mark of Testing", Proc: 25, ProcMax: 25,
				Until: r.t0.Add(time.Minute), Rules: EffectRules{Harm: true}})
		}
		before := r.npc.HP
		r.w.mu.Lock()
		r.w.petHitLocked(r.ps, r.npc, r.t0)
		r.w.mu.Unlock()
		return before - r.npc.HP
	}
	plain, marked := hit(false), hit(true)
	if plain <= 0 || marked != plain+25 {
		t.Errorf("a pet's hit took %d unmarked and %d marked, want the mark's 25 more", plain, marked)
	}
}
