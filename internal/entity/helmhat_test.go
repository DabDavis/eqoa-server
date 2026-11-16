package entity

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A hat and an aura stack on the helm (2026-11-16: the owner, "when i put
// /aura on dusts turban turns it into a cloth cap"): a GM's shell (charaura 5)
// on a character wearing a hat ITEM (itemaura kind 10 on the helm record)
// leaves the hat in AuraHelmHat and the shell in AuraHelm. A client that
// answered bit 8 is sent both (365 bytes: the shell in the helm's place, the
// hat after the hands' seconds); every older client the hat in the helm's
// place (the turban drawn, never the record's fallback helm), the shell not.
// The hat comes from the character's own row first; a hidden helm shows no
// item hat; a hat alone sends the same bytes to an older client as before.
func TestAHatAndAnAuraStackOnTheHelm(t *testing.T) {
	if HatAuraStateSize != 365 || AuraHelmHat != 13 || AuraWire != 14 {
		t.Fatalf("sizes %d %d %d", HatAuraStateSize, AuraHelmHat, AuraWire)
	}
	turban := database.ItemAura{PatternID: 714388, Kind: database.AuraHat, Source: 0xAA489CBB}
	shell := database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraShell}
	w := NewWorld()
	w.SetItemAuras(map[int]database.ItemAura{714388: turban})
	p := ornatePlayer()
	p.CharServerID = 197
	p.WornMask, p.WornPieces = 0, [AuraSlots]int{}
	p.SetWornPiece(AuraHelm, 714388)

	// the hat alone: in AuraHelmHat; an older client's bytes as before
	alone := w.PieceAurasOf(p)
	if alone == nil || alone[AuraHelmHat] != turban || alone[AuraHelm] != (database.ItemAura{}) {
		t.Fatalf("the turban alone: %+v", alone)
	}
	disc := SerializePlayerUpdate(100002, p)
	var before PieceAuras
	before[AuraHelm] = turban
	if !bytes.Equal(SecondAuraChannel(disc, &p.Dyes, alone), SecondAuraChannel(disc, &p.Dyes, &before)) ||
		!bytes.Equal(AuraChannel(disc, &p.Dyes, alone), AuraChannel(disc, &p.Dyes, &before)) {
		t.Fatal("an older client's channel changed for a hat alone")
	}

	// a GM's shell on the helm: both
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraHelm] = shell
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own})
	pa := w.PieceAurasOf(p)
	if pa == nil || pa[AuraHelm] != shell || pa[AuraHelmHat] != turban {
		t.Fatalf("the turban and the shell: %+v", pa)
	}
	s := lookSession(t, 44)
	for _, set := range []func(bool){s.SetLiveLook, s.SetArmorLook, s.SetPieceAuras, s.SetBodyAura, s.SetCapeAura, s.SetHandAura, s.SetSecondAura} {
		set(true)
	}
	old := ForViewerAuras(s, disc, &p.Dyes, pa)
	helmAt := ArmorStateSize + AuraHelm*PieceAuraSize
	if len(old) != SecondAuraStateSize || !bytes.Equal(old[helmAt:helmAt+PieceAuraSize], auraBytes(turban)) {
		t.Fatalf("a 355 client: %d bytes, helm % x", len(old), old[helmAt:helmAt+PieceAuraSize])
	}
	s.SetHatAura(true)
	got := ForViewerAuras(s, disc, &p.Dyes, pa)
	if len(got) != HatAuraStateSize || !bytes.Equal(got[helmAt:helmAt+PieceAuraSize], auraBytes(shell)) ||
		!bytes.Equal(got[SecondAuraStateSize:], auraBytes(turban)) || !bytes.Equal(got, HatAuraChannel(disc, &p.Dyes, pa)) {
		t.Fatalf("a 365 client: %d bytes", len(got))
	}
	s.SetSecondAura(false)
	if s.HatAura() || len(ForViewerAuras(s, disc, &p.Dyes, pa)) != HandAuraStateSize {
		t.Fatal("bit 8 without bit 7 counts")
	}

	// a hidden helm: no item hat, and the shell (a piece's) none
	p.HiddenParts = HideHelm
	if a := w.PieceAurasOf(p); a != nil {
		t.Fatalf("a hidden turban: %+v", a)
	}
	p.HiddenParts = 0

	// the character's own hat first: over the record's
	kappa := database.ItemAura{Kind: database.AuraHat, Source: database.KappaHat}
	own[AuraHelm] = kappa
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own})
	if a := w.PieceAurasOf(p); a == nil || a[AuraHelmHat] != kappa || a[AuraHelm] != (database.ItemAura{}) {
		t.Fatalf("the character's own hat: %+v", a)
	}
	// and a shell on a worn helm with no hat: the helm's place, no hat
	w.SetItemAuras(nil)
	own[AuraHelm] = shell
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 197}: own})
	if a := w.PieceAurasOf(p); a == nil || a[AuraHelm] != shell || a[AuraHelmHat] != (database.ItemAura{}) {
		t.Fatalf("a shell, no hat: %+v", a)
	}
}
