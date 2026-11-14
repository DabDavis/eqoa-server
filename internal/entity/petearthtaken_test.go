package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Live petition 5057: an earth pet takes a creature's melee at the warrior's share of its owner's
// level (petearthtaken.go): 58 at 55% is 32; another pet takes it whole, as does an earth pet with
// no balance.
func TestAnEarthPetTakesATanksShare(t *testing.T) {
	for _, c := range []struct {
		earth, balance bool
		want           int
	}{{true, true, 32}, {false, true, 58}, {true, false, 58}} {
		r := newTagRig(t)
		if c.balance {
			r.w.balance = SeedBalance().WithClasses(nil).WithClassLevels([]database.BalanceClassLevel{
				{Class: classWarrior, Level: 1, MeleeTakenPct: 55}, {Class: classWarrior, Level: 60, MeleeTakenPct: 55}})
		}
		r.ps.earth = c.earth
		r.w.mu.Lock()
		got := r.w.petTakenLocked(r.pet, 58)
		r.w.mu.Unlock()
		if got != c.want {
			t.Errorf("earth %v, balance %v: took %d of 58, want %d", c.earth, c.balance, got, c.want)
		}
	}
}
