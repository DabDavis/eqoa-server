package database

import (
	"path/filepath"
	"testing"
)

func TestPlayerTermsAuthorityVersionsAndAtomicSignup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteAccountStore(db)
	account, err := s.CreateWithPlayerTerms("player", "test hash", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	account.PasswordHash = "test hash"
	mirror := NewAccountMirror(db, path)
	if ok, err := mirror.HasPlayerTerms(account.ID, "1.1"); ok || err != nil {
		t.Fatalf("stale version accepted: %v", err)
	}
	if err := db.AcceptPlayerTerms(account, "1.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE account_terms_acceptances SET accepted_at = '2000-01-01 00:00:00' WHERE version = '1.1'"); err != nil {
		t.Fatal(err)
	}
	if err := db.AcceptPlayerTerms(account, "1.1"); err != nil {
		t.Fatal(err)
	}
	var stamp string
	if err := db.QueryRow("SELECT accepted_at FROM account_terms_acceptances WHERE version = '1.1'").Scan(&stamp); err != nil || stamp != "2000-01-01 00:00:00" {
		t.Fatal("replay changed the first receipt")
	}
	if ok, err := mirror.HasPlayerTerms(account.ID, "1.1"); !ok || err != nil {
		t.Fatalf("mirror missed canonical receipt: %v", err)
	}
	if ok, err := NewAccountMirror(db, filepath.Join(t.TempDir(), "absent.db")).HasPlayerTerms(account.ID, "1.1"); ok || err == nil {
		t.Fatal("missing authority admitted a player")
	}
	if !snapshotRules["account_terms_acceptances"].clear {
		t.Fatal("public snapshots contain consent history")
	}
	if _, err := db.Exec("DELETE FROM accountinfo WHERE accountid = ?", account.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM account_terms_acceptances").Scan(&n); err != nil || n != 0 {
		t.Fatal("removed account retained receipts")
	}
	if _, err := db.Exec("DROP TABLE account_terms_acceptances"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWithPlayerTerms("another", "hash", "1.1"); err == nil {
		t.Fatal("signup succeeded without its receipt")
	}
	if _, err := s.GetByUsername("another"); err == nil {
		t.Fatal("partial signup")
	}
}
