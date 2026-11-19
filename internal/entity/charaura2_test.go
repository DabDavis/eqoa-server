package entity

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A character's own hand second aura (charaura's color2 ... nodes2,
// 2026-11-16: a GM's sword "will keep the red aura it has and stack the
// twinkle"): his row's first in the main hand's place and its second in the
// main hand's second, both of the held item's auras given way; another
// wearer of the same item keeps the item's two; a row with a first alone
// still takes the item's second away (as before); a client that never
// asked for second auras gets the 335 bytes.
func TestACharactersOwnSecondAura(t *testing.T) {
	const sword = 714382
	itemFirst := database.ItemAura{PatternID: sword, Color: 0x3060FFFF, Kind: database.AuraEpic, Source: 0x88EC092D}
	itemSecond := database.ItemAura{PatternID: sword, Kind: database.AuraParticles, Source: 0x6C151A60}
	red := database.ItemAura{Color: 0x000000FF, Kind: database.AuraParticles}
	twinkle := database.ItemAura{Kind: database.AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	w := NewWorld()
	w.SetItemAuras(map[int]database.ItemAura{sword: itemFirst})
	w.SetItemSecondAuras(map[int]database.ItemAura{sword: itemSecond})
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraMainHand] = red
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own, {CharID: 198}: own})
	w.SetCharSecondAuras(map[database.CharAuraKey][2]database.ItemAura{{CharID: 197}: {twinkle, {}}})
	dust := ornatePlayer()
	dust.CharServerID = 197
	dust.WornMask, dust.WornPieces = 0, [AuraSlots]int{}
	dust.SetWornPiece(AuraMainHand, sword)
	pa := w.PieceAurasOf(dust)
	if pa == nil || pa[AuraMainHand] != red || pa[AuraMainHand2] != twinkle || pa[AuraOffHand] != (database.ItemAura{}) || pa[AuraOffHand2] != (database.ItemAura{}) {
		t.Fatalf("Dust's: %+v", pa)
	}
	disc := SerializePlayerUpdate(100003, dust)
	s := lookSession(t, 44)
	s.SetLiveLook(true)
	s.SetArmorLook(true)
	s.SetPieceAuras(true)
	s.SetBodyAura(true)
	s.SetCapeAura(true)
	s.SetHandAura(true)
	old := ForViewerAuras(s, disc, &dust.Dyes, pa)
	if len(old) != HandAuraStateSize || !bytes.Equal(old[CapeAuraStateSize:], append(auraBytes(red), make([]byte, PieceAuraSize)...)) {
		t.Fatalf("the hands' clients: %d bytes", len(old))
	}
	s.SetSecondAura(true)
	got := ForViewerAuras(s, disc, &dust.Dyes, pa)
	want := append(append(append([]byte(nil), old...), auraBytes(twinkle)...), make([]byte, PieceAuraSize)...)
	if len(got) != SecondAuraStateSize || !bytes.Equal(got, want) {
		t.Fatalf("the second auras' clients: tail % x", got[HandAuraStateSize:])
	}
	// another wearer: the item's two
	other := ornatePlayer()
	other.CharServerID = 500
	other.WornMask, other.WornPieces = 0, [AuraSlots]int{}
	other.SetWornPiece(AuraMainHand, sword)
	if a := w.PieceAurasOf(other); a == nil || a[AuraMainHand] != itemFirst || a[AuraMainHand2] != itemSecond {
		t.Fatalf("another wearer: %+v", a)
	}
	// a first alone: the item's second gone, as before
	first := ornatePlayer()
	first.CharServerID = 198
	first.WornMask, first.WornPieces = 0, [AuraSlots]int{}
	first.SetWornPiece(AuraMainHand, sword)
	if a := w.PieceAurasOf(first); a == nil || a[AuraMainHand] != red || a[AuraMainHand2] != (database.ItemAura{}) {
		t.Fatalf("a first alone: %+v", a)
	}
	// a second alone (the row's first none): the item's first gone too
	w.SetCharAuras(nil)
	if a := w.PieceAurasOf(dust); a == nil || a[AuraMainHand] != (database.ItemAura{}) || a[AuraMainHand2] != twinkle {
		t.Fatalf("a second alone: %+v", a)
	}
}
