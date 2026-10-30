package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The group search of SOE's live client (VIWndGroupSearch, live CLIENT
// 0x013623C8..; the live-client research's livediff/specs/lfg.md, outside this
// repository) and the Group menu's
// Show/Hide Chat, which the disc already had. The registration and the search
// are entity/lfg.go's.
//
//	C>S 0x0B01 u8 1, u8 show (1 Show Chat, 0 Hide Chat)       the disc's VIClientPart2EFC8::Send
//	C>S 0x0801 u8 1, u8 show: the same, from the live client   0x012EBDB8
//	C>S 0x0801 u8 2: register (menu 1851, /lfg on|register)    0x012EC2B8
//	C>S 0x0801 u8 3: unregister (menu 1852, /lfg off|unregister) 0x012EC3E0
//	C>S 0x0801 u8 4, u8 type (0 players for a group, 1 groups for a player),
//	           zz class mask, zz race mask (the class mask FIRST), u8 min level,
//	           u8 max level                                     0x012EC530
//	S>C 0x0840 u8 0x40, zz count, count x {string8 name (at most 23), u8 level,
//	           zz class, zz race, f32 x, f32 y, f32 z, u8 world, u32 id}
//	                                                            Recv 0x012EBED8, reader 0x012EBF38
//
// A search must be answered with exactly one 0x0840 sub 0x40, none found
// included: the client marks itself busy as it sends one, shows "Searching"
// and sends no other until the answer (or a new connection) clears it. The
// live client sends nothing for a member of a group who does not lead it
// (word 1848), and says nothing of a registration (it keeps no state, and has
// no word for one), so the server says it. A PS2 sends 0x0B01 alone and has no
// case for 0x0840; its player registers by typing "#lfg on" (a PS2 keeps an
// unknown /command to itself and says nothing to the server, but a # line goes
// out as chat), and a client that forwards its /commands (vanilla) by "/lfg on".

const (
	lfgSubShow       = 1
	lfgSubRegister   = 2
	lfgSubUnregister = 3
	lfgSubSearch     = 4

	lfgResults = 0x40 // 0x0840's only sub the client reads
)

// lfgNotLeader is the live client's word 1848, said in red as it says it.
const lfgNotLeader = "You are not a group leader."

// LFGHandler takes 0x0801 and the disc's 0x0B01.
type LFGHandler struct {
	world *entity.World
}

// NewLFGHandler creates the group search's handler.
func NewLFGHandler(w *entity.World) *LFGHandler { return &LFGHandler{world: w} }

// RegisterHandlers registers 0x0801 and 0x0B01.
func (h *LFGHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeLFG, h.HandleLFG)
	r.Register(protocol.OpcodeGroupShow, h.HandleGroupShow)
}

// HandleGroupShow is the disc's 0x0B01 {u8 1, u8 show}.
func (h *LFGHandler) HandleGroupShow(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 || data[0] != 1 {
		log.Printf("GroupShow: %s sent % X", sess.RemoteAddr(), data)
		return nil
	}
	h.groupShow(sess, data[1] != 0)
	return nil
}

// HandleLFG is the live client's 0x0801: a sub-op and its fields.
func (h *LFGHandler) HandleLFG(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		if len(data) >= 1 && data[0] == lfgSubSearch {
			sendLFGResults(sess, nil) // always one answer to a search
		}
		return nil
	}
	switch data[0] {
	case lfgSubShow:
		if len(data) < 2 {
			return nil
		}
		h.groupShow(sess, data[1] != 0)
	case lfgSubRegister, lfgSubUnregister:
		lfgRegister(h.world, sess, data[0] == lfgSubRegister)
	case lfgSubSearch:
		h.search(sess, data[1:])
	default:
		log.Printf("LFG: %s sent sub %d (% X)", sess.RemoteAddr(), data[0], data)
	}
	return nil
}

// groupShow is the Group menu's Show Chat / Hide Chat.
func (h *LFGHandler) groupShow(sess *session.Session, show bool) {
	if h.world == nil || !h.world.SetGroupChatShown(sess, show) {
		return
	}
	if show {
		SendColoredChat(sess, "Group chat is shown.", lfgBlue[0], lfgBlue[1], lfgBlue[2])
	} else {
		SendColoredChat(sess, "Group chat is hidden. Show Chat on the Group menu shows it again.", lfgBlue[0], lfgBlue[1], lfgBlue[2])
	}
}

// lfgBlue is the group chat's colour (0x069E's (.5, .5, 1)), for the group
// search's lines.
var lfgBlue = [3]uint8{128, 128, 255}

