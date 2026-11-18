package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ogaccount.go — accountinfo.og: an [OG] account (the owner, 2026-11-18, for
// the Vanilla Test server: "make a tag for accounts to be [OG] Accounts ...
// this [OG] tag shows players you opt-out of the QoL changes to EQOA and only
// use the most basic upgrades like FPS, widescreen, no touchscreen, no
// mouse"). Chosen once, when the account is made (the account pages'
// Register, STATION's Create Account), and never the player's to change after:
// a GM may, on the owner's panel (set_account_og, audited and revertible).
// Every account made before it is 0. A column of accountinfo, so a shard's
// mirrored copy (AccountMirror) has it within seconds; what it does is the
// shard's (world.og_accounts, handlers/ogaccount.go).

// ensureAccountOG adds accountinfo.og (initSchema).
func (db *SQLiteDB) ensureAccountOG() error {
	ok, err := hasColumn(db.DB, "accountinfo", "og")
	if err != nil {
		return fmt.Errorf("accountinfo.og: %w", err)
	}
	if !ok {
		if _, err := db.Exec(`ALTER TABLE accountinfo ADD COLUMN og INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("accountinfo.og: %w", err)
		}
	}
	return nil
}

// AccountOG reports whether the account is an [OG] one; ErrAccountNotFound
// for none.
func (db *SQLiteDB) AccountOG(accountID int) (bool, error) {
	var og int
	err := db.QueryRow(`SELECT COALESCE(og, 0) FROM accountinfo WHERE accountid = ?`, accountID).Scan(&og)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrAccountNotFound
		}
		return false, err
	}
	return og != 0, nil
}

// SetAccountOG sets the account's choice. Only account creation calls it with
// on; a GM's change goes through the panel's audited row write.
func (db *SQLiteDB) SetAccountOG(accountID int, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := db.Exec(`UPDATE accountinfo SET og = ? WHERE accountid = ?`, v, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// SetAccountOG on the account store (the account pages', STATION's).
func (s *SQLiteAccountStore) SetAccountOG(accountID int, on bool) error {
	return s.db.SetAccountOG(accountID, on)
}

// AccountOG on the account store.
func (s *SQLiteAccountStore) AccountOG(accountID int) (bool, error) {
	return s.db.AccountOG(accountID)
}
