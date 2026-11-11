package database

import (
	"fmt"
	"time"
)

// charplayed is when each character last entered the world (its login), kept
// for good: the order in which a client whose select page holds fewer than an
// account's characters (a PS2's 8, handlers/charslots.go) is listed the most
// recently played. at is Unix seconds. A character with no row yet (one last
// played before the table) goes by when it last left the world (charlogout,
// norent.go), else as never played, the newest made first.
const charPlayedSchema = `
	CREATE TABLE IF NOT EXISTS charplayed (
		serverid INTEGER PRIMARY KEY,
		at       INTEGER NOT NULL
	);`

func (db *SQLiteDB) ensureCharPlayed() error {
	if _, err := db.Exec(charPlayedSchema); err != nil {
		return fmt.Errorf("charplayed: %w", err)
	}
	return nil
}

// StampPlayed keeps that a character entered the world at at.
func (db *SQLiteDB) StampPlayed(serverID uint32, at time.Time) error {
	_, err := db.Exec(`INSERT INTO charplayed (serverid, at) VALUES (?, ?)
		ON CONFLICT (serverid) DO UPDATE SET at = excluded.at`, serverID, at.Unix())
	return err
}

// AccountPlayed is when each of an account's characters was last played, in
// Unix seconds (charplayed's, else charlogout's), by server id; a character
// never played is not in it.
func (db *SQLiteDB) AccountPlayed(accountID int) (map[uint32]int64, error) {
	rows, err := db.Query(`SELECT c.serverid, COALESCE(p.at, l.at, 0)
		FROM characters c
		LEFT JOIN charplayed p ON p.serverid = c.serverid
		LEFT JOIN charlogout l ON l.serverid = c.serverid
		WHERE c.accountid = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]int64{}
	for rows.Next() {
		var id uint32
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		if at > 0 {
			out[id] = at
		}
	}
	return out, rows.Err()
}
