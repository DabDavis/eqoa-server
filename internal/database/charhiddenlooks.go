package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// charhiddenlooks: the equip slots whose look a character hides (the owner,
// 2026-11-11: "lets add a hide display option to equipped armor/weapon, still
// equipped, but no model displayed"; entity/hidelook.go). locs is a bit per
// equip slot (CLIENT's equip locations 0-21, the cape's 22): bit n set, what is
// worn in slot n is sent as nothing worn. A character with no row hides
// nothing; a row is taken away when its last slot is shown again. Set by the
// player from a client that offers it (protocol.OpcodeHideLookSet).
const charHiddenLooksSchema = `
	CREATE TABLE IF NOT EXISTS charhiddenlooks (
		serverid INTEGER PRIMARY KEY,
		locs     INTEGER NOT NULL
	);`

// ensureCharHiddenLooks makes table charhiddenlooks (initSchema).
func (db *SQLiteDB) ensureCharHiddenLooks() error {
	if _, err := db.Exec(charHiddenLooksSchema); err != nil {
		return fmt.Errorf("charhiddenlooks: %w", err)
	}
	return nil
}

// CharHiddenLooks is the equip slots whose look the character hides, a bit
// each; 0 none.
func (db *SQLiteDB) CharHiddenLooks(serverID uint32) (uint32, error) {
	var locs int64
	err := db.QueryRow(`SELECT locs FROM charhiddenlooks WHERE serverid = ?`, serverID).Scan(&locs)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return uint32(locs), nil
}

// SetCharHiddenLooks keeps the character's hidden equip slots (0 takes its
// row away).
func (db *SQLiteDB) SetCharHiddenLooks(serverID uint32, locs uint32) error {
	if err := db.ensureCharHiddenLooks(); err != nil {
		return err
	}
	if locs == 0 {
		_, err := db.Exec(`DELETE FROM charhiddenlooks WHERE serverid = ?`, serverID)
		return err
	}
	_, err := db.Exec(`INSERT INTO charhiddenlooks (serverid, locs) VALUES (?, ?)
		ON CONFLICT(serverid) DO UPDATE SET locs = excluded.locs`, serverID, int64(locs))
	return err
}
