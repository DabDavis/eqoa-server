package entity

import (
	"math"
	"testing"
)

// A broken weapon still hits: a fifth of the roll, a third of the floor.
func TestBrokenWeaponWorth(t *testing.T) {
	if r, f := rollWorth(0), floorWorth(0); math.Abs(r-0.195) > 0.001 || math.Abs(f-0.343) > 0.001 {
		t.Errorf("a broken weapon's worth: roll %.3f, floor %.3f", r, f)
	}
	if r, f := rollWorth(1), floorWorth(1); r != 1 || f != 1 {
		t.Errorf("a weapon in repair's worth: roll %v, floor %v", r, f)
	}
}

// Petition 2478 (the owner, 2026-11-20): a level 60 monk with a 641 weapon,
// 580 STR and 575 DEX hit a level 60 creature for one number, 295, under a
// warrior's top with a 666 weapon and 395 STR. A melee class rolls the tank's
// bottom and defence share now: a roll with a bottom under its top, its top
// over a warrior's of the same weapon and stats.
func TestAMeleeClassRollsUnderItsTopAndOverATank(t *testing.T) {
	npc := &NPC{Level: 60}
	mk := func(class int) *Player {
		p := &Player{Class: class, Level: 60, Str: 580, Dex: 575}
		p.Gear.WeaponDamage, p.Gear.WeaponCond = 641, 1
		return p
	}
	_, low, high := playerMeleeRoll(mk(classMonk), npc)
	_, tlow, thigh := playerMeleeRoll(mk(classWarrior), npc)
	if low >= high {
		t.Errorf("the monk's roll %d-%d: no spread", low, high)
	}
	if high <= thigh || high-thigh != int(math.Round(offenceArchs[archMelee].High-offenceArchs[archTank].High)) {
		t.Errorf("the monk's top %d against the warrior's %d", high, thigh)
	}
	if high < 450 {
		t.Errorf("the monk's top %d on a level 60 creature: still the old line's", high)
	}
	t.Logf("monk %d-%d, warrior %d-%d", low, high, tlow, thigh)
}
