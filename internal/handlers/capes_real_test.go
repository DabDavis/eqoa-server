package handlers

import (
	"bytes"
	"os"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// On a real database with tools/seed_capes.py applied (EQOA_REAL_DB, a copy):
// every Cape Tailor is a merchant whose catalog (npcitem under its own NPCID)
// is every cape record, each of the cape's wear slot with a style, for every
// class and race, NO TRADE and worth nothing, and its list is no class shop's,
// so every class is offered all of them within the client's 40; a record goes
// out to a PS2 as a plain item and to a client that draws capes as a cape. The
// control: no other NPC sells a cape.
func TestRealCapeTailorsSellEveryCape(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var capes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE equipslot = ?`, wearCape).Scan(&capes); err != nil || capes == 0 {
		t.Skipf("no capes in %s (%v): apply tools/seed_capes.py to a copy", path, err)
	}
	// who sells a cape at all, by catalog key (npcitem.npcid), in one query
	sellers := map[int]int{}
	rows, err := db.Query(`SELECT n.npcid, COUNT(*) FROM npcitem n JOIN itempattern p ON p.patternid = n.patternid
		WHERE p.equipslot = ? GROUP BY n.npcid`, wearCape)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		sellers[id] = n
	}
	rows.Close()
	npcs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	tailors := 0
	for _, n := range npcs {
		if n.NPCName != "Cape Tailor" {
			if sellers[n.NPCID] != 0 {
				t.Errorf("%q (%d) sells %d capes", n.NPCName, n.ID, sellers[n.NPCID])
			}
			continue
		}
		tailors++
		wares, err := inv.GetNPCItems(n.NPCID)
		if err != nil {
			t.Fatal(err)
		}
		sells := sellers[n.NPCID]
		npc := &entity.NPC{Name: n.NPCName, NPCID: n.NPCID, NPCType: n.NPCType}
		if n.NPCType&npcFlagMerchant == 0 || n.NPCID != n.ID || classShop(npc) {
			t.Errorf("tailor %d: type %#x, NPCID %d, a class shop %v", n.ID, n.NPCType, n.NPCID, classShop(npc))
		}
		if sells != capes || len(wares) != capes || len(wares) > invListCap {
			t.Errorf("tailor %d sells %d records, %d capes, of %d", n.ID, len(wares), sells, capes)
		}
		for _, w := range wares {
			p := w.Pattern
			style, _ := capeLook(&p)
			if style == 0 || p.ClassUse&0x7FFF != 0x7FFF || p.RaceUse == 0 || p.Trade != 1 || p.ItemCost != 0 || p.LevelReq > 1 {
				t.Errorf("%q (%d): style %d, classes %#x, races %#x, trade %d, cost %d, level %d",
					p.ItemName, p.PatternID, style, p.ClassUse, p.RaceUse, p.Trade, p.ItemCost, p.LevelReq)
			}
			ps2 := build(protocol.OpcodeMerchantBox, 4096, func(b *buffer.Writer) { writeItemPattern(b, &w, false) })
			plain := w
			plain.Pattern.EquipSlot = -1
			if want := build(protocol.OpcodeMerchantBox, 4096, func(b *buffer.Writer) { writeItemPattern(b, &plain, true) }); !bytes.Equal(ps2, want) {
				t.Errorf("%q goes out to a PS2 as more than a plain item", p.ItemName)
			}
		}
	}
	if tailors == 0 {
		t.Fatal("no Cape Tailor in the database")
	}
	t.Logf("%d tailors, each selling the %d capes", tailors, capes)
}
