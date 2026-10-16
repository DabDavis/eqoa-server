package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The fifth field of a dumped item is its quick slot: Sandstorm's dump sends
// -1 for an inventory item and 0 for a bank item.
func TestDumpQuickSlotIsNotTheLocation(t *testing.T) {
	if got := dumpQuickSlot(&database.CharInventoryEntry{Location: 0}); got != -1 {
		t.Errorf("inventory item: quick slot %d, want -1", got)
	}
	if got := dumpQuickSlot(&database.CharInventoryEntry{Location: 1}); got != 0 {
		t.Errorf("bank item: quick slot %d, want 0", got)
	}
}
