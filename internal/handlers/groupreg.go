package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// groupreg.go — the group search's registration as vanilla shows it, and
// "Allow anyone in my group to invite" (the GM Jibrish's petition 1476,
// 2026-11-11: "Enable the group register window / service. Players can
// register for group, allowing them to show up in other players searches
// through that window. They can toggle register on or off. ... A setting box
// bottom right 'Allow anyone in my group to invite' yes/no, per character,
// persistent, applies to regular groups only, live for the existing group
// when its leader toggles it").
//
// The window and the search are SOE's live client's (lfg.go, entity/lfg.go:
// Community > Group > Search, Register, Unregister, 0x0801/0x0840). The live
// client keeps no registration state; this server offers its standing
// (protocol.OpcodeGroupRegOffer, a u8 layout, 1, at every world entry), and a
// client that answers 1 is told it (OpcodeGroupRegState, entity/uitold.go):
// registered or not, its own setting, and whether its group's members invite
// now. Vanilla shows the first two at the window's foot and toggles them (a
// registration by 0x0801's sub 2 and 3, the setting by
// OpcodeGroupInvitesSetting), and keeps CLIENT's leader-only rule (word 640)
// while its group's leader has the setting off (entity/groupinvites.go). A
// PS2 ignores the offer.

// groupRegLayout is OpcodeGroupRegState's layout the offer names.
const groupRegLayout = 1

// SendGroupRegOffer offers the session's client its group search standing.
func SendGroupRegOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeGroupRegOffer, build(protocol.OpcodeGroupRegOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(groupRegLayout)
	}))
}

// GroupRegHandler takes the answer to the offer and the setting.
type GroupRegHandler struct {
	world *entity.World
	db    *database.SQLiteDB
}

// NewGroupRegHandler is the handler over the World and the database (nil:
// the setting kept in memory alone). It loads the settings kept.
func NewGroupRegHandler(w *entity.World, db *database.SQLiteDB) *GroupRegHandler {
	h := &GroupRegHandler{world: w, db: db}
	if w != nil && db != nil {
		shut, err := db.GroupInvitesShut()
		if err != nil {
			log.Printf("GroupReg: the invite settings not read: %v", err)
		} else {
			w.SetGroupInvitesShut(shut)
			log.Printf("GroupReg: %d characters let only their group's leader invite", len(shut))
		}
	}
	return h
}

// RegisterHandlers registers the answer and the setting.
func (h *GroupRegHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeGroupRegOffer, h.HandleAnswer)
	r.Register(protocol.OpcodeGroupInvitesSetting, h.HandleSetting)
}

// HandleAnswer is the client's answer: a u8, 1 to be told, 0 to stop.
func (h *GroupRegHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("GroupReg: a short answer from %s", sess.RemoteAddr())
		return nil
	}
	sess.SetGroupReg(data[0] != 0)
	return nil
}

// The setting's lines (ours), in the group chat's blue as the search's.
const (
	groupInvitesOpenLine = "Anyone in your group may invite into it."
	groupInvitesShutLine = "Only you invite into a group you lead."
	groupInvitesRaidLine = " A raid group's members invite whatever it says."
)

// HandleSetting is "Allow anyone in my group to invite": a u8, 1 yes, 0 no.
func (h *GroupRegHandler) HandleSetting(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		return nil
	}
	open := data[0] != 0
	char, _ := h.world.SetGroupInvitesOpen(sess, open)
	if char == 0 {
		return nil
	}
	if h.db != nil {
		if err := h.db.SetGroupInvitesOpen(char, open); err != nil {
			log.Printf("GroupReg: character %d's invite setting not kept: %v", char, err)
		}
	}
	line := groupInvitesOpenLine
	if !open {
		line = groupInvitesShutLine + groupInvitesRaidLine
	}
	SendColoredChat(sess, line, lfgBlue[0], lfgBlue[1], lfgBlue[2])
	log.Printf("GroupReg: character %d lets %s invite", char, map[bool]string{true: "anyone in its group", false: "only its group's leader"}[open])
	return nil
}
