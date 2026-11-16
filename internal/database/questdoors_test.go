package database

import (
	"path/filepath"
	"testing"
)

// A door's row reads whole (questdoors, made at the start); a row with no box
// on a side, or an outside place not of three numbers, is an error.
func TestQuestDoorsRead(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "doors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if ds, err := db.QuestDoors(); err != nil || len(ds) != 0 {
		t.Fatalf("a new database's doors: %+v %v", ds, err)
	}
	if _, err := db.Exec(`INSERT INTO questdoors (npcid, inner, outer, outside, text) VALUES
		(118859, '[[25707.5, 44, 3227, 25718, 58, 3235]]', '[[25690, 51, 3215, 25706.2, 60, 3250]]', '[25700, 54.3, 3231]', 'The Guardian bars your way.')`); err != nil {
		t.Fatal(err)
	}
	ds, err := db.QuestDoors()
	if err != nil || len(ds) != 1 {
		t.Fatalf("%+v %v", ds, err)
	}
	d := ds[0]
	if d.NPCID != 118859 || d.QuestID != 0 || d.Item != 0 || len(d.Inner) != 1 || len(d.Outer) != 1 ||
		d.Outside != [3]float32{25700, 54.3, 3231} || d.Text != "The Guardian bars your way." {
		t.Errorf("read %+v", d)
	}
	if !d.Inner[0].Holds(25712, 54.4, 3232) || d.Outer[0].Holds(25712, 54.4, 3232) || !d.Outer[0].Holds(25705.8, 55, 3231.9) {
		t.Error("the boxes hold the wrong places")
	}
	for _, bad := range []string{`UPDATE questdoors SET outer = ''`, `UPDATE questdoors SET outside = '[1, 2]'`} {
		if _, err := db.Exec(`UPDATE questdoors SET outer = '[[25690, 51, 3215, 25706.2, 60, 3250]]', outside = '[25700, 54.3, 3231]'`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(bad); err != nil {
			t.Fatal(err)
		}
		if _, err := db.QuestDoors(); err == nil {
			t.Errorf("%s: read", bad)
		}
	}
}
