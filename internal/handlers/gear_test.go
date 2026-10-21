package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// What is worn counts, by the dump's stat ids; what is carried or banked
// does not, and the weapon's damage is the primary's.
func TestWornGear(t *testing.T) {
	n := func(v int) *int { return &v }
	items := []database.CharInventoryEntry{
		{EquipLoc: 5, Pattern: database.ItemPatternRecord{Sta: n(20), AC: n(300), HPMAX: n(100), FR: n(5)}}, // chest
		{EquipLoc: 14, Pattern: database.ItemPatternRecord{Str: n(10), WeaponDamage: 560}},                  // primary
		{EquipLoc: -1, Pattern: database.ItemPatternRecord{Sta: n(50)}},                                     // carried
		{EquipLoc: 0, Location: 1, Pattern: database.ItemPatternRecord{Sta: n(70)}},                         // banked
	}
	g := wornGear(items)
	if g.Sta != 20 || g.Str != 10 || g.AC != 300 || g.MaxHP != 100 || g.FR != 5 || g.WeaponDamage != 560 {
		t.Errorf("gear %+v", g)
	}
}
