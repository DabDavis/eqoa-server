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

// A seed value revised since a table was made: the seed rows still at the
// old value take the new one, a row a GM changed (or wrote) keeps its own.
func TestBalanceSeedRevisionsReachAnOlderTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100,
		damage_pct REAL NOT NULL DEFAULT 100, con_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balancelevel (level, pet_hp_pct, source) VALUES (50, 510, 'seed'), (60, 777, 'seed'), (45, 410, 'gm');`); err != nil {
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
		got[r.Level] = r.PetHPPct
	}
	if got[50] != 357 || got[60] != 777 || got[45] != 410 {
		t.Errorf("pet_hp_pct after the open: %v (want 50's seed revised to 357, a GM's 777 and a gm row's 410 left)", got)
	}
}

// balanceclasslevel is seeded only with a fresh balance: a database whose
// balance tables were made before (eqoaclassic's of 2026-10-29) gets it
// empty at the next open, its rows tools/tune_balance.py's to write with the
// rest of the tuning; the control: a new database gets the seed rows.
func TestBalanceClassLevelsSeedOnlyAFreshBalance(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100,
		damage_pct REAL NOT NULL DEFAULT 100, con_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balancelevel (level, hp_pct, source) VALUES (30, 300, 'seed');
		CREATE TABLE balanceclass (class INTEGER PRIMARY KEY, melee_taken_pct REAL NOT NULL DEFAULT 100,
		note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balanceclass (class, melee_taken_pct, source) VALUES (6, 65, 'seed');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.BalanceClassLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("an older balance's new balanceclasslevel: %d rows (want none: the tool's to fill)", len(rows))
	}
	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if rows, err = fresh.BalanceClassLevels(); err != nil || len(rows) != len(BalanceSeedClassLevels) || len(rows) == 0 {
		t.Errorf("a new database: %d rows, %v (want the %d seed rows)", len(rows), err, len(BalanceSeedClassLevels))
	}
}

// A balanceclasslevel made before its pet column gets it at the next open,
// every row 100 (the whole of balancelevel's pet hit points); the control:
// its melee column is kept as it was.
func TestBalanceClassLevelPetColumnAddedToAnOlderTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL,
		melee_taken_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (class, level));
		INSERT INTO balanceclasslevel (class, level, melee_taken_pct, source) VALUES (6, 30, 55, 'gm');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.BalanceClassLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].MeleeTakenPct != 55 || rows[0].PetHPPct != 100 || rows[0].DealtPct != 100 {
		t.Errorf("after the open: %+v (want the GM's 55 kept and the pet and dealt columns 100)", rows)
	}
}

// A balanceclasslevel made with its pet column but before its dealt column
// (eqoaclassic's of 2026-10-30 00:27) gets it at the next open, every row
// 100 (all of a player's damage); the control: the rows' other columns kept.
// A new database's seed rows carry theirs (balanceseed.go; 100 where unset).
func TestBalanceClassLevelDealtColumnAddedToAnOlderTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL,
		melee_taken_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (class, level));
		INSERT INTO balanceclasslevel (class, level, melee_taken_pct, pet_hp_pct, source) VALUES (13, 10, 44, 100, 'seed'),
		(11, 30, 90, 160, 'seed');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.BalanceClassLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].MeleeTakenPct != 90 || rows[0].PetHPPct != 160 || rows[0].DealtPct != 100 ||
		rows[1].MeleeTakenPct != 44 || rows[1].DealtPct != 100 {
		t.Errorf("after the open: %+v (want both rows kept, dealt 100)", rows)
	}
	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	seeded, err := fresh.BalanceClassLevels()
	if err != nil || len(seeded) != len(BalanceSeedClassLevels) {
		t.Fatalf("a new database: %d rows, %v", len(seeded), err)
	}
	for i, r := range seeded {
		if want := BalanceSeedClassLevels[i].Dealt(); r.DealtPct != want {
			t.Errorf("seed row %d %d: dealt %g, want %g", r.Class, r.Level, r.DealtPct, want)
		}
	}
}

// pet_special_hits (the spectre's instant, 2026-10-30) reaches a balancelevel
// made before it with the column's default, 1.5, on every row, the seed's and
// a GM's alike; a new table's seed rows carry it; a GM's value of it is kept
// over the next open (the control: the column is added once, not reset).
func TestBalancePetSpecialHitsColumn(t *testing.T) {
	for _, r := range BalanceSeedLevels {
		if r.PetSpecialHits != BalancePetSpecialHitsDefault {
			t.Fatalf("seed row %d: pet_special_hits %g, want %g", r.Level, r.PetSpecialHits, BalancePetSpecialHitsDefault)
		}
	}
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100,
		damage_pct REAL NOT NULL DEFAULT 100, con_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100, art_cooldown_ms REAL NOT NULL DEFAULT 0, regen_pct REAL NOT NULL DEFAULT 15,
		note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO balancelevel (level, hp_pct, source) VALUES (30, 300, 'seed'), (33, 310, 'gm');`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	open := func() map[int]float64 {
		t.Helper()
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
			got[r.Level] = r.PetSpecialHits
		}
		return got
	}
	if got := open(); len(got) != 2 || got[30] != 1.5 || got[33] != 1.5 {
		t.Errorf("an older table: %v, want 1.5 on both rows", got)
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE balancelevel SET pet_special_hits = 5 WHERE level = 33`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if got := open(); got[30] != 1.5 || got[33] != 5 {
		t.Errorf("a GM's 5 over the next open: %v", got)
	}

	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	rows, err := fresh.BalanceLevels()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.PetSpecialHits != 1.5 {
			t.Fatalf("a new table's row %d: %g", r.Level, r.PetSpecialHits)
		}
	}
	if BalancePetSpecialHitsProblem(3) != "" || BalancePetSpecialHitsProblem(0) != "" || BalancePetSpecialHitsProblem(-1) == "" ||
		BalancePetSpecialHitsProblem(21) == "" {
		t.Error("bounds: want 0-20")
	}
}
