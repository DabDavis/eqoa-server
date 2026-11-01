package entity

import (
	"testing"
	"time"
)

// A pet's maximum hit points take its buffs (the user, 2026-11-01, of the
// shaman's Avatar: "a one time max hp buff and massive stat buff"): a spell
// raising the maximum by 300 and healing 300, on a pet at full health,
// raises it and heals it to the new maximum, once; the raise lasts the
// effect's time, the balance setting the pet's hit points again keeps it,
// and at the effect's end the maximum and the hit points come back down.
// The control: a heal alone on the pet at full health changes nothing.
// Before the change the raise was never made, so the pet stayed at its own
// maximum, full, and the heal was lost.
func TestAPetsMaximumTakesItsBuffs(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	own := pet.MaxHP
	if pet.HP != own || own <= 0 {
		t.Fatalf("the pet came at %d/%d", pet.HP, own)
	}
	land := func(sp CastSpell, at time.Duration) {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		cs := &castState{spell: sp, caster: r.p.ServerID, targets: []EntityID{pet.ServerID}}
		r.w.landOnPetLocked(cs, r.p, r.sess, pet, r.at(at))
	}
	heal := wbSpell(9201, 20, 1, "Test Mending", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300}]}`)
	land(heal, 20*time.Second)
	if pet.MaxHP != own || pet.HP != own {
		t.Fatalf("the control: a heal on a full pet left it %d/%d; want %d/%d", pet.HP, pet.MaxHP, own, own)
	}
	giant := wbSpell(9202, 20, 1, "Test Colossus", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":60,"stack":1,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300}]}`)
	land(giant, 21*time.Second)
	if pet.MaxHP != own+300 || pet.HP != own+300 {
		t.Fatalf("at its landing: %d/%d; want %d/%d (raised, then healed to it)", pet.HP, pet.MaxHP, own+300, own+300)
	}
	r.tick(40 * time.Second)
	if pet.MaxHP != own+300 || pet.HP != own+300 {
		t.Errorf("19 s on: %d/%d; want %d/%d, no second heal and the raise kept", pet.HP, pet.MaxHP, own+300, own+300)
	}
	r.w.mu.Lock()
	r.w.rebalanceLocked(pet) // a balance table saved under the buff
	r.w.mu.Unlock()
	if pet.MaxHP != own+300 {
		t.Errorf("after the balance set its hit points again: maximum %d; want %d", pet.MaxHP, own+300)
	}
	r.tick(82 * time.Second) // past its minute
	if pet.MaxHP != own || pet.HP != own || len(pet.Effects) != 0 {
		t.Errorf("after its end: %d/%d with %d effects; want %d/%d, none", pet.HP, pet.MaxHP, len(pet.Effects), own, own)
	}
}
