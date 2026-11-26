package database

import (
	"database/sql"
	"errors"
)

// CreateWithPlayerTerms records the affirmative signup and the new account together.
func (s *SQLiteAccountStore) CreateWithPlayerTerms(username, hash, version string) (*AccountRecord, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := insertAccount(tx, username, hash)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO account_terms_acceptances(accountid, version) VALUES (?, ?)`, account.ID, version); err != nil {
		return nil, err
	}
	return account, tx.Commit()
}

// AcceptPlayerTerms rechecks the credential at the write. A repeat acceptance
// keeps the first UTC timestamp, and a new version has its own receipt.
func (db *SQLiteDB) AcceptPlayerTerms(account *AccountRecord, version string) error {
	result, err := db.Exec(`INSERT INTO account_terms_acceptances(accountid, version)
		SELECT accountid, ? FROM accountinfo WHERE accountid = ? AND CAST(pwhash AS TEXT) = ?
		AND COALESCE(acctstatus, 0) >= 0 AND COALESCE(pwtemp, 0) = 0
		ON CONFLICT(accountid, version) DO UPDATE SET version = excluded.version`, version, account.ID, account.PasswordHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("account credentials or status changed")
	}
	return err
}

func hasPlayerTerms(db *sql.DB, id int, version string) (bool, error) {
	var allowed bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM account_terms_acceptances
		JOIN accountinfo USING(accountid) WHERE accountid = ? AND version = ?
		AND COALESCE(acctstatus, 0) >= 0 AND COALESCE(pwtemp, 0) = 0)`, id, version).Scan(&allowed)
	return allowed, err
}

func (db *SQLiteDB) HasPlayerTerms(id int, version string) (bool, error) {
	return hasPlayerTerms(db.DB, id, version)
}

// A shard with mirrored accounts asks the account authority, including after
// restore. A stale local database must never admit an unaccepted account.
func (m *AccountMirror) HasPlayerTerms(id int, version string) (bool, error) {
	db, err := m.open()
	if err != nil {
		return false, err
	}
	defer db.Close()
	return hasPlayerTerms(db, id, version)
}
