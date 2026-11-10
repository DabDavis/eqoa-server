package handlers

// jump.go — a player's jump (entity/jump.go; the user, 2026-11-09: "lets add
// jump to the game"; world.jump). The client jumps (vanilla's walker.rs); the
// server offers it with protocol.OpcodeJump and takes a jump's arc from a
// player whose client answered 1 (the movement check, entity/movecheck.go).
//
// CLIENT's walk has no jump, so a PS2 is never sent the offer, not even to be
// ignored: it goes to a client that answered the bug report offer
// (bugreport.go), which every vanilla build since 2026-11-05 does at its world
// entry and a PS2 never does. A client that does not jump never answers it, and
// its rises in the air are judged as anyone's.

import (
	"encoding/binary"
	"log"
	"sync/atomic"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

var jumpsOn atomic.Bool

// SetJumps is world.jump: whether a client is offered the jump (the World's
// own, entity.World.SetJumps, is what the movement check reads).
func SetJumps(on bool) { jumpsOn.Store(on) }

// buildJump is protocol.OpcodeJump from the server: a u32, 1 offered, 0 not
// (any more).
func buildJump(on bool) []byte {
	return build(protocol.OpcodeJump, 6, func(w *buffer.Writer) {
		if on {
			w.WriteUint32(1)
		} else {
			w.WriteUint32(0)
		}
	})
}

// OfferJump offers sess's client the jump, on a server with jumps; it reports
// whether it did. Only for a client of the server's own (the file's comment).
func OfferJump(sess *session.Session) bool {
	if !jumpsOn.Load() {
		return false
	}
	queueReliable(sess, protocol.OpcodeJump, buildJump(true))
	return true
}

// SendJumpsOff tells sess's client it jumps no more: one offered the jump before
// a restart into a server without them (keep.go).
func SendJumpsOff(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeJump, buildJump(false))
}

// JumpHandler takes a client's protocol.OpcodeJump: whether it jumps.
type JumpHandler struct {
	world *entity.World
}

// NewJumpHandler creates the handler.
func NewJumpHandler(w *entity.World) *JumpHandler { return &JumpHandler{world: w} }

// RegisterHandlers registers the opcode.
func (h *JumpHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeJump, h.HandleJump)
}

// HandleJump makes the client's answer its player's: a u32, nonzero it jumps
// (taken only on a server with jumps: one without never offered them), 0 it
// does not. Nothing is answered.
func (h *JumpHandler) HandleJump(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("Jump: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetJumpClient(sess, on) {
		log.Printf("Jump: %s's word (%v) not taken: no player in the world, or no jumps here", sess.RemoteAddr(), on)
		return nil
	}
	log.Printf("Jump: %s %s", sess.RemoteAddr(), map[bool]string{true: "jumps", false: "does not jump"}[on])
	return nil
}
