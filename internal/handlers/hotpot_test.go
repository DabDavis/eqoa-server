package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// An item's HoT and PoT go out by CLIENT's stat ids (FindStatWord's table:
// 12 is "HoT", 13 "PoT"), so the tooltip names each as its column does
// (petition 1091: the belt's HoT 25 read "PoT 25"), and the worn sum credits
// the HoT to health and the PoT to power as before. The control: the other
// stats keep their ids.
func TestAnItemsHoTAndPoTGoOutByTheClientsIDs(t *testing.T) {
	hot, pot, ac := 25, 7, 20
	p := database.ItemPatternRecord{HoT: &hot, PoT: &pot, AC: &ac}
	s := itemPatternStats(&p)
	if s[12] != 25 || s[13] != 7 || s[14] != 20 || len(s) != 3 {
		t.Fatalf("stats %v: want 12 (HoT) 25, 13 (PoT) 7, 14 (AC) 20", s)
	}
	g := wornGear([]database.CharInventoryEntry{{EquipLoc: 11, Location: 0, Pattern: p}})
	if len(g.HoTItems) != 1 || g.HoTItems[0] != 25 || len(g.PoTItems) != 1 || g.PoTItems[0] != 7 || g.AC != 20 {
		t.Fatalf("worn: HoT %v PoT %v AC %d; want [25] [7] 20", g.HoTItems, g.PoTItems, g.AC)
	}
}
