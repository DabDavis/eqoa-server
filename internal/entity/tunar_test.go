package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestStartingTunar(t *testing.T) {
	for _, c := range []struct{ tunar, bank, want int }{
		{0, 0, DefaultTunar}, // never stored
		{524, 0, 524},
		{0, 300, 0}, // all of it banked
		{12, 34, 12},
	} {
		if got := StartingTunar(&database.CharacterRecord{Tunar: c.tunar, BankTunar: c.bank}); got != c.want {
			t.Errorf("StartingTunar(%d, %d) = %d, want %d", c.tunar, c.bank, got, c.want)
		}
	}
}
