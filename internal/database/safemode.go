package database

import (
	"fmt"
	"time"
)

// charsafemode: the characters a player put in safe mode with /testinvis
// (handlers/safemode.go, entity/safemode.go: no creature attacks them on
// sight), a row each, kept until /testinvis is typed again so the mode
// outlives a logout and any restart. since is when (UTC, RFC 3339).
const charSafeModeSchema = `
	CREATE TABLE IF NOT EXISTS charsafemode (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT ''
	);`

// ensureCharSafeMode makes table charsafemode (initSchema).
func (db *SQLiteDB) ensureCharSafeMode() error {
	if _, err := db.Exec(charSafeModeSchema); err != nil {
		return fmt.Errorf("charsafemode: %w", err)
	}
	return nil
}

// SafeMode reports whether the character is in safe mode (it has its row).
func (db *SQLiteDB) SafeMode(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charsafemode WHERE serverid = ?`, serverID).Scan(&n)
	return n > 0, err
}

// SetSafeMode puts the character in safe mode (on) or takes it out.
func (db *SQLiteDB) SetSafeMode(serverID uint32, on bool) error {
	if !on {
		_, err := db.Exec(`DELETE FROM charsafemode WHERE serverid = ?`, serverID)
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO charsafemode (serverid, since) VALUES (?, ?)`,
		serverID, time.Now().UTC().Format(time.RFC3339))
	return err
}
