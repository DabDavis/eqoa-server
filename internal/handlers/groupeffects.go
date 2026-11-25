package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The group members' effects for the tiny icons a client draws beside their bars
// (protocol.OpcodeGroupEffects, entity/groupeffects.go).
//
// Once the player is in the world the server offers them (a u32, the newest version it
// sends, entity.GroupEffectsNewest: 2); a client that draws them answers the version it
// reads (1 icons only, 2 with each icon's time left and name: petition 2962) to have them
// and 0 to stop, and from then on is sent its members' effects (OpcodeGroupEffectsData)
// whenever they change. A client from before version 2 answers 1 whatever the offer. A PS2 ignores the offer, as it ignores
// OpcodePetEffects', and is never sent any. Under world.vanilla_ui and on an [OG]
// character's session (QoLOnFor) nothing is offered, as the pet's effects are not: the
// client's QoLPetEffects bit turns its members' icons off with its pet's.

// buildGroupEffectsOffer is OpcodeGroupEffects from the server: a u32, the newest version.
func buildGroupEffectsOffer() []byte {
	return build(protocol.OpcodeGroupEffects, 6, func(w *buffer.Writer) {
		w.WriteUint32(entity.GroupEffectsNewest)
	})
}

// SendGroupEffectsOffer offers the session's client its group members' effects.
func SendGroupEffectsOffer(sess *session.Session) {
	if !QoLOnFor(sess) { // world.vanilla_ui, an [OG] character (vanillaui.go, ogaccount.go)
		return
	}
	queueReliable(sess, protocol.OpcodeGroupEffects, buildGroupEffectsOffer())
}

// GroupEffectsHandler takes a client's OpcodeGroupEffects: whether it wants its group
// members' effects.
type GroupEffectsHandler struct {
	world *entity.World
}

// NewGroupEffectsHandler creates the group effects handler.
func NewGroupEffectsHandler(w *entity.World) *GroupEffectsHandler {
	return &GroupEffectsHandler{world: w}
}

// RegisterHandlers registers the group effects handler.
func (h *GroupEffectsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeGroupEffects, h.HandleGroupEffects)
}

// HandleGroupEffects makes the client's answer its player's: a u32, the version it reads
// (1, 2; more reads as the newest) to be sent its group members' effects, 0 to stop.
// Under the QoL lock it is a 0, as version 1's was. Nothing is answered.
func (h *GroupEffectsHandler) HandleGroupEffects(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("GroupEffects: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	v := min(binary.LittleEndian.Uint32(data[:4]), entity.GroupEffectsNewest)
	if !qolAnswerFor(sess, v != 0) {
		v = 0
	}
	if !h.world.SetGroupEffectsVersion(sess, v) {
		log.Printf("GroupEffects: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	if v == 0 {
		log.Printf("GroupEffects: %s stopped", sess.RemoteAddr())
	} else {
		log.Printf("GroupEffects: %s draws them (version %d)", sess.RemoteAddr(), v)
	}
	return nil
}
