package database

import (
	"database/sql"
	"fmt"
	"time"
)

// charhidemercs: each character's choice of other players' mercenaries in
// its own view (petition 1858; handlers/hidemercs.go, entity/hidemercs.go), a
// row each, so the choice outlives a logout and any restart. hidden is the
// choice: 1 /hide mercs, 0 /show mercs; a character with no row has made none
// and has them hidden (the owner, 2026-11-13: "lets make /hidemerc on by
// default"). Rows from before the column were /hide mercs's: 1. since is when
// (UTC, RFC 3339).
const charHideMercsSchema = `
	CREATE TABLE IF NOT EXISTS charhidemercs (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT '',
		hidden   INTEGER NOT NULL DEFAULT 1
	);`

// MercsHiddenByDefault is a character's choice before it makes one.
const MercsHiddenByDefault = true

// ensureCharHideMercs makes table charhidemercs (initSchema), with its hidden
// column on a table from before it.
func (db *SQLiteDB) ensureCharHideMercs() error {
	if _, err := db.Exec(charHideMercsSchema); err != nil {
		return fmt.Errorf("charhidemercs: %w", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('charhidemercs') WHERE name = 'hidden'`).Scan(&n); err != nil {
		return fmt.Errorf("charhidemercs: %w", err)
	}
	if n == 0 {
		if _, err := db.Exec(`ALTER TABLE charhidemercs ADD COLUMN hidden INTEGER NOT NULL DEFAULT 1`); err != nil {
			return fmt.Errorf("charhidemercs: %w", err)
		}
	}
	return nil
}

// MercsHidden reports whether the character has other players' mercenaries
// hidden: its choice, else MercsHiddenByDefault.
func (db *SQLiteDB) MercsHidden(serverID uint32) (bool, error) {
	var hidden int
	err := db.QueryRow(`SELECT hidden FROM charhidemercs WHERE serverid = ?`, serverID).Scan(&hidden)
	if err == sql.ErrNoRows {
		return MercsHiddenByDefault, nil
	}
	return hidden != 0, err
}

// SetMercsHidden keeps the character's choice: /hide mercs (on) or /show mercs.
func (db *SQLiteDB) SetMercsHidden(serverID uint32, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := db.Exec(`INSERT INTO charhidemercs (serverid, since, hidden) VALUES (?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET since = excluded.since, hidden = excluded.hidden`,
		serverID, time.Now().UTC().Format(time.RFC3339), v)
	return err
}
