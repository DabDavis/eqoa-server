package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A login takes the row's purse as stored: a character who has spent and
// banked down to 0 stays at 0 (no refill; the exploit closed).
func TestStartingTunar(t *testing.T) {
	for _, c := range []struct{ tunar, bank, want int }{
		{0, 0, 0}, // played down to nothing: stays nothing
		{524, 0, 524},
		{0, 300, 0}, // all of it banked
		{12, 34, 12},
		{5000, 0, 5000},
	} {
		r := &database.CharacterRecord{Tunar: c.tunar, BankTunar: c.bank, OwnBankTunar: c.bank}
		if got := StartingTunar(r); got != c.want {
			t.Errorf("StartingTunar(%d, %d) = %d, want %d", c.tunar, c.bank, got, c.want)
		}
	}
}
