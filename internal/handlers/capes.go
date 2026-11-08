package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// capes.go — the cape slot, a server's own (world.capes; entity/capes.go says why and what the
// in-world message carries): a cape's item conventions, the offer and its answer, the capes of a
// character list, and what a client that does not draw capes is sent in their place.
//
// A cape is an item of wear slot 28 (itempattern.equipslot) worn in equip slot 22
// (charinventory.equiploc), its record's model its style and its colour a tint
// (tools/seed_capes.py makes them). CLIENT knows neither number: its fit check (fitsEquip) and
// its slot names (FindWearSlotWord) end at 27, and its item list's equip slots at 21. So:
//   - a client that draws capes asks for them, answering the offer that follows a session's
//     first character list (protocol.OpcodeCapes, as the live look's); from then on the records
//     it is sent keep 28 and 22, it wears and takes off capes with the game's own messages
//     (InteractItem 0x003E, EquipItem 0x003F, UnequipItem 0x0040: two bytes hold 22 as well as
//     21), and it is told the capes of its characters at select (OpcodeCapesList) and of the
//     players in its view in the world (OpcodeCapesInView);
//   - every other client (a PS2, which ignores the offer, and every client of a server without
//     capes) is sent a cape as a plain item: wear -1, never worn (wireWear, wireEquipLoc). It is
//     never sent 0x003F or 0x0040 naming slot 22 (queueEquip, queueUnequip), its InteractItem on a
//     cape is refused (capeRefusal), and what takes a worn item off before moving it (the bank, a
//     trade, a destroy, a sale) takes a worn cape off in the database alone, as the client never
//     knew it was worn.

const (
	// wearCape is a cape's wear slot: none of CLIENT's (its fit check's jump
	// table runs -1..27).
	wearCape = 28
	// equipCape is the equip slot a cape is worn in: past CLIENT's 0-21.
	equipCape = 22
)

// capeRefusal is the line a client that does not draw capes is answered with
// when it asks to wear one (in the scribe refusals' red).
const capeRefusal = "Capes are worn only in a client that shows them."

// readsCapes is whether sess's client draws capes: it answered the offer 1,
// on a server with them.
func readsCapes(sess *session.Session) bool {
	return entity.CapesOn() && sess != nil && sess.Capes()
}

// wireWear is a record's wear slot as a client reads it: a cape's to one
// that does not draw capes is -1, CLIENT's not wearable (a plain item, on
// the item menu's Misc tab); every other as it is.
func wireWear(wear int, capes bool) int {
	if wear == wearCape && !capes {
		return -1
	}
	return wear
}

// wireEquipLoc is a record's equip slot as a client reads it: one worn in
// the cape's to a client that does not draw capes is -1, not worn.
func wireEquipLoc(loc int, capes bool) int {
	if loc == equipCape && !capes {
		return -1
	}
	return loc
}

// isCape is whether a record is a cape (wear slot 28).
func isCape(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == wearCape
}

// capeLook is the style and colour of a cape record: its model (1-255; past
// 255 it is 255, and 0 or below draws nothing) and its colour as an armour
// piece's is read (entity.ArmorColor: 0, white or a disc tint index is none,
// the art's own).
func capeLook(p *database.ItemPatternRecord) (uint8, uint32) {
	style := max(0, min(255, int(p.Model)))
	return uint8(style), entity.ArmorColor(p.Color)
}

// queueEquip queues EquipItem (0x003F) for the record at list index i into
// equip slot slot, unless the slot is the cape's and the client does not draw
// capes: such a client was sent the cape as an item it cannot wear, and CLIENT
// knows no slot 22.
func queueEquip(sess *session.Session, i, slot int) {
	if slot == equipCape && !readsCapes(sess) {
		return
	}
	sess.Queue().AddWithSequence(equipItemMessage(i, slot))
}

// queueUnequip queues UnequipItem (0x0040) for equip slot slot, as queueEquip.
func queueUnequip(sess *session.Session, slot int) {
	if slot == equipCape && !readsCapes(sess) {
		return
	}
	sess.Queue().AddWithSequence(unequipItemMessage(slot))
}

