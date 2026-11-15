package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// More answers in an NPC's box than CLIENT's six, for a client that shows
// them (session/dialogroom.go, protocol.OpcodeDialogRoom; the owner,
// 2026-11-15: "lets increase the dialog box for heroic dungeons, coachmen
// etc. to stack higher before pagination").
//
// Once the player is in the world the server offers the most answers it puts
// in a box (session.MaxDialogRoom); a client that shows more answers with its
// own number, and every box the session is asked from then on holds up to the
// least of the two (dialogRoom): a list longer than that is paged, room-1
// answers and "More..." a box (navPage for the lists with "Back" as well). A
// PS2 ignores the offer and never answers, so its boxes are what they were.

// dialogRoom is the answers sess's client shows in one 0x0034 box:
// maxDialogOptions (CLIENT's six) unless it said more.
func dialogRoom(sess *session.Session) int {
	if sess == nil {
		return maxDialogOptions
	}
	return sess.DialogRoom()
}

// buildDialogRoomOffer is OpcodeDialogRoom from the server: the most answers
// it puts in a box, a u32.
func buildDialogRoomOffer(room int) []byte {
	return build(protocol.OpcodeDialogRoom, 6, func(w *buffer.Writer) {
		w.WriteUint32(uint32(room))
	})
}

// SendDialogRoomOffer offers the session's client the server's most answers
// in a box.
func SendDialogRoomOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeDialogRoom, buildDialogRoomOffer(session.MaxDialogRoom))
}

// DialogRoomHandler takes a client's OpcodeDialogRoom: how many answers it
// shows in a box.
type DialogRoomHandler struct{}

// NewDialogRoomHandler creates the dialog room handler.
func NewDialogRoomHandler() *DialogRoomHandler { return &DialogRoomHandler{} }

// RegisterHandlers registers the dialog room handler.
func (h *DialogRoomHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDialogRoom, h.HandleDialogRoom)
}

// HandleDialogRoom makes the client's room (within six and
// session.MaxDialogRoom) the session's: every box asked from then on holds up
// to that many answers. Nothing is answered.
func (h *DialogRoomHandler) HandleDialogRoom(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("DialogRoom: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	asked := int(min(binary.LittleEndian.Uint32(data[:4]), 255))
	sess.SetDialogRoom(asked)
	log.Printf("DialogRoom: %s shows %d answers a box (asked %d; the server's most %d)",
		sess.RemoteAddr(), sess.DialogRoom(), asked, session.MaxDialogRoom)
	return nil
}

// navPage is the page'th box of a list of n with "More..." and "Back" as
// CLIENT's room permits: the items it shows ([from, to)) and whether More
// and Back follow them. A box never holds more than room answers: the first
// page room-1 items and More (all n when they fit beside nothing), a middle
// page room-2 and both, the last page what is left (up to room-1) and Back.
// A page past the last is the last.
func navPage(n, page, room int) (from, to int, more, back bool) {
	room = max(room, 3)
	if n <= room-1 || page <= 0 {
		return 0, min(n, room-1), n > room-1, false
	}
	// page 0 holds room-1, each further page room-2 until the rest fit in
	// room-1 beside Back
	from = room - 1
	for p := 1; ; p++ {
		if n-from <= room-1 || p == page {
			if n-from <= room-1 {
				return from, n, false, true
			}
			return from, from + room - 2, true, true
		}
		from += room - 2
	}
}
