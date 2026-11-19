package entity

import (
	"testing"
	"time"
)

// A new pet takes none of the old one's buffs (Live petition 5254:
// "When casting a new pet, while a pet is already casted, the new pet should
// not inherit the old pet's buffs").
func TestANewPetTakesNoneOfTheOldOnesBuffs(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	own := pet.MaxHP
	giant := wbSpell(9202, 20, 1, "Test Colossus", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":600,"stack":1,"level":20,
		"effects":[{"id":"0002","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300},
			{"id":"0003","type":"Statistic Change","kind":"stat","stat":"ac","amount":50}]}`)
	r.w.mu.Lock()
	r.w.landOnPetLocked(&castState{spell: giant, caster: r.p.ServerID, targets: []EntityID{pet.ServerID}}, r.p, r.sess, pet, r.at(20*time.Second))
	r.w.mu.Unlock()
	if len(pet.Effects) == 0 || pet.MaxHP != own+300 {
		t.Fatalf("the buff did not take: %d effects, max %d", len(pet.Effects), pet.MaxHP)
	}
	r.p.Power = r.p.MaxPower
	pet2, _ := r.summon(t, fireElemental(), 40*time.Second)
	if pet2 == pet {
		t.Fatal("no new pet")
	}
	r.tick(60 * time.Second)
	r.w.mu.Lock()
	fx := r.w.petEffectSetLocked(r.p, r.at(60*time.Second))
	r.w.mu.Unlock()
	if len(pet2.Effects) != 0 || len(fx) != 0 || pet2.MaxHP != own {
		t.Errorf("the new pet has %d effects (%d shown), max %d (own %d)", len(pet2.Effects), len(fx), pet2.MaxHP, own)
	}
}
