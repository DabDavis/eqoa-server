package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petition 1952 (the GM Jibwiz): "Only the CM upgraded version of wizard familiars should
// attack when hit only. The default spell summoned familiar was permanently passive". The
// spell's familiar fights nothing: an attack command is refused and a creature fighting its
// master or it is left alone. The mastery's guardian refuses a command too, stays out of a
// creature's fight with another, and takes on a creature fighting it or its master.
func TestWizardFamiliarsFightOnlyAsGuardians(t *testing.T) {
	for _, c := range []struct {
		name, element string
		want          int
	}{
		{"Arch Familiar", "arcane", familiarPlain}, {"Gargoyle Familiar", "arcane", familiarPlain},
		{"Minor Familiar", "arcane", familiarPlain}, {"Arch Guardian", "arcane", familiarGuardian},
		{"Guardian", "arcane", familiarGuardian}, {"Gargoyle Guardian", "arcane", familiarGuardian},
		{"Menacing Defender", "", familiarNone}, {"Fire Elementaling", "fire", familiarNone},
		{"Mummified Guardian", "", familiarNone},
	} {
		s := &petSheet{PetStats: database.PetStats{Name: c.name, Element: c.element}}
		if got := s.familiar(); got != c.want {
			t.Errorf("%q: %d, want %d", c.name, got, c.want)
		}
	}

	for _, kind := range []string{"Arch Familiar", "Arch Guardian"} {
		r := newPetRig(t)
		pet, _ := r.summon(t, fireElemental(), 0)
		pet.sheet = &petSheet{PetStats: database.PetStats{Name: kind, Element: "arcane", Type: database.PetTypeMelee}}
		ps := r.w.pets[r.p.ServerID]
		ps.stance = petDefensive
		r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
		r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(10*time.Second))
		r.w.mu.Lock()
		if ps.target != 0 {
			t.Errorf("%s: told to attack, it took %d", kind, ps.target)
		}
		// a creature fighting another (no master, no familiar): left alone
		r.enemy.CurrentEnemy, r.enemy.Threats = 4242, map[EntityID]int64{4242: 10}
		r.w.petChooseTargetLocked(ps, r.at(11*time.Second))
		if ps.target != 0 {
			t.Errorf("%s: took on a creature fighting another", kind)
		}
		// fighting its master
		r.enemy.CurrentEnemy, r.enemy.Threats = r.p.ServerID, map[EntityID]int64{r.p.ServerID: 10}
		r.w.petChooseTargetLocked(ps, r.at(12*time.Second))
		want := EntityID(0)
		if kind == "Arch Guardian" {
			want = r.enemy.ServerID
		}
		if ps.target != want {
			t.Errorf("%s: a creature on its master: target %d, want %d", kind, ps.target, want)
		}
		// fighting it
		ps.target = 0
		r.enemy.CurrentEnemy, r.enemy.Threats = pet.ServerID, map[EntityID]int64{pet.ServerID: 10}
		r.w.petChooseTargetLocked(ps, r.at(13*time.Second))
		if ps.target != want {
			t.Errorf("%s: a creature on it: target %d, want %d", kind, ps.target, want)
		}
		r.w.mu.Unlock()
	}
}
