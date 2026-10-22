package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
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

// The look is the worn items' alone: a weapon taken off leaves the hand, and
// one worn again goes in the hand its equip slot names, the second hand's
// weapon there whatever came first. The control: filling without clearing,
// first hand free first, as the server did, keeps the blade taken off.
func TestWornLookFollowsWhatIsWorn(t *testing.T) {
	blade := database.CharInventoryEntry{EquipLoc: 14, Pattern: database.ItemPatternRecord{EquipSlot: 14, Model: -1485745917}}
	shield := database.CharInventoryEntry{EquipLoc: 17, Pattern: database.ItemPatternRecord{EquipSlot: 13, Model: 876411255}}
	tunic := database.CharInventoryEntry{EquipLoc: 5, Pattern: database.ItemPatternRecord{EquipSlot: 5, Model: 9}}
	p := &entity.Player{}
	wornLook(p, []database.CharInventoryEntry{blade, shield, tunic})
	if p.Primary != 0xA7715103 || p.Secondary != 0 || p.Shield != 0x343CF977 || p.Chest != 9 {
		t.Fatalf("worn: %+v", p)
	}

	off := blade
	off.EquipLoc = -1
	nude := tunic
	nude.EquipLoc = -1
	wornLook(p, []database.CharInventoryEntry{off, shield, nude})
	if p.Primary != 0 || p.Shield != 0x343CF977 || p.Chest != 0 {
		t.Errorf("taken off: primary %#x chest %d, want 0 and 0", p.Primary, p.Chest)
	}

	second := blade
	second.EquipLoc = 15
	wornLook(p, []database.CharInventoryEntry{second, blade})
	if p.Primary != 0xA7715103 || p.Secondary != 0xA7715103 {
		t.Errorf("both hands: %#x %#x", p.Primary, p.Secondary)
	}
	wornLook(p, []database.CharInventoryEntry{second})
	if p.Primary != 0 || p.Secondary != 0xA7715103 {
		t.Errorf("second hand alone: primary %#x secondary %#x, want 0 and the blade", p.Primary, p.Secondary)
	}

	old := func(p *entity.Player, items []database.CharInventoryEntry) {
		for _, item := range items {
			if item.EquipLoc >= 0 && item.Pattern.EquipSlot == 14 {
				if p.Primary == 0 {
					p.Primary = uint32(item.Pattern.Model)
				} else {
					p.Secondary = uint32(item.Pattern.Model)
				}
			}
		}
	}
	c := &entity.Player{}
	old(c, []database.CharInventoryEntry{blade})
	old(c, []database.CharInventoryEntry{off})
	if c.Primary == 0 {
		t.Error("the control: filled without clearing, the blade taken off must stay in the hand")
	}
}
