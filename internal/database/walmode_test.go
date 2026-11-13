package database

import (
	"path/filepath"
	"testing"
)

// The database opens in WAL mode (sqlite.go walMode, the owner 2026-11-13),
// a new file and one made before in the rollback journal alike, and a reader
// holding a read open does not keep a write from committing.
func TestTheDatabaseIsWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode %q (%v), want wal", mode, err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = DELETE`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path) // a file left in the rollback journal comes back WAL
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("reopened: journal mode %q (%v), want wal", mode, err)
	}

	// a second connection's open read does not hold the writer off
	other, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS walprobe (n INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM walprobe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO walprobe VALUES (1)`); err != nil {
		t.Fatalf("a write with a read open beside it: %v", err)
	}
}
