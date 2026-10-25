package database

import "testing"

// A character's mastery points are kept by character, a second save
// replaces the first, and a character with no row has none. The control: a
// save under another character leaves this one's alone.
func TestMasteryRoundTrip(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/cm.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	if got, err := s.LoadMastery(7); err != nil || got != (MasteryRow{}) {
		t.Fatalf("no row: %+v (%v), want none", got, err)
	}
	a := MasteryRow{Points: 12, Spent: 765, Percent: 100, XP: 4321}
	if err := s.SaveMastery(7, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMastery(9, MasteryRow{Points: 1, Percent: 5}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadMastery(7); got != a {
		t.Fatalf("loaded %+v, want %+v", got, a)
	}
	b := MasteryRow{Points: 13, Spent: 765, Percent: 40, XP: 0}
	if err := s.SaveMastery(7, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadMastery(7); got != b {
		t.Errorf("after a second save %+v, want %+v", got, b)
	}
	if got, _ := s.LoadMastery(9); got != (MasteryRow{Points: 1, Percent: 5}) {
		t.Errorf("the control: character 9's row is %+v", got)
	}
}
