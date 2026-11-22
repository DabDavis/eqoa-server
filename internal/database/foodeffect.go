package database

import (
	"database/sql"
	"fmt"
)

// foodeffect: how long a meal or a drink lasts and what ends it, by item
// (entity/consumable.go). EQOAEmu's workbook gives each food and drink its
// time and its DisspellOnCast and DisspellOnContact (the import of
// 2026-11-01, the user: "yes add the food too"; tools/import_eqoaemu_spells.py
// writes them, source "eqoaemu"). duration_s 0 is the minute
// (entity.ConsumableDuration); ends_on_cast: any spell its eater casts ends
// it; ends_on_contact: a fight ends it (auto-attack engaged, a blow landed
// or taken), and it is refused in one. An item with no row keeps the minute,
// a meal ending in a fight and a drink running through one.
const foodEffectSchema = `
	CREATE TABLE IF NOT EXISTS foodeffect (
		patternid       INTEGER PRIMARY KEY,
		duration_s      INTEGER NOT NULL DEFAULT 0,
		ends_on_cast    INTEGER NOT NULL DEFAULT 0,
		ends_on_contact INTEGER NOT NULL DEFAULT 0,
		source          TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureFoodEffect makes table foodeffect (initSchema).
func (db *SQLiteDB) ensureFoodEffect() error {
	if _, err := db.Exec(foodEffectSchema); err != nil {
		return fmt.Errorf("foodeffect: %w", err)
	}
	return nil
}

// FoodEffectRow is an item's row of foodeffect.
type FoodEffectRow struct {
	DurationS     int
	EndsOnCast    bool
	EndsOnContact bool
}

// FoodEffect is an item's foodeffect row; ok is false for an item with none.
// Read at every use, so a GM's change counts at the next one.
func (s *InventoryStore) FoodEffect(patternID int) (row FoodEffectRow, ok bool, err error) {
	var cast, contact int
	err = s.db.QueryRow(`SELECT duration_s, ends_on_cast, ends_on_contact FROM foodeffect WHERE patternid = ?`, patternID).
		Scan(&row.DurationS, &cast, &contact)
	if err == sql.ErrNoRows {
		return FoodEffectRow{}, false, nil
	}
	if err != nil {
		return FoodEffectRow{}, false, err
	}
	row.EndsOnCast, row.EndsOnContact = cast != 0, contact != 0
	return row, true, nil
}
