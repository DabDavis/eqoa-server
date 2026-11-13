package handlers

import "testing"

// A marks pass reads the player's pack once (questpass.go): inside a pass a
// row added to the database is not seen (the pass's one read stands), and
// once the pass ends the next read sees it. Before, every ask of the ~1,400
// quests read the pack again (64% of the server's CPU, 2026-11-13).
func TestAMarksPassReadsThePackOnce(t *testing.T) {
	f := newArchivistFixture(t, nil)
	qe, err := NewQuestEngine(f.db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (1, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Pass Pebble', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pebble, _ := res.LastInsertId()
	add := func() {
		if _, err := f.db.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, 1, 0, 0, ?, -1, 0, 0)`, f.p.CharServerID, pebble); err != nil {
			t.Fatal(err)
		}
	}
	add()
	qe.mu.Lock()
	defer qe.mu.Unlock()
	end := qe.beginPassLocked(f.p)
	if n := len(qe.heldItems(f.p)); n != 1 {
		t.Fatalf("the pass's first read: %d items, want 1", n)
	}
	add()
	if n := len(qe.heldItems(f.p)); n != 1 {
		t.Fatalf("a pass read its pack again: %d items, want the first read's 1", n)
	}
	if n := len(qe.shownItems(f.p)); n != 1 {
		t.Fatalf("a pass's shown items: %d, want the first read's 1", n)
	}
	end()
	if n := len(qe.heldItems(f.p)); n != 2 {
		t.Fatalf("after the pass: %d items, want 2", n)
	}
}
