package database

import (
	"path/filepath"
	"testing"
)

// The Hill Giant was code until the table: a database already holding one of its
// characters, the table new, is given its row, so a binary that starts before any
// data step keeps the character as it was. Once only: a GM's delete stands at the
// next start. The control: a database with no such character (a new server's)
// makes the table empty.
func TestTheFirstRaceRowComesWithTheTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "racedefs.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := db.RaceDefRows(); err != nil || len(got) != 0 {
		t.Fatalf("a new server's table: %v, %v; want empty", got, err)
	}
	store := NewSQLiteCharacterStore(db)
	if err := store.Create(&CharacterRecord{CharName: "Bigfellow", AccountID: 1, Level: 60, Race: 11, ModelID: -5117396}); err != nil {
		t.Fatal(err)
	}
	if err := db.ensureRaceDefs(); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.RaceDefRows(); len(got) != 0 {
		t.Fatalf("the table was there: %v, want no row made", got)
	}

	// as a database from before the table: dropped, with the character in it
	if _, err := db.Exec(`DROP TABLE racedefs`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.RaceDefRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("the rows: %v, want the Hill Giant", got)
	}
	r := got[0]
	if r.ID != 11 || r.Name != "Hill Giant" || r.ShortName != "HGT" || r.WireRace != 9 || r.Bonus != [7]int{25, 20, -5, -5, 0, 0, 0} ||
		r.BodyMale != -5117396 || r.BodyFemale != -5117396 || r.Source != "seed" {
		t.Errorf("the row: %+v", r)
	}

	if _, err := db.Exec(`DELETE FROM racedefs`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got, _ := db.RaceDefRows(); len(got) != 0 {
		t.Fatalf("after a GM's delete and a start: %v, want it to stand", got)
	}
}

// CharactersOfRaces names the characters of those races and no others, by race
// and then name; no races asks nothing.
func TestCharactersOfRaces(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "racechars.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteCharacterStore(db)
	for _, c := range []CharacterRecord{
		{CharName: "Zed", AccountID: 1, Race: 11, Sex: 1, ModelID: -7}, {CharName: "Amy", AccountID: 1, Race: 12},
		{CharName: "Bob", AccountID: 1, Race: 11}, {CharName: "Gnomey", AccountID: 1, Race: 3},
	} {
		if err := store.Create(&c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.CharactersOfRaces([]int{11, 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "Bob" || got[1].Name != "Zed" || got[2].Name != "Amy" {
		t.Fatalf("got %+v", got)
	}
	if got[1].Sex != 1 || got[1].ModelID != -7 || got[1].Race != 11 {
		t.Errorf("Zed's row: %+v", got[1])
	}
	if got, err := db.CharactersOfRaces(nil); err != nil || got != nil {
		t.Errorf("no races: %v, %v", got, err)
	}
}
