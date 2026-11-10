package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A tint on the bodies of the NPCs of a name, for a client that draws one
// (protocol.OpcodeNPCTintsOffer, entity/npctint.go, table npctint).
//
// Once the player is in the world the server offers the tints (a u8, 1, the
// layout); a client that draws them answers OpcodeNPCTintsAnswer, a u8, 1 to
// have them (it drops what it held) and 0 to stop, and from a 1 on is sent
// them (OpcodeNPCTints) at once when there are any, and again whenever a GM
// changes them. A PS2 ignores the offer, as it ignores OpcodeNumberTagsOffer's,
// is sent none and draws every NPC in its body's own colours.

// npcTintsLayout is the layout of OpcodeNPCTints the offer names:
// entity.EncodeNPCTints's.
const npcTintsLayout = 1

// buildNPCTintsOffer is OpcodeNPCTintsOffer from the server: a u8, the layout.
func buildNPCTintsOffer() []byte {
	return build(protocol.OpcodeNPCTintsOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(npcTintsLayout)
	})
}

// SendNPCTintsOffer offers the session's client the NPCs' body tints.
func SendNPCTintsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeNPCTintsOffer, buildNPCTintsOffer())
}

// NPCTintsHandler takes a client's OpcodeNPCTintsAnswer.
type NPCTintsHandler struct {
	world *entity.World
}

// NewNPCTintsHandler creates the NPC tints' handler.
func NewNPCTintsHandler(w *entity.World) *NPCTintsHandler { return &NPCTintsHandler{world: w} }

// RegisterHandlers registers the NPC tints' answer.
func (h *NPCTintsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeNPCTintsAnswer, h.HandleNPCTints)
}

// HandleNPCTints makes the client's answer its session's: a u8, nonzero to be
// sent the tints, 0 to stop (a wider answer is read by its first byte, so a
// u32's 1 and 0 say the same). On, the tints there are go at once.
func (h *NPCTintsHandler) HandleNPCTints(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("NPCTints: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	sess.SetNPCTints(on)
	log.Printf("NPCTints: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[on])
	if on && h.world != nil {
		h.world.RefreshNPCTints(sess)
	}
	return nil
}
