package database

import (
	"testing"
	"time"
)

// A first meeting makes the row (its time the first seen) and a later one changes
// nothing; a kill counts up on the row, making it when the meeting was not written;
// the kind and the zone are matched in any case, and another zone is another row. The
// controls: a note with no character or no kind writes nothing, and another
// character's rows are not read.
func TestBestiaryCountsMeetingsAndKills(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/bestiary.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	notes := []BestiaryNote{
		{ServerID: 7, Kind: "gnoll pup", Zone: "Blackburrow", At: t0},
		{ServerID: 7, Kind: "gnoll pup", Zone: "Blackburrow", At: t0.Add(time.Hour)},
		{ServerID: 7, Kind: "Gnoll Pup", Zone: "blackburrow", At: t0.Add(2 * time.Hour), Kill: true},
		{ServerID: 7, Kind: "gnoll pup", Zone: "Qeynos Hills", At: t0.Add(3 * time.Hour), Kill: true},
		{ServerID: 7, Kind: "a bat", Zone: "Blackburrow", At: t0.Add(4 * time.Hour), Kill: true},
		{ServerID: 7, Kind: "a bat", Zone: "Blackburrow", At: t0.Add(5 * time.Hour), Kill: true},
		{ServerID: 0, Kind: "rat", Zone: "x", At: t0},
		{ServerID: 7, Kind: "  ", Zone: "x", At: t0},
		{ServerID: 8, Kind: "rat", Zone: "Freeport", At: t0},
	}
	if err := db.RecordBestiary(notes[:3]); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBestiary(notes[3:]); err != nil {
		t.Fatal(err)
	}
	got, err := db.BestiaryOf(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("rows: %+v", got)
	}
	want := []BestiaryEntry{
		{Kind: "a bat", Zone: "Blackburrow", FirstSeen: t0.Add(4 * time.Hour), Kills: 2, LastKill: t0.Add(5 * time.Hour)},
		{Kind: "gnoll pup", Zone: "Blackburrow", FirstSeen: t0, Kills: 1, LastKill: t0.Add(2 * time.Hour)},
		{Kind: "gnoll pup", Zone: "Qeynos Hills", FirstSeen: t0.Add(3 * time.Hour), Kills: 1, LastKill: t0.Add(3 * time.Hour)},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if other, _ := db.BestiaryOf(8); len(other) != 1 || other[0].Kills != 0 || !other[0].LastKill.IsZero() {
		t.Errorf("another character's: %+v", other)
	}
	if none, _ := db.BestiaryOf(0); len(none) != 0 {
		t.Errorf("no character: %+v", none)
	}
}