// buildCapesOffer is OpcodeCapes from the server: a u32, 1.
func buildCapesOffer() []byte {
	return build(protocol.OpcodeCapes, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendCapesOffer offers the session's client capes, once a session, on a
// server with them (after its first character list, as the live look's).
func SendCapesOffer(sess *session.Session) {
	if !entity.CapesOn() {
		return
	}
	if sess.OfferCapesOnce() {
		queueReliable(sess, protocol.OpcodeCapes, buildCapesOffer())
	}
}

// listCape is one entry of OpcodeCapesList: a character and its cape.
type listCape struct {
	serverID uint32
	style    uint8
	color    uint32
}

// characterCape is the cape a character wears among its items (wornLook's:
// the record worn in the cape's slot), none when its style is 0.
func characterCape(serverID uint32, items []database.CharInventoryEntry) (listCape, bool) {
	var p entity.Player
	wornLook(&p, items)
	return listCape{serverID, p.CapeStyle, p.CapeColor}, p.CapeStyle != 0
}

// encodeCapesList is OpcodeCapesList's body: a u8 count, then each entry's
// u32 ServerID, u8 style and u32 colour, little-endian (the first 255).
func encodeCapesList(capes []listCape) []byte {
	if len(capes) > 255 {
		capes = capes[:255]
	}
	out := make([]byte, 1, 1+9*len(capes))
	out[0] = byte(len(capes))
	for _, c := range capes {
		out = binary.LittleEndian.AppendUint32(out, c.serverID)
		out = append(out, c.style)
		out = binary.LittleEndian.AppendUint32(out, c.color)
	}
	return out
}

// sendCapesList sends a client that draws capes the capes of a list's
// characters; nothing to any other.
func sendCapesList(sess *session.Session, capes []listCape) {
	if !readsCapes(sess) {
		return
	}
	body := encodeCapesList(capes)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCapesList)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeCapesList)), body...))
	sess.Queue().AddWithSequence(msg)
}

// capesOfAccount is the capes the characters of sess's account wear, read
// as its character list reads their items.
func (h *CharacterHandler) capesOfAccount(sess *session.Session) []listCape {
	if h.store == nil || h.inventoryStore == nil || sess.AccountID() <= 0 {
		return nil
	}
	records, err := h.store.GetByAccountID(sess.AccountID())
	if err != nil {
		log.Printf("Capes: account %d's characters: %v", sess.AccountID(), err)
		return nil
	}
	var out []listCape
	for _, c := range records {
		items, err := h.inventoryStore.GetCharacterItems(uint32(c.ServerID))
		if err != nil {
			log.Printf("Capes: %s's items: %v", c.CharName, err)
			continue
		}
		if lc, ok := characterCape(uint32(c.ServerID), items); ok {
			out = append(out, lc)
		}
	}
	return out
}

// CapesHandler takes a client's answer to the cape offer.
type CapesHandler struct {
	chars *CharacterHandler
}

// NewCapesHandler creates the capes' handler; chars reads a list's capes.
func NewCapesHandler(chars *CharacterHandler) *CapesHandler {
	return &CapesHandler{chars: chars}
}

// RegisterHandlers registers it.
func (h *CapesHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeCapes, h.HandleCapes)
}

// HandleCapes takes the client's answer: a u32, nonzero to draw capes, 0 to
// stop. A 1 is taken only on a server with capes and a session they were
// offered on (a client cannot ask for what was never offered). A client still
// choosing a character is sent its characters' capes at once; in the world the
// next pass sends its view's (entity.VisibilityState.ResendCapes).
func (h *CapesHandler) HandleCapes(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("Capes: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if on && (!entity.CapesOn() || !sess.CapesOffered()) {
		log.Printf("Capes: %s asked for capes, which were not offered (world.capes %v)", sess.RemoteAddr(), entity.CapesOn())
		return nil
	}
	sess.SetCapes(on)
	log.Printf("Capes: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws capes", false: "stops drawing capes"}[on])
	if !on {
		return nil
	}
	entity.GetOrCreateVisibility(sess).ResendCapes()
	if h.chars != nil && sess.State() == session.StateCharacterSelect && !sess.CharacterInWorld() {
		sendCapesList(sess, h.chars.capesOfAccount(sess))
	}
	return nil
}
