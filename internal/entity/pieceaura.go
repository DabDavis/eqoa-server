package entity

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// pieceaura.go: an aura on a worn piece (the owner, 2026-11-13: Mr. Moogie's
// aura "can be a template to make any model have particle effects ... not just
// a template for whole models, but even pieces of armor like bracers only if
// that piece has aura. or only a breastplate etc. and they can have their own
// aura color each"; claude-notes/auras-20261113.md).
//
// Table itemaura (database/itemaura.go) names an aura for an item record; a
// character wearing the item shows it on that piece. A client that draws this
// server's armour (ArmorChannel's 225 bytes) and asked for the pieces' auras
// too (bit 3 of its answer to the live look's offer, handlers/livelook.go) is
// sent, after the trims, seven pieces' auras (PieceAuraSize each, in
// AuraPieces' order): 295 bytes on every channel it has, a creature's and a
// bare player's all none. It tells the formats apart by the channel's length;
// SetEntityState moves the buffer's length, and the XOR delta runs over the
// shorter of the two (session.StateUpdateData), so a change of format is a
// change like any other. A PS2 and every other client are sent what they were.
// An NPC of an npcs row wears no item records (its armour is the row's sets):
// its aura is its name's (npctint.go, layout 3). A mercenary is dressed in
// item records (handlers/merc.go), and its pieces show theirs as a player's.

// The worn pieces an aura can be on, in the channel's order: the armour slots
// as the dyes number them (database.DyeChest ... DyeHelm), then the robe. The
// same order as the hidden parts' bits (HideChest ... HideRobe).
const (
	AuraChest = iota
	AuraBracer
	AuraGloves
	AuraLegs
	AuraBoots
	AuraHelm
	AuraRobe
	AuraPieces
)

// AuraBody is the eighth aura (database.CharAuraBody): a character's own body
// and underclothes, which every worn piece covers (charaura alone; the
// owner, 2026-11-13: "a wholebody just applies it to body and underclothes,
// armor will cover it up"). AuraSlots counts it.
const (
	AuraBody  = AuraPieces
	AuraSlots = AuraPieces + 1
)

// PieceAuraSize is one piece's aura on the channel: r, g, b, a, a u8 kind, a
// u32 source DictID little-endian, a u8 node count; all zero none.
const PieceAuraSize = 10

// AuraStateSize is the channel to a client that draws the pieces' auras: the
// live channel, the trims, then the seven pieces' auras.
const AuraStateSize = ArmorStateSize + AuraPieces*PieceAuraSize

// BodyAuraStateSize is the channel to a client that draws the body's aura
// too (session.BodyAura, the live look's answer bit 4): the seven pieces'
// auras, then the body's. A client that asked for the pieces alone is sent
// AuraStateSize, as before.
const BodyAuraStateSize = AuraStateSize + PieceAuraSize

// PieceAuras is a wearer's auras by piece, then its body's (AuraBody); the
// zero value is none.
type PieceAuras [AuraSlots]database.ItemAura

// putPieceAuras appends n of the auras (the seven pieces', and the body's
// with n AuraSlots; a nil: none).
func putPieceAuras(out []byte, a *PieceAuras, n int) []byte {
	for i := 0; i < n; i++ {
		var p database.ItemAura
		if a != nil {
			p = a[i]
		}
		out = binary.BigEndian.AppendUint32(out, p.Color)
		out = append(out, p.Kind)
		out = binary.LittleEndian.AppendUint32(out, p.Source)
		out = append(out, p.Nodes)
	}
	return out
}

// AuraChannel is ArmorChannel with the pieces' auras after it: a client that
// draws them's.
func AuraChannel(disc []byte, d *Dyes, a *PieceAuras) []byte {
	return putPieceAuras(ArmorChannel(disc, d), a, AuraPieces)
}

// BodyAuraChannel is AuraChannel with the body's aura after the pieces': a
// client that draws it's.
func BodyAuraChannel(disc []byte, d *Dyes, a *PieceAuras) []byte {
	return putPieceAuras(ArmorChannel(disc, d), a, AuraSlots)
}

// ForViewerAuras is ForViewer with the wearer's pieces' auras (nil: none),
// which go after the trims to a client that asked for them.
func ForViewerAuras(sess *session.Session, disc []byte, d *Dyes, a *PieceAuras) []byte {
	if sess != nil && sess.PieceAuras() && len(disc) == EntityStateSize {
		if sess.BodyAura() {
			return BodyAuraChannel(disc, d, a)
		}
		return AuraChannel(disc, d, a)
	}
	return ForViewer(sess, disc, d)
}

// itemAuraState is the world's itemaura rows, by patternid; swapped whole.
type itemAuraState struct {
	by atomic.Pointer[map[int]database.ItemAura]
	// chars is charaura's rows, by whose (database.CharAuraKey)
	chars atomic.Pointer[map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura]
}

