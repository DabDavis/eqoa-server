package entity

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A held item's second aura (2026-11-16: Icingdeath's Sword of Virtue look and
// its freezing): sent after the hands' first auras, 355 bytes, to a client that
// answered bit 7 alone; every older client the 335 bytes it was; a character's
// own hand aura takes the hand whole (no second).
func TestAHeldItemsSecondAura(t *testing.T) {
	if SecondAuraStateSize != 355 || AuraMainHand2 != 11 || AuraOffHand2 != 12 {
		t.Fatalf("sizes %d %d %d", SecondAuraStateSize, AuraMainHand2, AuraOffHand2)
	}
	sov := database.ItemAura{PatternID: 715041, Color: 0x3060FFFF, Kind: database.AuraEpic, Source: 0x88EC092D}
	freeze := database.ItemAura{PatternID: 715041, Color: 0x3060FFFF, Kind: database.AuraParticles, Source: 0x6C151A60}
	ring := database.ItemAura{PatternID: 715040, Color: 0x6EA8FFFF, Kind: database.AuraParticles, Source: 0x1D875ECD, Nodes: 100}
	w := NewWorld()
	w.SetItemAuras(map[int]database.ItemAura{715041: sov, 715040: sov})
	w.SetItemSecondAuras(map[int]database.ItemAura{715041: freeze, 715040: ring})
	p := ornatePlayer()
	p.CharServerID = 300
	p.WornMask, p.WornPieces = 0, [AuraSlots]int{}
	p.SetWornPiece(AuraMainHand, 715041)
	p.SetWornPiece(AuraOffHand, 715040)
	pa := w.PieceAurasOf(p)
	if pa == nil || pa[AuraMainHand] != sov || pa[AuraMainHand2] != freeze || pa[AuraOffHand] != sov || pa[AuraOffHand2] != ring {
		t.Fatalf("auras %+v", pa)
	}
	disc := SerializePlayerUpdate(100002, p)
	s := lookSession(t, 43)
	s.SetLiveLook(true)
	s.SetArmorLook(true)
	s.SetPieceAuras(true)
	s.SetBodyAura(true)
	s.SetCapeAura(true)
	s.SetHandAura(true)
	old := ForViewerAuras(s, disc, &p.Dyes, pa)
	if len(old) != HandAuraStateSize || !bytes.Equal(old[CapeAuraStateSize:], append(auraBytes(sov), auraBytes(sov)...)) {
		t.Fatalf("the hands' clients: %d bytes", len(old))
	}
	s.SetSecondAura(true)
	got := ForViewerAuras(s, disc, &p.Dyes, pa)
	want := append(append(append([]byte(nil), old...), auraBytes(freeze)...), auraBytes(ring)...)
	if len(got) != SecondAuraStateSize || !bytes.Equal(got, want) || !bytes.Equal(got, SecondAuraChannel(disc, &p.Dyes, pa)) {
		t.Fatalf("the second auras' clients: %d bytes, tail % x", len(got), got[HandAuraStateSize:])
	}
	s.SetHandAura(false)
	if s.SecondAura() || len(ForViewerAuras(s, disc, &p.Dyes, pa)) != CapeAuraStateSize {
		t.Fatal("bit 7 without bit 6 counts")
	}
	// a second alone (no first) shows; a character's own hand aura takes it away
	w.SetItemAuras(nil)
	if a := w.PieceAurasOf(p); a == nil || a[AuraMainHand] != (database.ItemAura{}) || a[AuraMainHand2] != freeze {
		t.Fatalf("a second alone: %+v", a)
	}
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraMainHand] = database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 300}: own})
	if a := w.PieceAurasOf(p); a == nil || a[AuraMainHand] != own[AuraMainHand] || a[AuraMainHand2] != (database.ItemAura{}) || a[AuraOffHand2] != ring {
		t.Fatalf("the character's own hand: %+v", a)
	}
	// worn, not held: no second
	q := ornatePlayer()
	q.CharServerID = 301
	q.WornMask, q.WornPieces = 0, [AuraSlots]int{}
	q.SetWornPiece(AuraChest, 715041)
	if a := w.PieceAurasOf(q); a != nil {
		t.Fatalf("a second on a worn piece: %+v", a)
	}
}
