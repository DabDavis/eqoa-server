package entity

import (
	"testing"
	"time"
)

// An enchanter's pet's taunt gives 70% of the threat it gave (petition 1928, pettaunthate.go): beside
// a creature a player has 10,000 threat on, another's pet's taunt takes it on top (10,001 and its hit);
// an enchanter's pet's taunt gives 70% of that, and the creature stays on the player. A dummy-free rig;
// the hit on fixed dice.
func TestAnEnchantersPetsTauntGivesSeventyPercent(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	const other EntityID = 7777
	got := map[bool]int64{}
	for _, enchanter := range []bool{false, true} {
		r := newTagRig(t)
		r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
		r.npc.Threats = map[EntityID]int64{other: 10_000}
		r.npc.CurrentEnemy = other
		r.ps.enchanter = enchanter
		r.w.mu.Lock()
		r.w.petTauntLocked(r.ps, r.npc, r.t0)
		r.w.mu.Unlock()
		got[enchanter] = r.npc.Threats[r.pet.ServerID]
		switch {
		case !enchanter && (got[false] <= 10_001 || r.npc.CurrentEnemy != r.pet.ServerID):
			t.Errorf("another's pet's taunt: threat %d, enemy %d: want above 10001 and the pet", got[false], r.npc.CurrentEnemy)
		case enchanter && r.npc.CurrentEnemy != other:
			t.Errorf("an enchanter's pet's taunt at %d took the creature (%d) from the player's 10000", got[true], r.npc.CurrentEnemy)
		}
	}
	if want := pctOf(got[false], 70); got[true] != want {
		t.Errorf("an enchanter's pet's taunt gave %d threat, want 70%% of %d: %d", got[true], got[false], want)
	}
}

// A templated taunt (an animation's, the "pet animation" row) is cut the same way for an enchanter's
// pet, and left whole for another's (a magician's earth pet: TestTheEarthPetTauntsTanksAndBuffsItself).
func TestAnEnchantersTemplatedPetsTauntIsCut(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	got := map[bool]int64{}
	for _, enchanter := range []bool{false, true} {
		r := newTagRig(t)
		r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
		r.npc.Threats = map[EntityID]int64{7777: 10_000}
		r.ps.enchanter = enchanter
		r.w.mu.Lock()
		r.w.petTemplateTauntLocked(r.ps, &templateAbility{}, r.npc, r.t0) // no figure: a melee hit at 100%
		r.w.mu.Unlock()
		got[enchanter] = r.npc.Threats[r.pet.ServerID]
	}
	if got[false] <= 10_001 || got[true] != pctOf(got[false], 70) {
		t.Errorf("a templated taunt: %d for another's pet, %d for an enchanter's: want 70%% of the first", got[false], got[true])
	}
}

// Petition 5034 (the GM Jibrish): a magician's earth pet's taunt gives 95% of the threat it gave, the
// raise over the top with its margin and the taunt's hit alike (pettaunthate.go); a pet of neither
// mark is left whole, an enchanter's still 70%.
func TestAnEarthPetsTauntGivesNinetyFivePercent(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	got := map[string]int64{}
	for _, kind := range []string{"other", "earth", "enchanter"} {
		r := newTagRig(t)
		r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
		r.npc.Threats = map[EntityID]int64{7777: 10_000}
		r.ps.earth, r.ps.enchanter = kind == "earth", kind == "enchanter"
		r.w.mu.Lock()
		r.w.petTemplateTauntLocked(r.ps, &templateAbility{}, r.npc, r.t0)
		r.w.mu.Unlock()
		got[kind] = r.npc.Threats[r.pet.ServerID]
	}
	if got["other"] <= 10_001 || got["earth"] != pctOf(got["other"], 95) || got["enchanter"] != pctOf(got["other"], 70) {
		t.Errorf("a templated taunt: %d whole, %d an earth pet's, %d an enchanter's: want 95%% and 70%% of the first",
			got["other"], got["earth"], got["enchanter"])
	}
}

// A magician's earth pets (the "pet earth" template's: Earth Elemental) are marked earth at the
// summoning; a water or air elemental is not, nor the rig's Badi (its own "pet badi" template).
func TestTheMagiciansEarthPetsAreMarkedEarth(t *testing.T) {
	for _, c := range []struct {
		spell CastSpell
		class int
		want  bool
	}{
		{earthElemental, classMagician, true},
		{summonBadi, classMagician, false},
		{waterElemental, classMagician, false},
		{airElemental, classMagician, false},
	} {
		r, _ := newPetTemplateRig(t)
		f := petTemplateFight(t, r, c.spell, c.class, 100*time.Millisecond, false)
		if f.ps == nil || f.ps.earth != c.want {
			t.Errorf("%s by class %d: earth %v, want %v", c.spell.Name, c.class, f.ps != nil && f.ps.earth, c.want)
		}
	}
}
