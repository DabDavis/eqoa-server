package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// statfigures.go — the real offence and defence figures for a client's own stat
// window (petition 2925; the rules are the World's, entity/statfigures.go): the
// offer and the client's answer (protocol.OpcodeStatFigures). A PS2 ignores the
// offer.

// statFiguresLayout is OpcodeStatFiguresData's layout, the one the offer names.
const statFiguresLayout = 1

// buildStatFiguresOffer is OpcodeStatFigures from the server: a u8, the layout.
func buildStatFiguresOffer() []byte {
	return build(protocol.OpcodeStatFigures, 3, func(w *buffer.Writer) { w.WriteByte(statFiguresLayout) })
}

// SendStatFiguresOffer offers the session's client the figures, except where
// Character > Summary's stat caps are the game's (QoLStatCaps: world.vanilla_ui,
// an [OG] character's QoLOGOff), whose clients turn their stat window off.
func SendStatFiguresOffer(sess *session.Session) {
	if vanillaUIBits(sess)&QoLStatCaps != 0 {
		return
	}
	queueReliable(sess, protocol.OpcodeStatFigures, buildStatFiguresOffer())
}

// StatFiguresHandler takes a client's answer.
type StatFiguresHandler struct {
	world *entity.World
}

// NewStatFiguresHandler creates the figures' handler.
func NewStatFiguresHandler(w *entity.World) *StatFiguresHandler { return &StatFiguresHandler{world: w} }

// RegisterHandlers registers the answer.
func (h *StatFiguresHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeStatFigures, h.HandleStatFigures)
}

// HandleStatFigures makes the client's answer its player's: a u8, nonzero to be
// sent the figures (its stat window open), 0 to stop. Where the offer is not
// made (QoLStatCaps) an answer is taken as 0. Nothing is answered.
func (h *StatFiguresHandler) HandleStatFigures(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		log.Printf("StatFigures: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0 && vanillaUIBits(sess)&QoLStatCaps == 0
	if !h.world.SetStatFigures(sess, on) {
		log.Printf("StatFigures: %s has no player in the world", sess.RemoteAddr())
	}
	return nil
}
