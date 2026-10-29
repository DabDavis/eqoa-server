package database

import (
	"database/sql"
	"testing"
)

// A balancelevel table made before a column (2026-10-29's first tables) gets
// it at the next open: the rows still the seed's take the seed's value of it,
// a GM's row the column's default; a new table has it with its seed rows.
func TestBalanceColumnsAddedToAnOlderTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100,
		damage_pct REAL NOT NULL DEFAULT 100, con_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balancelevel (level, hp_pct, source) VALUES (30, 300, 'seed'), (33, 310, 'gm');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.BalanceLevels()
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]float64{}
	for _, r := range rows {
		got[r.Level] = r.ArtCooldownMS
	}
	if len(rows) != 2 || got[30] != 1500 || got[33] != 0 {
		t.Errorf("after the open: %+v (want the seed row 1500, the GM's 0, no rows added)", rows)
	}

	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	rows, err = fresh.BalanceLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(BalanceSeedLevels) || rows[0].ArtCooldownMS != BalanceSeedLevels[0].ArtCooldownMS {
		t.Errorf("a new table: %d rows, first %+v", len(rows), rows[0])
	}
}
