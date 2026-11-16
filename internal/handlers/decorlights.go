package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Strings of decoration lights, for a client that draws them
// (protocol.OpcodeDecorLightsOffer, entity/decorlights.go, tables
// lighttemplates and lightstrings; the owner, 2026-11-16: the Christmas tree's
// lights "in different colors").
//
// Once the player is in the world the server offers them (a u8, the layout,
// 1); a client that draws them answers OpcodeDecorLightsAnswer, a u8, the
// layout it reads (0 to stop), and is sent every string of a season that is on
// (OpcodeDecorLights) at once, and again whenever they change: a season turned,
// a GM's edit. A PS2 ignores the offer and is sent none: it sees the tree and
// no lights.

// buildDecorLightsOffer is OpcodeDecorLightsOffer from the server: a u8, the layout.
func buildDecorLightsOffer() []byte {
	return build(protocol.OpcodeDecorLightsOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(entity.DecorLightsLayout)
	})
}

// SendDecorLightsOffer offers the session's client the decoration lights.
func SendDecorLightsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeDecorLightsOffer, buildDecorLightsOffer())
}

// DecorLightsHandler takes a client's OpcodeDecorLightsAnswer.
type DecorLightsHandler struct {
	world *entity.World
}

// NewDecorLightsHandler creates the decoration lights' handler.
func NewDecorLightsHandler(w *entity.World) *DecorLightsHandler { return &DecorLightsHandler{world: w} }

// RegisterHandlers registers the decoration lights' answer.
func (h *DecorLightsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDecorLightsAnswer, h.HandleDecorLights)
}

// HandleDecorLights makes the client's answer its session's: a u8, the layout
// it reads (capped at the server's), 0 to stop. On, the lights go at once.
func (h *DecorLightsHandler) HandleDecorLights(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("DecorLights: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	layout := min(data[0], entity.DecorLightsLayout)
	sess.SetDecorLights(layout)
	log.Printf("DecorLights: %s %s (layout %d)", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[layout != 0], layout)
	if layout != 0 && h.world != nil {
		h.world.RefreshDecorLights(sess)
	}
	return nil
}
