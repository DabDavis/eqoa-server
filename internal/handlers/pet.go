package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// PetHandler takes the pet commands (entity/pet.go).
type PetHandler struct {
	gameWorld *entity.World
}

// NewPetHandler makes the pet commands' handler.
func NewPetHandler(gw *entity.World) *PetHandler {
	return &PetHandler{gameWorld: gw}
}

// RegisterHandlers registers 0x0041.
func (h *PetHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodePetCommands, h.HandlePetCommand)
}

// HandlePetCommand is 0x0041 (CLIENT's PetCommand, the start menu's Pet
// entries and the quick chat's |24-|30): a u8 command, and after an attack's
// (8) the u32 target as the client has it. CLIENT sends one only with a pet.
func (h *PetHandler) HandlePetCommand(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	r := newClientReader(msg)
	cmd := r.u8()
	var target uint32
	if cmd == 8 {
		target = r.u32()
	}
	if r.err {
		log.Printf("Pet: a 0x0041 of %d bytes from %s", len(msg.Data()), sess.RemoteAddr())
		return nil
	}
	log.Printf("Pet: command %d (target %#x) from %s", cmd, target, sess.RemoteAddr())
	h.gameWorld.PetCommand(sess, cmd, target, time.Now())
	return nil
}
