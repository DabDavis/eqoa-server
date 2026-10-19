package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// GroupHandler takes the client's group opcodes; the group itself is the
// World's (entity/group.go, which lists the wire formats).
type GroupHandler struct {
	gameWorld *entity.World
}

func NewGroupHandler(gw *entity.World) *GroupHandler {
	return &GroupHandler{gameWorld: gw}
}

func (h *GroupHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeGroupInvite, h.handleInvite)
	r.Register(protocol.OpcodeAcceptGroupInvite, h.handleAccept)
	r.Register(protocol.OpcodeDeclineGroupInvite, h.handleDecline)
	r.Register(protocol.OpcodeLeaveGroup, h.handleLeave)
	r.Register(protocol.OpcodeBootGroupMember, h.handleBoot)
	r.Register(protocol.OpcodeDisbandGroup, h.handleDisband)
	log.Printf("Group: registered handlers")
}

// u32 reads the id each of these opcodes starts with.
func groupID(msg *protocol.Message) (uint32, *buffer.Reader, bool) {
	data := msg.Data()
	if len(data) < 4 {
		return 0, nil, false
	}
	r := buffer.NewReader(data)
	id, err := r.ReadUint32()
	return id, r, err == nil
}

func (h *GroupHandler) handleInvite(sess *session.Session, msg *protocol.Message) error {
	if id, _, ok := groupID(msg); ok && h.gameWorld != nil {
		h.gameWorld.GroupInvite(sess, id)
	}
	return nil
}

func (h *GroupHandler) handleAccept(sess *session.Session, msg *protocol.Message) error {
	if id, _, ok := groupID(msg); ok && h.gameWorld != nil {
		h.gameWorld.GroupAccept(sess, id)
	}
	return nil
}

func (h *GroupHandler) handleDecline(sess *session.Session, msg *protocol.Message) error {
	if id, r, ok := groupID(msg); ok && h.gameWorld != nil {
		why, _ := r.ReadByte()
		h.gameWorld.GroupDecline(sess, id, why)
	}
	return nil
}

func (h *GroupHandler) handleLeave(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld != nil {
		h.gameWorld.GroupLeave(sess)
	}
	return nil
}

func (h *GroupHandler) handleBoot(sess *session.Session, msg *protocol.Message) error {
	if id, _, ok := groupID(msg); ok && h.gameWorld != nil {
		h.gameWorld.GroupBoot(sess, id)
	}
	return nil
}

func (h *GroupHandler) handleDisband(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld != nil {
		h.gameWorld.GroupDisband(sess)
	}
	return nil
}
