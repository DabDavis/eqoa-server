package handlers

import (
	_ "embed"
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// cmFullTree is a REAL 0x1402 type=0x08 response captured from retail
// (pcaps/ps2_main_capture.pcap, packet #1711, 1302 bytes reassembled).
//
// This is a STOPGAP, not the CM system. The Class Mastery tree is not
// implemented, and the client hangs on an unanswered 0x1401 with no way to
// close the window — you cannot even back out of the menu. Replaying a
// known-good tree gets the window open and closable again.
//
// Canned bytes rather than an authored empty tree on purpose: the per-entry
// layout below the documented header is NOT fully known
// (claude-notes/cm-wire-format.md stops at "... more fields per entry"), so an
// invented "empty" tree would be a guess, and a malformed one hangs the client
// exactly the way no reply does. These bytes are what a real server sent to a
// real client that then rendered the menu.
//
// Consequences, which are visible in-game and intentional:
//   - the skills shown are the captured character's, not this one's
//   - the unspent-CM count in the header is the captured value (0x60 = 96)
//   - spending does nothing; see HandleClassMastery
//
//go:embed cmdata/fulltree.bin
var cmFullTree []byte

// Client action codes — byte 0 of the 0x1401 payload.
const (
	cmActionSelect  = 0x01 // select/inspect a skill (server acks)
	cmActionSpend   = 0x02 // spend a CM point on a skill
	cmActionOpen    = 0x04 // open the menu (server sends the full tree)
	cmActionBrowse  = 0x05 // browse into a category (server sends detail)
	cmRespSelectAck = 0x01 // 0x1402 type: echo of a select
	cmRespFullTree  = 0x08 // 0x1402 type: complete tree
)

// ClassMasteryHandler answers 0x1401 so the CM window can be opened and closed.
type ClassMasteryHandler struct {
	sessionManager *session.Manager
}

// NewClassMasteryHandler creates a new class mastery handler.
func NewClassMasteryHandler(sessionMgr *session.Manager) *ClassMasteryHandler {
	return &ClassMasteryHandler{sessionManager: sessionMgr}
}

// RegisterHandlers registers class mastery handlers with the registry.
func (h *ClassMasteryHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClassMastery, h.HandleClassMastery)
}

// HandleClassMastery handles 0x1401 (Class Mastery request).
//
// Wire format (claude-notes/cm-wire-format.md, confirmed against
// pcaps/ps2_main_capture.pcap):
//
//	byte(action) + int32_LE(0 reserved) [+ int32_LE(itemID) when 9 bytes]
//
// EVERY action must produce a reply. The window has no client-side timeout —
// an unanswered request leaves it searching forever with no way out, which is
// the bug this exists to fix. Where the response format is unknown, fall back
// to the select-ack, which is 9 bytes and fully documented.
func (h *ClassMasteryHandler) HandleClassMastery(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		log.Printf("ClassMastery: short payload (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}

	action := data[0]
	var itemID uint32
	if len(data) >= 9 {
		itemID = binary.LittleEndian.Uint32(data[5:9])
	}

	switch action {
	case cmActionOpen:
		log.Printf("ClassMastery: open from %s — replying with captured tree (%d bytes); "+
			"CM system not implemented, contents are not this character's",
			sess.RemoteAddr(), len(cmFullTree))
		h.sendRaw(sess, cmFullTree)

	case cmActionBrowse:
		// Retail replies with a type=0x09 category detail whose layout we do
		// not have. Re-sending the tree keeps the window responsive and leaves
		// it on a screen the client can already render, rather than risking a
		// malformed 0x09.
		log.Printf("ClassMastery: browse category %#x from %s — resending tree (0x09 detail not implemented)",
			itemID, sess.RemoteAddr())
		h.sendRaw(sess, cmFullTree)

	case cmActionSelect:
		h.sendSelectAck(sess, itemID)

	case cmActionSpend:
		// Deliberately NOT implemented: spending would have to debit real CM
		// points and persist a rank, and the tree above is not this character's
		// to begin with. Ack it so the window does not hang; nothing is spent.
		log.Printf("ClassMastery: spend on %#x from %s IGNORED (not implemented) — acking only",
			itemID, sess.RemoteAddr())
		h.sendSelectAck(sess, itemID)

	default:
		log.Printf("ClassMastery: unknown action %#x (itemID=%#x) from %s — acking to avoid a hang",
			action, itemID, sess.RemoteAddr())
		h.sendSelectAck(sess, itemID)
	}

	return nil
}

// sendSelectAck sends the 9-byte 0x1402 type=0x01 acknowledgement:
// byte(0x01) + int32_LE(0) + int32_LE(itemID). Verified on the wire — retail
// replies to `01 00 00 00 00 06 00 00 00` with exactly those bytes back.
func (h *ClassMasteryHandler) sendSelectAck(sess *session.Session, itemID uint32) {
	payload := make([]byte, 9)
	payload[0] = cmRespSelectAck
	binary.LittleEndian.PutUint32(payload[5:9], itemID)
	h.sendRaw(sess, payload)
}

// sendRaw queues a 0x1402 carrying payload verbatim.
func (h *ClassMasteryHandler) sendRaw(sess *session.Session, payload []byte) {
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMasteryServer)
	writer := buffer.NewWriter(resp.Span())
	writer.WriteUint16(uint16(protocol.OpcodeClassMasteryServer))
	for _, b := range payload {
		writer.WriteByte(b)
	}
	resp.SetSize(writer.Position())
	sess.Queue().AddWithSequence(resp)
}
