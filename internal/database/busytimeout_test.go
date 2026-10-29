package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The server's database waits for a lock another connection holds (a tool
// writing beside the server) instead of failing at once: its connection's
// busy_timeout is 5000, and a write made while another connection holds the
// write lock lands when it is let go. The control: a connection opened as the
// server opened it until 2026-10-29 (mattn's `_busy_timeout`, which the
// driver ignores) fails at once with the database locked.
func TestTheDatabaseWaitsForALock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ms int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&ms); err != nil || ms != 5000 {
		t.Fatalf("busy_timeout %d (%v), want 5000", ms, err)
	}
	if _, err := db.Exec(`CREATE TABLE busytest (n INTEGER)`); err != nil {
		t.Fatal(err)
	}

	// another connection holds the write lock for 300 ms
	hold := func() (release func()) {
		t.Helper()
		other, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		other.SetMaxOpenConns(1)
		tx, err := other.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO busytest VALUES (0)`); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			time.Sleep(300 * time.Millisecond)
			tx.Commit()
			other.Close()
			close(done)
		}()
		return func() { <-done }
	}

	release := hold()
	start := time.Now()
	if _, err := db.Exec(`INSERT INTO busytest VALUES (1)`); err != nil {
		t.Fatalf("a write under another's lock: %v", err)
	}
	if waited := time.Since(start); waited < 200*time.Millisecond {
		t.Errorf("the write waited %v: the lock was not held?", waited)
	}
	release()

	// the control: the old spelling
	old, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	old.SetMaxOpenConns(1)
	release = hold()
	_, err = old.Exec(`INSERT INTO busytest VALUES (2)`)
	release()
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("the control: a write on the old spelling under another's lock: %v, want the database locked", err)
	}
}
