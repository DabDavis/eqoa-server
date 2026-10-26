package entity

import (
	"testing"
	"time"
)

// A summoned pet fights as a warrior of its level whatever creature it is
// made after (F3 pass 18, the user: "at level 35 and above melee/war classes
// quad attack, and many pets/mobs are missing that"): a necromancer's level
// 40 pet made after a skeleton Return Home's npcclass row calls a
// necromancer swings Attack4, four hits a round. The controls: its template
// at the pet's level swings once (what the pet did when it took the
// template's class); an enchanter's pet of 44 still swings once; a charmed
// creature keeps its own class (a wizard crab of 34 swings once, where a
// warrior's Attack2 would be two).
func TestASummonedPetFightsAsAWarrior(t *testing.T) {
	hits := func(n *NPC) (int, string) {
		_, _, _, h := npcRound(n)
		return len(h), SwingEffectName(npcSwingKind(n))
	}

	r := newPetRig(t)
	r.p.Class, r.p.Level = 11, 45 // a necromancer
	skeleton := &NPC{ServerID: 3102, Name: "Skeleton", Level: 8, Class: 11, ModelID: 0x6AF98A81, Size: 1, X: 20000, Z: 20300}
	r.w.NPCs[skeleton.ServerID] = skeleton
	r.w.SpatialIndex.Insert(skeleton.ServerID, skeleton.X, skeleton.Z)
	bones := CastSpell{SpellID: 9100, DisplayIndex: 5, Level: 40, Scope: 0, Name: "Cavorting Bones",
		CastTime: 5 * time.Second, PowerCost: 500, Def: ParseSpellText("Summons a pet")}
	pet, _ := r.summon(t, bones, 0)
	if pet == nil || pet.Level != 40 || pet.ModelID != skeleton.ModelID {
		t.Fatalf("the necromancer's pet %+v, want a level 40 in the skeleton's body", pet)
	}
	if n, fx := hits(pet); pet.SwingFixed || pet.Class != 0 || n != 4 || fx != "Attack4" {
		t.Errorf("the necromancer's level 40 pet: %d hits (%s), class %d, fixed %v; want a warrior's Attack4", n, fx, pet.Class, pet.SwingFixed)
	}
	as := *skeleton
	as.Level = pet.Level
	if n, fx := hits(&as); n != 1 || fx != "Attack1" {
		t.Errorf("the control: its template at 40 swings %d (%s), want a caster's one", n, fx)
	}

	e := newPetRig(t)
	e.p.Class, e.p.Level = classEnchanter, 49
	golem, _ := e.summon(t, CastSpell{SpellID: 586, DisplayIndex: 3, Level: 44, Scope: 0, Name: "Iron Golem",
		CastTime: 5 * time.Second, PowerCost: 887, Def: ParseSpellText("Summons an Iron Golem to your defense")}, 0)
	if n, fx := hits(golem); n != 1 || fx != "Attack1" {
		t.Errorf("the enchanter's level 44 pet swings %d (%s), want one", n, fx)
	}

	c, _, _ := charmRig(t)
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	crab := c.enemy
	crab.Class = 13
	if began, _ := castCharm(t, c, captivation(), crabID, 0); !began || crab.Owner != c.p.ServerID {
		t.Fatalf("the charm did not take the crab: began %v, owner %d", began, crab.Owner)
	}
	if n, fx := hits(crab); crab.Class != 13 || n != 1 || fx != "Attack1" {
		t.Errorf("the charmed wizard crab of 34: class %d, %d hits (%s); want its own, one", crab.Class, n, fx)
	}
}
