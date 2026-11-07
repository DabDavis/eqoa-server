package database

import (
	"fmt"
)

// norent.go: the records whose copies vanish when their holder logs out (the user,
// 2026-11-07: "yes make no rent vanish at logout", for Jibrish's "Make all lord nagafens drop No
// Rent temporarily, we will turn that off later"). A record of rent 0 shows NO RENT and stays off
// the auction, and until now that was all: the 36 conjured records of the old data (a magician's
// Phase Staff, a rogue's Rogue's Dagger) stay on their holders, so a record vanishes only when a row
// here names it. It goes at the holder's next login, which is its logout with any way of leaving
// counted (a dropped link, a restart, a crash).

const noRentVanishSchema = `
	CREATE TABLE IF NOT EXISTS norentvanish (
		patternid INTEGER PRIMARY KEY,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

func (db *SQLiteDB) ensureNoRentVanish() error {
	if _, err := db.Exec(noRentVanishSchema); err != nil {
		return fmt.Errorf("norentvanish: %w", err)
	}
	return nil
}

// VanishNoRent takes the character's copies of norentvanish's records from its pack, its bank and what it
// wears, and reports their names, one a row.
func (db *SQLiteDB) VanishNoRent(serverID uint32) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT p.itemname FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
		WHERE c.serverid = ? AND c.patternid IN (SELECT patternid FROM norentvanish) ORDER BY c.itemid`, serverID)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	if _, err := tx.Exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid IN (SELECT patternid FROM norentvanish)`, serverID); err != nil {
		return nil, err
	}
	return names, tx.Commit()
}
