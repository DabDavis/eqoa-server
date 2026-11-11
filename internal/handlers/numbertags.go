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
// Once the player is in the world the server offers the tags (a u8, the
// newest layout it tells); a client that colours its numbers answers
// OpcodeNumberTagsAnswer, a u8, the layout it reads (1 or 2) to have them and
// 0 to stop, and from then on each 0x00DB it is sent of its own, its pet's or
// its merc's figure carries one byte more. Layout 1 (every vanilla before
// 2026-11-11 answers 1, whatever the offer names) has a merc's figures told as
// its pet's; layout 2 gives them the merc's bit (petition 1613: "Is there a
// way that the text color for mercs damage be changed to be different than
// the chars?"). A PS2 ignores the offer, as it ignores OpcodePetEffects', and
// is sent the retail body alone.

// numberTagsLayout is the newest tags' layout the offer names:
// entity/numbertags.go's.
const numberTagsLayout = 2

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

// HandleNumberTags makes the client's answer its session's: a u8, the layout
// it reads to have its figures tagged (one past the newest is the newest), 0
// to stop (a wider answer is read by its first byte, so a u32's 1 and 0 say
// the same). Nothing is answered.
func (h *NumberTagsHandler) HandleNumberTags(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("NumberTags: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	layout := min(data[0], numberTagsLayout)
	sess.SetNumberTagsLayout(layout)
	if layout == 0 {
		log.Printf("NumberTags: %s stopped", sess.RemoteAddr())
	} else {
		log.Printf("NumberTags: %s colours its numbers (layout %d)", sess.RemoteAddr(), layout)
	}
	return nil
}
