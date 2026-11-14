package database

import "testing"

// loginannounce (loginannounce.go): none at first; set, changed and cleared, each change a gm_audit row change with
// the row before and after (so the panel reverts it), setting what is there or clearing none writing nothing; and
// the announcement outlives the database closed and opened again, as a restart does.
func TestLoginAnnounceIsSetClearedAndKept(t *testing.T) {
	path := t.TempDir() + "/announce.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accountinfo (accountid, username, acctlevel) VALUES (1, 'jibwiz', 1)`); err != nil {
		t.Fatal(err)
	}
	cur := func() LoginAnnounce {
		t.Helper()
		a, err := db.LoginAnnounce()
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	audits := func() int {
		t.Helper()
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'loginannounce'`).Scan(&n)
		return n
	}
	who := GMEdit{AccountID: 1, Via: "game", IP: "203.0.113.5"}
	if a := cur(); a.Text != "" {
		t.Fatalf("a new database has %+v", a)
	}
	if old, err := db.SetLoginAnnounceAudited("", who, "clear"); err != nil || old.Text != "" || audits() != 0 {
		t.Fatalf("clearing none: %+v %v, %d audit lines", old, err, audits())
	}
	if _, err := db.SetLoginAnnounceAudited("Welcome to Heirloom Test", who, "set"); err != nil {
		t.Fatal(err)
	}
	if a := cur(); a.Text != "Welcome to Heirloom Test" || a.SetBy != "jibwiz" || a.Changed == "" {
		t.Fatalf("set: %+v", a)
	}
	if _, err := db.SetLoginAnnounceAudited("Welcome to Heirloom Test", who, "again"); err != nil || audits() != 1 {
		t.Fatalf("the same text again: %v, %d audit lines", err, audits())
	}
	old, err := db.SetLoginAnnounceAudited("Double experience tonight", who, "change")
	if err != nil || old.Text != "Welcome to Heirloom Test" {
		t.Fatalf("change: %+v %v", old, err)
	}
	var action, before, after, user string
	db.QueryRow(`SELECT action, COALESCE(before, ''), COALESCE(after, ''), username FROM gm_audit WHERE tbl = 'loginannounce' ORDER BY id DESC LIMIT 1`).
		Scan(&action, &before, &after, &user)
	if action != "update" || user != "jibwiz" || before == "" || after == "" {
		t.Fatalf("the change's line: %s %q -> %q by %s", action, before, after, user)
	}

	// a restart
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if a := cur(); a.Text != "Double experience tonight" || a.SetBy != "jibwiz" {
		t.Fatalf("after reopening: %+v", a)
	}

	if old, err := db.SetLoginAnnounceAudited("", who, "clear"); err != nil || old.Text != "Double experience tonight" {
		t.Fatalf("clear: %+v %v", old, err)
	}
	if a := cur(); a.Text != "" {
		t.Fatalf("cleared, it is %+v", a)
	}
	db.QueryRow(`SELECT action FROM gm_audit WHERE tbl = 'loginannounce' ORDER BY id DESC LIMIT 1`).Scan(&action)
	if action != "delete" || audits() != 3 {
		t.Fatalf("the clear's line: %s, %d lines", action, audits())
	}
}
