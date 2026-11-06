package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// charrez: what a resurrection may still give back of a character's latest
// death's experience debt (entity/resurrect.go; jib's review, F3 pass 19):
// debt is what that death added, left_ms the time its chance still has,
// counted while the character is in the world (as charbuffs' left_ms is); placed,
// world, x, y, z where that death happened, the way back a rez offers (a row from
// before 2026-11-06 has none). No row: nothing to recover. Saved with the
// character's debt (characters.debt) at a logout, a replacing login and a shutdown,
// read at the login.
const charRezSchema = `
	CREATE TABLE IF NOT EXISTS charrez (
		serverid INTEGER PRIMARY KEY,
		debt     INTEGER NOT NULL,
		left_ms  INTEGER NOT NULL
	);`

// ensureCharRez makes table charrez (initSchema).
func (db *SQLiteDB) ensureCharRez() error {
	if _, err := db.Exec(charRezSchema); err != nil {
		return fmt.Errorf("charrez: %w", err)
	}
	// where the death was, kept since 2026-11-06 (a row from before has none)
	for _, c := range []string{"placed INTEGER NOT NULL DEFAULT 0", "world INTEGER NOT NULL DEFAULT 0", "x REAL NOT NULL DEFAULT 0",
		"y REAL NOT NULL DEFAULT 0", "z REAL NOT NULL DEFAULT 0"} {
		if _, err := db.Exec(`ALTER TABLE charrez ADD COLUMN ` + c); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("charrez: %w", err)
		}
	}
	return nil
}

// RezRow is a character's chance of a rez as charrez keeps it; the zero row
// is none.
type RezRow struct {
	Debt   int
	LeftMS int64
	// Placed and the place are where the death was, the way back a rez offers
	Placed  bool
	World   uint8
	X, Y, Z float32
}

// LoadRez is a character's chance of a rez, or the zero row for none.
func (s *SQLiteCharacterStore) LoadRez(serverID uint32) (RezRow, error) {
	var r RezRow
	err := s.db.QueryRow(`SELECT debt, left_ms, placed, world, x, y, z FROM charrez WHERE serverid = ?`, serverID).
		Scan(&r.Debt, &r.LeftMS, &r.Placed, &r.World, &r.X, &r.Y, &r.Z)
	if err == sql.ErrNoRows {
		return RezRow{}, nil
	}
	return r, err
}

// SaveRez keeps a character's chance of a rez; a row with nothing to give (no debt
// and no place) or no time left takes the character's row away.
func (s *SQLiteCharacterStore) SaveRez(serverID uint32, r RezRow) error {
	if (r.Debt <= 0 && !r.Placed) || r.LeftMS <= 0 {
		_, err := s.db.Exec(`DELETE FROM charrez WHERE serverid = ?`, serverID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO charrez (serverid, debt, left_ms, placed, world, x, y, z) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET debt = excluded.debt, left_ms = excluded.left_ms, placed = excluded.placed,
			world = excluded.world, x = excluded.x, y = excluded.y, z = excluded.z`,
		serverID, r.Debt, r.LeftMS, r.Placed, r.World, r.X, r.Y, r.Z)
	return err
}
