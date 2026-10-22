package handlers

import (
	"bytes"
	"encoding/hex"
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

// worn is an item worn in equip slot e: its wear slot, attack type and model.
func worn(e, wear, attack int, model uint32) database.CharInventoryEntry {
	return database.CharInventoryEntry{EquipLoc: e, Pattern: database.ItemPatternRecord{EquipSlot: wear, AttackType: attack, Model: int64(int32(model))}}
}

// The hands as Sandstorm's channel carries them (the players in the PS2
// captures): item 0 the main hand, item 1 the off hand or the ranged weapon,
// item 2 the shield, each hand's attack type with it. The control: the rule
// before (wear slots 14 and 13 alone) shows none of the two-hander, the
// primary-only weapon, the held items or the bow.
func TestWornLookHands(t *testing.T) {
	type hands struct {
		primary, secondary, shield uint32
		types                      [2]byte
	}
	cases := []struct {
		what  string
		items []database.CharInventoryEntry
		want  hands
	}{
		// Rigalo, Luck, Lust: a two-hander alone in item 0, type 4
		{"two-hander", []database.CharInventoryEntry{worn(16, 15, 4, 0x7C7DBEA9)}, hands{0x7C7DBEA9, 0, 0, [2]byte{4, 0}}},
		// Hodstock: a primary-only weapon and a wand held in the off hand
		{"primary and held", []database.CharInventoryEntry{worn(14, 12, 3, 0xA36976DF), worn(20, 18, 0, 0x2EDA5089)}, hands{0xA36976DF, 0x2EDA5089, 0, [2]byte{3, 0}}},
		// Zaknafein: a sword and a shield
		{"sword and shield", []database.CharInventoryEntry{worn(17, 13, 0, 0x343CF977), worn(14, 12, 1, 0x3B99B7CD)}, hands{0x3B99B7CD, 0, 0x343CF977, [2]byte{1, 0}}},
		// Mcgregor: two rapiers
		{"two weapons", []database.CharInventoryEntry{worn(14, 14, 5, 0xF4DD991A), worn(15, 14, 5, 0xF4DD991A)}, hands{0xF4DD991A, 0xF4DD991A, 0, [2]byte{5, 5}}},
		// Ulgulu: an orb held in the main hand, a wand in the off hand
		{"two held", []database.CharInventoryEntry{worn(19, 18, 0, 0x8BB07164), worn(20, 18, 0, 0xCE553E09)}, hands{0x8BB07164, 0xCE553E09, 0, [2]byte{0, 0}}},
		// a bow takes every hand off: item 1, where its arrow is
		{"bow", []database.CharInventoryEntry{worn(18, 16, 7, 0x0DA9879A)}, hands{0, 0x0DA9879A, 0, [2]byte{0, 7}}},
		// a thrown weapon worn after a second weapon leaves it in the off hand
		{"thrown behind a second weapon", []database.CharInventoryEntry{worn(14, 12, 1, 0x3B99B7CD), worn(18, 17, 10, 0xB0A9CBD2), worn(15, 14, 1, 0x47D981E0)}, hands{0x3B99B7CD, 0x47D981E0, 0, [2]byte{1, 1}}},
		// carried, not worn
		{"carried", []database.CharInventoryEntry{worn(-1, 15, 4, 0x7C7DBEA9)}, hands{}},
	}
	for _, c := range cases {
		p := &entity.Player{Primary: 1, Secondary: 2, Shield: 3, PrimaryType: 9, SecondaryType: 9}
		wornLook(p, c.items)
		if got := (hands{p.Primary, p.Secondary, p.Shield, [2]byte{p.PrimaryType, p.SecondaryType}}); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.what, got, c.want)
		}
	}

	before := func(items []database.CharInventoryEntry) hands {
		var h hands
		for _, it := range items {
			if it.EquipLoc < 0 {
				continue
			}
			switch it.Pattern.EquipSlot {
			case 14:
				if it.EquipLoc == 15 {
					h.secondary = uint32(it.Pattern.Model)
				} else {
					h.primary = uint32(it.Pattern.Model)
				}
			case 13:
				h.shield = uint32(it.Pattern.Model)
			}
		}
		return h
	}
	for _, i := range []int{0, 1, 4, 5} {
		if got := before(cases[i].items); got.primary == cases[i].want.primary && got.secondary == cases[i].want.secondary {
			t.Errorf("the control: by wear slots 14 and 13 alone %s should not show", cases[i].what)
		}
	}
}

// A character list entry's gear against Sandstorm's own for the same worn
// items (Zaknafein in pcaps/ps2_sandstorm.pcap: robe 3, a sword and a
// shield, type 1, armour set 4 in every slot), byte for byte but the robe's
// and the armour's tints, which are not kept. Nothing worn is Louiexiii's
// entry, tints and all. The control: the blank entry sent before.
func TestCharacterGearIsSandstorms(t *testing.T) {
	hexes := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	zaknafein := hexes("0300000009cdb7993b0000000077f93c34010000040404040404090909090909")
	items := []database.CharInventoryEntry{worn(14, 12, 1, 0x3B99B7CD), worn(17, 13, 0, 0x343CF977), worn(21, 2, 0, 3)}
	for e, wear := range map[int]int{5: 5, 8: 7, 1: 19, 12: 10, 13: 11, 0: 1} {
		items = append(items, worn(e, wear, 0, 4))
	}
	g := characterGear(items)
	untinted := func(b []byte) []byte {
		c := append([]byte(nil), b...)
		c[4] = 0
		copy(c[26:], make([]byte, 6))
		return c
	}
	if !bytes.Equal(g, untinted(zaknafein)) {
		t.Errorf("gear %x\n want %x (Sandstorm's, tints aside)", g, untinted(zaknafein))
	}
	louiexiii := hexes("ffffffff00000000000000000000000000000000ffffffffffff000000000000")
	if g := characterGear(nil); !bytes.Equal(g, louiexiii) {
		t.Errorf("nothing worn: %x, want %x", g, louiexiii)
	}
	blank := hexes("ffffffff00000000000000000000000000000000ffffffffffff000000000000")
	if bytes.Equal(blank, untinted(zaknafein)) {
		t.Error("the control: the blank entry must not be Sandstorm's for a character in gear")
	}
}
