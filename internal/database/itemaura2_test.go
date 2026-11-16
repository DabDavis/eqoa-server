package database

import "testing"

// A held item's second aura (itemaura's color2 ... nodes2, 2026-11-16): an old
// table gains the columns, a row's second is read apart from its first (a
// hand's kinds alone), /aura item <id> second keeps it audited, clearing the
// first keeps the row for the second, and clearing both deletes it.
func TestItemSecondAuras(t *testing.T) {
	path := t.TempDir() + "/itemaura2.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	// a table made before the second auras: rebuilt as it was, then read again
	if _, err := db.Exec(`DROP TABLE itemaura; CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0,
		kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
		INSERT INTO itemaura (patternid, color, kind, source, nodes) VALUES (20, 812711935, 3, 1813322336, 0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, c := range itemAuraSecondCols {
		if has, err := hasColumn(db.DB, "itemaura", c); !has || err != nil {
			t.Fatalf("column %s: %v %v", c, has, err)
		}
	}
	if got, err := db.ItemAuras(); err != nil || len(got) != 1 || got[20].Kind != AuraParticles {
		t.Fatalf("the old row's first: %+v %v", got, err)
	}
	if got, err := db.ItemSecondAuras(); err != nil || len(got) != 0 {
		t.Fatalf("an old row has no second: %+v %v", got, err)
	}
	// seconds: a hand's kinds alone; a shell or an along past 100 left out
	if _, err := db.Exec(`UPDATE itemaura SET color2 = 812711935, kind2 = 7, source2 = 2297170221 WHERE patternid = 20;
		INSERT INTO itemaura (patternid, kind, kind2, color2) VALUES (21, 1, 2, 4278190335), (22, 1, 3, 0);
		INSERT INTO itemaura (patternid, kind, kind2, nodes2) VALUES (23, 1, 3, 101)`); err != nil {
		t.Fatal(err)
	}
	got, err := db.ItemSecondAuras()
	if err != nil || len(got) != 2 || got[20] != (ItemAura{PatternID: 20, Color: 0x3070FFFF, Kind: AuraEpic, Source: 0x88EC092D}) ||
		got[22] != (ItemAura{PatternID: 22, Kind: AuraParticles}) {
		t.Fatalf("seconds %+v %v", got, err)
	}
	if !ValidItemAuraSecond(AuraEnergy, 100) || ValidItemAuraSecond(AuraShell, 0) || ValidItemAuraSecond(AuraEpic, 101) || ValidItemAuraSecond(AuraDefault, 5) || !ValidItemAuraSecond(AuraDefault, 0) {
		t.Fatal("ValidItemAuraSecond")
	}

	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 14, 0, 1, 1, 0, 1, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Icingdeath', '', 3493413598, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid64, _ := res.LastInsertId()
	pid := int(pid64)
	who := GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	count := func(action string) int {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'itemaura' AND action = ?`, action).Scan(&n)
		return n
	}
	if _, err := db.SetItemAuraSecondAudited(ItemAura{PatternID: pid, Color: 0xFF0000FF, Kind: AuraShell}, who, "t"); err == nil {
		t.Fatal("a shell as a second aura kept")
	}
	// a second with no first: the row made, its first none
	freeze := ItemAura{PatternID: pid, Color: 0x3060FFFF, Kind: AuraParticles, Source: 0x6C151A60}
	if found, err := db.SetItemAuraSecondAudited(freeze, who, "t"); !found || err != nil {
		t.Fatalf("set: %v %v", found, err)
	}
	if s, _ := db.ItemSecondAuras(); s[pid] != freeze {
		t.Fatalf("the second: %+v", s[pid])
	}
	if f, _ := db.ItemAuras(); func() bool { _, ok := f[pid]; return ok }() {
		t.Fatal("a first aura shown on a row made for its second")
	}
	// the first set beside it, then cleared: the row kept for the second
	sov := ItemAura{PatternID: pid, Color: 0x3060FFFF, Kind: AuraParticles, Source: 0x88EC092D}
	if _, err := db.SetItemAuraAudited(sov, who, "t"); err != nil {
		t.Fatal(err)
	}
	if f, _ := db.ItemAuras(); f[pid] != sov {
		t.Fatalf("the first: %+v", f[pid])
	}
	if _, err := db.SetItemAuraAudited(ItemAura{PatternID: pid}, who, "t"); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.ItemSecondAuras(); s[pid] != freeze {
		t.Fatalf("clearing the first took the second: %+v", s[pid])
	}
	// the second cleared too: the row goes
	if _, err := db.SetItemAuraSecondAudited(ItemAura{PatternID: pid}, who, "t"); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM itemaura WHERE patternid = ?`, pid).Scan(&n)
	if n != 0 || count("insert") != 1 || count("update") != 2 || count("delete") != 1 {
		t.Fatalf("rows %d; audited %d/%d/%d", n, count("insert"), count("update"), count("delete"))
	}
}
