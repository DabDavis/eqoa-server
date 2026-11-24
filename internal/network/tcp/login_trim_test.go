package tcp

import (
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 2830 (2026-11-24): a phone keyboard's trailing space after
// the user name made a second account at the login's auto-register. Whitespace
// around a name is forgiven now: GetByUsername's rule, Create's trim.

func newTrimTestServer(t *testing.T) (*AuthServer, *database.SQLiteAccountStore, *database.SQLiteDB) {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "trim_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.DefaultConfig()
	cfg.Auth.BCryptCost = 4
	store := database.NewSQLiteAccountStore(db)
	return NewAuthServer(cfg, store, auth.NewTicketStore(0)), store, db
}

// oldSpaced makes an account as one made before the fix: its name stored with
// the whitespace (Create trims now, so it is written directly).
func oldSpaced(t *testing.T, db *database.SQLiteDB, name, password string) int {
	t.Helper()
	hash, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO accountinfo (username, pwhash, acctstatus, acctlevel, acctcreation) VALUES (?, ?, 1, 0, CURRENT_TIMESTAMP)`, name, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func accountCount(t *testing.T, db *database.SQLiteDB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accountinfo`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func idOf(t *testing.T, store *database.SQLiteAccountStore, name string) int {
	t.Helper()
	a, err := store.GetByUsername(name)
	if err != nil {
		t.Fatalf("GetByUsername(%q): %v", name, err)
	}
	return a.ID
}

// A first login typed with a space around the name registers the name alone,
// and the name typed either way is that one account from then on.
func TestTrimLoginRegistersTheTrimmedName(t *testing.T) {
	s, store, db := newTrimTestServer(t)
	if !login(t, s, "Menace ", "pw1234") {
		t.Fatal("first login with a trailing space failed")
	}
	a, err := store.GetByUsername("Menace")
	if err != nil {
		t.Fatalf("no account under the trimmed name: %v", err)
	}
	if a.Username != "Menace" {
		t.Fatalf("stored name %q, want %q", a.Username, "Menace")
	}
	for _, typed := range []string{"Menace", " Menace", "menace\t", "Menace "} {
		if !login(t, s, typed, "pw1234") {
			t.Fatalf("login as %q failed", typed)
		}
		if id := idOf(t, store, typed); id != a.ID {
			t.Fatalf("%q is account %d, want %d", typed, id, a.ID)
		}
	}
	if n := accountCount(t, db); n != 1 {
		t.Fatalf("%d accounts, want 1", n)
	}
}

// Create trims, and refuses a name of whitespace alone.
func TestTrimCreateStoresTheTrimmedName(t *testing.T) {
	_, store, _ := newTrimTestServer(t)
	a, err := store.Create("  padded \t", "x")
	if err != nil {
		t.Fatal(err)
	}
	if a.Username != "padded" {
		t.Fatalf("Create returned %q", a.Username)
	}
	if got, _ := store.GetByUsername("padded"); got == nil || got.Username != "padded" {
		t.Fatalf("stored as %+v", got)
	}
	if _, err := store.Create("   ", "x"); err == nil {
		t.Fatal("a blank name was created")
	}
	if _, err := store.GetByUsername("  "); err == nil {
		t.Fatal("a blank name found an account")
	}
}

// An account made before the fix with a space in its name still logs in,
// typed with the space or without, and its name counts as taken: no twin is
// registered beside it.
func TestTrimOldSpacedAccountStillWorks(t *testing.T) {
	s, store, db := newTrimTestServer(t)
	trailing := oldSpaced(t, db, "Phoneuser ", "secret1")
	leading := oldSpaced(t, db, " Leading", "secret2")
	cases := []struct {
		typed, pw string
		id        int
	}{
		{"Phoneuser ", "secret1", trailing},
		{"Phoneuser", "secret1", trailing},
		{"phoneuser", "secret1", trailing},
		{" Leading", "secret2", leading},
		{"Leading", "secret2", leading},
	}
	for _, c := range cases {
		if !login(t, s, c.typed, c.pw) {
			t.Fatalf("login as %q failed", c.typed)
		}
		if id := idOf(t, store, c.typed); id != c.id {
			t.Fatalf("%q is account %d, want %d", c.typed, id, c.id)
		}
	}
	if login(t, s, "Phoneuser", "wrong") {
		t.Fatal("wrong password accepted through the trimmed name")
	}
	if n := accountCount(t, db); n != 2 {
		t.Fatalf("%d accounts, want 2: a login registered a twin", n)
	}
}

// Both a spaced account and the trimmed one exist (650 and 730 of petition
// 2830): the trimmed one wins whichever way the name is typed; the spaced one
// is left as it is (empty after a merge).
func TestTrimPrefersTheTrimmedTwin(t *testing.T) {
	s, store, db := newTrimTestServer(t)
	spaced := oldSpaced(t, db, "Twin ", "phonepw")
	plain := oldSpaced(t, db, "Twin", "laptoppw")
	for _, typed := range []string{"Twin", "Twin ", " Twin", "twin "} {
		if id := idOf(t, store, typed); id != plain {
			t.Fatalf("%q is account %d, want the trimmed %d (spaced %d)", typed, id, plain, spaced)
		}
		if !login(t, s, typed, "laptoppw") {
			t.Fatalf("login as %q with the trimmed account's password failed", typed)
		}
	}
	if login(t, s, "Twin ", "phonepw") {
		t.Fatal("the spaced twin's password logged in: the spaced twin was reached")
	}
}

// Several old spaced accounts of one name and no trimmed one: the oldest.
func TestTrimSeveralSpacedTakesTheOldest(t *testing.T) {
	_, store, db := newTrimTestServer(t)
	first := oldSpaced(t, db, "Dup ", "a1b2")
	oldSpaced(t, db, " Dup", "c3d4")
	if id := idOf(t, store, "Dup"); id != first {
		t.Fatalf("Dup is account %d, want the oldest %d", id, first)
	}
}
