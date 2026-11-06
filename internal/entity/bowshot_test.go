package entity

import "testing"

// A bow's shot (the user, 2026-11-06: "at level 35 bow damage should double,
// because 2 arrows are shot for one hit", "tmac should hit 2000 on level 50
// training dummy with bow"): a level 60 elf ranger in heirlooms with the level
// 60 heirloom bow (754) shoots a level 50 training dummy for 2,000 at Range4
// (two arrows of 1,000), 1,000 at Range2 (one arrow). The controls: the same
// player's melee swing (Attack4) and a thrown potion's roll stay the roll.
func TestABowShotIsItsArrows(t *testing.T) {
	p := &Player{Class: classRanger, Level: 60, Str: 227, Dex: 366}
	p.Gear.WeaponDamage, p.Gear.WeaponCond = 754, 1
	npc := &NPC{Level: 50, Dummy: TrainingDummy}
	first := func(int) int { return 0 }
	roll := playerMeleeHit(p, npc, first)
	if roll != 474 {
		t.Fatalf("the setup: the melee roll %d, want 474", roll)
	}
	for _, c := range []struct {
		kind, potion, want int
	}{{swingRange4, 0, 2000}, {9, 0, 1000}, {swingRange1, 0, 1000}, {5, 0, roll}, {swingRange4, 50106, roll}} {
		p.Swing.Kind, p.Swing.Potion = c.kind, c.potion
		if got := playerShotHit(p, npc, first); got != c.want {
			t.Errorf("swing kind %d (potion %d): %d, want %d", c.kind, c.potion, got, c.want)
		}
	}
}
