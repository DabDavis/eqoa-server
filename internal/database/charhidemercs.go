package database

import (
	"fmt"
	"time"
)

// charhidemercs: the characters that hid other players' mercenaries from
// their own view with /hide mercs (petition 1858; handlers/hidemercs.go,
// entity/hidemercs.go), a row each, kept until /show mercs so the choice
// outlives a logout and any restart. since is when (UTC, RFC 3339).
const charHideMercsSchema = `
	CREATE TABLE IF NOT EXISTS charhidemercs (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT ''
	);`

// ensureCharHideMercs makes table charhidemercs (initSchema).
func (db *SQLiteDB) ensureCharHideMercs() error {
	if _, err := db.Exec(charHideMercsSchema); err != nil {
		return fmt.Errorf("charhidemercs: %w", err)
	}
	return nil
}

// MercsHidden reports whether the character hid other players' mercenaries
// (it has its row).
func (db *SQLiteDB) MercsHidden(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charhidemercs WHERE serverid = ?`, serverID).Scan(&n)
	return n > 0, err
}

// SetMercsHidden keeps the character's /hide mercs (on) or takes it away.
func (db *SQLiteDB) SetMercsHidden(serverID uint32, on bool) error {
	if !on {
		_, err := db.Exec(`DELETE FROM charhidemercs WHERE serverid = ?`, serverID)
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO charhidemercs (serverid, since) VALUES (?, ?)`,
		serverID, time.Now().UTC().Format(time.RFC3339))
	return err
}
