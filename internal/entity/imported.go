package entity

import (
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// imported.go — imported gear's damper where a record's figures do not hold
// it (database/imported.go: a transfer's gear is a copy of its record at
// database.ImportedShare of its stats). A weapon's proc and a worn item's
// hit effect are read by the record's name or id (weaponproc.go,
// wornshield.go), the copy's being its base's name, so they are scaled here
// for a copy: its damage, heal and power figures, never its chance.

// importedItems is every imported copy's base (copy -> base), nil for none.
var importedItems atomic.Pointer[map[int]int]

// SetImportedItems names the imported copies (database.ImportedMap), at the
// start and when a transfer makes more.
func SetImportedItems(m map[int]int) { importedItems.Store(&m) }

// ImportedBase is the record an imported copy was made from; false for a
// record that is none.
func ImportedBase(patternID int) (int, bool) {
	m := importedItems.Load()
	if m == nil {
		return 0, false
	}
	b, ok := (*m)[patternID]
	return b, ok
}

// IsImported reports whether a record is an imported copy.
func IsImported(patternID int) bool {
	_, ok := ImportedBase(patternID)
	return ok
}

// imported is the proc as an imported copy's weapon has it.
func (wp WeaponProc) imported() WeaponProc {
	f := database.ImportedFigure
	wp.Damage, wp.DamageTo, wp.Power, wp.RoundMost = f(wp.Damage), f(wp.DamageTo), f(wp.Power), f(wp.RoundMost)
	return wp
}

// imported is the hit effect as an imported copy's worn item has it.
func (s WornShield) imported() WornShield {
	f := database.ImportedFigure
	s.Damage, s.DamageTo, s.Heal, s.Power = f(s.Damage), f(s.DamageTo), f(s.Heal), f(s.Power)
	s.DamageMax, s.HealMax, s.HealPct = f(s.DamageMax), f(s.HealMax), f(s.HealPct)
	return s
}
