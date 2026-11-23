package database

import "testing"

// /quickchat reset (work board 1960): ResetHotkeys deletes the character's
// sets alone and says how many; a second reset finds none, and Hotkeys then
// returns none (so the handler sends the defaults).
func TestResetHotkeysDeletesOnlyTheCharactersSets(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/hotkeys.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	defaults := []HotkeySet{{ID: 0}, {ID: 4}, {ID: 8}}
	for _, id := range []uint32{7, 8} {
		if err := db.SetHotkeyChoice(id, defaults, 4, 1, HotkeyPair{Text: "|16", Title: "Shout!"}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := db.ResetHotkeys(7); err != nil || n != 3 {
		t.Fatalf("the reset: %d rows, %v; want 3", n, err)
	}
	if rows, err := db.Hotkeys(7); err != nil || len(rows) != 0 {
		t.Fatalf("after the reset: %d sets, %v", len(rows), err)
	}
	if n, err := db.ResetHotkeys(7); err != nil || n != 0 {
		t.Fatalf("a second reset: %d rows, %v; want 0", n, err)
	}
	if rows, _ := db.Hotkeys(8); len(rows) != 3 {
		t.Fatalf("the other character's sets: %d, want 3", len(rows))
	}
}
