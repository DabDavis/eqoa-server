package database

import (
	"database/sql"
	"fmt"
	"time"
)

// charlogout: when each character last left the world (its logout, its
// session's end, a shutdown's save), for its No Rent items
// (handlers/norent.go): read and taken away at its next login, so a
// character with no row (none since this table, or a server that stopped
// without its shutdown save) keeps them. at is Unix seconds.
const charLogoutSchema = `
	CREATE TABLE IF NOT EXISTS charlogout (
		serverid INTEGER PRIMARY KEY,
		at       INTEGER NOT NULL
	);`

// ensureCharLogout makes table charlogout (initSchema).
func (db *SQLiteDB) ensureCharLogout() error {
	if _, err := db.Exec(charLogoutSchema); err != nil {
		return fmt.Errorf("charlogout: %w", err)
	}
	return nil
}

// SaveLogout keeps when a character left the world.
func (s *SQLiteCharacterStore) SaveLogout(serverID uint32, at time.Time) error {
	_, err := s.db.Exec(`INSERT INTO charlogout (serverid, at) VALUES (?, ?)
		ON CONFLICT (serverid) DO UPDATE SET at = excluded.at`, serverID, at.Unix())
	return err
}

// TakeLogout is when a character last left the world, its row taken away
// with the reading; ok is false when it has none.
func (s *SQLiteCharacterStore) TakeLogout(serverID uint32) (at time.Time, ok bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return time.Time{}, false, err
	}
	defer tx.Rollback()
	var sec int64
	switch err := tx.QueryRow(`SELECT at FROM charlogout WHERE serverid = ?`, serverID).Scan(&sec); {
	case err == sql.ErrNoRows:
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, err
	}
	if _, err := tx.Exec(`DELETE FROM charlogout WHERE serverid = ?`, serverID); err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(sec, 0), true, tx.Commit()
}

// NoRentItems is a character's items whose pattern is No Rent
// (itempattern.rent 0, CLIENT's NoRent: the game's own items carry 1), worn,
// in the inventory and in the bank.
func (s *InventoryStore) NoRentItems(serverID uint32) ([]CharInventoryEntry, error) {
	items, err := s.GetCharacterItems(serverID)
	if err != nil {
		return nil, err
	}
	var out []CharInventoryEntry
	for _, e := range items {
		if e.Pattern.Rent == 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// HoldsPattern is whether a character holds any of an item, worn, in the
// inventory or in the bank (a lore item is held once: handlers/potion.go).
func (s *InventoryStore) HoldsPattern(serverID uint32, patternID int) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND patternid = ?`,
		serverID, patternID).Scan(&n)
	return n > 0, err
}
