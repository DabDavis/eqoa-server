package database

import "testing"

// The rows kept for a larger client (itemlists.go) are read by nothing in the
// game and changed only by MoveItemListRows, which moves a row only from
// where it was found. The controls: the same calls on a held row.
func TestKeptRowsAreOutOfTheGame(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/kept.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &CharacterRecord{CharName: "Keeper", AccountID: 1}
	if err := NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (1, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 100, 0, 32767, 1023, 0, 0, 0, 0, 'Gem', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pat, _ := res.LastInsertId()
	inv := NewInventoryStore(db)
	sid := rec.ServerID
	held, _ := inv.AddCharItem(sid, int(pat), 5, 100, 0, LocPack)
	kept, _ := inv.AddCharItem(sid, int(pat), 7, 100, 0, LocPack)
	moved, err := inv.MoveItemListRows(sid, []ItemListMove{{ItemID: kept, From: LocPack, To: LocPackKept, ListNum: 0}, {ItemID: held, From: LocBank, To: LocBankKept}})
	if err != nil || !moved[0] || moved[1] {
		t.Fatalf("moves %v %v: the second was not where it said", moved, err)
	}
	items, err := inv.GetCharacterItems(sid)
	if err != nil || len(items) != 1 || items[0].ItemID != held {
		t.Fatalf("read %+v %v", items, err)
	}
	if _, err := inv.GetCharItem(sid, kept); err == nil {
		t.Error("GetCharItem read a kept row")
	}
	for name, err := range map[string]error{
		"stack": inv.SetItemStack(sid, kept, 1), "hp": inv.SetItemRemainHP(sid, kept, 1), "charges": inv.SetItemCharges(sid, kept, 1),
		"move": inv.MoveCharItem(sid, kept, LocBank), "delete": inv.DeleteCharItem(sid, kept),
	} {
		if err == nil {
			t.Errorf("%s of a kept row", name)
		}
	}
	rows, err := inv.ItemListRows(sid)
	if err != nil || len(rows) != 2 || rows[1].ItemID != kept || rows[1].Location != LocPackKept {
		t.Fatalf("all rows: %+v %v", rows, err)
	}
	var stack int
	if err := db.QueryRow(`SELECT stackleft FROM charinventory WHERE itemid = ?`, kept).Scan(&stack); err != nil || stack != 7 {
		t.Errorf("the kept row's stack %d %v", stack, err)
	}
	if p, b, err := inv.KeptItemRows(sid); err != nil || p != 1 || b != 0 {
		t.Errorf("kept %d %d %v", p, b, err)
	}
	if p, _, err := inv.KeptPatterns(sid); err != nil || !p[int(pat)] {
		t.Errorf("kept patterns %v %v", p, err)
	}
	// the controls, on the held row
	if err := inv.SetItemStack(sid, held, 6); err != nil {
		t.Errorf("a held row's stack: %v", err)
	}
	if _, err := inv.GetCharItem(sid, held); err != nil {
		t.Errorf("a held row: %v", err)
	}
	// and back
	if moved, err := inv.MoveItemListRows(sid, []ItemListMove{{ItemID: kept, From: LocPackKept, To: LocPack, ListNum: 9}}); err != nil || !moved[0] {
		t.Fatalf("back: %v %v", moved, err)
	}
	if items, _ := inv.GetCharacterItems(sid); len(items) != 2 {
		t.Errorf("%d rows after it came back", len(items))
	}
	if _, err := inv.MoveItemListRows(sid, []ItemListMove{{ItemID: kept, From: LocPack, To: 9}}); err == nil {
		t.Error("a move to no place")
	}
}
