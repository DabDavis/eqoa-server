package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// buffwindow.go — the buff window's messages (the GM Jibrish's petition 1761; the rules are
// the World's, entity/buffwindow.go): the offer and the client's answer
// (protocol.OpcodeBuffWindow) and a removal (OpcodeBuffRemove). A PS2 ignores the offer.

// buildBuffWindowOffer is OpcodeBuffWindow from the server: a u8, 1.
func buildBuffWindowOffer() []byte {
	return build(protocol.OpcodeBuffWindow, 3, func(w *buffer.Writer) { w.WriteByte(1) })
}

// SendBuffWindowOffer offers the session's client the buff window's set.
func SendBuffWindowOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeBuffWindow, buildBuffWindowOffer())
}

// BuffWindowHandler takes a client's answer and its removals.
type BuffWindowHandler struct {
	world *entity.World
}

// NewBuffWindowHandler creates the buff window handler.
func NewBuffWindowHandler(w *entity.World) *BuffWindowHandler { return &BuffWindowHandler{world: w} }

// RegisterHandlers registers the buff window's messages.
func (h *BuffWindowHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeBuffWindow, h.HandleBuffWindow)
	r.Register(protocol.OpcodeBuffRemove, h.HandleBuffRemove)
}

// HandleBuffWindow makes the client's answer its player's: a u8, nonzero to be sent the
// set, 0 to stop. Nothing is answered.
func (h *BuffWindowHandler) HandleBuffWindow(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		log.Printf("BuffWindow: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	if !h.world.SetBuffWindow(sess, on) {
		log.Printf("BuffWindow: %s has no player in the world", sess.RemoteAddr())
	}
	return nil
}

// HandleBuffRemove is a removal: u8 whose, u32 id, u32 icon.
func (h *BuffWindowHandler) HandleBuffRemove(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 9 || h.world == nil {
		log.Printf("BuffRemove: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	h.world.RemoveBuff(sess, data[0], binary.LittleEndian.Uint32(data[1:5]), binary.LittleEndian.Uint32(data[5:9]))
	return nil
}
