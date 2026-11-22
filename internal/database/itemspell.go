package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// itemspell.go — items whose Use casts a spell (work board 737's code ask 4, "charged items that cast a
// spell"; the owner, 2026-11-18: Lord Valkanith's Rune of the Bonewing, "100 charges pet looks like death
// knight pet. it gives 40 damage shield 20 heal shield and pr/dr buff +45", and his Rune of Defilement,
// an area power tap round its user; tools/valkanith_loot_20261118.py makes both rows). A row names the
// item record and the spell (spellpattern + spelleffect, as any ability's) its Use casts
// (handlers/itemspell.go): a summoning spell's pet comes at once, as the Stone of Guenhwyvar's does
// (entity.SummonByItem: its petstats row its body, numbers and the buffs it keeps on its owner), any other
// spell through the World's own casting as an item's (CastSpell.ItemCast: its cast time and landing, no
// lockout, no power). Each Use that casts or summons spends one of the item's charges (itemeffect.charges,
// the copy's remaincharge: the last takes the item), none when its pet is out already or within its rest
// (rest_s: the least time between two of one character's Uses of that item; 0 none). The item's class,
// race and level masks are asked as a worn item's are. Read at each Use: a GM's change counts at once.
const itemSpellSchema = `
	CREATE TABLE IF NOT EXISTS itemspell (
		patternid INTEGER PRIMARY KEY,
		spellid   INTEGER NOT NULL,
		rest_s    REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureItemSpells makes table itemspell (initSchema).
func (db *SQLiteDB) ensureItemSpells() error {
	if _, err := db.Exec(itemSpellSchema); err != nil {
		return fmt.Errorf("itemspell: %w", err)
	}
	return nil
}

// ItemSpellRow is an item that casts a spell, as table itemspell has it.
type ItemSpellRow struct {
	PatternID int
	SpellID   int
	Rest      time.Duration
}

// ItemSpell is the spell the item's Use casts, false for an item that casts none (or no table).
func (s *InventoryStore) ItemSpell(patternID int) (ItemSpellRow, bool, error) {
	var r ItemSpellRow
	var rest float64
	err := s.db.QueryRow(`SELECT patternid, spellid, rest_s FROM itemspell WHERE patternid = `+legacyRecordSQL, patternID, patternID).
		Scan(&r.PatternID, &r.SpellID, &rest)
	if errors.Is(err, sql.ErrNoRows) {
		return ItemSpellRow{}, false, nil
	}
	if err != nil {
		return ItemSpellRow{}, false, err
	}
	r.Rest = time.Duration(rest * float64(time.Second))
	return r, true, nil
}
