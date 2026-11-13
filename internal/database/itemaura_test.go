package database

import "testing"

// npctint's aura columns (2026-11-13): read beside the glow; a row of
// particles alone kept (Shows) though it shows nothing at layout 2
// (ShowsGlow); a kind or node count past what the client reads drops the
// aura, the row kept by its glow; a table made before the auras (the glow's,
// 2026-11-13 early) takes the three columns, its rows of the default aura.
func TestNPCTintAuras(t *testing.T) {
	path := t.TempDir() + "/aura.db"
	old, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`DROP TABLE npctint`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE npctint (name TEXT PRIMARY KEY, color INTEGER NOT NULL DEFAULT 4294967295,
		glow INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO npctint (name, glow) VALUES ('Mr. Moogie', 4284506265)`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO npctint (name, glow, aura_kind, aura_source, aura_nodes) VALUES
		('a skeleton', 1090470143, 4, 2297170221, 12),
		('a wisp', 0, 3, 0, 0),
		('a shell', 0, 2, 0, 0),
		('too many', 1090470143, 2, 0, 33),
		('bad kind', 1090470143, 9, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	got, err := db.NPCTints()
	want := []NPCTint{
		{Name: "a skeleton", Color: 0xFFFFFFFF, Glow: 0x40FF40FF, AuraKind: AuraBoth, AuraSource: 0x88EC092D, AuraNodes: 12},
		{Name: "a wisp", Color: 0xFFFFFFFF, AuraKind: AuraParticles},
		{Name: "bad kind", Color: 0xFFFFFFFF, Glow: 0x40FF40FF},
		{Name: "mr. moogie", Color: 0xFFFFFFFF, Glow: 0xFF606099},
		{Name: "too many", Color: 0xFFFFFFFF, Glow: 0x40FF40FF},
	}
	if err != nil || len(got) != len(want) {
		t.Fatalf("tints %+v, %v; want %+v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tint %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if !got[1].Shows() || got[1].ShowsGlow() {
		t.Fatal("particles alone: shown at layout 3 alone")
	}
	for _, c := range []struct {
		glow uint32
		kind uint8
		want bool
	}{{0, AuraDefault, false}, {0xFF0000FF, AuraDefault, true}, {0xFF0000FF, AuraNone, false}, {0, AuraShell, false},
		{0xFF000000, AuraShell, false}, {0, AuraParticles, true}, {0, AuraBoth, true}, {0xFF0000FF, 9, true}, {0, 9, false}} {
		if AuraShows(c.glow, c.kind) != c.want {
			t.Errorf("AuraShows(%08X, %d) = %v", c.glow, c.kind, !c.want)
		}
	}
}

// itemaura: made with the schema, empty; rows read by patternid, one that
// draws nothing (no colour and the default kind, kind none) or out of range
// left out. SetItemAuraAudited makes, changes and (an aura of nothing)
// deletes a row, each audited, and changes nothing for an item that is not.
// SetNPCTintAuraAudited keeps a row's tint and finds it by its key.
func TestItemAuras(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/itemaura.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if got, err := db.ItemAuras(); err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO itemaura (patternid, color, kind, source, nodes) VALUES
		(10, 812711935, 0, 0, 0), (11, 0, 3, 2297170221, 2), (12, 0, 0, 0, 0), (13, 4278190335, 1, 0, 0),
		(14, 4278190335, 2, 0, 40), (15, 8589934591, 2, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	got, err := db.ItemAuras()
	want := map[int]ItemAura{10: {PatternID: 10, Color: 0x3070FFFF}, 11: {PatternID: 11, Kind: AuraParticles, Source: 0x88EC092D, Nodes: 2}}
	if err != nil || len(got) != len(want) || got[10] != want[10] || got[11] != want[11] {
		t.Fatalf("auras %+v, %v; want %+v", got, err, want)
	}

	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 7, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Glowing Bracer', '', 4, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	who := GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	count := func(action string) int {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'itemaura' AND action = ?`, action).Scan(&n)
		return n
	}
	if found, err := db.SetItemAuraAudited(ItemAura{PatternID: 987654, Color: 0xFF0000FF}, who, "t"); found || err != nil {
		t.Fatalf("no such item: %v %v", found, err)
	}
	a := ItemAura{PatternID: int(pid), Color: 0xFF2020FF, Kind: AuraBoth, Source: 0x88EC092D, Nodes: 3}
	if found, err := db.SetItemAuraAudited(a, who, "t"); !found || err != nil {
		t.Fatalf("set: %v %v", found, err)
	}
	a.Color = 0x2020FFFF
	if _, err := db.SetItemAuraAudited(a, who, "t"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ItemAuras(); got[int(pid)] != a {
		t.Fatalf("the row: %+v, want %+v", got[int(pid)], a)
	}
	if _, err := db.SetItemAuraAudited(ItemAura{PatternID: int(pid)}, who, "t"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ItemAuras(); len(got) != 2 || count("insert") != 1 || count("update") != 1 || count("delete") != 1 {
		t.Fatalf("after the delete: %d rows; audited %d/%d/%d", len(got), count("insert"), count("update"), count("delete"))
	}

	// npctint: a row of a tint, written as a GM typed its name, keeps its tint
	if _, err := db.Exec(`INSERT INTO npctint (name, color) VALUES ('Yhafiran $', 1435259135)`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNPCTintAuraAudited("YHAFIRAN", 0x40FF40FF, AuraShell, 0, 0, who, "t"); err != nil {
		t.Fatal(err)
	}
	tints, _ := db.NPCTints()
	if len(tints) != 1 || tints[0] != (NPCTint{Name: "yhafiran", Color: 0x558C50FF, Glow: 0x40FF40FF, AuraKind: AuraShell}) {
		t.Fatalf("tints %+v", tints)
	}
	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM npctint`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("%d npctint rows: a second made", rows)
	}
}
