package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// An npcdialog table from before the use column gets it at the start, every old row a talk's (empty), and a row
// written with a use reads back with it.
func TestNPCDialogUseColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '',
		race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
		source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO npcdialog (name, line) VALUES ('Old', 'Hello.')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcdialog (name, line, use) VALUES ('Merchant', 'My wares.', 'shop')`); err != nil {
		t.Fatal(err)
	}
	InvalidateNPCDialog()
	lines, err := db.NPCDialog()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range lines {
		got[l.Name] = l.Use
	}
	if u, ok := got["Old"]; !ok || u != DialogUseTalk {
		t.Errorf("the old row: use %q (%v), want a talk's", u, ok)
	}
	if got["Merchant"] != DialogUseShop {
		t.Errorf("the shop row: use %q", got["Merchant"])
	}
}
