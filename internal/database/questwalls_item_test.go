package database

import (
	"path/filepath"
	"testing"
)

// A wall's inner boxes read as written, each corner pair in any order; a box
// of other than six numbers is an error, as is a row of one (QuestWalls).
func TestWallBoxesRead(t *testing.T) {
	bs, err := ParseWallBoxes(`[[4627.3, -16, 3624, 4598, -7, 3596], [4510, -7, 3595, 4680, 10, 3840]]`)
	if err != nil || len(bs) != 2 {
		t.Fatalf("%v %v", bs, err)
	}
	if b := bs[0]; b.X0 != 4598 || b.X1 != 4627.3 || b.Z0 != 3596 || b.Z1 != 3624 || b.Y0 != -16 || b.Y1 != -7 {
		t.Errorf("box 1 %+v", b)
	}
	if !bs[0].Holds(4614, -12, 3604) || bs[0].Holds(4630, -12, 3604) || bs[0].Holds(4614, -1, 3604) {
		t.Error("box 1 holds the wrong places")
	}
	if bs, err := ParseWallBoxes(""); err != nil || bs != nil {
		t.Errorf("none: %v %v", bs, err)
	}
	if _, err := ParseWallBoxes(`[[1, 2, 3]]`); err == nil {
		t.Error("a box of three numbers read")
	}
}

// A table made before the item's columns gets them at the start, and its rows
// read with none; a row of an item reads whole.
func TestQuestWallsGetTheItemsColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE questwalls`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE questwalls (npcid INTEGER PRIMARY KEY, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO questwalls (npcid, questid, step) VALUES (7, 101312, 2)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO questwalls (npcid, questid, item, inner, text) VALUES (8, 0, 1012, '[[0,0,0,1,1,1]]', 'no suit')`); err != nil {
		t.Fatal(err)
	}
	ws, err := db.QuestWalls()
	if err != nil || len(ws) != 2 {
		t.Fatalf("%+v %v", ws, err)
	}
	if ws[0].Item != 0 || ws[0].Inner != nil || ws[0].QuestID != 101312 {
		t.Errorf("the old row %+v", ws[0])
	}
	if ws[1].Item != 1012 || len(ws[1].Inner) != 1 || ws[1].Text != "no suit" {
		t.Errorf("the item's row %+v", ws[1])
	}
	if _, err := db.Exec(`UPDATE questwalls SET inner = 'nonsense' WHERE npcid = 8`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.QuestWalls(); err == nil {
		t.Error("a row whose inner does not read was read")
	}
}
