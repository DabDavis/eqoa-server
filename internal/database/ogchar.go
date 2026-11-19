package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ogchar.go — [OG] characters (the owner, 2026-11-19: "lets make [OG] status a
// char tag like [HC] the account status is confusing new players"). [OG] was an
// account's (accountinfo.og, ogaccount.go, chosen at the account's creation);
// it is now a character's, chosen when the character is new and permanent, the
// way [HC] Hardcore is (hardcore.go). Columns of characters, 0 for every
// character made before them:
//
//	og         1: an [OG] character: on a shard with world.og_accounts its
//	           sessions have the game's own UI and its name is drawn "[OG] Name"
//	           (handlers/ogaccount.go)
//	og_chosen  1: the choice is made (at creation by a client that offers it,
//	           in the question box a character's first world entry asks, or by
//	           the account migration below); 0: still open while the character
//	           is new (HardcoreState.OGChoiceOpen: the same newness as Hardcore's)
//
// accountinfo.og stays as a column (the GM panel shows it), but nothing reads it
// for play any more: MigrateAccountOG made every character of an [OG] account an
// [OG] character, once, on the shard where [OG] meant something, and a character
// made since on such an account chooses for itself.

// ogCharColumns are the columns ensureCharOG adds to characters.
var ogCharColumns = []struct{ name, def string }{
	{"og", "INTEGER NOT NULL DEFAULT 0"},
	{"og_chosen", "INTEGER NOT NULL DEFAULT 0"},
}

// ogMigrationSchema is the account migration's record: one row in
// og_char_migration once it ran (whether it applied: only where
// world.og_accounts is on), and each character it made [OG] in
// og_char_migrated (its account, when), for an undo.
const ogMigrationSchema = `
	CREATE TABLE IF NOT EXISTS og_char_migration (
		id          INTEGER PRIMARY KEY CHECK (id = 1),
		migrated_at INTEGER NOT NULL,
		applied     INTEGER NOT NULL,
		characters  INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS og_char_migrated (
		serverid    INTEGER PRIMARY KEY,
		accountid   INTEGER NOT NULL,
		migrated_at INTEGER NOT NULL
	);`

// ensureCharOG adds the [OG] columns to characters and the migration's tables
// (initSchema; after ensureHardcore, whose select reads them).
func (db *SQLiteDB) ensureCharOG() error {
	for _, c := range ogCharColumns {
		ok, err := hasColumn(db.DB, "characters", c.name)
		if err != nil {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
		if ok {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE characters ADD COLUMN ` + c.name + ` ` + c.def); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
	}
	if _, err := db.Exec(ogMigrationSchema); err != nil {
		return fmt.Errorf("og_char_migration: %w", err)
	}
	return nil
}

// OGChoiceOpen reports a character whose [OG] choice is still to be made: one
// made since the columns (hc_born), not chosen, still at the level it was made
// at with no experience (as Hardcore's, ChoiceOpen).
func (h HardcoreState) OGChoiceOpen() bool {
	return !h.OGChosen && !h.OG && h.Born > 0 && h.Level == h.BornLevel && h.TotalXP == 0 && !h.Fallen()
}

// CharacterOG reports whether the character is an [OG] one; ErrCharacterNotFound
// for none.
func (db *SQLiteDB) CharacterOG(serverID uint32) (bool, error) {
	var og int
	err := db.QueryRow(`SELECT COALESCE(og, 0) FROM characters WHERE serverid = ?`, serverID).Scan(&og)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrCharacterNotFound
		}
		return false, err
	}
	return og != 0, nil
}

// ChooseOG makes a character's [OG] choice: [OG] (on) or not. Only an open
// choice is taken (never one made, never an [OG] character's): changed reports
// whether it was. A GM's change goes through the panel's audited row write
// (set_character_og).
func (db *SQLiteDB) ChooseOG(serverID uint32, on bool) (changed bool, err error) {
	v := 0
	if on {
		v = 1
	}
	res, err := db.Exec(`UPDATE characters SET og = ?, og_chosen = 1
		WHERE serverid = ? AND COALESCE(og_chosen, 0) = 0 AND COALESCE(og, 0) = 0`, v, serverID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// OGMigration is what MigrateAccountOG did.
type OGMigration struct {
	Ran        bool // this call ran it (false: it had run before)
	Applied    bool // characters were made [OG] (world.og_accounts on)
	Characters int  // how many, this call or the first
}

// MigrateAccountOG moves [OG] from the accounts to their characters, ONCE per
// database: with apply (world.og_accounts, the shard where an [OG] account
// meant something), every character of an account with accountinfo.og = 1 is
// made an [OG] character with its choice made (og = 1, og_chosen = 1), each
// written down in og_char_migrated; without, nothing changes. Either way the
// run is recorded in og_char_migration and never runs again, so the account's
// flag is never re-applied: a character made later on an [OG] account chooses
// for itself, and a shard that turns world.og_accounts on later does not take
// the account flag up then. A fallen Hardcore character is migrated too (it
// keeps its tag on the memorial).
func (db *SQLiteDB) MigrateAccountOG(apply bool, now time.Time) (OGMigration, error) {
	tx, err := db.Begin()
	if err != nil {
		return OGMigration{}, err
	}
	defer tx.Rollback()
	var applied, count int
	switch err := tx.QueryRow(`SELECT applied, characters FROM og_char_migration WHERE id = 1`).Scan(&applied, &count); {
	case err == nil:
		return OGMigration{Applied: applied != 0, Characters: count}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return OGMigration{}, err
	}
	m := OGMigration{Ran: true, Applied: apply}
	if apply {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO og_char_migrated (serverid, accountid, migrated_at)
			SELECT c.serverid, c.accountid, ? FROM characters c JOIN accountinfo a ON a.accountid = c.accountid
			WHERE COALESCE(a.og, 0) != 0 AND COALESCE(c.og, 0) = 0`, now.Unix()); err != nil {
			return OGMigration{}, err
		}
		res, err := tx.Exec(`UPDATE characters SET og = 1, og_chosen = 1
			WHERE serverid IN (SELECT serverid FROM og_char_migrated WHERE migrated_at = ?) AND COALESCE(og, 0) = 0`, now.Unix())
		if err != nil {
			return OGMigration{}, err
		}
		n, _ := res.RowsAffected()
		m.Characters = int(n)
	}
	a := 0
	if apply {
		a = 1
	}
	if _, err := tx.Exec(`INSERT INTO og_char_migration (id, migrated_at, applied, characters) VALUES (1, ?, ?, ?)`,
		now.Unix(), a, m.Characters); err != nil {
		return OGMigration{}, err
	}
	return m, tx.Commit()
}
