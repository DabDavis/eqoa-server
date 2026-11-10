package database

import (
	"testing"
	"time"
)

// A character's last logout (charlogout, for its No Rent items: handlers/norent.go) is read once:
// the reading takes it away, a later logout writes over an earlier one, and one character's row is
// not another's. Deleting the character takes its row too.
func TestCharLogoutIsReadOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/logout.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, ok, err := db.TakeLogout(7); err != nil || ok {
		t.Fatalf("a character that never left: ok %v (%v)", ok, err)
	}
	at := time.Unix(1_800_000_000, 0)
	if err := db.SaveLogout(7, at); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLogout(7, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLogout(8, at); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.TakeLogout(7)
	if err != nil || !ok || !got.Equal(at.Add(time.Minute)) {
		t.Fatalf("read %v %v (%v), want the later logout", got, ok, err)
	}
	if _, ok, _ := db.TakeLogout(7); ok {
		t.Error("the row was read twice")
	}
	if _, ok, _ := db.TakeLogout(8); !ok {
		t.Error("another character's row went with the first's")
	}

	store := NewSQLiteCharacterStore(db)
	rec := &CharacterRecord{CharName: "Leaver", AccountID: 1, Level: 10}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLogout(rec.ServerID, at); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(rec.ServerID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.TakeLogout(rec.ServerID); ok {
		t.Error("a deleted character's logout was kept")
	}
}
