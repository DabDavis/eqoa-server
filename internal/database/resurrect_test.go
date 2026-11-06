package database

import "testing"

// A character's chance of a rez is kept by character, a second save replaces
// the first, and one with no debt or no time left takes the row away; a
// character with no row has none. The control: a save under another
// character leaves this one's alone. The table is made with the schema.
func TestRezRoundTrip(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/rez.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	if got, err := s.LoadRez(7); err != nil || got != (RezRow{}) {
		t.Fatalf("no row: %+v (%v), want none", got, err)
	}
	a := RezRow{Debt: 6137969, LeftMS: 1_200_000}
	if err := s.SaveRez(7, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRez(9, RezRow{Debt: 2195, LeftMS: 60_000}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadRez(7); got != a {
		t.Fatalf("loaded %+v, want %+v", got, a)
	}
	b := RezRow{Debt: 862615, LeftMS: 1_799_000}
	if err := s.SaveRez(7, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadRez(7); got != b {
		t.Errorf("after a second save %+v, want %+v", got, b)
	}
	for _, none := range []RezRow{{}, {Debt: 862615}, {LeftMS: 5000}, {Debt: 862615, LeftMS: -1}} {
		if err := s.SaveRez(7, b); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveRez(7, none); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charrez WHERE serverid = 7`).Scan(&n); err != nil || n != 0 {
			t.Errorf("saving %+v left %d rows (%v)", none, n, err)
		}
	}
	if got, _ := s.LoadRez(9); got != (RezRow{Debt: 2195, LeftMS: 60_000}) {
		t.Errorf("the control: character 9's row is %+v", got)
	}
	// a death's place is kept with it (petition 1130), a low level's with no debt too,
	// and a second start of the schema leaves the row as it is
	placed := RezRow{LeftMS: 90_000, Placed: true, World: 3, X: 25146.277, Y: 54.125, Z: 16733.584}
	if err := s.SaveRez(7, placed); err != nil {
		t.Fatal(err)
	}
	if err := db.ensureCharRez(); err != nil {
		t.Fatalf("the schema a second time: %v", err)
	}
	if got, err := s.LoadRez(7); err != nil || got != placed {
		t.Errorf("a place: loaded %+v (%v), want %+v", got, err, placed)
	}
}
