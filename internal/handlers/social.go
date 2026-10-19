package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// SocialHandler takes the buddy and ignore list opcodes; the lists are the
// World's (entity/social.go, which lists the wire formats).
type SocialHandler struct {
	gameWorld *entity.World
}

func NewSocialHandler(gw *entity.World) *SocialHandler {
	return &SocialHandler{gameWorld: gw}
}

func (h *SocialHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeIgnoreAdd, h.handleIgnoreAdd)
	r.Register(protocol.OpcodeIgnoreRemove, h.handleIgnoreRemove)
	r.Register(protocol.OpcodeBuddyAdd, h.handleBuddyAdd)
	r.Register(protocol.OpcodeBuddyAccept, h.handleBuddyAccept)
	r.Register(protocol.OpcodeBuddyRefuse, h.handleBuddyRefuse)
	r.Register(protocol.OpcodeBuddyRemove, h.handleBuddyRemove)
	log.Printf("Social: registered handlers")
}

// name8 reads VIMemStream::WriteString's u32 length and bytes.
func name8(msg *protocol.Message) (string, bool) {
	r := buffer.NewReader(msg.Data())
	n, err := r.ReadUint32()
	if err != nil || n > 64 {
		return "", false
	}
	s, err := r.ReadStringUTF8(int(n))
	return s, err == nil
}

func (h *SocialHandler) handleIgnoreAdd(sess *session.Session, msg *protocol.Message) error {
	if name, ok := name8(msg); ok && h.gameWorld != nil {
		h.gameWorld.IgnoreAdd(sess, name)
	}
	return nil
}

func (h *SocialHandler) handleIgnoreRemove(sess *session.Session, msg *protocol.Message) error {
	if id, _, ok := groupID(msg); ok && h.gameWorld != nil {
		h.gameWorld.IgnoreRemove(sess, id)
	}
	return nil
}

func (h *SocialHandler) handleBuddyAdd(sess *session.Session, msg *protocol.Message) error {
	if name, ok := name8(msg); ok && h.gameWorld != nil {
		h.gameWorld.BuddyAdd(sess, name)
	}
	return nil
}

func (h *SocialHandler) handleBuddyAccept(sess *session.Session, msg *protocol.Message) error {
	if id, _, ok := groupID(msg); ok && h.gameWorld != nil {
		h.gameWorld.BuddyAnswer(sess, id, true, 0)
	}
	return nil
}

func (h *SocialHandler) handleBuddyRefuse(sess *session.Session, msg *protocol.Message) error {
	id, r, ok := groupID(msg)
	if !ok || h.gameWorld == nil {
		return nil
	}
	reason, err := r.ReadByte()
	if err != nil {
		return nil
	}
	h.gameWorld.BuddyAnswer(sess, id, false, reason)
	return nil
}

func (h *SocialHandler) handleBuddyRemove(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, err := r.Read7BitEncodedInt64()
	if err == nil && h.gameWorld != nil {
		h.gameWorld.BuddyRemove(sess, id)
	}
	return nil
}
