package database

import (
	"fmt"
	"time"
)

// charlockouts: a character's abilities still locked out (entity/cast.go's
// castLockout: the cast time, the recast and 300 ms from the cast's start),
// a row a spell, written at each end of the character's session (a logout, a
// dropped client's timeout, a login taking it over, the server's shutdown and
// its restarts) and read at the login (entity/lockoutsave.go). Until
// 2026-11-09 they lived in the World alone, so a logout and a login made
// every recast ready again (the owner: "when eve players logout and login the
// recast times of abilities are being reset, so can exploit"). A recast runs
// on the wall clock, the time logged out counting, as an item's cooldown does
// (potioncooldowns): until is when the lockout ends, in Unix milliseconds,
// and a row past it is nothing.
const charLockoutSchema = `
	CREATE TABLE IF NOT EXISTS charlockouts (
		serverid INTEGER NOT NULL,
		spellid  INTEGER NOT NULL,
		until    INTEGER NOT NULL,
		PRIMARY KEY (serverid, spellid)
	);`

// ensureCharLockouts makes table charlockouts (initSchema).
func (db *SQLiteDB) ensureCharLockouts() error {
	if _, err := db.Exec(charLockoutSchema); err != nil {
		return fmt.Errorf("charlockouts: %w", err)
	}
	return nil
}

// LockoutRow is one of a character's lockouts as charlockouts keeps it: the
// spell locked out and when its lockout ends.
type LockoutRow struct {
	SpellID int
	Until   time.Time
}

// SaveLockouts replaces a character's kept lockouts with rows: none leaves it
// none. A spell given twice keeps the later end.
func (s *SQLiteCharacterStore) SaveLockouts(serverID uint32, rows []LockoutRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM charlockouts WHERE serverid = ?`, serverID); err != nil {
		tx.Rollback()
		return err
	}
	for _, r := range rows {
		if _, err := tx.Exec(`INSERT INTO charlockouts (serverid, spellid, until) VALUES (?, ?, ?)
			ON CONFLICT (serverid, spellid) DO UPDATE SET until = max(until, excluded.until)`,
			serverID, r.SpellID, r.Until.UnixMilli()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LoadLockouts is a character's kept lockouts still running at now, by spell.
func (s *SQLiteCharacterStore) LoadLockouts(serverID uint32, now time.Time) ([]LockoutRow, error) {
	rows, err := s.db.Query(`SELECT spellid, until FROM charlockouts WHERE serverid = ? AND until > ? ORDER BY spellid`,
		serverID, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LockoutRow
	for rows.Next() {
		var r LockoutRow
		var until int64
		if err := rows.Scan(&r.SpellID, &until); err != nil {
			return nil, err
		}
		r.Until = time.UnixMilli(until)
		out = append(out, r)
	}
	return out, rows.Err()
}
