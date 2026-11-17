package database

import (
	"fmt"
	"time"
)

// chargmnoaggro: the GM characters no creature attacks on sight, the TEST
// menu's Safe Mode or #aggro off (handlers/gmnoaggro.go; the owner,
// 2026-11-16: "also keep gm safemode on across restarts"), a row each, kept
// until Safe Mode or #aggro on turns it off, so the mode outlives a logout
// and every restart. since is when (UTC, RFC 3339).
const charGMNoAggroSchema = `
	CREATE TABLE IF NOT EXISTS chargmnoaggro (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT ''
	)`

// ensureCharGMNoAggro makes table chargmnoaggro (initSchema).
func (db *SQLiteDB) ensureCharGMNoAggro() error {
	if _, err := db.Exec(charGMNoAggroSchema); err != nil {
		return fmt.Errorf("chargmnoaggro: %w", err)
	}
	return nil
}

// GMNoAggro reports whether creatures leave the GM's character alone (it has
// its row).
func (db *SQLiteDB) GMNoAggro(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM chargmnoaggro WHERE serverid = ?`, serverID).Scan(&n)
	return n > 0, err
}

// SetGMNoAggro keeps the GM's Safe Mode (on) or takes it away.
func (db *SQLiteDB) SetGMNoAggro(serverID uint32, on bool) error {
	if !on {
		_, err := db.Exec(`DELETE FROM chargmnoaggro WHERE serverid = ?`, serverID)
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO chargmnoaggro (serverid, since) VALUES (?, ?)`,
		serverID, time.Now().UTC().Format(time.RFC3339))
	return err
}
