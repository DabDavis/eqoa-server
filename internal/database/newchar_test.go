package database

import (
	"testing"
)

func newCharDB(t *testing.T) (*SQLiteDB, *SQLiteCharacterStore) {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/newchar.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (10, 4, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Mending', ''), (11, 1, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Spark', ''),
		        (12, 1, 0, 0, 16880, 2, 15, 1, 2, 0, 5, -1, 'Going Home', ''), (13, 9, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Sword Art', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (7, 10, 'test'), (7, 11, 'test'), (7, 12, 'test'), (0, 13, 'test'), (0, 12, 'test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db, NewSQLiteCharacterStore(db)
}

type bookRow struct{ spell, order, on, where, lockout, show int }

func book(t *testing.T, db *SQLiteDB, sid uint32) []bookRow {
	t.Helper()
	rows, err := db.Query(`SELECT spellid, addedorder, onHotBar, whereonBar, unk1, showhide FROM spells WHERE serverid = ? ORDER BY addedorder`, sid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []bookRow
	for rows.Next() {
		var r bookRow
		if err := rows.Scan(&r.spell, &r.order, &r.on, &r.where, &r.lockout, &r.show); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// With the rules on: the rules' level whatever the record asked, and the
// class's spells in level then name order, off the bar, as a scribed spell
// goes in; another class's are not given.
func TestCreateNewWithTheRules(t *testing.T) {
	db, s := newCharDB(t)
	s.SetNewCharacterRules(NewCharacterRules{Level: 50, AllSpells: true})
	c := &CharacterRecord{CharName: "Tester", AccountID: 1, TClass: 7, Level: 3}
	n, err := s.CreateNew(c)
	if err != nil || n != 3 {
		t.Fatalf("spells %d, %v", n, err)
	}
	if got, _ := s.GetByServerID(c.ServerID); got.Level != 50 {
		t.Errorf("level %d", got.Level)
	}
	want := []bookRow{{12, 0, 0, -1, 0, -1}, {11, 1, 0, -1, 0, -1}, {10, 2, 0, -1, 0, -1}}
	got := book(t, db, c.ServerID)
	if len(got) != len(want) {
		t.Fatalf("book %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("book[%d] %+v, want %+v", i, got[i], want[i])
		}
	}
	// granting again adds only what is missing, after what is there
	if _, err := db.Exec(`DELETE FROM spells WHERE serverid = ? AND spellid = 11`, c.ServerID); err != nil {
		t.Fatal(err)
	}
	if n, err := s.GrantClassSpells(c.ServerID, 7); err != nil || n != 1 {
		t.Fatalf("regrant %d, %v", n, err)
	}
	if got := book(t, db, c.ServerID); len(got) != 3 || got[2] != (bookRow{11, 3, 0, -1, 0, -1}) {
		t.Errorf("after the regrant %+v", got)
	}
}

// The control: with no rules, level 1 whatever the record asked (a client's
// creation message says a level), and no spells.
func TestCreateNewWithoutRules(t *testing.T) {
	db, s := newCharDB(t)
	c := &CharacterRecord{CharName: "Plain", AccountID: 1, TClass: 7, Level: 60}
	if n, err := s.CreateNew(c); err != nil || n != 0 {
		t.Fatalf("spells %d, %v", n, err)
	}
	if got, _ := s.GetByServerID(c.ServerID); got.Level != 1 {
		t.Errorf("level %d, want 1", got.Level)
	}
	if got := book(t, db, c.ServerID); len(got) != 0 {
		t.Errorf("book %+v", got)
	}
	s.SetNewCharacterRules(NewCharacterRules{Level: 99})
	c2 := &CharacterRecord{CharName: "Capped", AccountID: 1, TClass: 0}
	if _, err := s.CreateNew(c2); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetByServerID(c2.ServerID); got.Level != 60 {
		t.Errorf("level %d, want at most 60", got.Level)
	}
}
