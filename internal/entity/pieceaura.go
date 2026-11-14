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
// armor will cover it up"). AuraCape is the ninth (database.CharAuraCape),
// the worn cape's (capes.go's slot; the owner, 2026-11-14: "also cannot
// apply aura to capes yet"): after the body's, so every index before it
// stays. AuraMainHand and AuraOffHand are the tenth and eleventh
// (database.CharAuraMainHand, CharAuraOffHand): the item held in each hand
// (the off hand's on a shield held there too), charaura alone, of kind
// particles or epic (database.AuraEpic: an epic weapon's own glow, sheen and
// particles on whatever the hand holds; the owner, 2026-11-14: "give zaeires
// equipped weapons the necro's tainted aura from the epic sickle"). AuraSlots
// counts them.
const (
	AuraBody     = AuraPieces
	AuraCape     = AuraPieces + 1
	AuraMainHand = AuraPieces + 2
	AuraOffHand  = AuraPieces + 3
	AuraSlots    = AuraPieces + 4
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

// CapeAuraStateSize is the channel to a client that draws the cape's aura
// too (session.CapeAura, the live look's answer bit 5): the body's channel,
// then the cape's aura. A client that asked for the body's alone is sent
// BodyAuraStateSize, as before.
const CapeAuraStateSize = BodyAuraStateSize + PieceAuraSize

// HandAuraStateSize is the channel to a client that draws the held items'
// auras too (session.HandAura, the live look's answer bit 6): the cape's
// channel, then the main hand's aura and the off hand's (335 bytes). A
// client that asked for the cape's alone is sent CapeAuraStateSize, as
// before.
const HandAuraStateSize = CapeAuraStateSize + 2*PieceAuraSize

// PieceAuras is a wearer's auras by piece, then its body's (AuraBody), its
// cape's (AuraCape) and its hands' (AuraMainHand, AuraOffHand); the zero
// value is none.
type PieceAuras [AuraSlots]database.ItemAura

// putPieceAuras appends n of the auras (the seven pieces', the body's with n
// AuraBody+1, the cape's with n AuraCape+1, the hands' with n AuraSlots; a
// nil: none).
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
	return putPieceAuras(ArmorChannel(disc, d), a, AuraBody+1)
}

// CapeAuraChannel is BodyAuraChannel with the cape's aura after the body's:
// a client that draws it's.
func CapeAuraChannel(disc []byte, d *Dyes, a *PieceAuras) []byte {
	return putPieceAuras(ArmorChannel(disc, d), a, AuraCape+1)
}

// HandAuraChannel is CapeAuraChannel with the hands' auras after the cape's
// (the main hand's, then the off hand's): a client that draws them's.
func HandAuraChannel(disc []byte, d *Dyes, a *PieceAuras) []byte {
	return putPieceAuras(ArmorChannel(disc, d), a, AuraSlots)
}

// ForViewerAuras is ForViewer with the wearer's pieces' auras (nil: none),
// which go after the trims to a client that asked for them.
func ForViewerAuras(sess *session.Session, disc []byte, d *Dyes, a *PieceAuras) []byte {
	if sess != nil && sess.PieceAuras() && len(disc) == EntityStateSize {
		if sess.HandAura() {
			return HandAuraChannel(disc, d, a)
		}
		if sess.CapeAura() {
			return CapeAuraChannel(disc, d, a)
		}
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
// (own, charaura's) or else its item's (itemaura), the worn cape (AuraCape,
// not hidden: HideCape) the same, its body's own aura whatever it wears
// (the client hides it under the pieces), and its hands' own (charaura
// alone) whatever they hold: the client draws a hand's on the item held
// there, nothing on an empty hand (or one whose look is hidden, which the
// channel sends empty).
func (w *World) pieceAuras(pieces *[AuraSlots]int, mask uint16, hidden uint16, own *[database.CharAuraSlots]database.ItemAura) *PieceAuras {
	if w == nil {
		return nil
	}
	m := w.itemAuras.by.Load()
	var out PieceAuras
	any := false
	for i := 0; i < AuraSlots; i++ {
		if ownOnly(i) || mask&(1<<i) == 0 || hidden&auraHides(i) != 0 {
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
			if a.Kind == database.AuraCirclet && i != AuraHelm || a.Kind == database.AuraEpic || a.Kind == database.AuraEnergy {
				continue // a circlet is a helm's alone, epic and energy a held item's (database.ValidItemAura)
			}
			out[i], any = a, true
		}
	}
	for _, i := range [...]int{AuraBody, AuraMainHand, AuraOffHand} {
		if own != nil && own[i].Shows() {
			out[i], any = own[i], true
			continue
		}
		// a held item's own aura (itemaura on its record: the Heirloom Shield
		// of Twisted Virtue's energy skin), of a hand's kinds alone
		if i != AuraBody && m != nil && mask&(1<<i) != 0 {
			if a, ok := (*m)[pieces[i]]; ok && database.HandAuraKind(int64(a.Kind)) {
				out[i], any = a, true
			}
		}
	}
	// a circlet on the head (database.AuraCirclet) whatever is worn there or
	// hidden: the client draws it while no helm is drawn
	if own != nil && own[AuraHelm].Kind == database.AuraCirclet {
		out[AuraHelm], any = own[AuraHelm], true
	}
	if !any {
		return nil
	}
	return &out
}

// ownOnly reports whether aura slot i is a character's own alone (charaura),
// whatever it wears: the body's and the hands'.
func ownOnly(i int) bool {
	return i == AuraBody || i == AuraMainHand || i == AuraOffHand
}

// auraHides is the hidden part (Player.HiddenParts) that hides piece i's
// aura: the pieces' bits are in their order (HideChest ... HideRobe), the
// cape's is HideCape.
func auraHides(i int) uint16 {
	if i == AuraCape {
		return HideCape
	}
	return 1 << i
}

// SetWornPiece records the item worn on an aura's piece (handlers' wornLook),
// the cape's (AuraCape) and the held items' (AuraMainHand, AuraOffHand: a held
// item's own aura) too; the body (AuraBody) is no piece.
func (p *Player) SetWornPiece(piece, patternID int) {
	if piece < 0 || piece >= AuraSlots || piece == AuraBody {
		return
	}
	p.WornPieces[piece] = patternID
	p.WornMask |= 1 << piece
}
