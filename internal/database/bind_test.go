package database

import "testing"

// A bind point is kept by character, a second bind replaces the first, a
// character that never bound has none, and a deleted character's goes with
// it. The control: another character's stays.
func TestBindRoundTrip(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/bind.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	if _, ok, err := s.LoadBind(7); ok || err != nil {
		t.Fatalf("no row: bound %v (%v), want not bound", ok, err)
	}
	a := BindRow{World: 0, X: 23594.3, Y: 32.6, Z: 6297.9, Facing: 1.5, Spiritmaster: "Spiritmaster Savi"}
	b := BindRow{World: 2, X: 7000, Y: 54, Z: 11000, Spiritmaster: "Spiritmaster Walana"}
	if err := s.SaveBind(7, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBind(9, b); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := s.LoadBind(7); !ok || got != a {
		t.Fatalf("loaded %+v (%v), want %+v", got, ok, a)
	}
	if err := s.SaveBind(7, b); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.LoadBind(7); got != b {
		t.Errorf("after a second bind %+v, want %+v", got, b)
	}
	c := &CharacterRecord{CharName: "Bound", AccountID: 1}
	if err := s.Create(c); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBind(c.ServerID, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(c.ServerID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.LoadBind(c.ServerID); ok {
		t.Errorf("deleted character %d's bind point is still kept", c.ServerID)
	}
	if got, ok, _ := s.LoadBind(9); !ok || got != b {
		t.Errorf("the control: character 9's is %+v (%v)", got, ok)
	}
}
