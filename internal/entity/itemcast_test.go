package entity

import "testing"

// An item's cast (CastSpell.ItemCast, handlers/itemspell.go): the Rune of Defilement's "Drain Power by 150, PB AE"
// begun with no target drains each creature round its user and gives the user each 150 it took; it sets no
// lockout (no ability of the book); the creature beyond its reach gives nothing.
func TestAnItemsAreaPowerTap(t *testing.T) {
	for _, line := range []string{"Target, Drain Power by 150, PB AE"} {
		r := newAreaRig(t)
		r.p.Power = 100
		a := r.creature(2001, "a decaying skeleton", 1004, 1000)
		b := r.creature(2002, "a decaying skeleton", 1000, 1005)
		far := r.creature(2003, "a decaying skeleton", 1040, 1000)
		for _, n := range []*NPC{a, b, far} {
			n.Level = 1
		}
		spell := spellOf(760355, 0, "Defilement", line)
		spell.ItemCast, spell.PowerCost, spell.CastTime = true, 0, 0
		if spell.Area.Centre != AreaOnCaster || !spell.Def.Harmful {
			t.Fatalf("%q: area %+v, harmful %v", line, spell.Area, spell.Def.Harmful)
		}
		r.cast(t, spell, 0)
		if len(r.p.Lockouts) != 0 {
			t.Errorf("an item's cast set %d lockouts", len(r.p.Lockouts))
		}
		if got := r.p.Power - 100; got <= 0 || got%150 != 0 || got > 300 {
			t.Errorf("%q gave %d power, want 150 a creature in reach (two)", line, got)
		}
		if far.CurrentEnemy != 0 {
			t.Errorf("the creature 40 off was reached")
		}
	}
}
