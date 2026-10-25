package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Others' power for the bars a client draws under their health
// (protocol.OpcodePowerBars, entity/powerbars.go).
//
// Once the player is in the world the server offers it (a u32, 1); a client
// that draws the bars answers 1 to have them and 0 to stop, and from then on
// is sent the power of its group's members, its target and its target's
// target (OpcodePowerBarsData) whenever it changes. A PS2 ignores the offer,
// as it ignores OpcodeEntityView's, and is never sent any.

// buildPowerBarsOffer is OpcodePowerBars from the server: a u32, 1.
func buildPowerBarsOffer() []byte {
	return build(protocol.OpcodePowerBars, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendPowerBarsOffer offers the session's client others' power.
func SendPowerBarsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodePowerBars, buildPowerBarsOffer())
}

// PowerBarsHandler takes a client's OpcodePowerBars: whether it wants others'
// power.
type PowerBarsHandler struct {
	world *entity.World
}

// NewPowerBarsHandler creates the power bars handler.
func NewPowerBarsHandler(w *entity.World) *PowerBarsHandler { return &PowerBarsHandler{world: w} }

// RegisterHandlers registers the power bars handler.
func (h *PowerBarsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodePowerBars, h.HandlePowerBars)
}

// HandlePowerBars makes the client's answer its player's: a u32, nonzero to
// be sent others' power, 0 to stop. Nothing is answered.
func (h *PowerBarsHandler) HandlePowerBars(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("PowerBars: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetPowerBars(sess, on) {
		log.Printf("PowerBars: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("PowerBars: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[on])
	return nil
}
