package database

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// A table of one merc a character (2026-11-10 to 11-12: its key the
// character's id alone) is made the table of slots at the open: each row its
// slot 1, every column kept; a second open changes nothing; three hires a
// character then live side by side.
func TestMercSlotsMigration(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE charmerc (serverid INTEGER PRIMARY KEY, name TEXT NOT NULL, class INTEGER NOT NULL, race INTEGER NOT NULL,
			sex INTEGER NOT NULL DEFAULT 0, face INTEGER NOT NULL DEFAULT 0, hair_color INTEGER NOT NULL DEFAULT 0,
			hair_length INTEGER NOT NULL DEFAULT 0, hair_style INTEGER NOT NULL DEFAULT 0, stance INTEGER NOT NULL DEFAULT 2,
			dead_until TEXT NOT NULL DEFAULT '', hired TEXT NOT NULL DEFAULT '', path INTEGER NOT NULL DEFAULT -1,
			pet TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO charmerc (serverid, name, class, race, sex, stance, path, pet) VALUES (77, 'Ralthas', 9, 2, 1, 3, 1, 'air')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	for range 2 {
		db, err := NewSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		r, ok, err := db.Merc(77, 1)
		if err != nil || !ok || r.Slot != 1 || r.Name != "Ralthas" || r.Class != 9 || r.Race != 2 || r.Sex != 1 || r.Stance != 3 ||
			r.Path != 1 || r.Pet != "air" {
			t.Fatalf("migrated: %+v %v %v", r, ok, err)
		}
		db.Close()
	}
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for s := 2; s <= MercSlots; s++ {
		if err := db.SetMerc(MercRow{ServerID: 77, Slot: s, Name: "M", Class: s, Path: -1}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Mercs(77)
	if err != nil || len(rows) != 3 || rows[2].Class != 3 {
		t.Fatalf("three hires: %+v %v", rows, err)
	}
	if err := db.SetMercStance(77, 2, 1); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := db.Merc(77, 1); r.Stance != 3 {
		t.Errorf("slot 2's stance reached slot 1: %d", r.Stance)
	}
	if err := db.DeleteMerc(77, 2); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.Mercs(77); len(rows) != 2 {
		t.Errorf("after one dismissal: %d rows", len(rows))
	}
	if err := db.DeleteMercs(77); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.Mercs(77); len(rows) != 0 {
		t.Errorf("after the character's deletion: %d rows", len(rows))
	}
}
