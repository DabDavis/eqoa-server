package database

import (
	"reflect"
	"sort"
	"testing"
)

// The defaults are the capture's 331, in its own order: standing high to
// low, then name.
func TestDefaultFactionsAreTheCapturesInItsOrder(t *testing.T) {
	if len(DefaultFactions) != 331 {
		t.Fatalf("%d factions, want the capture's 331", len(DefaultFactions))
	}
	sorted := append([]FactionDefault(nil), DefaultFactions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Standing != sorted[j].Standing {
			return sorted[i].Standing > sorted[j].Standing
		}
		return sorted[i].Name < sorted[j].Name
	})
	if !reflect.DeepEqual(sorted, DefaultFactions) {
		t.Error("the defaults are not in the window's order")
	}
	seen := map[string]bool{}
	for _, f := range DefaultFactions {
		if seen[f.Name] {
			t.Errorf("%q twice", f.Name)
		}
		seen[f.Name] = true
	}
}

// A new database is given the defaults; a GM's edit of them survives the
// next start (it is filled only while empty).
func TestFactionsTableIsFilledOnceAndKept(t *testing.T) {
	path := t.TempDir() + "/f.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.FactionDefaults()
	if err != nil || !reflect.DeepEqual(got, DefaultFactions) {
		t.Fatalf("a new database's factions: %d (%v), want the %d defaults in order", len(got), err, len(DefaultFactions))
	}
	if _, err := db.Exec(`UPDATE factions SET standing = -700 WHERE name = 'Citizens of Freeport'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM factions WHERE name = 'Test Faction'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, _ = db.FactionDefaults()
	if len(got) != len(DefaultFactions)-1 {
		t.Errorf("after a restart: %d factions, want the GM's %d", len(got), len(DefaultFactions)-1)
	}
	for _, f := range got {
		if f.Name == "Citizens of Freeport" && f.Standing != -700 {
			t.Errorf("Citizens of Freeport starts at %d, want the GM's -700", f.Standing)
		}
		if f.Name == "Test Faction" {
			t.Error("a deleted faction came back")
		}
	}
}

// A character's standings round the table: a second save replaces the first,
// an empty one leaves none, another character's are its own; SetFaction keeps
// one.
func TestCharacterFactionsRoundTrip(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/cf.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	m := map[string]int{"Citizens of Freeport": -24, "The Freeport Militia": -49}
	if err := s.SaveFactions(7, m); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFactions(9, map[string]int{"Citizens of Qeynos": 5}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadFactions(7); err != nil || !reflect.DeepEqual(got, m) {
		t.Fatalf("loaded %v (%v), want %v", got, err, m)
	}
	if err := s.SaveFactions(7, map[string]int{"Citizens of Freeport": -100}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadFactions(7); !reflect.DeepEqual(got, map[string]int{"Citizens of Freeport": -100}) {
		t.Errorf("after a second save: %v", got)
	}
	if err := s.SetFaction(7, "Citizens of Freeport", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFaction(7, "Klick`Anon Watch", 299); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadFactions(7); !reflect.DeepEqual(got, map[string]int{"Citizens of Freeport": 3, "Klick`Anon Watch": 299}) {
		t.Errorf("after SetFaction: %v", got)
	}
	if err := s.SaveFactions(7, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadFactions(7); len(got) != 0 {
		t.Errorf("after an empty save: %v", got)
	}
	if got, _ := s.LoadFactions(9); !reflect.DeepEqual(got, map[string]int{"Citizens of Qeynos": 5}) {
		t.Errorf("another character's: %v", got)
	}
}

// The hits are read whole; one row per NPC name and faction.
func TestFactionHitsAreRead(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/fh.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, r := range []FactionHitRow{
		{"Guard Dahvel", "Citizens of Freeport", -25},
		{"Guard Dahvel", "The Freeport Militia", -50},
		{"a gnoll pup", "Citizens of Qeynos", 2},
	} {
		if _, err := db.Exec(`INSERT INTO factionhits (npc_name, faction, delta) VALUES (?, ?, ?)`, r.NPCName, r.Faction, r.Delta); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO factionhits (npc_name, faction, delta) VALUES ('Guard Dahvel', 'Citizens of Freeport', -1)`); err == nil {
		t.Error("a second row for one NPC name and faction was taken")
	}
	got, err := db.FactionHits()
	if err != nil {
		t.Fatal(err)
	}
	want := []FactionHitRow{
		{"Guard Dahvel", "Citizens of Freeport", -25},
		{"Guard Dahvel", "The Freeport Militia", -50},
		{"a gnoll pup", "Citizens of Qeynos", 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hits %v, want %v", got, want)
	}
}
