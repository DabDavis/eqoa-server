package handlers

import (
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// rechargeFake is an inventory of rows and the full charges of records.
type rechargeFake struct {
	rows []database.CharInventoryEntry
	full map[int]int
	set  map[int]int
}

func (f *rechargeFake) GetCharacterItems(uint32) ([]database.CharInventoryEntry, error) { return f.rows, nil }
func (f *rechargeFake) ItemCharges(p int) int                                          { return f.full[p] }
func (f *rechargeFake) SetItemCharges(_ uint32, itemID, n int) error {
	f.set[itemID] = n
	return nil
}

// Petition 1985: Master Lizzlen restores a pack item's charges to its full count, the client told
// by the item taken off its list and given again; a worn one is named and left; an item at its
// full count, one with no charges (a sword) and a stack (potions) are left alone.
func TestLizzlenRechargesClickies(t *testing.T) {
	mgr := session.NewManager()
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &entity.Player{CharServerID: 77, Name: "Hoofbite"}
	item := func(id, pat int, name string, charges, equip, stack, maxStack int) database.CharInventoryEntry {
		return database.CharInventoryEntry{ItemID: id, StackLeft: stack, Charges: charges, EquipLoc: equip,
			Location: database.LocPack, Pattern: database.ItemPatternRecord{PatternID: pat, ItemName: name, MaxStack: maxStack}}
	}
	rows := []database.CharInventoryEntry{
		item(10, 960149, "Ring of Speed", 37, -1, 1, 1),
		item(11, 960150, "Worn Clicky", 5, 3, 1, 1),
		item(12, 960151, "Full Clicky", 50, -1, 1, 1),
		item(13, 500, "Sword", 0, -1, 1, 1),
		item(14, 600, "Potion", 0, -1, 5, 20),
	}
	f := &rechargeFake{rows: rows, full: map[int]int{960149: 100, 960150: 20, 960151: 50, 600: 3}, set: map[int]int{}}
	sess.SetInventoryList(newInvList(rows))
	done, worn, err := rechargeItems(sess, p, f)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(done, []string{"Ring of Speed (100 charges)"}) || !slices.Equal(worn, []string{"Worn Clicky"}) {
		t.Fatalf("done %q, worn %q", done, worn)
	}
	if len(f.set) != 1 || f.set[10] != 100 {
		t.Fatalf("written %v", f.set)
	}
	ops := queuedOps(sess)
	if !slices.Contains(ops, protocol.OpcodeRemoveInvItem) || !slices.Contains(ops, protocol.OpcodeAddInvItem) {
		t.Fatalf("the client told %v", ops)
	}
	l := sessionInvList(sess)
	if at := l.indexOfItem(10); at != 4 {
		t.Fatalf("the ring at list index %d, want the end (4)", at)
	}
}
