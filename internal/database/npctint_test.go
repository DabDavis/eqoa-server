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
	want := []NPCTint{{Name: "lady vox", Color: 0xFF0000FF}, {Name: "yhafiran", Color: 0x558C50FF}}
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

// The glow (petition 1893): read beside the colour, a row of a glow alone kept (it draws something), a glow of alpha
// 0 or black none; a table made before the glow takes its column, its rows unlit.
func TestNPCTintGlows(t *testing.T) {
	path := t.TempDir() + "/glow.db"
	old, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`DROP TABLE npctint`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE npctint (name TEXT PRIMARY KEY, color INTEGER NOT NULL DEFAULT 4294967295,
		note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO npctint (name, color) VALUES ('Yhafiran', 1435259135)`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO npctint (name, color, glow) VALUES ('Mr. Moogie', 4294967295, 4284506265),
		('dim', 4294967295, 4284506112), ('dark', 4294967295, 255)`); err != nil {
		t.Fatal(err)
	}
	got, err := db.NPCTints()
	want := []NPCTint{{Name: "mr. moogie", Color: 0xFFFFFFFF, Glow: 0xFF606099}, {Name: "yhafiran", Color: 0x558C50FF}}
	if err != nil || len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("tints %+v, %v; want %+v", got, err, want)
	}
}

// A hat on an NPC by name (2026-11-17: Santa Claus's Santa Hat): an npctint
// row of aura kind 10 is read (layout 3 sends it: Shows), with no tint or
// glow, its source and its style (HatSanta in nodes) as written; a layout 2
// client is told nothing of it (ShowsGlow). The control: a kind past 10 is
// dropped on read (the row, tinting nothing, left out).
func TestNPCHat(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/npchat.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npctint (name, color, glow, aura_kind, aura_source, aura_nodes) VALUES
		('Santa Claus', 0, 0, 10, ?, 1), ('Toymaker Tinsel', 0, 0, 11, ?, 1)`, int64(GnomeCap), int64(GnomeCap)); err != nil {
		t.Fatal(err)
	}
	got, err := db.NPCTints()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != NPCClassKey("Santa Claus") || got[0].AuraKind != AuraHat || got[0].AuraSource != GnomeCap || got[0].AuraNodes != HatSanta {
		t.Fatalf("read: %+v", got)
	}
	if !got[0].Shows() || got[0].ShowsGlow() {
		t.Fatal("a hat shows at layout 3 alone")
	}
	if !ValidNPCAuraKind(AuraHat) || ValidNPCAuraKind(11) || ValidAuraKind(AuraHat) {
		t.Fatal("kinds")
	}
}
