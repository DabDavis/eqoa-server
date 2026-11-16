package database

import "testing"

// itemowner is made with the schema; a row binds its record to its character,
// and a record with none is no one's.
func TestItemOwnerReadsItsRow(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/owner.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO itemowner (patternid, serverid, note) VALUES (715043, 327, 'Drizzt')`); err != nil {
		t.Fatal(err)
	}
	if id, ok, err := db.ItemOwner(715043); err != nil || !ok || id != 327 {
		t.Fatalf("715043: %d %v %v, want 327", id, ok, err)
	}
	if id, ok, err := db.ItemOwner(715044); err != nil || ok || id != 0 {
		t.Fatalf("715044: %d %v %v, want no one", id, ok, err)
	}
}
