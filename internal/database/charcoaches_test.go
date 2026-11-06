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

// Every ledger at once (a coachman's offer, handlers/coachledger.go) signs in
// one go each town not yet signed, under its own source, and reports how many
// it signed; a town signed before keeps its row (source and time), and a
// second time signs none. The control: another character's are untouched.
func TestEveryCoachLedgerAtOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/coaches.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	first := time.Date(2026, 10, 30, 1, 30, 0, 0, time.UTC)
	s.SignCoachLedger(7, "Freeport", first)
	s.SignCoachLedger(9, "Neriak", first)
	at := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)
	towns := []string{"Freeport", "Highpass Hold", "Neriak", "Qeynos"}
	if n, err := s.SignCoachLedgers(7, towns, CoachLedgersAll, at); err != nil || n != 3 {
		t.Fatalf("signed %d (%v), want the 3 not signed", n, err)
	}
	rows := map[string][2]string{}
	r, err := db.Query(`SELECT town, source, signed FROM charcoaches WHERE serverid = 7`)
	if err != nil {
		t.Fatal(err)
	}
	for r.Next() {
		var town, source, when string
		r.Scan(&town, &source, &when)
		rows[town] = [2]string{source, when}
	}
	r.Close()
	want := map[string][2]string{
		"Freeport":      {"talk", "2026-10-30T01:30:00Z"},
		"Highpass Hold": {CoachLedgersAll, "2026-11-06T03:00:00Z"},
		"Neriak":        {CoachLedgersAll, "2026-11-06T03:00:00Z"},
		"Qeynos":        {CoachLedgersAll, "2026-11-06T03:00:00Z"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows %v, want %v", rows, want)
	}
	for town, w := range want {
		if rows[town] != w {
			t.Errorf("%s: %v, want %v", town, rows[town], w)
		}
	}
	if n, err := s.SignCoachLedgers(7, towns, CoachLedgersAll, at.Add(time.Hour)); err != nil || n != 0 {
		t.Errorf("a second time signed %d (%v), want none", n, err)
	}
	if got, _ := s.CoachTowns(9); len(got) != 1 || !got["Neriak"] {
		t.Errorf("the control: character 9's are %v", got)
	}
	if SecretLikeName(CoachLedgersAll) {
		t.Errorf("%q reads like a secret", CoachLedgersAll)
	}
}
