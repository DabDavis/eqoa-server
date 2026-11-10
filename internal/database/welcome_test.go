package database

import "testing"

// charwelcome (welcome.go): a character with no row is due; taking a day makes
// it not due that day, once (a second take the same day is refused, so two
// sessions cannot both send the box), and due again the next day; never makes
// it due on no day and refuses every take, for that character alone, until it
// is taken off; and a character deleted takes its row with it.
func TestCharWelcomeKeepsTheDayAndNever(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/welcome.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	due := func(id uint32, day string) bool {
		t.Helper()
		d, err := db.WelcomeDue(id, day)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	take := func(id uint32, day string) bool {
		t.Helper()
		ok, err := db.TakeWelcomeDay(id, day)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	never := func(id uint32) bool {
		t.Helper()
		n, err := db.WelcomeNever(id)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	if !due(7, "2027-01-15") || never(7) {
		t.Fatal("a character never asked is not due")
	}
	if !take(7, "2027-01-15") {
		t.Fatal("the day's first take refused")
	}
	if due(7, "2027-01-15") || take(7, "2027-01-15") {
		t.Fatal("the day taken twice")
	}
	if !due(7, "2027-01-16") || !take(7, "2027-01-16") {
		t.Fatal("the next day not due")
	}

	if err := db.SetWelcomeNever(7, true); err != nil {
		t.Fatal(err)
	}
	if !never(7) || due(7, "2027-01-17") || take(7, "2027-01-17") {
		t.Fatal("never did not stop the box")
	}
	// the control: another character is its own
	if !due(8, "2027-01-17") || never(8) {
		t.Fatal("never reached another character")
	}
	// never on a character with no row yet makes one
	if err := db.SetWelcomeNever(9, true); err != nil || !never(9) || due(9, "2027-01-17") {
		t.Fatalf("never on a new row: %v", err)
	}
	if err := db.SetWelcomeNever(7, false); err != nil {
		t.Fatal(err)
	}
	if never(7) || !due(7, "2027-01-17") {
		t.Fatal("never taken off left the box off")
	}

	// a deleted character's row goes with it
	chars := NewSQLiteCharacterStore(db)
	rec := &CharacterRecord{CharName: "Welcomed", AccountID: 1, Level: 1}
	if err := chars.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWelcomeNever(rec.ServerID, true); err != nil || !never(rec.ServerID) {
		t.Fatalf("its never: %v", err)
	}
	if err := chars.Delete(rec.ServerID); err != nil {
		t.Fatal(err)
	}
	if never(rec.ServerID) || !due(rec.ServerID, "2027-01-17") {
		t.Fatal("a deleted character's row kept")
	}
}
