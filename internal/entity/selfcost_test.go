package entity

import (
	"testing"
	"time"
)

// selfSpell is a free self line cast in 2 s (the numbers below are made up).
func selfSpell(id int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Scope: 0, Name: name, Icon: 0x12345678,
		CastTime: 2 * time.Second, Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// A self line's own lost hit points are its caster's price, in full (the
// user's ruling of 2026-10-29, after the test lanes' report found every one
// dropped): a cannibalize trades them for power; a price the caster cannot
// pay kills it, and nothing more of the spell lands.
func TestSelfLinePaysItsPrice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Power = 500, 100
	c.land(t, selfSpell(9401, "Little Cannibalize", "Self, Decrease Hitpoints by 30, Increase Power by 8"), c.me, 0)
	if c.p.HP != 470 || c.p.Power != 108 {
		t.Fatalf("HP %d power %d, want 470 and 108", c.p.HP, c.p.Power)
	}
	c.p.HP = 20
	c.land(t, selfSpell(9402, "Big Cannibalize", "Self, Decrease Hitpoints by 30, Increase Power by 8"), c.me, 10*time.Second)
	if !c.p.IsDead() || c.p.Power != 108 {
		t.Fatalf("dead %v, power %d: the price that kills gives nothing", c.p.IsDead(), c.p.Power)
	}
}

// A self line's loss each tick (Lich) is paid at once and every tick while it
// lasts, beside its power; Revert Lich ends it (no necromancer has Remove
// Illusion), and its ticks with it.
func TestLichCostsEachTickUntilReverted(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Power = 500, 100
	c.land(t, selfSpell(9403, "Lich", "Self, Sprite Change, Decrease HP per tic by 7, Increase Power per tic by 5, 30min"), c.me, 0)
	land := 2*time.Second + castLand
	if c.p.HP != 493 || c.p.Power != 105 {
		t.Fatalf("at its landing: HP %d power %d, want 493 and 105", c.p.HP, c.p.Power)
	}
	c.tick(land + 6*time.Second)
	c.tick(land + 12*time.Second)
	if c.p.HP != 479 || c.p.Power != 115 {
		t.Fatalf("after two more ticks: HP %d power %d, want 479 and 115", c.p.HP, c.p.Power)
	}
	revert := selfSpell(9404, "Revert Lich", "Self, Ends the dread form")
	if revert.Def.FormKind != FormEnd {
		t.Fatalf("Revert Lich reads as %+v, want an end of the form", revert.Def)
	}
	c.land(t, revert, c.me, land+13*time.Second)
	if len(c.p.Effects) != 0 {
		t.Fatalf("after Revert Lich: %+v", c.p.Effects)
	}
	hp := c.p.HP
	for s := 20; s <= 60; s += 6 {
		c.tick(land + time.Duration(s)*time.Second)
	}
	if c.p.HP != hp {
		t.Errorf("HP %d after Revert Lich, want %d: it still ticks", c.p.HP, hp)
	}
}

// The tick that leaves nothing kills its bearer, and the death takes the form
// off: no tick after it.
func TestLichTickCanKill(t *testing.T) {
	c := newCaster(t)
	c.p.HP = 10
	c.land(t, selfSpell(9405, "Lich", "Self, Sprite Change, Decrease HP per tic by 7, 30min"), c.me, 0)
	land := 2*time.Second + castLand
	c.tick(land + 6*time.Second)
	if !c.p.IsDead() || len(c.p.Effects) != 0 {
		t.Fatalf("HP %d, effects %+v: the second tick should kill and the death end the form", c.p.HP, c.p.Effects)
	}
}
