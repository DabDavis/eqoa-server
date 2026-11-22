package entity

import (
	"database/sql"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// ascended.go — Ascended gear's tenth more where a record's figures do not
// hold it (database/ascended.go: a tuned raid boss's drop is a copy of its
// record, "Ascended <name>", each stat a tenth more; petition 1446). A
// weapon's proc and a worn item's hit effect are read by the record's name or
// id (weaponproc.go, wornshield.go), so an Ascended record's are its base's,
// read by its base's record and name, and scaled here: their damage, heal and
// power figures a tenth more (database.AscendedFigure), never their chance.
// An import's copy of an Ascended record is both: a tenth more, then damped.

// ascendedItems is every Ascended record's base (copy -> base), nil for none.
var ascendedItems atomic.Pointer[map[int]int]

// SetAscendedItems names the Ascended records (database.AscendedMap), at the
// start, every minute after (a tool's new records need no restart) and when
// a quest's reward makes one.
func SetAscendedItems(m map[int]int) { ascendedItems.Store(&m) }

// LoadAscendedItems reads the Ascended records out of db (SetAscendedItems)
// and says how many there are.
func LoadAscendedItems(db *sql.DB) (int, error) {
	m, err := database.AscendedMap(db)
	if err != nil {
		return 0, err
	}
	SetAscendedItems(m)
	return len(m), nil
}

// AscendedBase is the record an Ascended record was made from; false for a
// record that is none.
func AscendedBase(patternID int) (int, bool) {
	m := ascendedItems.Load()
	if m == nil {
		return 0, false
	}
	b, ok := (*m)[patternID]
	return b, ok
}

// AscendedItems is every Ascended record's base as last read (copy -> base;
// never written to), nil for none.
func AscendedItems() map[int]int {
	if m := ascendedItems.Load(); m != nil {
		return *m
	}
	return nil
}

// IsAscended reports whether a record is an Ascended record.
func IsAscended(patternID int) bool {
	_, ok := AscendedBase(patternID)
	return ok
}

// copyRecord is what a record's proc and hit effect are read by and how they
// scale: an import's copy is its base's, damped; an Ascended record its base's
// (by its base's name), a tenth more; an import of an Ascended record both.
func copyRecord(patternID int, name string) (record int, lookup string, imported, ascended bool) {
	record, lookup = patternID, name
	if b, ok := ImportedBase(record); ok {
		record, imported = b, true
	}
	if b, ok := AscendedBase(record); ok {
		record, ascended = b, true
		lookup = database.AscendedBaseName(lookup)
	}
	return record, lookup, imported, ascended
}

// ascended is the proc as an Ascended record's weapon has it.
func (wp WeaponProc) ascended() WeaponProc {
	f := database.AscendedFigure
	wp.Damage, wp.DamageTo, wp.Power, wp.RoundMost = f(wp.Damage), f(wp.DamageTo), f(wp.Power), f(wp.RoundMost)
	return wp
}

// ascended is the hit effect as an Ascended record's worn item has it.
func (s WornShield) ascended() WornShield {
	f := database.AscendedFigure
	s.Damage, s.DamageTo, s.Heal, s.Power = f(s.Damage), f(s.DamageTo), f(s.Heal), f(s.Power)
	s.DamageMax, s.HealMax, s.HealPct = f(s.DamageMax), f(s.HealMax), f(s.HealPct)
	return s
}
