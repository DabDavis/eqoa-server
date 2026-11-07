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
	r.Register(protocol.OpcodeRaidRosterAnswer, h.handleRaidRosterAnswer)
	log.Printf("Group: registered handlers")
}

// buildMemberInvitesOffer is protocol.OpcodeMemberInvites from the server:
// a u32, 1.
func buildMemberInvitesOffer() []byte {
	return build(protocol.OpcodeMemberInvites, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendMemberInvitesOffer tells the session's client that any member of a
// group invites into it here (entity's GroupInvite); vanilla then drops
// CLIENT's leader-only rule, a PS2 ignores it.
func SendMemberInvitesOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeMemberInvites, buildMemberInvitesOffer())
}

// SendRaidRosterOffer offers the session's client the whole of a raid group
// (protocol.OpcodeRaidRoster, entity/raidroster.go): a u32, 1. A client that can show
// it answers 0x7E19; a PS2 ignores it and keeps its window of four.
func SendRaidRosterOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeRaidRosterOffer, build(protocol.OpcodeRaidRosterOffer, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	}))
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

// handleRaidRosterAnswer makes a client's answer to the offer its player's: a u32,
// nonzero to be sent the roster, 0 to stop. Nothing is answered.
func (h *GroupHandler) handleRaidRosterAnswer(sess *session.Session, msg *protocol.Message) error {
	id, _, ok := groupID(msg)
	if !ok || h.gameWorld == nil {
		log.Printf("RaidRoster: short answer from %s", sess.RemoteAddr())
		return nil
	}
	if !h.gameWorld.SetRaidRoster(sess, id != 0) {
		log.Printf("RaidRoster: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("RaidRoster: %s %s", sess.RemoteAddr(), map[bool]string{true: "shows it", false: "stopped"}[id != 0])
	return nil
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
