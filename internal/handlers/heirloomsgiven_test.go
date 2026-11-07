package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// world.heirlooms: false gives no kept item at a login, though their records
// exist; the control, on (the default), gives each, once.
func TestWorldHeirloomsOffGivesNothing(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/kept.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Holder", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{GenieLampName, WolfFangName, SummoningPortalName} {
		if _, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 0, 0, 0, ?, '', 0, 0)`, name); err != nil {
			t.Fatal(err)
		}
	}
	h := NewCharacterHandler(store, nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	held := func() (n int) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ?`, rec.ServerID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Cleanup(func() { SetHeirloomsGiven(true) })
	SetHeirloomsGiven(false)
	h.giveKeptItems(rec.ServerID)
	if n := held(); n != 0 {
		t.Fatalf("world.heirlooms false: %d items given", n)
	}
	SetHeirloomsGiven(true)
	h.giveKeptItems(rec.ServerID)
	h.giveKeptItems(rec.ServerID)
	if n := held(); n != 3 {
		t.Fatalf("on: %d items held, want the three", n)
	}
}
