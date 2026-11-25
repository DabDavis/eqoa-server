package database

import (
	"database/sql"
	"fmt"
	"time"
)

// charprefarm: each character's /prefarm choice (the owner, 2026-11-25: "/prefarm
// off option for players that removes the quest loot drops from mobs, rule applies
// to group if player is group leader"; handlers/prefarm.go), a row each, so the
// choice outlives a logout and any restart. off is the choice: 1 /prefarm off (a
// quest's loot step item drops on the kill's corpse only for a player of the kill
// on the step), 0 /prefarm on; a character with no row has made none and has it
// on, the game as before. since is when (UTC, RFC 3339).
const charPrefarmSchema = `
	CREATE TABLE IF NOT EXISTS charprefarm (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT '',
		off      INTEGER NOT NULL DEFAULT 0
	);`

// ensureCharPrefarm makes table charprefarm (initSchema).
func (db *SQLiteDB) ensureCharPrefarm() error {
	if _, err := db.Exec(charPrefarmSchema); err != nil {
		return fmt.Errorf("charprefarm: %w", err)
	}
	return nil
}

// PrefarmOff reports whether the character has /prefarm off: its choice, else
// on (false).
func (db *SQLiteDB) PrefarmOff(serverID uint32) (bool, error) {
	var off int
	err := db.QueryRow(`SELECT off FROM charprefarm WHERE serverid = ?`, serverID).Scan(&off)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return off != 0, err
}

// SetPrefarmOff keeps the character's choice: /prefarm off (off) or /prefarm on.
func (db *SQLiteDB) SetPrefarmOff(serverID uint32, off bool) error {
	v := 0
	if off {
		v = 1
	}
	_, err := db.Exec(`INSERT INTO charprefarm (serverid, since, off) VALUES (?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET since = excluded.since, off = excluded.off`,
		serverID, time.Now().UTC().Format(time.RFC3339), v)
	return err
}
