package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ogaccount.go — accountinfo.og: an [OG] account, as [OG] was from
// 2026-11-18 (the owner, for the Vanilla Test server: "make a tag for accounts
// to be [OG] Accounts"), chosen when the account was made. Since 2026-11-19
// [OG] is a CHARACTER's (ogchar.go: "lets make [OG] status a char tag like [HC]
// the account status is confusing new players"): no page offers the account's
// choice any more, and nothing reads this column for play. It stays, for the
// panel and for MigrateAccountOG, which made every character of an [OG]
// account an [OG] character once. A column of accountinfo, so a shard's
// mirrored copy (AccountMirror) has it.

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

// SetAccountOG sets the account's flag (tests; no page sets it any more).
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

// SetAccountOG on the account store.
func (s *SQLiteAccountStore) SetAccountOG(accountID int, on bool) error {
	return s.db.SetAccountOG(accountID, on)
}

// AccountOG on the account store.
func (s *SQLiteAccountStore) AccountOG(accountID int) (bool, error) {
	return s.db.AccountOG(accountID)
}
