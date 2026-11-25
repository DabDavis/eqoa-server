package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 3127 ("my merc troll bard has the cm troll pet and the
// pet is attacking"): a troll bard merc of 30, no summoning in its book but
// its race's Swamp Friend 3, summoned that pet (the workbook's non-combat
// sort: 90 HP, no ability) and sent it at every creature its hirer fought.
// A merc's non-combat pet takes no target, its hirer swinging or not; and a
// merc whose only summoning makes such a pet summons nothing. The control: the
// same book with no workbook row summons its pet (mercWithPet).
func TestMercNonCombatPetFightsNothing(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	ps := ms.pet
	crab := r.creature("crab", 28, 8)
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	r.w.mu.Lock()
	ps.npc.sheet = &petSheet{PetStats: database.PetStats{Name: "Swamp Friend", Type: database.PetTypeNonCombat}}
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	ms.ordered, ms.target = crab.ServerID, crab.ServerID
	r.w.mu.Unlock()
	r.run(10*time.Second, 12*time.Second)
	if ps.target != 0 {
		t.Errorf("a merc's non-combat pet took target %d", ps.target)
	}

	r2 := newMercRig(t)
	r2.w.mu.Lock()
	r2.w.templates.sheets = map[int]*petSheet{901798: {PetStats: database.PetStats{Name: "Swamp Friend", Type: database.PetTypeNonCombat}}}
	r2.w.mu.Unlock()
	swamp := bookSpell(901798, "Swamp Friend 3", 0, 3*time.Second, 5*time.Second, 10, "Summons a swamp friend to aid you")
	swamp.Level = 6
	kit := r2.kit(10, 0, 0, swamp)
	kit.RacialPets = []int{901798}
	if MercPetFor(kit) == nil {
		t.Fatal("the kit's book has no summoning: the test tests nothing")
	}
	ms2 := r2.hire(kit, MercAssist)
	r2.run(0, 10*time.Second)
	if ms2.pet != nil {
		t.Errorf("a merc summoned %q, a pet that does nothing", ms2.pet.npc.Name)
	}
	if ms2.npc.Power < ms2.npc.MaxPower {
		t.Errorf("the merc's power %d/%d: it cast the summoning", ms2.npc.Power, ms2.npc.MaxPower)
	}
	r2.w.mu.Lock()
	want := r2.w.mercPetWantLocked(ms2)
	r2.w.mu.Unlock()
	if want != nil {
		t.Errorf("the merc wants %q, a pet that does nothing", want.Name)
	}
}
