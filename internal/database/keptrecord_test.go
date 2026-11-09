package database

import "testing"

// EnsureItemRecord (keptrecord.go) on a fresh database: a record made once at
// the first free id from its start, found by name after that and never
// rewritten. The records are made up.
func TestEnsureItemRecordMakesItOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/kept.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := ItemPatternRecord{ItemName: "Pebble of Elsewhere", ItemDesc: "A made-up stone.", ItemIcon: 414194432,
		EquipSlot: -1, Trade: 1, Rent: 1, LevelReq: 1, MaxStack: 1, Duration: 80, ClassUse: 32767, RaceUse: 32767,
		ProcAnim: 7, Lore: 1, Color: 0xFFFFFFFF}
	// 900100 is taken by another record: the next free id is the stone's
	if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent,
		unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6,
		craft, itemname, itemdesc, model, color) VALUES (900100, 5, 0, 1, 0, 3, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0,
		0, 0, 'Someone Else''s Hat', '', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	id, made, err := EnsureItemRecord(db.DB, rec, 900100)
	if err != nil || !made || id != 900101 {
		t.Fatalf("made %v at %d (%v), want 900101 (900100 taken)", made, id, err)
	}
	var name string
	var icon, color int64
	var slot, trade, lore, stack, procanim, str, ac int
	if err := db.QueryRow(`SELECT itemname, itemicon, color, equipslot, trade, lore, maxstack, procanim, str, AC
		FROM itempattern WHERE patternid = ?`, id).Scan(&name, &icon, &color, &slot, &trade, &lore, &stack, &procanim, &str, &ac); err != nil {
		t.Fatalf("the record: %v (a stat left NULL?)", err)
	}
	if name != rec.ItemName || icon != 414194432 || color != 0xFFFFFFFF || slot != -1 || trade != 1 || lore != 1 || stack != 1 ||
		procanim != 7 || str != 0 || ac != 0 {
		t.Errorf("the record %q icon %d colour %#x slot %d trade %d lore %d stack %d type %d str %d AC %d", name, icon, color, slot,
			trade, lore, stack, procanim, str, ac)
	}
	// a GM's edit stands: found again by its name, unchanged
	if _, err := db.Exec(`UPDATE itempattern SET itemicon = 42, itemdesc = 'Edited.' WHERE patternid = ?`, id); err != nil {
		t.Fatal(err)
	}
	again, made, err := EnsureItemRecord(db.DB, rec, 900100)
	if err != nil || made || again != id {
		t.Fatalf("again: made %v at %d (%v), want the record %d found", made, again, err, id)
	}
	if err := db.QueryRow(`SELECT itemicon FROM itempattern WHERE patternid = ?`, id).Scan(&icon); err != nil || icon != 42 {
		t.Errorf("the GM's icon became %d (%v)", icon, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE itemname = ?`, rec.ItemName).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d records of the name, want one", n)
	}

	// the control: a record of the name worn somewhere is another item; one
	// worn nowhere is made beside it
	hat := rec
	hat.ItemName, hat.EquipSlot = "Someone Else's Hat", -1
	hid, made, err := EnsureItemRecord(db.DB, hat, 900100)
	if err != nil || !made || hid != 900102 {
		t.Errorf("the hat worn nowhere: made %v at %d (%v), want 900102 beside the worn one", made, hid, err)
	}
}
