package handlers

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The merchants a real server holds (EQOA_REAL_DB names its database, a copy
// of which is read), after tools/stock_merchants.py: every NPC with the
// merchant flag but the ones the tool left out (merchant_stock 'left out'
// rows: Keepers, beasts, ...) opens on a catalog, through the same
// GetNPCItems by npc_id the window reads, within the list's 40, and every
// item in a catalog has a price but the ones Sandstorm itself sells at 0 and
// a GM's (merchant_stock 'free' and 'gm price'). Skipped without a database,
// and on one the tool has not stocked: none is in this repository.
func TestRealMerchantsSellSomething(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "merchants.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'merchant_stock'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/stock_merchants.py has not stocked this database")
	}
	leftOut, exempt := map[int]string{}, map[int]bool{}
	rows, err := db.Query(`SELECT what, key, note FROM merchant_stock WHERE what IN ('left out', 'free', 'gm price')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var what, note string
		var key int
		if err := rows.Scan(&what, &key, &note); err != nil {
			t.Fatal(err)
		}
		if what == "left out" {
			leftOut[key] = note
		} else {
			exempt[key] = true
		}
	}
	rows.Close() // one connection: no lookup while the rows are open

	npcs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	var empty, unpriced, over []string
	merchants, stocked, items := 0, 0, 0
	for _, npc := range npcs {
		if npc.NPCType&0xFF&npcFlagMerchant == 0 {
			continue
		}
		merchants++
		var catalog []database.CharInventoryEntry
		if npc.NPCID != 0 {
			if catalog, err = inv.GetNPCItems(npc.NPCID); err != nil {
				t.Fatalf("%s (row %d): %v", npc.NPCName, npc.ID, err)
			}
		}
		if len(catalog) == 0 {
			if _, ok := leftOut[npc.ID]; !ok {
				empty = append(empty, npc.NPCName)
			}
			continue
		}
		stocked++
		items += len(catalog)
		if len(catalog) > invListCap {
			over = append(over, npc.NPCName)
		}
		for _, e := range catalog {
			if merchantPrice(e.Pattern.ItemCost) <= 0 && !exempt[e.Pattern.PatternID] {
				unpriced = append(unpriced, npc.NPCName+": "+e.Pattern.ItemName)
			}
		}
		// the window's message builds for it
		if len(merchantBoxPayload(uint32(npc.ID), catalog[:min(len(catalog), invListCap)], merchantGreeting)) == 0 {
			t.Errorf("%s: an empty 0x00B7", npc.NPCName)
		}
	}
	sort.Strings(empty)
	sort.Strings(unpriced)
	t.Logf("%d merchants: %d stocked (%d catalog rows), %d left out, %d items Sandstorm sells at 0 or a GM's",
		merchants, stocked, items, len(leftOut), len(exempt))
	if len(empty) > 0 {
		t.Errorf("%d merchants sell nothing and were not left out: %v", len(empty), firstOf(empty, 40))
	}
	if len(unpriced) > 0 {
		t.Errorf("%d catalog items have no price: %v", len(unpriced), firstOf(unpriced, 40))
	}
	if len(over) > 0 {
		t.Errorf("%d catalogs are longer than the window's %d: %v", len(over), invListCap, over)
	}
}

func firstOf(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
