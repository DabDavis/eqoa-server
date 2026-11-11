package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mercui.go — the mercs as vanilla shows them (the GM Jibrish's petitions,
// 2026-11-11): 1475, "Mercenary's need a way for players to dismiss them. Put
// the option under the group options in main menu", and 1488, "Change
// mercenary name colors to match that of pets, with a slight reddish tint".
//
// Where world.mercs is on, the server offers (protocol.OpcodeMercsOffer, a
// u8 layout, 1) once the player is in the world; a client that answers 1 is
// told the mercs in its world by name and whether its own is out
// (OpcodeMercsData, entity/uitold.go: at once and whenever it changes). A
// merc's entity channel says pet as a pet's does (byte 142, 3), so its name
// is drawn in the pet's colour on every client; the names let vanilla tint a
// merc's red. Its Community > Group > Dismiss Mercenary, shown while its own
// is out, sends OpcodeMercDismiss, which is /merc dismiss. A PS2 ignores the
// offer and keeps /merc dismiss (a PS2's "/t merc dismiss").

// mercsLayout is OpcodeMercsData's layout the offer names.
const mercsLayout = 1

// SendMercsOffer offers the session's client the mercs' names, where
// world.mercs is on; elsewhere nothing goes.
func SendMercsOffer(sess *session.Session) {
	if !MercsOn() {
		return
	}
	queueReliable(sess, protocol.OpcodeMercsOffer, build(protocol.OpcodeMercsOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(mercsLayout)
	}))
}

// MercUIHandler takes the client's answer to the offer and its Dismiss
// Mercenary.
type MercUIHandler struct {
	world *entity.World
	mercs *Mercs
}

// NewMercUIHandler is the handler over the World and the mercs' handler.
func NewMercUIHandler(w *entity.World, m *Mercs) *MercUIHandler {
	return &MercUIHandler{world: w, mercs: m}
}

// RegisterHandlers registers the answer and the dismissal.
func (h *MercUIHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeMercsOffer, h.HandleAnswer)
	r.Register(protocol.OpcodeMercDismiss, h.HandleDismiss)
}

// HandleAnswer is the client's answer: a u8, 1 to be told, 0 to stop.
func (h *MercUIHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("Mercs: a short answer from %s", sess.RemoteAddr())
		return nil
	}
	sess.SetMercsTold(data[0] != 0)
	return nil
}

// HandleDismiss is Group > Dismiss Mercenary: /merc dismiss.
func (h *MercUIHandler) HandleDismiss(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil || h.mercs == nil {
		return nil
	}
	p := h.world.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	if !MercsOn() {
		SendErrorMessage(sess, mercOffLine)
		return nil
	}
	h.mercs.dismiss(sess, p)
	return nil
}