// SetCharAuras sets the characters' own auras (table charaura, as
// database.CharAuras reads it): each shows at the next tick.
func (w *World) SetCharAuras(by map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura) {
	m := make(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura, len(by))
	for k, v := range by {
		m[k] = v
	}
	w.itemAuras.chars.Store(&m)
}

// charAuras is whose own auras, nil none.
func (w *World) charAuras(k database.CharAuraKey) *[database.CharAuraSlots]database.ItemAura {
	m := w.itemAuras.chars.Load()
	if m == nil || k.CharID == 0 {
		return nil
	}
	if a, ok := (*m)[k]; ok {
		return &a
	}
	return nil
}

// SetItemAuras sets the items' auras (table itemaura, as database.ItemAuras
// reads it). Every channel is serialized again each tick, so each wearer's
// pieces show the new ones at the next.
func (w *World) SetItemAuras(by map[int]database.ItemAura) {
	m := make(map[int]database.ItemAura, len(by))
	for k, v := range by {
		m[k] = v
	}
	w.itemAuras.by.Store(&m)
}

// LoadItemAuras reads table itemaura into the world: at the start and after a
// GM's edit. It reports how many items have an aura.
func (w *World) LoadItemAuras(db *database.SQLiteDB) (int, error) {
	by, err := db.ItemAuras()
	if err != nil {
		return 0, fmt.Errorf("itemaura: %w", err)
	}
	w.SetItemAuras(by)
	// and the characters' own (charaura.go), which the same edits reload
	chars, err := db.CharAuras()
	if err != nil {
		return len(by), fmt.Errorf("charaura: %w", err)
	}
	w.SetCharAuras(chars)
	return len(by), nil
}

// ItemAuraCount is how many items the world holds an aura for.
func (w *World) ItemAuraCount() int {
	if m := w.itemAuras.by.Load(); m != nil {
		return len(*m)
	}
	return 0
}

// PieceAurasOf is the auras a player's worn pieces show (nil when none
// does): each worn piece's item's (Player.WornPieces), a hidden piece's none.
func (w *World) PieceAurasOf(p *Player) *PieceAuras {
	if p == nil {
		return nil
	}
	return w.pieceAuras(&p.WornPieces, p.WornMask, p.HiddenParts, w.charAuras(database.CharAuraKey{CharID: p.CharServerID}))
}

// NPCPieceAurasOf is the auras an NPC dressed in item records shows (a
// mercenary, in its heirlooms: MercLook.WornPieces); nil for every other.
func (w *World) NPCPieceAurasOf(n *NPC) *PieceAuras {
	if n == nil {
		return nil
	}
	var own *[database.CharAuraSlots]database.ItemAura
	if k, ok := n.MercAuraKey(); ok {
		own = w.charAuras(k)
	}
	return w.pieceAuras(&n.WornPieces, n.WornMask, 0, own)
}

// MercAuraKey is a mercenary's key in charaura: its owner's character and its
// hire slot; false for every other NPC.
func (n *NPC) MercAuraKey() (database.CharAuraKey, bool) {
	if !n.isMerc() || n.merc.owner == nil || n.merc.owner.CharServerID == 0 {
		return database.CharAuraKey{}, false
	}
	return database.CharAuraKey{CharID: n.merc.owner.CharServerID, Merc: n.merc.slot}, true
}

// pieceAuras is a wearer's auras: each worn piece (not hidden) its own aura
// (own, charaura's) or else its item's (itemaura), and its body's own aura
// whatever it wears (the client hides it under the pieces).
func (w *World) pieceAuras(pieces *[AuraPieces]int, mask uint8, hidden uint16, own *[database.CharAuraSlots]database.ItemAura) *PieceAuras {
	if w == nil {
		return nil
	}
	m := w.itemAuras.by.Load()
	var out PieceAuras
	any := false
	for i := 0; i < AuraPieces; i++ {
		if mask&(1<<i) == 0 || hidden&(1<<i) != 0 {
			continue
		}
		if own != nil && own[i].Shows() {
			out[i], any = own[i], true
			continue
		}
		if m == nil {
			continue
		}
		if a, ok := (*m)[pieces[i]]; ok {
			out[i], any = a, true
		}
	}
	if own != nil && own[AuraBody].Shows() {
		out[AuraBody], any = own[AuraBody], true
	}
	if !any {
		return nil
	}
	return &out
}

// SetWornPiece records the item worn on an aura's piece (handlers' wornLook).
func (p *Player) SetWornPiece(piece, patternID int) {
	if piece < 0 || piece >= AuraPieces {
		return
	}
	p.WornPieces[piece] = patternID
	p.WornMask |= 1 << piece
}
