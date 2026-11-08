package entity

import (
	"testing"
	"time"
)

// markOfFaith is the cleric's Mark of Faith as the server keeps it since
// 2026-11-08 (its spelleffect row: a harmful mark on an enemy, mark_heal 65 for
// a minute).
func markOfFaith() CastSpell {
	return wbSpell(701183, 60, 1, "Mark of Faith", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,
		"min_level":36,"req_equipment":"Any","sheet_type":"Mark","effects":[{"id":"015F","type":"Mark","kind":"mark_heal","amount":65}]}`)
}

// Mark of Faith (the user, 2026-11-08: "mark of faith should target enemy only and
// anyone hitting that target with the mark of faith is healed for 65"): cast at
// the crab it lands on the crab, and the caster's weapon hit on it heals the caster
// 65; one short of its most by less is healed only up to it. The controls: cast at
// a friend (the caster itself) it does not land; a hit on the crab once the mark
// has ended heals nothing.
func TestMarkOfFaithHealsWhoeverHitsTheMarked(t *testing.T) {
	noResist(t)
	c, crab := casterAndCrab(t)
	sp := markOfFaith()
	if c.w.StartCast(c.sess, sp, c.me, c.at(0)) {
		c.tick(sp.CastTime + castLand)
		if c.p.hasEffect(sp.SpellID) {
			t.Fatal("Mark of Faith landed on its caster, a friend")
		}
	}
	c.p.Power = c.p.MaxPower
	if !c.w.StartCast(c.sess, sp, c.npcO, c.at(10*time.Second)) {
		t.Fatalf("Mark of Faith at the crab did not begin: %x", c.sent())
	}
	c.tick(10*time.Second + sp.CastTime + castLand)
	if heal := crab.markHealOf(c.at(12 * time.Second)); heal != 65 {
		t.Fatalf("the crab's mark heals %d, want 65 (effects %d)", heal, len(crab.Effects))
	}
	c.p.HP = c.p.MaxHPNow() - 500
	c.w.mu.Lock()
	c.w.markHealPlayerLocked(c.sess, c.p, crab, c.at(12*time.Second))
	c.w.mu.Unlock()
	if got := c.p.HP - (c.p.MaxHPNow() - 500); got != 65 {
		t.Errorf("a hit on the marked crab healed %d, want 65", got)
	}
	c.p.HP = c.p.MaxHPNow() - 20
	c.w.mu.Lock()
	c.w.markHealPlayerLocked(c.sess, c.p, crab, c.at(13*time.Second))
	c.w.mu.Unlock()
	if c.p.HP != c.p.MaxHPNow() {
		t.Errorf("20 short: %d of %d, want full", c.p.HP, c.p.MaxHPNow())
	}
	// the control: the mark over (a minute and more after its landing)
	c.p.HP = c.p.MaxHPNow() - 500
	c.w.mu.Lock()
	c.w.markHealPlayerLocked(c.sess, c.p, crab, c.at(90*time.Second))
	c.w.mu.Unlock()
	if c.p.HP != c.p.MaxHPNow()-500 {
		t.Errorf("a hit after the mark ended healed %d", c.p.HP-(c.p.MaxHPNow()-500))
	}
}
