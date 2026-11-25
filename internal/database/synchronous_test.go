package database

import (
	"path/filepath"
	"testing"
)

// The server's database runs with synchronous = NORMAL (SynchronousDSN, the
// owner 2026-11-25): 1, against SQLite's default FULL (2), on its connection.
func TestTheDatabaseIsSynchronousNormal(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`PRAGMA synchronous`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("synchronous %d (%v), want 1 (NORMAL)", n, err)
	}
}
