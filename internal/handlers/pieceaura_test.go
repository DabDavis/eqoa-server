package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The worn pieces' auras (entity/pieceaura.go; claude-notes/auras-20261113.md).

// A client that answers the live look's offer with bit 3 beside bit 2 (vanilla
// a95b250 answers 15) is granted the pieces' auras: the look in force's third
// u32 is 12, and its character list is the armour's, unchanged (character
// select carries no auras). Bit 3 without bit 2, or without bit 0, grants
// nothing; an answer of bit 2 alone keeps its third u32 at 4 (the control).
func TestThePiecesAurasAreGrantedWithTheArmour(t *testing.T) {
	f := newArmorListFixture(t)
	van := f.session(51)
	f.h.SendCharacterSelectEQOA(van)
	f.answer(t, van, 1|2|liveLookArmor|liveLookAuras)
	if d := sentOp(van, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor|liveLookAuras || !van.PieceAuras() {
		t.Fatalf("the auras in force: %x", d)
	}
	if gear, whole := listGearOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.ArmorGearSize); len(gear) != 1 || !whole {
		t.Fatal("the list is not the armour's")
	}
	arm := f.session(52)
	f.h.SendCharacterSelectEQOA(arm)
	f.answer(t, arm, 1|liveLookArmor)
	if d := sentOp(arm, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor || arm.PieceAuras() {
		t.Fatalf("the armour alone: %x", d)
	}
	noArmor := f.session(53)
	f.answer(t, noArmor, 1|liveLookAuras)
	if d := sentOp(noArmor, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != 0 || noArmor.PieceAuras() {
		t.Fatalf("bit 3 without the armour: %x", d)
	}
	noLive := f.session(54)
	f.answer(t, noLive, liveLookArmor|liveLookAuras)
	if noLive.PieceAuras() || noLive.LiveLook() {
		t.Fatal("bits 2 and 3 without bit 0 granted something")
	}
	if van.BodyAura() {
		t.Fatal("the body's aura granted without bit 4")
	}
	// bit 4 too: the body's aura (305 bytes), in force's third u32 4|8|16
	body := f.session(55)
	f.answer(t, body, 1|liveLookArmor|liveLookAuras|liveLookBody)
	if d := sentOp(body, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor|liveLookAuras|liveLookBody || !body.BodyAura() {
		t.Fatalf("the body's aura in force: %x", d)
	}
	if body.CapeAura() {
		t.Fatal("the cape's aura granted without bit 5")
	}
	// bit 5 too: the cape's aura (315 bytes), in force's third u32 4|8|16|32
	cape := f.session(57)
	f.answer(t, cape, 1|liveLookArmor|liveLookAuras|liveLookBody|liveLookCape)
	if d := sentOp(cape, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor|liveLookAuras|liveLookBody|liveLookCape || !cape.CapeAura() {
		t.Fatalf("the cape's aura in force: %x", d)
	}
	noBody := f.session(58)
	f.answer(t, noBody, 1|liveLookArmor|liveLookAuras|liveLookCape)
	if d := sentOp(noBody, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor|liveLookAuras || noBody.CapeAura() {
		t.Fatalf("bit 5 without bit 4: %x", d)
	}
	noPieces := f.session(56)
	f.answer(t, noPieces, 1|liveLookArmor|liveLookBody)
	if d := sentOp(noPieces, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor || noPieces.BodyAura() {
		t.Fatalf("bit 4 without bit 3: %x", d)
	}
	// it answers again without bit 3: off
	f.answer(t, van, 1|liveLookArmor)
	if van.PieceAuras() {
		t.Fatal("bit 3 dropped: still on")
	}
}

// wornLook records each worn piece's item on its aura's piece, the robe's
// too, so an itemaura row of the item shows on that piece alone: a bracer's
// and a robe's here, the boots of no aura none. Nothing worn: no auras (the
// control), and taking the bracer off takes its aura.
func TestWornLookPutsEachPiecesAura(t *testing.T) {
	item := func(id, slot, loc int) database.CharInventoryEntry {
		return database.CharInventoryEntry{EquipLoc: loc, Pattern: database.ItemPatternRecord{PatternID: id, EquipSlot: slot, Model: 4}}
	}
	items := []database.CharInventoryEntry{item(700, 7, 3), item(701, 2, 4), item(702, 11, 5), item(703, 5, -1)}
	w := entity.NewWorld()
	w.SetItemAuras(map[int]database.ItemAura{
		700: {PatternID: 700, Color: 0xFF2020FF},
		701: {PatternID: 701, Color: 0x20FF20FF, Kind: database.AuraBoth, Nodes: 6},
		703: {PatternID: 703, Color: 0x3060FFFF}, // a chest in the pack, not worn
	})
	var p entity.Player
	wornLook(&p, items)
	a := w.PieceAurasOf(&p)
	if a == nil {
		t.Fatal("no auras")
	}
	want := entity.PieceAuras{entity.AuraBracer: {PatternID: 700, Color: 0xFF2020FF}, entity.AuraRobe: {PatternID: 701, Color: 0x20FF20FF, Kind: database.AuraBoth, Nodes: 6}}
	if *a != want {
		t.Fatalf("auras %+v, want %+v", *a, want)
	}
	wornLook(&p, items[1:])
	if a := w.PieceAurasOf(&p); a == nil || a[entity.AuraBracer] != (database.ItemAura{}) {
		t.Fatalf("the bracer off: %+v", a)
	}
	wornLook(&p, nil)
	if w.PieceAurasOf(&p) != nil || p.WornMask != 0 {
		t.Fatal("nothing worn: auras")
	}
	// a cape worn in the cape's slot (capes.go): its item's aura on the cape;
	// hidden, none; in the pack, none
	w.SetItemAuras(map[int]database.ItemAura{717001: {PatternID: 717001, Color: 0xFF2020FF, Kind: database.AuraTrim}})
	cape := database.CharInventoryEntry{EquipLoc: equipCape, Pattern: database.ItemPatternRecord{PatternID: 717001, EquipSlot: wearCape, Model: 1}}
	wornLook(&p, []database.CharInventoryEntry{cape})
	if a := w.PieceAurasOf(&p); a == nil || a[entity.AuraCape].Kind != database.AuraTrim {
		t.Fatalf("the worn cape's aura: %+v", a)
	}
	p.HiddenLocs = 1 << equipCape
	wornLook(&p, []database.CharInventoryEntry{cape})
	if w.PieceAurasOf(&p) != nil {
		t.Fatal("a hidden cape's aura shows")
	}
	p.HiddenLocs = 0
	cape.EquipLoc = -1
	wornLook(&p, []database.CharInventoryEntry{cape})
	if w.PieceAurasOf(&p) != nil {
		t.Fatal("a cape in the pack shows its aura")
	}
}
