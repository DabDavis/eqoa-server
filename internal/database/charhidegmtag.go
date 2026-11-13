package database

import (
	"fmt"
	"time"
)

// charhidegmtag: the GM characters that hid their [GM] tag with /hide gmtag
// (the owner, 2026-11-13: "for gm accounts let them be able to hide GM tag
// from their chars"; handlers/hidegmtag.go), a row each, kept until /show
// gmtag so the choice outlives a logout and every restart.
const charHideGMTagSchema = `
	CREATE TABLE IF NOT EXISTS charhidegmtag (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL
	)`

// ensureCharHideGMTag makes table charhidegmtag (initSchema).
func (db *SQLiteDB) ensureCharHideGMTag() error {
	if _, err := db.Exec(charHideGMTagSchema); err != nil {
		return fmt.Errorf("charhidegmtag: %w", err)
	}
	return nil
}

// GMTagHidden reports whether the character hid its [GM] tag (it has its row).
func (db *SQLiteDB) GMTagHidden(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charhidegmtag WHERE serverid = ?`, serverID).Scan(&n)
	return n > 0, err
}

// SetGMTagHidden keeps the character's /hide gmtag (on) or takes it away.
func (db *SQLiteDB) SetGMTagHidden(serverID uint32, on bool) error {
	if !on {
		_, err := db.Exec(`DELETE FROM charhidegmtag WHERE serverid = ?`, serverID)
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO charhidegmtag (serverid, since) VALUES (?, ?)`,
		serverID, time.Now().UTC().Format(time.RFC3339))
	return err
}
