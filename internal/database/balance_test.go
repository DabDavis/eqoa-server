package database

import (
	"database/sql"
	"testing"
)

// A balancelevel table made before a column (2026-10-29's first tables) gets
// it at the next open: the rows still the seed's take the seed's value of it,
// a GM's row the column's default (regen_pct's is jib's 15); a new table has
// it with its seed rows.
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
	got, regen := map[int]float64{}, map[int]float64{}
	for _, r := range rows {
		got[r.Level], regen[r.Level] = r.ArtCooldownMS, r.RegenPct
	}
	if len(rows) != 2 || got[30] != 1500 || got[33] != 0 {
		t.Errorf("after the open: %+v (want the seed row 1500, the GM's 0, no rows added)", rows)
	}
	if regen[30] != 5 || regen[33] != 15 {
		t.Errorf("regen_pct: seed row %g, GM's %g; want 5 (the seed's) and 15 (the column's default)", regen[30], regen[33])
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

// An older balancecon gets the root columns: its seed rows the seed's.
func TestBalanceRootColumnsAddedToAnOlderTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balancecon (diff INTEGER PRIMARY KEY, damage_pct REAL NOT NULL DEFAULT 100,
		taken_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balancecon (diff, damage_pct, source) VALUES (5, 150, 'seed'), (7, 160, 'gm');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.BalanceCons()
	if err != nil {
		t.Fatal(err)
	}
	var five BalanceCon
	for _, r := range BalanceSeedCons {
		if r.Diff == 5 {
			five = r
		}
	}
	if len(rows) != 2 || rows[0].RootResistPct != five.RootResistPct || rows[0].RootBreakPct != five.RootBreakPct ||
		rows[1].RootResistPct != 0 || rows[1].RootBreakPct != 0 {
		t.Errorf("after the open: %+v", rows)
	}
}
