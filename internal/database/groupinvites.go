package database

import "fmt"

// chargroupinvites: each character's "Allow anyone in my group to invite"
// (the GM Jibrish's petition 1476, 2026-11-11; entity/groupinvites.go), a row
// a character that set it: open 1 yes, 0 no. A character with no row has it
// on (any member invites, as since 2026-11-06).
const charGroupInvitesSchema = `
	CREATE TABLE IF NOT EXISTS chargroupinvites (
		serverid INTEGER PRIMARY KEY,
		open     INTEGER NOT NULL DEFAULT 1
	);`

// ensureGroupInvites makes table chargroupinvites (initSchema).
func (db *SQLiteDB) ensureGroupInvites() error {
	if _, err := db.Exec(charGroupInvitesSchema); err != nil {
		return fmt.Errorf("chargroupinvites: %w", err)
	}
	return nil
}

// GroupInvitesShut is every character whose setting is off.
func (db *SQLiteDB) GroupInvitesShut() (map[uint32]bool, error) {
	rows, err := db.Query(`SELECT serverid FROM chargroupinvites WHERE open = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]bool{}
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SetGroupInvitesOpen keeps the character's setting.
func (db *SQLiteDB) SetGroupInvitesOpen(serverID uint32, open bool) error {
	v := 0
	if open {
		v = 1
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO chargroupinvites (serverid, open) VALUES (?, ?)`, serverID, v)
	return err
}
