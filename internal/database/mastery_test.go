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

// The carry to a new level table runs once: every row's total converted,
// the version kept, and a second run (a restart) changes nothing. The
// control: a new version runs again.
func TestMigrateTotalXPRunsOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/xp.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	for i, total := range []int{1000, 0, 77} {
		if _, err := db.Exec(`INSERT INTO characters (charName, accountid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle, faceoption, sex, totalXP)
			VALUES (?, 1, 0, 0, 0, 0, 10, 0, 0, 0, 0, 0, ?)`, "C"+string(rune('a'+i)), total); err != nil {
			t.Fatal(err)
		}
	}
	double := func(level, total int) int { return total * 2 }
	if n, err := s.MigrateTotalXP("v1", double); err != nil || n != 2 {
		t.Fatalf("first run: %d rows (%v), want 2", n, err)
	}
	if n, _ := s.MigrateTotalXP("v1", double); n != 0 {
		t.Fatalf("a second run changed %d rows", n)
	}
	var total int
	db.QueryRow(`SELECT totalXP FROM characters WHERE charName = 'Ca'`).Scan(&total)
	if total != 2000 {
		t.Errorf("converted once: %d, want 2000", total)
	}
	if n, _ := s.MigrateTotalXP("v2", double); n != 2 {
		t.Errorf("the control: a new version must run (%d rows)", n)
	}
}
