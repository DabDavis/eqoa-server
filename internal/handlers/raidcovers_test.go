package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A boss's priceless piece (a quest ring, a key) is rolled; a boss's sellable junk is
// not (the owner, 2026-11-13: the Ascended Ring of Veeshan went to the first looter).
func TestABossesPricelessPieceIsRolled(t *testing.T) {
	ring := &database.ItemPatternRecord{ItemName: "Ascended Ring of Veeshan", MaxStack: 1, EquipSlot: -1}
	if !raidCovers(ring, true) {
		t.Error("a boss's priceless ring is not rolled")
	}
	if raidCovers(ring, false) {
		t.Error("a common creature's priceless piece is rolled")
	}
	junk := &database.ItemPatternRecord{ItemName: "rusty bone", MaxStack: 1, EquipSlot: -1, ItemCost: 12}
	if raidCovers(junk, true) {
		t.Error("a boss's sellable junk is rolled")
	}
}
