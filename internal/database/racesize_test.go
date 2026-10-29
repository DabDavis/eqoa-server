package database

import (
	"path/filepath"
	"testing"
)

// Table racesize is made with jib's gnomes at 0.9 (F3 20261029-212125), and
// only when it is made: a GM's delete of that row, or a change, stands at the
// next start. The control: another race has no row, so it is drawn at its
// own size.
func TestRaceSizesAreMadeWithTheGnomes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "racesize.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.RaceSizes()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[3] != 0.9 {
		t.Fatalf("the table as made: %v, want the gnomes at 0.9 only", got)
	}
	if _, ok := got[0]; ok {
		t.Fatal("the control: humans have a row")
	}
	if _, err := db.Exec(`DELETE FROM racesize WHERE race = 3`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO racesize (race, size) VALUES (9, 1.1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = db.RaceSizes()
	if err != nil {
		t.Fatal(err)
	}
	if _, back := got[3]; back || len(got) != 1 || got[9] != 1.1 {
		t.Fatalf("after a start: %v, want the GM's ogres alone", got)
	}
	for _, c := range []struct {
		size float64
		ok   bool
	}{{0.9, true}, {1, true}, {0.25, true}, {4, true}, {0, false}, {0.2, false}, {40, false}, {-1, false}} {
		if ValidRaceSize(c.size) != c.ok {
			t.Errorf("ValidRaceSize(%v) = %v", c.size, !c.ok)
		}
	}
}
