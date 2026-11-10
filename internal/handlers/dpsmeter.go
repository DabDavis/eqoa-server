package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A damage meter for a client that draws one (protocol.OpcodeDPSMeterOffer,
// entity/dpsmeter.go; the owner, 2026-11-10: "lets add dps meters, will work
// solo, group, raid").
//
// Once the player is in the world the server offers it (a u8, 1, the data's
// layout); a client that draws a meter answers OpcodeDPSMeterAnswer, a u8, 1 to
// be sent its group's fight and 0 to stop, and may reset its fight under way
// (OpcodeDPSMeterReset). A PS2 ignores the offer, as it ignores
// OpcodeNumberTagsOffer, and is sent nothing.

// dpsMeterLayout is the data's layout the offer names: entity/dpsmeter.go's.
const dpsMeterLayout = 1

// buildDPSMeterOffer is OpcodeDPSMeterOffer from the server: a u8, the layout.
func buildDPSMeterOffer() []byte {
	return build(protocol.OpcodeDPSMeterOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(dpsMeterLayout)
	})
}

// SendDPSMeterOffer offers the session's client a damage meter.
func SendDPSMeterOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeDPSMeterOffer, buildDPSMeterOffer())
}

// DPSMeterHandler takes a client's OpcodeDPSMeterAnswer and OpcodeDPSMeterReset.
type DPSMeterHandler struct {
	world *entity.World
}

// NewDPSMeterHandler creates the damage meter's handler.
func NewDPSMeterHandler(w *entity.World) *DPSMeterHandler { return &DPSMeterHandler{world: w} }

// RegisterHandlers registers the meter's answer and reset.
func (h *DPSMeterHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDPSMeterAnswer, h.HandleDPSMeter)
	r.Register(protocol.OpcodeDPSMeterReset, h.HandleDPSMeterReset)
}

// HandleDPSMeter makes the client's answer its player's: a u8, nonzero to be
// sent the meter, 0 to stop (a wider answer is read by its first byte, so a
// u32's 1 and 0 say the same). Nothing is answered.
func (h *DPSMeterHandler) HandleDPSMeter(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		log.Printf("DPSMeter: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	if !h.world.SetDPSMeter(sess, on) {
		log.Printf("DPSMeter: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("DPSMeter: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws a meter", false: "stopped"}[on])
	return nil
}

// HandleDPSMeterReset drops the client's fight under way (its body, a u8 1, is
// not read: the message is the ask). Nothing is answered.
func (h *DPSMeterHandler) HandleDPSMeterReset(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil || !h.world.ResetDPSMeter(sess) {
		log.Printf("DPSMeter: a reset from %s with no meter", sess.RemoteAddr())
		return nil
	}
	log.Printf("DPSMeter: %s reset its fight", sess.RemoteAddr())
	return nil
}
