package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The swing follows the client's list, not the database's row order:
// AutoAttackTick takes the FIRST item worn in 14, 16 or 18 as it walks
// m_inventory, and that item decides TargetInRange's reach.
func TestSwingItemIsTheFirstInTheClientsList(t *testing.T) {
	bow := database.CharInventoryEntry{ItemID: 11, EquipLoc: 18, Pattern: database.ItemPatternRecord{PatternID: 1, AttackType: 9, Unk5: 30}}
	sword := database.CharInventoryEntry{ItemID: 12, EquipLoc: 14, Pattern: database.ItemPatternRecord{PatternID: 2, AttackType: 3}}
	banked := database.CharInventoryEntry{ItemID: 13, EquipLoc: 16, Location: 1, Pattern: database.ItemPatternRecord{PatternID: 3, AttackType: 3}}
	rows := []database.CharInventoryEntry{bow, sword, banked}

	// The client's list holds the sword first.
	list := newInvList([]database.CharInventoryEntry{sword, bow})
	got := inventoryListOrder(list, rows)
	if len(got) != 2 || got[0].ItemID != 12 || got[1].ItemID != 11 {
		t.Fatalf("order %v, want the sword then the bow, the bank left out", ids(got))
	}
	// Without a list: as stored, carried only.
	if got := inventoryListOrder(nil, rows); len(got) != 2 || got[0].ItemID != 11 {
		t.Fatalf("no list: %v", ids(got))
	}

	// A bow beside a hand weapon is GetSwing's kind 0 either way; the first
	// item sets the reach: the sword in the client's order, the bow in the
	// stored one.
	var worn [21]bool
	worn[14], worn[18] = true, true
	first := got[0]
	if s := entity.SwingFor(1, 40, worn, true, first.Pattern.AttackType, first.Pattern.Unk5); s.Kind != 0 || s.Reach != 4.5 {
		t.Errorf("client's order: kind %d reach %v, want 0 and the sword's 4.5", s.Kind, s.Reach)
	}
	stored := inventoryListOrder(nil, rows)[0]
	if s := entity.SwingFor(1, 40, worn, true, stored.Pattern.AttackType, stored.Pattern.Unk5); s.Kind != 0 || s.Reach != 30 {
		t.Errorf("stored order: kind %d reach %v, want 0 and the bow's 30", s.Kind, s.Reach)
	}
}

func ids(rs []database.CharInventoryEntry) []int {
	var out []int
	for _, r := range rs {
		out = append(out, r.ItemID)
	}
	return out
}

// PlayEmote's table (0x00BCFC38): wave 0x001E/56, bow 0x0042/57, point
// 0x0043/60, cheer 0x0044/61.
func TestEmoteOpcodesAreCLIENTsPlayEmote(t *testing.T) {
	want := map[uint16]byte{0x001E: 56, 0x0042: 57, 0x0043: 60, 0x0044: 61}
	if len(emoteActions) != len(want) {
		t.Fatalf("%d emotes", len(emoteActions))
	}
	for op, a := range emoteActions {
		if want[uint16(op)] != a {
			t.Errorf("0x%04X -> %d, want %d", uint16(op), a, want[uint16(op)])
		}
	}
}
