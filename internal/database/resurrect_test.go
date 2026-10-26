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
}
