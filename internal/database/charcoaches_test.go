package database

import (
	"testing"
	"time"
)

// A ledger signed is kept by character and town, once: signing it again
// reports nothing new and leaves the row (its time) as it was; a character
// that signed nothing has none, and a deleted character's go with it. The
// controls: another character's stay, and nothing of the table is named like
// a secret, which would refuse every database snapshot (snapshot.go).
func TestCoachLedgersAreKept(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/coaches.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	if towns, err := s.CoachTowns(7); err != nil || len(towns) != 0 {
		t.Fatalf("no row: %v (%v), want none", towns, err)
	}
	first := time.Date(2026, 10, 30, 1, 30, 0, 0, time.UTC)
	if signed, err := s.SignCoachLedger(7, "Freeport", first); err != nil || !signed {
		t.Fatalf("first signing: %v (%v), want signed", signed, err)
	}
	if signed, err := s.SignCoachLedger(7, "Freeport", first.Add(time.Hour)); err != nil || signed {
		t.Fatalf("second signing: %v (%v), want already signed", signed, err)
	}
	var source, when string
	db.QueryRow(`SELECT source, signed FROM charcoaches WHERE serverid = 7 AND town = 'Freeport'`).Scan(&source, &when)
	if source != "talk" || when != "2026-10-30T01:30:00Z" {
		t.Errorf("the row: source %q signed %q, want talk at the first signing", source, when)
	}
	s.SignCoachLedger(7, "Highpass Hold", first)
	s.SignCoachLedger(9, "Neriak", first)
	if towns, _ := s.CoachTowns(7); len(towns) != 2 || !towns["Freeport"] || !towns["Highpass Hold"] {
		t.Errorf("character 7 signed %v, want Freeport and Highpass Hold", towns)
	}

	c := &CharacterRecord{CharName: "Signer", AccountID: 1}
	if err := s.Create(c); err != nil {
		t.Fatal(err)
	}
	s.SignCoachLedger(c.ServerID, "Qeynos", first)
	if err := s.Delete(c.ServerID); err != nil {
		t.Fatal(err)
	}
	if towns, _ := s.CoachTowns(c.ServerID); len(towns) != 0 {
		t.Errorf("deleted character %d's ledgers are still kept: %v", c.ServerID, towns)
	}
	if towns, _ := s.CoachTowns(9); !towns["Neriak"] {
		t.Errorf("the control: character 9's are %v", towns)
	}
	for _, name := range []string{"charcoaches", "serverid", "town", "source", "signed"} {
		if SecretLikeName(name) {
			t.Errorf("%q reads like a secret: every snapshot would be refused", name)
		}
	}
}
