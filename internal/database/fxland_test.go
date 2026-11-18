package database

import "testing"

// Table fxland is made with work board #1643's five rows, by the entry's decimal DictIDs (as spelleffect.fxdict
// stores them), once: a GM's delete of a seed stands when the database is opened again.
func TestFXLandSeeds(t *testing.T) {
	path := t.TempDir() + "/fx.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, bad, err := db.FXLand()
	if err != nil || len(bad) != 0 {
		t.Fatalf("FXLand: %v %v", bad, err)
	}
	want := map[uint32]float64{129220811: 2000, 3932384717: 2000, 2064967201: 2500, 2064967202: 2500, 2064967203: 2500}
	if len(rows) != len(want) {
		t.Errorf("%d rows, want %d: %v", len(rows), len(want), rows)
	}
	for fx, ms := range want {
		if rows[fx] != ms {
			t.Errorf("%#08x: %v, want %v", fx, rows[fx], ms)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fxland WHERE source = 'seed' AND fxdict = 3932384717`).Scan(&n); err != nil || n != 1 {
		t.Errorf("the warriors' seed stored unsigned: %d %v", n, err)
	}
	if _, err := db.Exec(`DELETE FROM fxland WHERE fxdict = 129220811`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO fxland (fxdict, land_ms) VALUES (1, 20000)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, bad, err = db.FXLand()
	if err != nil || len(rows) != 4 || len(bad) != 1 {
		t.Errorf("opened again: %v rows, bad %v (%v); want the delete kept and the 20000 left out", rows, bad, err)
	}
}
