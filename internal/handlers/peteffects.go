package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The pet's effects for the row a client draws under its pet's bar
// (protocol.OpcodePetEffects, entity/peteffects.go).
//
// Once the player is in the world the server offers them (a u32, 1); a client
// that draws them answers 1 to have them and 0 to stop, and from then on is
// sent its pet's effects (OpcodePetEffectsData) whenever they change. A PS2
// ignores the offer, as it ignores OpcodePowerBars', and is never sent any:
// its effects window is the player's own effects alone.

// buildPetEffectsOffer is OpcodePetEffects from the server: a u32, 1.
func buildPetEffectsOffer() []byte {
	return build(protocol.OpcodePetEffects, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendPetEffectsOffer offers the session's client its pet's effects.
func SendPetEffectsOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodePetEffects, buildPetEffectsOffer())
}

// PetEffectsHandler takes a client's OpcodePetEffects: whether it wants its
// pet's effects.
type PetEffectsHandler struct {
	world *entity.World
}

// NewPetEffectsHandler creates the pet effects handler.
func NewPetEffectsHandler(w *entity.World) *PetEffectsHandler { return &PetEffectsHandler{world: w} }

// RegisterHandlers registers the pet effects handler.
func (h *PetEffectsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodePetEffects, h.HandlePetEffects)
}

// HandlePetEffects makes the client's answer its player's: a u32, nonzero to
// be sent its pet's effects, 0 to stop. Nothing is answered.
func (h *PetEffectsHandler) HandlePetEffects(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("PetEffects: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetPetEffects(sess, on) {
		log.Printf("PetEffects: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("PetEffects: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[on])
	return nil
}
