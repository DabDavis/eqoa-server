package database

import (
	"reflect"
	"testing"
)

// A purchase keeps the rank and the points together, and both are there
// after the database is closed and opened again (a restart); a removal takes
// the rank and keeps the points it gave back. The controls: another
// character's ranks are its own, and a removal of one rank leaves the rest.
func TestCMOwnershipSurvivesAReopen(t *testing.T) {
	path := t.TempDir() + "/cm.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSQLiteCharacterStore(db)
	if got, err := s.LoadOwnedCMs(7); err != nil || len(got) != 0 {
		t.Fatalf("no purchase: %v (%v), want none", got, err)
	}
	if err := s.BuyCM(7, OwnedCM{ID: 1, Cost: 10}, MasteryRow{Points: 90, Spent: 10, Percent: 50}); err != nil {
		t.Fatal(err)
	}
	if err := s.BuyCM(7, OwnedCM{ID: 2, Cost: 15}, MasteryRow{Points: 75, Spent: 25, Percent: 50}); err != nil {
		t.Fatal(err)
	}
	if err := s.BuyCM(9, OwnedCM{ID: 3001, Cost: 20}, MasteryRow{Points: 5, Spent: 20}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = NewSQLiteCharacterStore(db)
	got, err := s.LoadOwnedCMs(7)
	if err != nil || !reflect.DeepEqual(got, []OwnedCM{{1, 10}, {2, 15}}) {
		t.Fatalf("after a reopen: %v (%v), want ranks 1 and 2", got, err)
	}
	if m, _ := s.LoadMastery(7); m != (MasteryRow{Points: 75, Spent: 25, Percent: 50}) {
		t.Errorf("points after a reopen: %+v", m)
	}
	if err := s.RemoveCMs(7, []int{2}, MasteryRow{Points: 90, Spent: 10, Percent: 50}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadOwnedCMs(7); !reflect.DeepEqual(got, []OwnedCM{{1, 10}}) {
		t.Errorf("after removing rank 2: %v, want rank 1 alone", got)
	}
	if m, _ := s.LoadMastery(7); m.Points != 90 || m.Spent != 10 {
		t.Errorf("points after the removal: %+v", m)
	}
	if got, _ := s.LoadOwnedCMs(9); !reflect.DeepEqual(got, []OwnedCM{{3001, 20}}) {
		t.Errorf("the control: character 9's ranks are %v", got)
	}
}

// The catalog reads back as written, and a rank's spell lists read as
// numbers. The control: a row written disabled reads disabled.
func TestCMCatalogReadsBack(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/cm.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO cmcatalog (id, category, classes, races, mastery, rank, name, parent, cost, level, spent,
		minstats, requires, effects, adds, add_spells, removes, remove_spells, description, enabled, source)
		VALUES (2, 0, 32767, 2047, 'Brawn', 2, 'Brawn 2', 1, 15, 20, 0, 'STR:125', '', 'STR:2,STRMax:20', '', '', '', '', 'Strength.', 1, 'test'),
		       (3005, 3, 1, 2047, 'Battle Cry', 1, 'Battle Cry 1', 0, 20, 20, 0, '', 'Taunt@1', 'AC:25', 'Battle Cry@20', '501', 'Taunt@1', '500', '', 0, 'test')`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.CMCatalog()
	if err != nil || len(rows) != 2 {
		t.Fatalf("%d rows (%v)", len(rows), err)
	}
	b := rows[0]
	if b.ID != 2 || b.Parent != 1 || b.Cost != 15 || b.Level != 20 || b.MinStats != "STR:125" || b.Effects != "STR:2,STRMax:20" || !b.Enabled || b.Races != 2047 {
		t.Errorf("Brawn 2 read as %+v", b)
	}
	if c := rows[1]; c.Enabled || c.Category != 3 || !reflect.DeepEqual(SpellIDs(c.AddSpells), []int{501}) || !reflect.DeepEqual(SpellIDs(c.RemoveSpells), []int{500}) {
		t.Errorf("Battle Cry 1 read as %+v", c)
	}
	if got := SpellIDs("12, x, 0, 34"); !reflect.DeepEqual(got, []int{12, 34}) {
		t.Errorf("SpellIDs: %v", got)
	}
}
