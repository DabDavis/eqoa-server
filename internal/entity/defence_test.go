package entity

import "testing"

// rollRange is the roll's least and most on the player, through hitOn
// itself: the lowest draw and the highest.
func rollRange(r meleeRoll, p *Player) (lo, hi int) {
	return r.hitOn(p, func(int) int { return 0 }), r.hitOn(p, func(n int) int { return n - 1 })
}

func TestArchetypeOf(t *testing.T) {
	want := map[int]archetype{0: archTank, 2: archTank, 3: archTank, 1: archMelee, 4: archMelee, 5: archMelee, 6: archMelee,
		7: archPriest, 8: archPriest, 9: archPriest, 10: archCaster, 11: archCaster, 12: archCaster, 13: archCaster, 14: archCaster}
	for class, a := range want {
		if got := archetypeOf(class); got != a {
			t.Errorf("class %d: archetype %d, want %d", class, got, a)
		}
	}
}

// The beach's creatures: the Lv34 Desert Crab hit a shadow knight of 1763
// AC its floor, 17, every time, and an enchanter of 167 AC and 397 AGI 17,
// 17, 19 and 45; the Lv35 Gharial hit the knight 18 every time.
func TestCreatureRolls(t *testing.T) {
	knight := &Player{Class: 3, AC: 1763, Agi: 95}
	enchanter := &Player{Class: 12, AC: 167, Agi: 397}
	crab, gharial := creatureRoll(34), creatureRoll(35)
	if lo, hi := rollRange(crab, knight); lo != 17 || hi != 17 {
		t.Errorf("the crab on the knight: %d..%d, Sandstorm's 17 every time", lo, hi)
	}
	if lo, hi := rollRange(gharial, knight); lo != 18 || hi != 18 {
		t.Errorf("the gharial on the knight: %d..%d, Sandstorm's 18 every time", lo, hi)
	}
	if lo, hi := rollRange(crab, enchanter); lo != 17 || hi < 45 {
		t.Errorf("the crab on the enchanter: %d..%d, Sandstorm's 17 to 45", lo, hi)
	}
	// no hit is less than one, whatever the level
	if lo, _ := rollRange(creatureRoll(0), &Player{}); lo != 1 {
		t.Errorf("a level 0 creature's least: %d, want 1", lo)
	}
}

// The dummies every tank measured took the floor from, every time: a shadow
// knight of 1763 AC every Training Dummy to Lv55 and the Regular Lv55; a
// bard of 1430 AC and 207 AGI the Training Dummies to Lv45.
func TestDummyFloorsOnTanks(t *testing.T) {
	knight := &Player{Class: 3, AC: 1763, Agi: 95}
	bard := &Player{Class: 5, AC: 1430, Agi: 207}
	floors := map[int]int{20: 8, 25: 10, 30: 12, 35: 14, 45: 24, 50: 21, 55: 30}
	for level, f := range floors {
		r := npcMeleeRoll(&NPC{Level: level, Dummy: TrainingDummy})
		if lo, hi := rollRange(r, knight); lo != f || hi != f {
			t.Errorf("Training Dummy Lv%d on the knight: %d..%d, Sandstorm's %d", level, lo, hi, f)
		}
		if level <= 45 {
			if lo, hi := rollRange(r, bard); lo != f || hi != f {
				t.Errorf("Training Dummy Lv%d on the bard: %d..%d, Sandstorm's %d", level, lo, hi, f)
			}
		}
	}
	if lo, hi := rollRange(npcMeleeRoll(&NPC{Level: 55, Dummy: RegularDummy}), knight); lo != 30 || hi != 30 {
		t.Errorf("Regular Dummy Lv55 on the knight: %d..%d, Sandstorm's 30", lo, hi)
	}
}
