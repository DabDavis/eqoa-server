package handlers

import (
	"encoding/binary"
	"log"
	"sync/atomic"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// More NPCs and players in view than CLIENT's 24, and farther, for a client
// that keeps them (session/entityslots.go, protocol.OpcodeEntityView).
//
// Once the player is in the world (the first 0x0014) the server offers the most
// entity slots it fills and the farthest it looks for them; a client that keeps
// more answers with its own two numbers, and the visibility system fills that
// many from that far from its next pass on. A PS2 ignores the offer and never
// answers, so it keeps the 24 within 300 it has always had.

// maxEntitySlots and maxViewDistance are the most a client may ask for
// (world.max_entity_slots, world.max_view_distance).
var maxEntitySlots, maxViewDistance atomic.Int32

func init() {
	maxEntitySlots.Store(session.MaxEntitySlots)
	maxViewDistance.Store(session.MaxViewDistance)
}

// SetMaxEntitySlots sets the most entity slots a client may have filled: 0 for
// every one the protocol has (session.MaxEntitySlots), 24 or fewer for none
// past CLIENT's.
func SetMaxEntitySlots(n int) {
	if n == 0 {
		n = session.MaxEntitySlots
	}
	maxEntitySlots.Store(int32(session.ClampEntitySlots(n, session.MaxEntitySlots)))
}

// MaxEntitySlots is the most entity slots a client may have filled.
func MaxEntitySlots() int { return int(maxEntitySlots.Load()) }

// SetMaxViewDistance sets the farthest a client may have its entities looked
// for: 0 for session.MaxViewDistance, session.ViewDistance (300) or less for
// no farther than every client's.
func SetMaxViewDistance(d int) {
	if d == 0 {
		d = session.MaxViewDistance
	}
	maxViewDistance.Store(int32(session.ClampViewDistance(d, session.MaxViewDistance)))
}

// MaxViewDistance is the farthest a client may have its entities looked for.
func MaxViewDistance() int { return int(maxViewDistance.Load()) }

// buildEntityViewOffer is OpcodeEntityView from the server: the most entity
// slots it fills and the farthest it looks for them, two u32s.
func buildEntityViewOffer(slots, view int) []byte {
	return build(protocol.OpcodeEntityView, 10, func(w *buffer.Writer) {
		w.WriteUint32(uint32(slots))
		w.WriteUint32(uint32(view))
	})
}

// SendEntityViewOffer offers the session's client the server's entity slots
// and view distance, when either is more than every client has (nothing is
// offered when both are CLIENT's: an answer would change nothing).
func SendEntityViewOffer(sess *session.Session) {
	slots, view := MaxEntitySlots(), MaxViewDistance()
	if slots <= session.RetailEntitySlots && view <= session.ViewDistance {
		return
	}
	queueReliable(sess, protocol.OpcodeEntityView, buildEntityViewOffer(slots, view))
}

// EntityViewHandler takes a client's OpcodeEntityView: how many entity slots
// it keeps and how far it draws them.
type EntityViewHandler struct{}

// NewEntityViewHandler creates the entity view handler.
func NewEntityViewHandler() *EntityViewHandler { return &EntityViewHandler{} }

// RegisterHandlers registers the entity view handler.
func (h *EntityViewHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeEntityView, h.HandleEntityView)
}

// HandleEntityView makes the client's entity slots (within RetailEntitySlots
// and MaxEntitySlots) and view distance (within session.ViewDistance and
// MaxViewDistance; a message without one leaves it) the session's: the
// visibility system fills that many from that far from its next pass on
// (entity.VisibilityState.fit), and gives up any slot past them. Nothing is
// answered.
func (h *EntityViewHandler) HandleEntityView(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	conn := sess.Connection()
	if len(data) < 4 || conn == nil {
		log.Printf("EntityView: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	asked := int(binary.LittleEndian.Uint32(data[:4]))
	conn.SetEntitySlots(session.ClampEntitySlots(asked, MaxEntitySlots()))
	view := -1
	if len(data) >= 8 {
		view = int(binary.LittleEndian.Uint32(data[4:8]))
		conn.SetViewDistance(session.ClampViewDistance(view, MaxViewDistance()))
	}
	log.Printf("EntityView: %s keeps %d entity slots within %d (asked %d within %d; the server's most %d within %d)",
		sess.RemoteAddr(), conn.EntitySlots(), conn.ViewDistance(), asked, view, MaxEntitySlots(), MaxViewDistance())
	return nil
}
