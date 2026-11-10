package database

import "testing"

// npctint is made with the schema, empty, and read keyed as npcclass keys a
// name, whatever case, trailing space or '$' a GM typed; a colour stored
// signed is the same 32 bits; a row that tints nothing (alpha 0, white) or
// names no one is left out, and so is a colour wider than 32 bits. The
// control: the rows that tint are read as written, in the keys' order.
func TestNPCTints(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/npctint.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, err := db.NPCTints()
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO npctint (name, color, source) VALUES
		('Yhafiran ', 1435259135, 'seed'),
		('Lady Vox$', -16776961, 'gm'),
		('moss snake', 2155905024, 'gm'),
		('a white one', 4294967295, 'gm'),
		('   ', 1435259135, 'gm'),
		('too wide', 8589934591, 'gm')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO npctint (name) VALUES ('Untinted')`); err != nil {
		t.Fatal(err)
	}
	got, err = db.NPCTints()
	want := []NPCTint{{"lady vox", 0xFF0000FF}, {"yhafiran", 0x558C50FF}}
	if err != nil || len(got) != len(want) {
		t.Fatalf("tints %v, %v; want %v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tint %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	for c, changes := range map[uint32]bool{0x558C50FF: true, 0x558C5001: true, 0x558C5000: false, 0xFFFFFFFF: false, 0xFFFFFF80: false, 0x000000FF: true} {
		if NPCTintChanges(c) != changes {
			t.Errorf("NPCTintChanges(%08X) = %v", c, !changes)
		}
	}
	for c, ok := range map[int64]bool{0: true, 0xFFFFFFFF: true, -1: true, -(1 << 31): true, 1 << 32: false, -(1 << 31) - 1: false} {
		if ValidNPCTint(c) != ok {
			t.Errorf("ValidNPCTint(%d) = %v", c, !ok)
		}
	}
}
