package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hidelook.go — the worn looks a character hides (entity/hidelook.go; the
// owner, 2026-11-11: "lets add a hide display option to equipped
// armor/weapon, still equipped, but no model displayed"). A client that can
// set them is offered them at world entry with the slots hidden now
// (OpcodeHideLookOffer); it sets one with OpcodeHideLookSet and is answered
// the slots after it (OpcodeHideLookState). The choice is the character's,
// kept in table charhiddenlooks, and is in its look at once: the channel's
// next pass, the capes', and the next character list. A PS2 is never asked.

// hideLookLayout is the layout of OpcodeHideLookOffer the offer names.
const hideLookLayout = 1

// hiddenLooks is the equip slots whose look the character hides, 0 without a
// database.
func (h *CharacterHandler) hiddenLooks(serverID uint32) uint32 {
	if h == nil || h.store == nil || h.store.DB() == nil {
		return 0
	}
	locs, err := h.store.DB().CharHiddenLooks(serverID)
	if err != nil {
		log.Printf("HideLook: character %d's hidden looks: %v", serverID, err)
		return 0
	}
	return locs
}

// buildHideLookOffer is OpcodeHideLookOffer: the layout, then the hidden slots.
func buildHideLookOffer(locs uint32) []byte {
	return build(protocol.OpcodeHideLookOffer, 7, func(w *buffer.Writer) {
		w.WriteByte(hideLookLayout)
		w.WriteUint32(locs)
	})
}

// buildHideLookState is OpcodeHideLookState: the hidden slots.
func buildHideLookState(locs uint32) []byte {
	return build(protocol.OpcodeHideLookState, 6, func(w *buffer.Writer) {
		w.WriteUint32(locs)
	})
}

// SendHideLookOffer offers the session's client to hide its worn looks, with
// the slots its character hides now. A PS2 ignores it.
func (h *CharacterHandler) SendHideLookOffer(sess *session.Session) {
	var locs uint32
	if h.gameWorld != nil {
		if p := h.gameWorld.SessionPlayer(sess); p != nil {
			locs = p.HiddenLocs
		}
	}
	queueReliable(sess, protocol.OpcodeHideLookOffer, buildHideLookOffer(locs))
}

// HideLookHandler takes a client's OpcodeHideLookSet.
type HideLookHandler struct {
	chars *CharacterHandler
}

// NewHideLookHandler creates it; chars keeps the choice and dresses the
// character again.
func NewHideLookHandler(chars *CharacterHandler) *HideLookHandler {
	return &HideLookHandler{chars: chars}
}

// RegisterHandlers registers it.
func (h *HideLookHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeHideLookSet, h.HandleSet)
}

// HandleSet is OpcodeHideLookSet: a u8 equip slot, a u8 1 to hide its look, 0
// to show it (any session's: it is the character's own look, and a keep
// restart keeps no offer on the session). The choice is kept,
// the character's look made again from what it wears (so the channel's next
// pass and the capes' carry it), and the client answered the slots after it.
func (h *HideLookHandler) HandleSet(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		log.Printf("HideLook: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	c := h.chars
	if c == nil || c.gameWorld == nil {
		return nil
	}
	p := c.gameWorld.SessionPlayer(sess)
	if p == nil || p.CharServerID == 0 {
		return nil
	}
	slot, hide := int(data[0]), data[1] != 0
	if slot >= entity.HideLookSlots {
		log.Printf("HideLook: %s: no equip slot %d", p.Name, slot)
		queueReliable(sess, protocol.OpcodeHideLookState, buildHideLookState(p.HiddenLocs))
		return nil
	}
	locs := p.HiddenLocs &^ (1 << uint(slot))
	if hide {
		locs |= 1 << uint(slot)
	}
	if locs != p.HiddenLocs && c.store != nil && c.store.DB() != nil {
		if err := c.store.DB().SetCharHiddenLooks(p.CharServerID, locs); err != nil {
			log.Printf("HideLook: %s's hidden looks %#x: %v", p.Name, locs, err)
			queueReliable(sess, protocol.OpcodeHideLookState, buildHideLookState(p.HiddenLocs))
			return nil
		}
	}
	p.HiddenLocs = locs
	if c.inventoryStore != nil {
		c.applyEquipmentFromDB(p, p.CharServerID, false) // reads the choice back and dresses the look
	}
	log.Printf("HideLook: %s %s slot %d's look (hidden %#x, parts %#x)", p.Name, map[bool]string{true: "hides", false: "shows"}[hide], slot, locs, p.HiddenParts)
	queueReliable(sess, protocol.OpcodeHideLookState, buildHideLookState(locs))
	return nil
}
