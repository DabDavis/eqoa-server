package entity

import (
	"testing"
	"time"
)

// An enchanter's pet swings once a round whatever its level, Grand
// Animation twice, and a magician's as the creature its body is (F3
// 20261025-164209: "Enchanter pets currently dual hit (20+) and quad"). The
// control: the same level 44 golem without the enchanter's swing makes the
// report's four hits a round (a warrior's Attack4 from level 35).
func TestAnEnchantersPetSwingsOnceARound(t *testing.T) {
	hits := func(pet *NPC) (int, string, time.Duration) {
		length, _, _, h := npcRound(pet)
		return len(h), SwingEffectName(npcSwingKind(pet)), length
	}
	golem := CastSpell{SpellID: 586, DisplayIndex: 3, Level: 44, Scope: 0, Name: "Iron Golem",
		CastTime: 5 * time.Second, PowerCost: 887, Def: ParseSpellText("Summons an Iron Golem to your defense")}
	grand := CastSpell{SpellID: 9001, DisplayIndex: 4, Level: 60, Scope: 0, Name: "Grand Animation",
		CastTime: 5 * time.Second, PowerCost: 900, Def: ParseSpellText("Summons a pet")}

	r := newPetRig(t)
	r.p.Class, r.p.Level = classEnchanter, 49
	pet, _ := r.summon(t, golem, 0)
	if n, fx, length := hits(pet); n != 1 || fx != "Attack1" || length != 3200*time.Millisecond || pet.Level != 44 {
		t.Errorf("the enchanter's level %d Iron Golem: %d hits a round (%s, %v), want one (Attack1, 3.2 s)", pet.Level, n, fx, length)
	}
	plain := *pet
	plain.SwingFixed = false
	if n, fx, _ := hits(&plain); n != 4 || fx != "Attack4" {
		t.Errorf("the control: without the enchanter's swing the golem makes %d hits (%s), want the report's four", n, fx)
	}

	pet, _ = r.summon(t, grand, 20*time.Second)
	if n, fx, _ := hits(pet); n != 2 || fx != "Attack2" {
		t.Errorf("Grand Animation: %d hits a round (%s), want two (Attack2)", n, fx)
	}

	m := newPetRig(t) // a magician of 45
	fe, _ := m.summon(t, fireElemental(), 0)
	if n, fx, _ := hits(fe); fe.SwingFixed || n != 4 || fx != "Attack4" {
		t.Errorf("a magician's level 44 Fire Elemental: %d hits (%s), fixed %v; want its body's Attack4", n, fx, fe.SwingFixed)
	}
}
