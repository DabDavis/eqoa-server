package database

import "testing"

// A GM's act that is no row change (#reward's buff, petition 1501) is a "live" line with no table, as the panel's
// kicks are: the GM named by its account, the detail kept. The control: an account with no name is refused.
func TestAuditGMLive(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/live.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO accountinfo (accountid, username) VALUES (44, 'jibrish')`); err != nil {
		t.Skipf("accountinfo here: %v", err)
	}
	id, err := db.AuditGMLive(GMEdit{AccountID: 44, Via: "game", IP: "10.0.0.1"}, "#reward in the game: GM's Reward on Bob")
	if err != nil || id == 0 {
		t.Fatalf("the line: %d (%v)", id, err)
	}
	var user, via, action, tbl, detail string
	db.QueryRow(`SELECT username, via, action, tbl, detail FROM gm_audit WHERE id = ?`, id).Scan(&user, &via, &action, &tbl, &detail)
	if user != "jibrish" || via != "game" || action != "live" || tbl != "" || detail != "#reward in the game: GM's Reward on Bob" {
		t.Errorf("the line: %s %s %s %q %q", user, via, action, tbl, detail)
	}
	if _, err := db.AuditGMLive(GMEdit{AccountID: 99, Via: "game"}, "x"); err == nil {
		t.Error("an account with no name was written")
	}
}
