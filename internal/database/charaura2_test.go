package database

import "testing"

// charaura's hands' second auras (color2 ... nodes2; 2026-11-16, Dust's
// sword: "it will keep the red aura it has and stack the twinkle"): a table
// made before them is given the columns; a hand row's second is read beside
// its first, a second on a slot not a hand's (or of a kind not a hand's, or
// past 100 along) dropped; SetCharAuraSecondAudited keeps the row's first,
// makes a row with its first none, refuses a piece's slot and a shell, and
// deletes a row left with neither; SetCharAuraAudited's off keeps a row that
// still has a second.
func TestCharAuraSecond(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/charaura2.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// a table as it was before the second auras
	if _, err := db.Exec(`DROP TABLE charaura`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
		color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
		INSERT INTO charaura (charid, merc, slot, color, kind) VALUES (197, 0, 9, 255, 3)`); err != nil {
		t.Fatal(err)
	}
	if err := db.ensureCharAura(); err != nil {
		t.Fatal(err)
	}
	for _, col := range charAuraSecondCols {
		if has, err := hasColumn(db.DB, "charaura", col); err != nil || !has {
			t.Fatalf("column %s not added: %v", col, err)
		}
	}
	first := ItemAura{Color: 0x000000FF, Kind: AuraParticles}
	twinkle := ItemAura{Kind: AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	k := CharAuraKey{CharID: 197}
	if s, err := db.CharSecondAuras(); err != nil || len(s) != 0 {
		t.Fatalf("no seconds yet: %+v %v", s, err)
	}
	if err := db.SetCharAuraSecondAudited(k, CharAuraMainHand, twinkle, GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.CharAuras()
	s, _ := db.CharSecondAuras()
	if got[k][CharAuraMainHand] != first || s[k][0] != twinkle || s[k][1] != (ItemAura{}) {
		t.Fatalf("Dust's: first %+v second %+v", got[k], s[k])
	}
	// refused: a piece's slot, a shell, past 100 along
	for _, bad := range []struct {
		slot int
		a    ItemAura
	}{{CharAuraBody, twinkle}, {CharAuraHelm, twinkle}, {CharAuraOffHand, ItemAura{Color: 0xFF2020FF, Kind: AuraShell}},
		{CharAuraOffHand, ItemAura{Kind: AuraParticles, Nodes: 101}}} {
		if err := db.SetCharAuraSecondAudited(k, bad.slot, bad.a, GMEdit{}, "test"); err == nil {
			t.Errorf("slot %d %+v kept", bad.slot, bad.a)
		}
	}
	// an off hand with no row: made, its first none
	k2 := CharAuraKey{CharID: 300, Merc: 1}
	epic := ItemAura{Kind: AuraEpic, Source: 0x88EC092D}
	if err := db.SetCharAuraSecondAudited(k2, CharAuraOffHand, epic, GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.CharAuras()
	s, _ = db.CharSecondAuras()
	if _, ok := got[k2]; ok || s[k2][1] != epic {
		t.Fatalf("a second alone: first %+v second %+v", got[k2], s[k2])
	}
	// the first cleared: the row kept for its second
	if err := db.SetCharAuraAudited(k, CharAuraMainHand, ItemAura{}, GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.CharAuras()
	s, _ = db.CharSecondAuras()
	if _, ok := got[k]; ok || s[k][0] != twinkle {
		t.Fatalf("first off: first %+v second %+v", got[k], s[k])
	}
	// and the second cleared: neither left, the row goes
	if err := db.SetCharAuraSecondAudited(k, CharAuraMainHand, ItemAura{}, GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charaura WHERE charid = 197`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows left: %d %v", n, err)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'charaura'`).Scan(&audits); err != nil || audits != 4 {
		t.Fatalf("audit lines: %d %v", audits, err)
	}
	// read: a second on a piece's slot, of a shell, past 100: left out
	if _, err := db.Exec(`INSERT INTO charaura (charid, merc, slot, color, kind, color2, kind2, source2, nodes2) VALUES
		(400, 0, 7, 4280295679, 2, 0, 3, 495410893, 100), (400, 0, 9, 0, 1, 4280295679, 2, 0, 0), (400, 0, 10, 0, 1, 0, 3, 0, 101)`); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.CharSecondAuras(); s[CharAuraKey{CharID: 400}] != ([2]ItemAura{}) {
		t.Fatalf("bad seconds read: %+v", s[CharAuraKey{CharID: 400}])
	}
}
