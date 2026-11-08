package database

import (
	"path/filepath"
	"testing"
)

// A sub GM is a tester whose gm_testers row says subgm (the user, 2026-11-08:
// "make his account a sub gm new category above tester than can fly"): SubGM
// reads it at each call. The controls: a plain tester, an account with no row,
// and a server whose tester table is missing (an error, read in the game as no).
func TestASubGMIsATestersRowMarkedSo(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "subgm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteAccountStore(db)
	if _, err := s.SubGM(94); err == nil {
		t.Error("no tester table, and no error")
	}
	if _, err := db.Exec(`CREATE TABLE gm_testers (accountid INTEGER PRIMARY KEY, granted_by TEXT NOT NULL DEFAULT '',
		granted_at TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', subgm INTEGER NOT NULL DEFAULT 0);
		INSERT INTO gm_testers (accountid, subgm) VALUES (94, 1), (130, 0)`); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   int
		want bool
	}{{94, true}, {130, false}, {7, false}} {
		if got, err := s.SubGM(c.id); err != nil || got != c.want {
			t.Errorf("account %d: %v (%v), want %v", c.id, got, err, c.want)
		}
	}
	if _, err := db.Exec(`UPDATE gm_testers SET subgm = 0 WHERE accountid = 94`); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SubGM(94); got {
		t.Error("the mark taken away, still a sub GM")
	}
}
