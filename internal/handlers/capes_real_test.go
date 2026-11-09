package handlers

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// On a real database with tools/seed_capes.py applied (EQOA_REAL_DB, a copy):
// every cape is an heirloom (named so: an import never damps one) of the
// cape's wear slot with a style, for every class and race, NO TRADE and worth
// nothing; the genie's capes (capeStock) sell every one; every Heirloom
// Outfitter sells the same capes after its own stock, and for every class its
// list to a client that draws capes stays within the client's 40, while to a
// PS2 it is what it was (no cape in it). The control: no other NPC sells a
// cape.
func TestRealHeirloomCapes(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := database.NewInventoryStore(db)
	all, err := inv.GetNPCItems(capeStock)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Skipf("no capes in %s: apply tools/seed_capes.py to a copy", path)
	}
	capes := map[int]bool{}
	for _, w := range all {
		p := w.Pattern
		style, _ := capeLook(&p)
		if !isCape(&p) || style == 0 || p.ClassUse&0x7FFF != 0x7FFF || p.RaceUse == 0 || p.Trade != 1 || p.ItemCost != 0 ||
			p.LevelReq > 1 || database.IsImportedGear(p.EquipSlot, p.MaxStack, p.ItemName) {
			t.Errorf("%q (%d): wear %d, style %d, classes %#x, races %#x, trade %d, cost %d, level %d, damped by an import %v",
				p.ItemName, p.PatternID, p.EquipSlot, style, p.ClassUse, p.RaceUse, p.Trade, p.ItemCost, p.LevelReq,
				database.IsImportedGear(p.EquipSlot, p.MaxStack, p.ItemName))
		}
		capes[p.PatternID] = true
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE equipslot = ?`, wearCape).Scan(&n); err != nil || n != len(capes) {
		t.Errorf("%d cape records, the genie sells %d (%v)", n, len(capes), err)
	}
	// who sells a cape at all, by catalog key, in one query
	sellers := map[int]int{}
	rows, err := db.Query(`SELECT n.npcid, COUNT(*) FROM npcitem n JOIN itempattern p ON p.patternid = n.patternid
		WHERE p.equipslot = ? GROUP BY n.npcid`, wearCape)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, k int
		if err := rows.Scan(&id, &k); err != nil {
			t.Fatal(err)
		}
		sellers[id] = k
	}
	rows.Close()
	npcs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	h := &NPCWindowHandler{inv: inv}
	outfitters, onEach := 0, -1
	for _, r := range npcs {
		if r.NPCName != heirloomOutfitterName {
			if sellers[r.NPCID] != 0 {
				t.Errorf("%q (%d) sells %d capes", r.NPCName, r.ID, sellers[r.NPCID])
			}
			continue
		}
		outfitters++
		if onEach < 0 {
			onEach = sellers[r.NPCID]
		}
		if sellers[r.NPCID] != onEach || onEach == 0 {
			t.Errorf("outfitter %d sells %d capes, the first %d", r.ID, sellers[r.NPCID], onEach)
		}
		npc := &entity.NPC{Name: r.NPCName, NPCID: r.NPCID, NPCType: r.NPCType}
		whole, err := inv.GetNPCItems(r.NPCID)
		if err != nil {
			t.Fatal(err)
		}
		for class := 0; class < 15; class++ {
			p := &entity.Player{Class: class}
			// the shop's own reading for the first outfitter (they all stock alike), its pieces for the rest
			list := func(capes bool) []database.CharInventoryEntry {
				if outfitters == 1 {
					got, err := h.waresFor(p, npc, capes)
					if err != nil {
						t.Fatal(err)
					}
					return got
				}
				w := whole
				if !capes {
					w = waresBut(w, isCape)
				}
				w = waresForClass(w, class)
				return w[:min(len(w), invListCap)]
			}
			drawn, ps2 := list(true), list(false)
			mine := 0
			for _, w := range waresForClass(whole, class) {
				if !isCape(&w.Pattern) {
					mine++
				}
			}
			if len(drawn) != mine+onEach || len(drawn) > invListCap {
				t.Errorf("outfitter %d, class %d: %d listed to a client that draws capes (%d of its own, %d capes; the client holds %d)",
					r.ID, class, len(drawn), mine, onEach, invListCap)
			}
			if len(ps2) != mine {
				t.Errorf("outfitter %d, class %d: %d listed to a PS2, want its %d", r.ID, class, len(ps2), mine)
			}
			for i, w := range drawn[mine:] {
				if !isCape(&w.Pattern) {
					t.Errorf("outfitter %d, class %d: line %d past its own stock is %q", r.ID, class, mine+i, w.Pattern.ItemName)
				}
			}
		}
	}
	if outfitters == 0 {
		t.Fatal("no Heirloom Outfitter in the database")
	}
	t.Logf("%d capes on the genie's list, %d on each of %d outfitters", len(capes), onEach, outfitters)
}
