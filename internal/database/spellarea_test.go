package database

import "testing"

// spellarea is made with the schema, empty; a spell's row is read whole, and a
// spell with none has none.
func TestSpellAreaRows(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/spellarea.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewSpellbookStore(db)
	if _, ok, err := s.SpellArea(700145); ok || err != nil {
		t.Fatalf("an empty table: ok %v, %v", ok, err)
	}
	if _, err := db.Exec(`INSERT INTO spellarea (spellid, centre, radius, maxtargets, note) VALUES
		(700145, 0, 20.5, 4, 'wider'), (701065, 2, 0, 0, 'Rampage round its caster'), (1624, -1, 0, 0, '')`); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]SpellAreaRow{700145: {0, 20.5, 4}, 701065: {2, 0, 0}, 1624: {-1, 0, 0}} {
		if got, ok, err := s.SpellArea(id); !ok || err != nil || got != want {
			t.Errorf("spell %d: %+v %v %v, want %+v", id, got, ok, err, want)
		}
	}
	if _, ok, _ := s.SpellArea(9999); ok {
		t.Error("a spell with no row has one")
	}
}