// lfgRegister registers or unregisters the session's character and says so;
// a member of a group who does not lead it is refused in red.
func lfgRegister(gw *entity.World, sess *session.Session, on bool) {
	status, was := gw.SetLFG(sess, on)
	if status == entity.LFGNoPlayer {
		return
	}
	if status == entity.LFGNotLeader {
		SendErrorMessage(sess, lfgNotLeader)
		return
	}
	leader := status == entity.LFGLeader
	var line string
	switch {
	case leader && on && was:
		line = "Your group is already registered as looking for more members."
	case leader && on:
		line = "Your group is registered as looking for more members."
	case leader && was:
		line = "Your group is no longer registered as looking for members."
	case leader:
		line = "Your group is not registered as looking for members."
	case on && was:
		line = "You are already registered as looking for a group."
	case on:
		line = "You are registered as looking for a group."
	case was:
		line = "You are no longer registered as looking for a group."
	default:
		line = "You are not registered as looking for a group."
	}
	SendColoredChat(sess, line, lfgBlue[0], lfgBlue[1], lfgBlue[2])
	log.Printf("LFG: %s registered %v (status %d)", sess.RemoteAddr(), on, status)
}

// search is sub 4: answered with one 0x0840 whatever it holds.
func (h *LFGHandler) search(sess *session.Session, fields []byte) {
	r := buffer.NewReader(fields)
	typ, e1 := r.ReadByte()
	classes, e2 := r.Read7BitEncodedInt64()
	races, e3 := r.Read7BitEncodedInt64()
	lo, e4 := r.ReadByte()
	hi, e5 := r.ReadByte()
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil {
		log.Printf("LFG: %s sent a short search (% X)", sess.RemoteAddr(), fields)
		sendLFGResults(sess, nil)
		return
	}
	rows := h.world.LFGSearch(sess, entity.LFGQuery{Type: typ, Classes: uint32(classes), Races: uint32(races),
		MinLevel: int(lo), MaxLevel: int(hi)})
	sendLFGResults(sess, rows)
	log.Printf("LFG: %s searched type %d, classes %#x, races %#x, levels %d-%d: %d found",
		sess.RemoteAddr(), typ, uint32(classes), uint32(races), lo, hi, len(rows))
}

// lfgResultsPayload is 0x0840 sub 0x40.
func lfgResultsPayload(rows []entity.LFGRow) []byte {
	return build(protocol.OpcodeLFGResults, 16+64*len(rows), func(w *buffer.Writer) {
		w.WriteByte(lfgResults)
		w.Write7BitEncodedInt64(int64(len(rows)))
		for _, row := range rows {
			writeString8(w, row.Name, 24)
			w.WriteByte(byte(min(max(row.Level, 0), 255)))
			w.Write7BitEncodedInt64(int64(row.Class))
			w.Write7BitEncodedInt64(int64(row.Race))
			w.WriteFloat32(row.X)
			w.WriteFloat32(row.Y)
			w.WriteFloat32(row.Z)
			w.WriteByte(row.World)
			w.WriteUint32(row.ID)
		}
	})
}

func sendLFGResults(sess *session.Session, rows []entity.LFGRow) {
	queueReliable(sess, protocol.OpcodeLFGResults, lfgResultsPayload(rows))
}

// lfgCommand is /lfg and #lfg: "on" or "register" registers, "off" or
// "unregister" unregisters (the live client's own words for its /lfg, 1876-
// 1878), nothing says where the character stands; anything else is the live
// client's own "Unrecognized command".
func lfgCommand(gw *entity.World, sess *session.Session, word, rest string) {
	switch arg := strings.ToLower(strings.TrimSpace(rest)); arg {
	case "on", "register":
		lfgRegister(gw, sess, true)
	case "off", "unregister":
		lfgRegister(gw, sess, false)
	case "":
		status, registered := gw.LFGStanding(sess)
		var line string
		switch {
		case status == entity.LFGNoPlayer:
			return
		case status == entity.LFGNotLeader:
			line = "You are in a group you do not lead: its leader registers it."
		case status == entity.LFGLeader && registered:
			line = "Your group is registered as looking for more members."
		case status == entity.LFGLeader:
			line = "Your group is not registered as looking for members."
		case registered:
			line = "You are registered as looking for a group."
		default:
			line = "You are not registered as looking for a group."
		}
		SendColoredChat(sess, line, lfgBlue[0], lfgBlue[1], lfgBlue[2])
		SendColoredChat(sess, fmt.Sprintf("You can register or unregister in chat with: %s on, %s off", word, word),
			lfgBlue[0], lfgBlue[1], lfgBlue[2])
	default:
		SendErrorMessage(sess, fmt.Sprintf("Unrecognized command %q.", strings.TrimSpace(word+" "+strings.TrimSpace(rest))))
	}
}
