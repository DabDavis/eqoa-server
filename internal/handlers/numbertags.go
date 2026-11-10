package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What made each figure, for a client that colours its numbers by it
// (protocol.OpcodeNumberTagsOffer, entity/numbertags.go; the user, 2026-11-09:
// "we can add the number colors to extended menu").
//
// Once the player is in the world the server offers the tags (a u8, 1); a
// client that colours its numbers answers OpcodeNumberTagsAnswer, a u8, 1 to
// have them and 0 to stop, and from a 1 on each 0x00DB it is sent of its own
// or its pet's figure carries one byte more. A PS2 ignores the offer, as it
// ignores OpcodePetEffects', and is sent the retail body alone.

// numberTagsLayout is the tags' layout the offer names: entity/numbertags.go's.
const numberTagsLayout = 1

// buildNumberTagsOffer is OpcodeNumberTagsOffer from the server: a u8, the
// layout.
func buildNumberTagsOffer() []byte {
	return build(protocol.OpcodeNumberTagsOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(numberTagsLayout)
	})
}

// SendNumberTagsOffer offers the session's client its figures' tags.
func SendNumberTagsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeNumberTagsOffer, buildNumberTagsOffer())
}

// NumberTagsHandler takes a client's OpcodeNumberTagsAnswer.
type NumberTagsHandler struct{}

// NewNumberTagsHandler creates the number tags' handler.
func NewNumberTagsHandler() *NumberTagsHandler { return &NumberTagsHandler{} }

// RegisterHandlers registers the number tags' answer.
func (h *NumberTagsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeNumberTagsAnswer, h.HandleNumberTags)
}

// HandleNumberTags makes the client's answer its session's: a u8, nonzero to
// have its figures tagged, 0 to stop (a wider answer is read by its first
// byte, so a u32's 1 and 0 say the same). Nothing is answered.
func (h *NumberTagsHandler) HandleNumberTags(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("NumberTags: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	sess.SetNumberTags(on)
	log.Printf("NumberTags: %s %s", sess.RemoteAddr(), map[bool]string{true: "colours its numbers", false: "stopped"}[on])
	return nil
}
