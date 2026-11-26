package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// One faction's standing on the HUD (protocol.OpcodeFactionTrackOffer,
// entity/factiontrack.go; Heirloom petition 3216, Cucumber: "track one chosen
// faction on screen").
//
// Once the player is in the world the server offers it (a u8, the data's
// layout); a client that draws a tracker answers OpcodeFactionTrackAnswer, the
// faction's name (wide), or nothing to stop, and is sent its standing at once
// and as it changes. A PS2 ignores the offer and is sent nothing; an [OG]
// character and world.vanilla_ui are offered nothing, and an answer from them
// is taken as "none" (vanillaui.go).

// buildFactionTrackOffer is OpcodeFactionTrackOffer from the server: a u8, the
// layout.
func buildFactionTrackOffer() []byte {
	return build(protocol.OpcodeFactionTrackOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(entity.FactionTrackLayout)
	})
}

// SendFactionTrackOffer offers the session's client a faction on its HUD.
func SendFactionTrackOffer(sess *session.Session) {
	if !QoLOnFor(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeFactionTrackOffer, buildFactionTrackOffer())
}

// FactionTrackHandler takes a client's OpcodeFactionTrackAnswer.
type FactionTrackHandler struct {
	world *entity.World
}

// NewFactionTrackHandler creates the faction tracker's handler.
func NewFactionTrackHandler(w *entity.World) *FactionTrackHandler {
	return &FactionTrackHandler{world: w}
}

// RegisterHandlers registers the tracker's answer.
func (h *FactionTrackHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeFactionTrackAnswer, h.HandleFactionTrack)
}

// HandleFactionTrack makes the client's answer its player's: a wide string,
// the faction to follow ("" none). Its standing goes at the tracker's next pass
// (entity.factionTrackSystem), nothing is answered here.
func (h *FactionTrackHandler) HandleFactionTrack(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil {
		return nil
	}
	name, ok := readWide(buffer.NewReader(msg.Data()))
	if !ok {
		log.Printf("FactionTrack: unreadable answer (%d bytes) from %s", len(msg.Data()), sess.RemoteAddr())
		return nil
	}
	if !qolAnswerFor(sess, name != "") {
		name = ""
	}
	got, in := h.world.SetFactionTrack(sess, name)
	switch {
	case !in:
		log.Printf("FactionTrack: %s has no player in the world", sess.RemoteAddr())
	case name == "":
		log.Printf("FactionTrack: %s follows no faction", sess.RemoteAddr())
	case got == "":
		log.Printf("FactionTrack: %s asked for %q, no one faction's name", sess.RemoteAddr(), name)
	default:
		log.Printf("FactionTrack: %s follows %q", sess.RemoteAddr(), got)
	}
	return nil
}
