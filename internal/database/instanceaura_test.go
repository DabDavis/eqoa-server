package database

import (
	"testing"
	"time"
)

// instanceaura: one copy's own aura and tag. Set, read and audited; a row
// with nothing left deleted; refused: a piece's kind, along past 100, a tag
// with a '|' or too long; carried to the new row of a whole-record move (the
// store's carry, a merc given the item and given it back, an auction's
// delivery), the cache moving with it; a part of a stack is never carried;
// HeldCopy finds the copy in each hand as wornLook does; the description is
// headed "Aura: <tag>".
func TestInstanceAura(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/inst.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); SetInstanceAuraCache(nil) })
	if _, err := db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Dust', 1, 197, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Other', 2, 198, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatalf("characters: %v", err)
	}
	inv := NewInventoryStore(db)
	pid := 714382
	if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (?, 0, 0, 1, 0, 14, 0, 0, 1, 0, 1, 10, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, 'Heirloom Sword of Twisted Virtue', '', 0, 0)`, pid); err != nil {
		t.Fatalf("itempattern: %v", err)
	}
	sword, err := inv.AddCharItem(197, pid, 1, 100, 0, LocPack)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.SetEquipLoc(197, sword, 14); err != nil {
		t.Fatal(err)
	}
	shadow := ItemAura{Color: 0x000000FF, Kind: AuraParticles}
	twinkle := ItemAura{Kind: AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	row := InstanceAura{ItemID: sword, Tag: "Shadowed, Twinkling", First: shadow, Second: twinkle, Note: "test"}
	if err := db.SetInstanceAuraAudited(row, GMEdit{Username: "dab", Via: "test"}, "test"); err != nil {
		t.Fatal(err)
	}
	got, ok := InstanceAuraHeld(sword)
	if !ok || got.Tag != row.Tag || got.First != shadow || got.Second != twinkle || got.Source != "gm" {
		t.Fatalf("cache: %+v %v", got, ok)
	}
	all, err := db.InstanceAuras()
	if err != nil || all[sword].Second != twinkle {
		t.Fatalf("read: %+v %v", all, err)
	}
	var audits int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'instanceaura'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audit lines: %d", audits)
	}
	for _, bad := range []InstanceAura{
		{ItemID: sword, First: ItemAura{Kind: AuraShell, Color: 0xFF0000FF}},
		{ItemID: sword, First: ItemAura{Kind: AuraParticles, Nodes: 101}},
		{ItemID: sword, Tag: "a|x"},
		{ItemID: sword, Tag: "Twenty five characters xx"},
		{ItemID: 0, Tag: "Virtuous"},
	} {
		if err := db.SetInstanceAuraAudited(bad, GMEdit{}, "test"); err == nil {
			t.Errorf("kept %+v", bad)
		}
	}
	// HeldCopy: the main hand's copy; the off hand empty
	if id, p, _, err := db.HeldCopy(197, 0, HandMain); err != nil || id != sword || p != pid {
		t.Fatalf("main hand: %d %d %v", id, p, err)
	}
	if id, _, _, err := db.HeldCopy(197, 0, HandOff); err != nil || id != 0 {
		t.Fatalf("off hand: %d %v", id, err)
	}
	// a whole record to a new row (a trade): carried, the cache too
	if err := inv.SetEquipLoc(197, sword, -1); err != nil {
		t.Fatal(err)
	}
	traded, err := inv.AddCharItem(198, pid, 1, 100, 0, LocPack)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.DeleteCharItem(197, sword); err != nil {
		t.Fatal(err)
	}
	if err := inv.CarryInstanceAura(sword, traded); err != nil {
		t.Fatal(err)
	}
	if _, ok := InstanceAuraHeld(sword); ok {
		t.Fatal("the old row keeps its aura")
	}
	if a, ok := InstanceAuraHeld(traded); !ok || a.Tag != row.Tag || a.ItemID != traded {
		t.Fatalf("traded: %+v %v", a, ok)
	}
	if e, err := inv.GetCharItem(198, traded); err != nil || e.AuraTag != row.Tag {
		t.Fatalf("the copy read: %q %v", e.AuraTag, err)
	}
	// to a merc and back
	if err := db.ensureMercItems(); err != nil {
		t.Fatal(err)
	}
	mid, gone, err := db.GiveMercItem(198, 1, traded, 1, 1)
	if err != nil || !gone {
		t.Fatalf("to the merc: %v %v", gone, err)
	}
	if a, ok := InstanceAuraHeld(-mid); !ok || a.Second != twinkle {
		t.Fatalf("the merc's: %+v %v", a, ok)
	}
	back, err := db.MercItemToPack(198, mid, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := InstanceAuraHeld(back); !ok || a.First != shadow {
		t.Fatalf("back in the pack: %+v %v", a, ok)
	}
	// the auction: listed from the bank, delivered as a new row
	if err := inv.MoveCharItem(198, back, LocBank); err != nil {
		t.Fatal(err)
	}
	l := AuctionLot{ID: 1, State: AuctionUnsold, Holder: 198}
	l.Item.ItemID, l.Item.StackLeft, l.Item.Pattern.PatternID, l.Item.Pattern.MaxStack = back, 1, pid, 1
	if err := inv.DeleteCharItem(198, back); err != nil {
		t.Fatal(err)
	}
	if err := db.ensureAuction(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auction_lots (lot, seller, seller_name, patternid, stack, remainhp, charges, min_bid, buyout, fee, listed_at, ends_at, state, holder, item_from)
		VALUES (1, 198, 'Other', ?, 1, 100, 0, 1, 0, 1, 0, 0, ?, 198, ?)`, pid, AuctionUnsold, back); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	d, err := deliverTx(tx, &l, time.Now())
	if err != nil || d == nil {
		tx.Rollback()
		t.Fatalf("delivered: %+v %v", d, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if a, ok := InstanceAuraHeld(d.Item.ItemID); !ok || a.Tag != row.Tag {
		t.Fatalf("delivered as %d: %+v %v", d.Item.ItemID, a, ok)
	}
	// a stack's part is never carried: the store's carry of nothing does nothing
	if err := inv.CarryInstanceAura(999999, 1000000); err != nil {
		t.Fatal(err)
	}
	// cleared: the row goes
	if err := db.SetInstanceAuraAudited(InstanceAura{ItemID: d.Item.ItemID}, GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, ok := InstanceAuraHeld(d.Item.ItemID); ok {
		t.Fatal("cleared, still read")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM instanceaura`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows left", n)
	}
	if s := InstanceTagged("Procs a 100 point DD.", "Virtuous", 255); s != "Aura: Virtuous\nProcs a 100 point DD." {
		t.Fatalf("tagged: %q", s)
	}
	if s := InstanceTagged("x", "", 255); s != "x" {
		t.Fatalf("no tag: %q", s)
	}
}
