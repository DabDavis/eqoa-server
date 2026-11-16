package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// itemowner: an item record that one character alone may wear (the owner,
// 2026-11-16: "give drizzt a set of the plate 9 armor that he alone can wear
// drizzt only", then "lets replace drizzt armoe with scale mail";
// tools/drizzt_armor_20261116.py). Keyed by the record (itempattern.patternid),
// so every copy of it is bound: a copy that reached anyone else (the account
// bank, a merc's pack, a GM's give) stays in his hands, and the server refuses
// to put it on (handlers.cannotWear). serverid is the one character
// (characters.serverid); no row, anyone the record's own masks allow.
const itemOwnerSchema = `
	CREATE TABLE IF NOT EXISTS itemowner (
		patternid INTEGER PRIMARY KEY,
		serverid  INTEGER NOT NULL,
		note      TEXT NOT NULL DEFAULT ''
	);`

// ensureItemOwner makes table itemowner (initSchema).
func (db *SQLiteDB) ensureItemOwner() error {
	if _, err := db.Exec(itemOwnerSchema); err != nil {
		return fmt.Errorf("itemowner: %w", err)
	}
	return nil
}

// ItemOwner is the one character that may wear item patternID, ok false when
// anyone may (no row). Read each time it is asked (an equip, a need roll), so
// a row written with the server running is in force at once.
func (db *SQLiteDB) ItemOwner(patternID int) (serverID uint32, ok bool, err error) {
	var id int64
	err = db.QueryRow(`SELECT serverid FROM itemowner WHERE patternid = ?`, patternID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return uint32(id), true, nil
}
