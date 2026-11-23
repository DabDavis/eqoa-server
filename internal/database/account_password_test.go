package database

import (
	"path/filepath"
	"testing"
)

func TestCompareAndSwapPasswordHash(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteAccountStore(db)
	for _, stored := range []any{nil, []byte("legacy"), "legacy"} {
		if _, err := db.Exec(`INSERT OR REPLACE INTO accountinfo (accountid, username, pwhash) VALUES (1, 'one', ?)`, stored); err != nil {
			t.Fatal(err)
		}
		account, err := store.GetByUsername("one")
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := store.CompareAndSwapPasswordHash(account.ID, account.PasswordHash, "current"); err != nil || !ok {
			t.Fatalf("upgrading %T: changed %v, error %v", stored, ok, err)
		}
		if ok, err := store.CompareAndSwapPasswordHash(account.ID, account.PasswordHash, "stale"); err != nil || ok {
			t.Fatalf("stale upgrade: changed %v, error %v", ok, err)
		}
		if got := accountOf(t, db, "one").PasswordHash; got != "current" {
			t.Fatalf("password after stale upgrade: %q", got)
		}
	}
}
