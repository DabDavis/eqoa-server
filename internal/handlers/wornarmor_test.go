package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The armour on the chest, as wornGear reads it (entity/armorbody.go): a worn chest piece's
// material, a robe over it cloth; one in the pack, nothing.
func TestWornGearReadsTheChestsArmour(t *testing.T) {
	item := func(slot int, model int64, loc int) database.CharInventoryEntry {
		return database.CharInventoryEntry{EquipLoc: loc, Pattern: database.ItemPatternRecord{EquipSlot: slot, Model: model}}
	}
	if g := wornGear([]database.CharInventoryEntry{item(wearChest, 4, 3)}); g.Armor != entity.ArmorPlate {
		t.Errorf("a breastplate: %v", g.Armor)
	}
	if g := wornGear([]database.CharInventoryEntry{item(wearChest, 4, 3), item(wearRobe, 0, 4)}); g.Armor != entity.ArmorCloth {
		t.Errorf("a robe over it: %v", g.Armor)
	}
	if g := wornGear([]database.CharInventoryEntry{item(wearChest, 7, -1)}); g.Armor != entity.ArmorNone {
		t.Errorf("one in the pack: %v", g.Armor)
	}
}
