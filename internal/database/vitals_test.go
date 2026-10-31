package database

import (
	"testing"
	"time"
)

// The hit points and power a character left with are taken once at its next
// login (a second login, after a session that saved nothing, is whole), a
// second save replaces the first, a clear forgets them, and a deleted
// character's go with it. The control: another character's stay.
func TestVitalsAreTakenOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/vitals.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	if _, _, _, ok, err := s.TakeVitals(7); ok || err != nil {
		t.Fatalf("no row: kept %v (%v), want nothing", ok, err)
	}
	before := time.Now().Add(-2 * time.Second)
	if err := s.SaveVitals(7, 40, 12); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveVitals(7, 55, 30); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveVitals(9, 1, 2); err != nil {
		t.Fatal(err)
	}
	if hp, power, saved, ok, err := s.TakeVitals(7); !ok || err != nil || hp != 55 || power != 30 ||
		saved.Before(before) || saved.After(time.Now().Add(2*time.Second)) {
		t.Fatalf("taken %d HP %d power saved %v (%v, %v), want 55 30 now", hp, power, saved, ok, err)
	}
	if _, _, _, ok, _ := s.TakeVitals(7); ok {
		t.Error("taken twice")
	}
	if err := s.ClearVitals(9); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok, _ := s.TakeVitals(9); ok {
		t.Error("cleared, and still there")
	}
	c := &CharacterRecord{CharName: "Wounded", AccountID: 1}
	if err := s.Create(c); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveVitals(c.ServerID, 3, 4); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveVitals(c.ServerID+1, 5, 6); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(c.ServerID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok, _ := s.TakeVitals(c.ServerID); ok {
		t.Error("a deleted character's hit points kept")
	}
	if _, _, _, ok, _ := s.TakeVitals(c.ServerID + 1); !ok {
		t.Error("the other character's went with it")
	}
}
