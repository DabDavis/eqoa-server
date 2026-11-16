package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A held copy's own aura (instanceaura; 2026-11-16: "it will have a tag
// 'Aura: Virtuous' ... so it doesnt just stick to all his weps"): shown in
// the hand's places, first and second, on whoever holds that copy; another
// copy of the same record keeps the record's aura; a character's own hand
// row still comes first; a row with a tag alone leaves the record's aura.
func TestInstanceAuraOnTheHeldCopy(t *testing.T) {
	const sword = 714382
	t.Cleanup(func() { database.SetInstanceAuraCache(nil) })
	record := database.ItemAura{PatternID: sword, Color: 0x3060FFFF, Kind: database.AuraEpic, Source: 0x88EC092D}
	shadow := database.ItemAura{Color: 0x000000FF, Kind: database.AuraParticles}
	twinkle := database.ItemAura{Kind: database.AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	taint := database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}
	w := NewWorld()
	w.SetItemAuras(map[int]database.ItemAura{sword: record})
	w.SetInstanceAuras(map[int]database.InstanceAura{
		1001: {ItemID: 1001, Tag: "Shadowed, Twinkling", First: shadow, Second: twinkle},
		1003: {ItemID: 1003, Tag: "Virtuous"}, // a tag alone
	})
	holder := func(cid uint32, copyID int) *Player {
		p := ornatePlayer()
		p.CharServerID = cid
		p.WornMask, p.WornPieces = 0, [AuraSlots]int{}
		p.SetWornPiece(AuraMainHand, sword)
		p.WornItems[AuraMainHand] = copyID
		return p
	}
	if a := w.PieceAurasOf(holder(197, 1001)); a == nil || a[AuraMainHand] != shadow || a[AuraMainHand2] != twinkle {
		t.Fatalf("the enchanted copy: %+v", a)
	}
	// the same copy in another's hand (traded): the same
	if a := w.PieceAurasOf(holder(500, 1001)); a == nil || a[AuraMainHand] != shadow || a[AuraMainHand2] != twinkle {
		t.Fatalf("the copy traded: %+v", a)
	}
	// another copy of the record: the record's
	if a := w.PieceAurasOf(holder(197, 1002)); a == nil || a[AuraMainHand] != record || a[AuraMainHand2] != (database.ItemAura{}) {
		t.Fatalf("another copy: %+v", a)
	}
	// a tag alone: the record's
	if a := w.PieceAurasOf(holder(197, 1003)); a == nil || a[AuraMainHand] != record {
		t.Fatalf("a tag alone: %+v", a)
	}
	if database.InstanceTag(1003) != "Virtuous" || database.InstanceTag(1002) != "" {
		t.Fatal("tags")
	}
	// the character's own hand row first
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraMainHand] = taint
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own})
	if a := w.PieceAurasOf(holder(197, 1001)); a == nil || a[AuraMainHand] != taint || a[AuraMainHand2] != (database.ItemAura{}) {
		t.Fatalf("the character's own: %+v", a)
	}
	// a mercenary's own record (-charmercitem.id)
	w.SetCharAuras(nil)
	w.SetInstanceAuras(map[int]database.InstanceAura{-7: {ItemID: -7, Tag: "Tainted", First: taint}})
	n := &NPC{}
	n.WornMask = 1 << AuraOffHand
	n.WornPieces[AuraOffHand] = sword
	n.WornItems[AuraOffHand] = -7
	if a := w.NPCPieceAurasOf(n); a == nil || a[AuraOffHand] != taint {
		t.Fatalf("the merc's: %+v", a)
	}
}
