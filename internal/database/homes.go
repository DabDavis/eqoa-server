package database

import (
	"fmt"
	"time"
)

// Player homes (entity/homes.go, handlers/homes.go; the owner, 2026-11-11). Three tables, named so a home's
// furnishings could come later in a table of their own keyed by its owner (charhomeitems, not made):
//
//   - charhomes: a home, a row a character that has gone to its own (owner, when it was first visited);
//   - charhomeguests: a home's invites (owner, guest, both names as they were, when): /home invite;
//   - charhomestay: where a character stands in the homes: whose home (host) and where it came from (the way
//     out's landing), written each time it goes into one; a login saved in the homes' world reads it.
const homesSchema = `
	CREATE TABLE IF NOT EXISTS charhomes (
		owner   INTEGER PRIMARY KEY,
		created TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS charhomeguests (
		owner      INTEGER NOT NULL,
		guest      INTEGER NOT NULL,
		owner_name TEXT NOT NULL DEFAULT '',
		guest_name TEXT NOT NULL DEFAULT '',
		since      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (owner, guest)
	);
	CREATE INDEX IF NOT EXISTS charhomeguests_guest ON charhomeguests (guest);
	CREATE TABLE IF NOT EXISTS charhomestay (
		serverid    INTEGER PRIMARY KEY,
		host        INTEGER NOT NULL,
		from_world  INTEGER NOT NULL DEFAULT 0,
		from_x      REAL NOT NULL DEFAULT 0,
		from_y      REAL NOT NULL DEFAULT 0,
		from_z      REAL NOT NULL DEFAULT 0,
		from_facing REAL NOT NULL DEFAULT 0
	);`

// ensureHomes makes the homes' tables (initSchema).
func (db *SQLiteDB) ensureHomes() error {
	if _, err := db.Exec(homesSchema); err != nil {
		return fmt.Errorf("homes: %w", err)
	}
	return nil
}

// HomeGuestRow is one invite.
type HomeGuestRow struct {
	Owner, Guest         uint32
	OwnerName, GuestName string
}

// HomeStayRow is one character's stay in the homes.
type HomeStayRow struct {
	Char, Host uint32
	FromWorld  int
	FromX      float32
	FromY      float32
	FromZ      float32
	FromFacing float32
}

// HomeGuestRows is every invite.
func (db *SQLiteDB) HomeGuestRows() ([]HomeGuestRow, error) {
	rows, err := db.Query(`SELECT owner, guest, owner_name, guest_name FROM charhomeguests ORDER BY owner, guest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeGuestRow
	for rows.Next() {
		var r HomeGuestRow
		if err := rows.Scan(&r.Owner, &r.Guest, &r.OwnerName, &r.GuestName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// HomeStayRows is every stay.
func (db *SQLiteDB) HomeStayRows() ([]HomeStayRow, error) {
	rows, err := db.Query(`SELECT serverid, host, from_world, from_x, from_y, from_z, from_facing FROM charhomestay`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeStayRow
	for rows.Next() {
		var r HomeStayRow
		if err := rows.Scan(&r.Char, &r.Host, &r.FromWorld, &r.FromX, &r.FromY, &r.FromZ, &r.FromFacing); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddHomeGuest keeps an invite (a second of the same pair changes nothing).
func (db *SQLiteDB) AddHomeGuest(r HomeGuestRow, now time.Time) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO charhomeguests (owner, guest, owner_name, guest_name, since) VALUES (?, ?, ?, ?, ?)`,
		r.Owner, r.Guest, r.OwnerName, r.GuestName, now.UTC().Format(time.RFC3339))
	return err
}

// RemoveHomeGuest takes an invite back; it reports whether there was one.
func (db *SQLiteDB) RemoveHomeGuest(owner, guest uint32) (bool, error) {
	res, err := db.Exec(`DELETE FROM charhomeguests WHERE owner = ? AND guest = ?`, owner, guest)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetHomeStay keeps a character's stay, and its host's home (charhomes) when it is its own.
func (db *SQLiteDB) SetHomeStay(r HomeStayRow, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO charhomestay (serverid, host, from_world, from_x, from_y, from_z, from_facing)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, r.Char, r.Host, r.FromWorld, r.FromX, r.FromY, r.FromZ, r.FromFacing); err != nil {
		return err
	}
	if r.Host == r.Char {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO charhomes (owner, created) VALUES (?, ?)`, r.Host, now.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearHomeStay forgets a character's stay.
func (db *SQLiteDB) ClearHomeStay(char uint32) error {
	_, err := db.Exec(`DELETE FROM charhomestay WHERE serverid = ?`, char)
	return err
}
