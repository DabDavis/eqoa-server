package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// An enchanter's pet swings once a round whatever its level, Grand
// Animation twice, and a magician's as a warrior of its level (F3
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
		t.Errorf("a magician's level 44 Fire Elemental: %d hits (%s), fixed %v; want a warrior's Attack4", n, fx, fe.SwingFixed)
	}
}

// An enchanter's pet taunts its target every third round it fights it: the
// creature, fighting the enchanter, turns on the pet and takes one more hit
// in that round (Grand Animation's 500). The control: a magician's Fire
// Elemental never taunts, and the creature stays on its owner.
func TestAnEnchantersPetTauntsEveryThirdRound(t *testing.T) {
	golem := CastSpell{SpellID: 586, DisplayIndex: 3, Level: 44, Scope: 0, Name: "Iron Golem",
		CastTime: 5 * time.Second, PowerCost: 887, Def: ParseSpellText("Summons an Iron Golem to your defense")}
	fight := func(r *petRig, pet *NPC, rounds int) (enemyAt []EntityID, hits int) {
		r.enemy.Level, r.enemy.HP, r.enemy.MaxHP = 30, 1_000_000, 1_000_000
		// it fights the owner, far above anything a pet's hits add: only a
		// taunt turns it
		r.w.updateAggro(r.enemy, r.p.ServerID, 1_000_000)
		ps := r.w.pets[r.p.ServerID]
		ps.target = r.enemy.ServerID
		pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
		length, _, _, _ := npcRound(pet)
		start := 20 * time.Second
		for i := 0; i < rounds; i++ {
			for d := time.Duration(0); d < length; d += 100 * time.Millisecond {
				r.w.mu.Lock()
				r.w.petSystem(r.at(start + time.Duration(i)*length + d))
				r.w.mu.Unlock()
			}
			enemyAt = append(enemyAt, r.enemy.CurrentEnemy)
		}
		for _, m := range r.sent() {
			if opOfMsg(m) == protocol.OpcodeDamageIndication {
				hits++
			}
		}
		return enemyAt, hits
	}

	r := newPetRig(t)
	r.p.Class, r.p.Level = classEnchanter, 49
	pet, _ := r.summon(t, golem, 0)
	r.sent()
	enemyAt, hits := fight(r, pet, 3)
	if enemyAt[0] != r.p.ServerID || enemyAt[1] != r.p.ServerID || enemyAt[2] != pet.ServerID {
		t.Errorf("the creature's enemy round by round %v, want the owner %d twice then the pet %d", enemyAt, r.p.ServerID, pet.ServerID)
	}
	if hits != 3+1 {
		t.Errorf("%d hits in three rounds of one, want three and the taunt's", hits)
	}

	g := newPetRig(t)
	g.p.Class, g.p.Level = classEnchanter, 60
	grand, _ := g.summon(t, CastSpell{SpellID: 9001, DisplayIndex: 4, Level: 60, Scope: 0, Name: "Grand Animation",
		CastTime: 5 * time.Second, PowerCost: 900, Def: ParseSpellText("Summons a pet")}, 0)
	g.sent()
	before := 1_000_000
	fight(g, grand, 3)
	if lost := before - g.enemy.HP; lost < grandAnimationTaunt {
		t.Errorf("Grand Animation's three rounds took %d, less than its taunt's %d", lost, grandAnimationTaunt)
	}

	m := newPetRig(t) // a magician's pet
	fe, _ := m.summon(t, fireElemental(), 0)
	m.sent()
	enemyAt, _ = fight(m, fe, 3)
	if enemyAt[2] != m.p.ServerID {
		t.Errorf("the control: a magician's pet turned the creature (%v)", enemyAt)
	}
}
