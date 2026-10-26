package database

import (
	"database/sql"
	"testing"
)

// A table made before roam's "as its kind" (roam defaulting to 1, so a row a
// GM added for its water made the creature wander) gets the default -1 at the
// start, its rows kept as they were; roam -1 reads as ByKind, 1 and 0 as
// before.
func TestNPCMoveRoamDefaultBecomesAsItsKind(t *testing.T) {
	path := t.TempDir() + "/npcmove.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
			water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')`,
		`INSERT INTO npcmove (name, roam, water, note, source) VALUES ('crocodile', 1, 'both', 'in and out', 'seed'),
			('hexbone', 0, '', 'stands', 'seed'), ('patron', 1, '', 'PoD', 'pod-seed')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if d, err := columnDefault(db, "npcmove", "roam"); err != nil || d != "-1" {
		t.Fatalf("roam's default %q, %v: want -1", d, err)
	}
	if _, err := db.Exec(`INSERT INTO npcmove (name, water) VALUES ('fire giant', 'land')`); err != nil {
		t.Fatal(err)
	}
	got, err := db.NPCMoveRules()
	if err != nil || len(got) != 4 {
		t.Fatalf("rules %v, %v", got, err)
	}
	want := map[string]NPCMoveRule{
		"crocodile":  {Roam: true, Water: "both"},
		"hexbone":    {},
		"patron":     {Roam: true},
		"fire giant": {ByKind: true, Water: "land"},
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %+v, want %+v", k, got[k], w)
		}
	}
	var note, source string
	if err := db.QueryRow(`SELECT note, source FROM npcmove WHERE name = 'patron'`).Scan(&note, &source); err != nil || note != "PoD" || source != "pod-seed" {
		t.Errorf("a row's other columns after the change: %q %q %v", note, source, err)
	}

	// the control: opened again, a table already at -1 is left as it is
	db.Close()
	db2, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	if got, err := db2.NPCMoveRules(); err != nil || len(got) != 4 || got["fire giant"] != want["fire giant"] {
		t.Errorf("a second start: %v, %v", got, err)
	}
}

// A new table is made with the default -1.
func TestNPCMoveNewTableIsAsItsKind(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/fresh.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if d, err := columnDefault(db, "npcmove", "roam"); err != nil || d != "-1" {
		t.Fatalf("roam's default %q, %v", d, err)
	}
}
