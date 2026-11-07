package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A race of racedefs by its own words, for a client that can show them
// (protocol.OpcodeRaceLabels, entity/racelabels.go).
//
// Once the player is in the world the server offers them (a u32, 1); a client that
// can answers 1 to have them and 0 to stop, and from then on is sent who reads
// differently (OpcodeRaceLabelsData), at once and whenever that changes. A PS2
// ignores the offer, as it ignores OpcodePowerBars', and is never sent any.

// buildRaceLabelsOffer is OpcodeRaceLabels from the server: a u32, 1.
func buildRaceLabelsOffer() []byte {
	return build(protocol.OpcodeRaceLabels, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendRaceLabelsOffer offers the session's client the race labels.
func SendRaceLabelsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeRaceLabels, buildRaceLabelsOffer())
}

// RaceLabelsHandler takes a client's OpcodeRaceLabels, and hears the logins that
// may change the list.
type RaceLabelsHandler struct {
	world *entity.World
	db    *database.SQLiteDB
}

// NewRaceLabelsHandler creates the race labels handler.
func NewRaceLabelsHandler(w *entity.World, db *database.SQLiteDB) *RaceLabelsHandler {
	return &RaceLabelsHandler{world: w, db: db}
}

// RegisterHandlers registers the handler, and has a login of a character of such a
// race tell the clients already in the world (its row may be newer than their
// lists; an ordinary character's login asks nothing of the database).
func (h *RaceLabelsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeRaceLabels, h.HandleRaceLabels)
	if h.world != nil {
		h.world.OnPlayerEntered(func(_ *session.Session, p *entity.Player) {
			if entity.HasRaceLabel(p.Race) {
				h.world.RefreshRaceLabels(h.db)
			}
		})
	}
}

// HandleRaceLabels makes the client's answer its player's: a u32, nonzero to be
// sent the list, 0 to stop. On, the list goes out at once, unless it is empty,
// which is what the client holds. Nothing is answered.
func (h *RaceLabelsHandler) HandleRaceLabels(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("RaceLabels: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetRaceLabels(sess, on) {
		log.Printf("RaceLabels: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	if on {
		h.world.RefreshRaceLabels(h.db)
	}
	log.Printf("RaceLabels: %s %s", sess.RemoteAddr(), map[bool]string{true: "shows them", false: "stopped"}[on])
	return nil
}
