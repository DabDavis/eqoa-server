package database

import "testing"

// Work board 658: npccoins is made at the start and read by the name's key;
// no row is the ordinary rule (ok false), a bad row's bounds put right.
func TestNPCCoins(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/coins.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv := NewInventoryStore(db)
	if _, _, ok, err := inv.NPCCoins("Snowgrin"); ok || err != nil {
		t.Fatalf("no row: ok %v err %v", ok, err)
	}
	if _, err := db.Exec(`INSERT INTO npccoins (npc_name, coins_min, coins_max, source) VALUES
		('snowgrin', 2202, 2202, 'video-audit'), ('thailav d` + "`" + `yal', 23, 23, 'video-audit'), ('odd', 9, 3, 'gm')`); err != nil {
		t.Fatal(err)
	}
	if lo, hi, ok, err := inv.NPCCoins("Snowgrin$ "); !ok || err != nil || lo != 2202 || hi != 2202 {
		t.Fatalf("Snowgrin: %d-%d ok %v err %v", lo, hi, ok, err)
	}
	if lo, _, ok, _ := inv.NPCCoins("Thailav D`yal"); !ok || lo != 23 {
		t.Fatalf("Thailav D`yal: %d ok %v", lo, ok)
	}
	if lo, hi, _, _ := inv.NPCCoins("odd"); lo != 9 || hi != 9 {
		t.Fatalf("a row with min above max read %d-%d", lo, hi)
	}
}
