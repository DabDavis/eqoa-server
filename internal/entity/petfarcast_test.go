package entity

import (
	"testing"
	"time"
)

// A magician's petition (2026-11-19): the Ghilan Noble sent at a creature from
// a distance "should be using either her debuff spell or her nuke depending on
// what is available as soon as she is in the spells range. Currently she waits
// until she physically strikes her opponent before casting." Sent at the crab
// from 25 off, her water nuke (range 30), due, is begun where she stands, out of her
// melee reach, and she walks on after it; with the nuke's range cut to 3 (less
// than her reach) she casts it only once beside the crab.
func TestAPetCastsAtItsFoeOnceInItsSpellsRange(t *testing.T) {
	firstNuke := func(nukeRange float32) (ms int, d float32) {
		r := ghilanRig(t)
		pet, _ := r.summon(t, summonGhilanNoble, 0)
		ps := r.w.pets[r.p.ServerID]
		for i := range ps.petAbilities() {
			a := &ps.petAbilities()[i]
			a.first = time.Hour // the rest of her kit not due
			if s := a.spellFor(petSpellClass(a, pet), pet.Level); s != nil && s.SpellID == 902166 {
				s.Range = nukeRange
				a.first = 0
			}
		}
		r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
		ox, oz := pet.X, pet.Z
		pet.X, pet.Y, pet.Z = r.enemy.X-25, r.enemy.Y, r.enemy.Z
		r.w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
		r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(9600*time.Millisecond))
		for ms := 9700; ms <= 30000; ms += 100 {
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			for _, c := range r.w.petCasts {
				if c.spell.SpellID == 902166 {
					d := distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z)
					r.w.mu.Unlock()
					return ms, d
				}
			}
			r.w.mu.Unlock()
		}
		return 0, 0
	}
	ms, d := firstNuke(30)
	if ms == 0 || d <= petReach {
		t.Fatalf("range 30: the nuke begun at %d ms, %.1f off the crab; want it begun out of her reach (%g)", ms, d, petReach)
	}
	if ms2, d2 := firstNuke(3); ms2 == 0 || d2 > petReach || ms2 <= ms {
		t.Errorf("range 3: the nuke begun at %d ms, %.1f off; want it only in her reach, after %d ms", ms2, d2, ms)
	}
}
