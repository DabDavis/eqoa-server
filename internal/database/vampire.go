package database

import (
	"fmt"
	"time"
)

// charvampire: the characters who are vampires, a row each (entity/vampire.go:
// the sun burns them by day). In the live game the bats' infection made one,
// before Hinder taught it to feed ("If infected, you may notice a DoT (-50)
// during daylight", EQOA-Data's vampire guide), and so here: the vampire
// quest's kill step of desmodus bats (EQOA-Data's Weres/vampire,
// tools/quests/data/weres.json) ends with a vampire event, a row of source
// quest. #vampire in the game (a GM's) and the GM panel's "Vampires" table
// set one too, source gm. No class, race, faction or buff marks one. since
// is when (UTC, RFC 3339), note why.
const charVampireSchema = `
	CREATE TABLE IF NOT EXISTS charvampire (
		serverid INTEGER PRIMARY KEY,
		since    TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureCharVampire makes table charvampire (initSchema).
func (db *SQLiteDB) ensureCharVampire() error {
	if _, err := db.Exec(charVampireSchema); err != nil {
		return fmt.Errorf("charvampire: %w", err)
	}
	return nil
}

// IsVampire reports whether the character is a vampire (it has its row).
func (db *SQLiteDB) IsVampire(serverID uint32) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charvampire WHERE serverid = ?`, serverID).Scan(&n)
	return n > 0, err
}

// Vampires is how many characters are vampires.
func (db *SQLiteDB) Vampires() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM charvampire`).Scan(&n)
	return n, err
}

// MarkVampire makes the character a vampire, as a quest's infection does
// (source quest, note why): no GM's change, so no audit line. It reports
// whether the character was not one before.
func (db *SQLiteDB) MarkVampire(serverID uint32, source, note string) (bool, error) {
	if err := db.ensureCharVampire(); err != nil {
		return false, err
	}
	res, err := db.Exec(`INSERT OR IGNORE INTO charvampire (serverid, since, note, source) VALUES (?, ?, ?, ?)`,
		serverID, time.Now().UTC().Format(time.RFC3339), note, source)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetVampire makes the character a vampire (on) or not, writing the change
// and its gm_audit line in one transaction as the GM panel writes a row edit,
// so the panel lists and reverts it. It returns the audit line's id, 0 when
// nothing changed (it already was, or was not).
func (db *SQLiteDB) SetVampire(serverID uint32, on bool, who GMEdit, detail string) (audit int64, err error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, err
	}
	if err := db.ensureCharVampire(); err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if who.Username == "" {
		if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username); err != nil {
			who.Username = fmt.Sprintf("account %d", who.AccountID)
		}
	}
	const sel = `SELECT * FROM charvampire WHERE serverid = ?`
	prev, err := fullRow(tx, sel, serverID)
	if err != nil {
		return 0, err
	}
	if (prev != nil) == on {
		return 0, nil
	}
	action := "insert"
	if on {
		_, err = tx.Exec(`INSERT INTO charvampire (serverid, since, note, source) VALUES (?, ?, ?, 'gm')`,
			serverID, time.Now().UTC().Format(time.RFC3339), "made a vampire by GM "+who.Username+" in the game")
	} else {
		action = "delete"
		_, err = tx.Exec(`DELETE FROM charvampire WHERE serverid = ?`, serverID)
	}
	if err != nil {
		return 0, err
	}
	after, err := fullRow(tx, sel, serverID)
	if err != nil {
		return 0, err
	}
	key := map[string]any{"serverid": int64(serverID)}
	if audit, err = auditRowChange(tx, who, action, "charvampire", key, prev, after, detail); err != nil {
		return 0, err
	}
	return audit, tx.Commit()
}
