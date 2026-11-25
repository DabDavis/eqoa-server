package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The player's harmful effects on creatures, for the icons a client draws under their name
// plates (protocol.OpcodeNPCDebuffs, entity/npcdebuffs.go; Heirloom petition 2997).
//
// Once the player is in the world the server offers them (a u32, 1); a client that draws
// them answers 1 to have them and 0 to stop, and from then on is sent its effects on the
// creatures it draws (OpcodeNPCDebuffsData) whenever they change. A PS2 ignores the offer,
// as it ignores OpcodeGroupEffects', and is never sent any. Under world.vanilla_ui and on
// an [OG] character's session (QoLOnFor) nothing is offered and an answer is taken as 0.

// buildNPCDebuffsOffer is OpcodeNPCDebuffs from the server: a u32, 1.
func buildNPCDebuffsOffer() []byte {
	return build(protocol.OpcodeNPCDebuffs, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendNPCDebuffsOffer offers the session's client its effects on the creatures it draws.
func SendNPCDebuffsOffer(sess *session.Session) {
	if !QoLOnFor(sess) { // world.vanilla_ui, an [OG] character (vanillaui.go, ogaccount.go)
		return
	}
	queueReliable(sess, protocol.OpcodeNPCDebuffs, buildNPCDebuffsOffer())
}

// NPCDebuffsHandler takes a client's OpcodeNPCDebuffs: whether it wants its effects on
// creatures.
type NPCDebuffsHandler struct {
	world *entity.World
}

// NewNPCDebuffsHandler creates the creatures' effects handler.
func NewNPCDebuffsHandler(w *entity.World) *NPCDebuffsHandler {
	return &NPCDebuffsHandler{world: w}
}

// RegisterHandlers registers the creatures' effects handler.
func (h *NPCDebuffsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeNPCDebuffs, h.HandleNPCDebuffs)
}

// HandleNPCDebuffs makes the client's answer its player's: a u32, nonzero to be sent its
// effects on the creatures it draws, 0 to stop. Nothing is answered.
func (h *NPCDebuffsHandler) HandleNPCDebuffs(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("NPCDebuffs: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := qolAnswerFor(sess, binary.LittleEndian.Uint32(data[:4]) != 0)
	if !h.world.SetNPCDebuffs(sess, on) {
		log.Printf("NPCDebuffs: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("NPCDebuffs: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[on])
	return nil
}
