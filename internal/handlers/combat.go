package handlers

import (
	"log"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
)

// CombatHandler handles combat-related messages.
type CombatHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

// NewCombatHandler creates a new combat handler.
func NewCombatHandler(sessionMgr *session.Manager) *CombatHandler {
	return &CombatHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference (called after world init).
func (h *CombatHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers combat handlers with the registry.
func (h *CombatHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeAttack, h.HandleAttack)
}

// HandleAttack handles a client auto-attack request (opcode 0x000F).
// C# reference: processes targetID, applies 500 damage, syncs via state channels.
// Wire format in: uint32 targetObjectID
func (h *CombatHandler) HandleAttack(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}

	reader := buffer.NewReader(data)
	targetObjectID, _ := reader.ReadUint32()

	log.Printf("Attack from %s: targetObjectID=%d", sess.RemoteAddr(), targetObjectID)

	if h.gameWorld == nil {
		return nil
	}

	h.gameWorld.ProcessAttack(sess, targetObjectID, 500)

	return nil
}
