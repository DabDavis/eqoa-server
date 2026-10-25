package handlers

import (
	_ "embed"
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
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
//   - the skills shown are the captured character's, not this one's: its
//     Recv08 is the token (0) and a u32 count, 0x60 (96 skills), then the
//     skills (VICMSkill::Read). The 0x60 is not unspent points: the windows'
//     points are this character's, the MemoryDump's mastery block and
//     0x1402 sub-op 3 (entity/mastery.go)
//   - buying and removing do nothing; see HandleClassMastery
//
//go:embed cmdata/fulltree.bin
var cmFullTree []byte

// 0x1401's first byte: CLIENT's requests (VIClientPart4EC58's senders,
// decomp viclientpart4ec58.cpp), each followed by the server's token (raw,
// what the last reply or the MemoryDump gave) and, but for Open, an argument.
const (
	cmShare  = 0x01 // Select(percent): the mastery points window's share of experience, 0-100
	cmBuy    = 0x02 // Send02(skill): buy a mastery (the inspect window)
	cmRemove = 0x03 // Send03(skill): remove one
	cmOpen   = 0x04 // Open(): the list
	cmBrowse = 0x05 // Browse(skill): one skill's page
)

// 0x1402's first byte: the replies CLIENT's Recv1402 dispatches. Only the
// ones that end CLIENT's wait (m_pending: 1, 2, 5, 6, 8 and 9) answer a
// request; a request left unanswered leaves the window waiting with no way
// out (no client-side timeout), and CLIENT sends nothing more meanwhile.
const (
	cmRespShare    = 0x01 // Recv01: token, share
	cmRespDone     = 0x02 // Recv02: token
	cmRespPoints   = 0x03 // Recv03: unspent, spent (not a request's: entity.SendMasteryPoints)
	cmRespFullTree = 0x08 // Recv08: token, the list (cmFullTree)
)

// masteryStore keeps a character's mastery points (database.SQLiteCharacterStore).
type masteryStore interface {
	SaveMastery(serverID uint32, m database.MasteryRow) error
}

// ClassMasteryHandler answers 0x1401: the share of experience going to
// mastery points, kept; the list (the captured one, below); and a reply
// that ends CLIENT's wait to everything else.
type ClassMasteryHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	store          masteryStore
}

// NewClassMasteryHandler creates a new class mastery handler.
func NewClassMasteryHandler(sessionMgr *session.Manager) *ClassMasteryHandler {
	return &ClassMasteryHandler{sessionManager: sessionMgr}
}

// SetGameWorld gives the handler the players' mastery points.
func (h *ClassMasteryHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// SetStore gives it where to keep them. Without one a share set lasts
// until the logout's save.
func (h *ClassMasteryHandler) SetStore(s masteryStore) {
	h.store = s
}

// RegisterHandlers registers class mastery handlers with the registry.
func (h *ClassMasteryHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClassMastery, h.HandleClassMastery)
}

// HandleClassMastery handles 0x1401: a sub-op byte, the token (u32) and its
// argument (u32). EVERY request is answered (cmReply).
func (h *ClassMasteryHandler) HandleClassMastery(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		log.Printf("ClassMastery: short payload (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	sub := data[0]
	token := binary.LittleEndian.Uint32(data[1:5])
	var arg uint32
	if len(data) >= 9 {
		arg = binary.LittleEndian.Uint32(data[5:9])
	}

	share := 0
	switch sub {
	case cmShare:
		share = h.setShare(sess, int(int32(arg)))
	case cmOpen:
		log.Printf("ClassMastery: open from %s: the captured list (%d bytes), not this character's",
			sess.RemoteAddr(), len(cmFullTree))
	case cmBrowse:
		log.Printf("ClassMastery: browse %#x from %s: the list again (a skill's page is not built)",
			arg, sess.RemoteAddr())
	case cmBuy, cmRemove:
		log.Printf("ClassMastery: %s %#x from %s: masteries are not sold yet",
			map[byte]string{cmBuy: "buy", cmRemove: "remove"}[sub], arg, sess.RemoteAddr())
		SendErrorMessage(sess, "Masteries cannot be bought or removed on this server yet.")
	default:
		log.Printf("ClassMastery: unknown sub-op %#x (%#x) from %s", sub, arg, sess.RemoteAddr())
	}
	h.sendRaw(sess, cmReply(sub, token, share))
	return nil
}

// cmReply is 0x1402's answer to a request (its payload after the opcode):
// the share for a share (Recv01, which ends the wait and shows it), the list
// for an open or a browse (Recv08, the captured one), and Recv02 (the token,
// nothing else) for anything else, which ends the wait and changes nothing:
// the old answer to every other sub-op, Recv01 with the request's argument,
// put a skill's id where CLIENT keeps the share.
func cmReply(sub byte, token uint32, share int) []byte {
	switch sub {
	case cmShare:
		b := make([]byte, 9)
		b[0] = cmRespShare
		binary.LittleEndian.PutUint32(b[1:5], token)
		binary.LittleEndian.PutUint32(b[5:9], uint32(share))
		return b
	case cmOpen, cmBrowse:
		return cmFullTree
	}
	b := make([]byte, 5)
	b[0] = cmRespDone
	binary.LittleEndian.PutUint32(b[1:5], token)
	return b
}

// setShare keeps the player's share of experience going to mastery points
// (CLIENT clamps it to 0-100 before sending, and so does this): the share
// now.
func (h *ClassMasteryHandler) setShare(sess *session.Session, pct int) int {
	pct = max(0, min(100, pct))
	if h.gameWorld == nil {
		return pct
	}
	m, ok := h.gameWorld.SetMasteryPercent(sess, pct)
	if !ok {
		return pct
	}
	if p := h.gameWorld.SessionPlayer(sess); p != nil && h.store != nil {
		if err := h.store.SaveMastery(p.CharServerID, MasteryRow(m)); err != nil {
			log.Printf("ClassMastery: %s's share of %d%% not kept: %v", p.Name, m.Percent, err)
		}
	}
	log.Printf("ClassMastery: %s gives %d%% of experience to masteries", sess.RemoteAddr(), m.Percent)
	return m.Percent
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

// MasteryRow is a player's mastery points as charmastery keeps them.
func MasteryRow(m entity.Mastery) database.MasteryRow {
	return database.MasteryRow{Points: m.Points, Spent: m.Spent, Percent: m.Percent, XP: m.XP}
}

// PlayerMastery is a charmastery row as the player holds it.
func PlayerMastery(r database.MasteryRow) entity.Mastery {
	return entity.Mastery{Points: r.Points, Spent: r.Spent, Percent: r.Percent, XP: r.XP}
}
