package handlers

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Tell-grams, the in-game mail, as CLIENT's gram client drives it
// (VIClientPart2EFD0, decomp modules/client/src/viclientpart2efd0.cpp).
//
// A request is message 0x0D01: a sub-op byte, then a zigzag length and that
// many bytes (an inner 0x0D41 carrying the sub-op again), then the fields.
// Every reply is 0x0D41: the sub-op, a status, the rest.
//
//	1 send    in: string to (8-bit), wide subject, wide text
//	          out: status 1 sent, 9 unknown recipient, 10 mailbox full,
//	          11 too many unread from you, 12 the system is busy
//	2 inbox   out: status 2, zigzag count, each header
//	3 open    in: zigzag id; out: status 3, zigzag id, wide text
//	4 delete  in: zigzag id; out: status 13 deleted (anything else fails)
//	5 (out only, no status): new mail
//
// A header (operator>>(VIMemStream &, VIGramHeader &)): u32 id, u32, u32,
// string from (24), wide subject (128 bytes), the date (u16 year, month,
// day, hour, minute, second) and a read byte.
//
// The mail lives in memory, keyed by the recipient's name; it is gone on a
// restart.

const (
	opcodeGramRequest protocol.GameOpcode = 0x0D01
	opcodeGramReply   protocol.GameOpcode = 0x0D41

	mailboxMax      = 100 // the client keeps 100 rows
	unreadPerSender = 10
)

type gram struct {
	id      uint32
	from    string
	subject string
	text    string
	date    time.Time
	read    bool
}

// MailHandler keeps every mailbox.
type MailHandler struct {
	gameWorld *entity.World
	charStore *database.SQLiteCharacterStore

	mu     sync.Mutex
	boxes  map[string][]*gram
	nextID uint32
}

func NewMailHandler(gw *entity.World, cs *database.SQLiteCharacterStore) *MailHandler {
	return &MailHandler{gameWorld: gw, charStore: cs, boxes: map[string][]*gram{}, nextID: 1}
}

func (h *MailHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeGramRequest, h.handleRequest)
	if h.gameWorld != nil {
		h.gameWorld.OnPlayerEntered(h.announceUnread)
	}
	log.Printf("Mail: registered handlers")
}

// newMailText is WORDS.TXT's word 1192. CLIENT shows nothing for 0x0D41's
// sub-op 5 (Recv0D41 only sets the gram client's m_x08, which nothing
// reads) and no code of the game's shows the word, so the server says it:
// a gram arrived and nobody knew (F3 20261021-001609).
const newMailText = "You have new mail."

// notifyNewMail is sub-op 5 and the line that says it.
func notifyNewMail(sess *session.Session) {
	gramReply(sess, 5, nil)
	sess.Queue().AddWithSequence(buildClientMessage(newMailText))
}

// announceUnread tells a player entering the world of grams it has not read.
func (h *MailHandler) announceUnread(sess *session.Session, p *entity.Player) {
	if h.unread(p.Name) > 0 {
		notifyNewMail(sess)
	}
}

// unread is how many grams in name's box are unread.
func (h *MailHandler) unread(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, g := range h.boxes[strings.ToLower(name)] {
		if !g.read {
			n++
		}
	}
	return n
}

// gramReply queues 0x0D41 with sub, then fill.
func gramReply(sess *session.Session, sub byte, fill func(w *buffer.Writer)) {
	queueReliable(sess, opcodeGramReply, build(opcodeGramReply, 32768, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	}))
}

// The dates are shown "PST".
func mailNow() time.Time {
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		return time.Now().In(loc)
	}
	return time.Now()
}

func writeGramHeader(w *buffer.Writer, g *gram) {
	w.WriteUint32(g.id)
	w.WriteUint32(0)
	w.WriteUint32(0)
	writeString8(w, g.from, 24)
	writeWideString(w, g.subject, 128)
	w.WriteUint16(uint16(g.date.Year()))
	w.WriteByte(byte(g.date.Month()))
	w.WriteByte(byte(g.date.Day()))
	w.WriteByte(byte(g.date.Hour()))
	w.WriteByte(byte(g.date.Minute()))
	w.WriteByte(byte(g.date.Second()))
	if g.read {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func readString8(r *buffer.Reader) (string, bool) {
	n, err := r.ReadUint32()
	if err != nil || n > 1024 {
		return "", false
	}
	s, err := r.ReadStringUTF8(int(n))
	return s, err == nil
}

func readWide(r *buffer.Reader) (string, bool) {
	n, err := r.ReadUint32()
	if err != nil || n > 1024 {
		return "", false
	}
	s, err := r.ReadStringUnicode(int(n))
	return s, err == nil
}

func (h *MailHandler) handleRequest(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil {
		return nil
	}
	// the inner 0x0D41 as a blob: skipped
	n, err := r.Read7BitEncodedInt64()
	if err != nil || n < 0 || int(n) > r.Remaining() {
		return nil
	}
	if err := r.Advance(int(n)); err != nil {
		return nil
	}
	if h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	if me == nil {
		return nil
	}
	mine := strings.ToLower(me.Name)
	switch sub {
	case 1:
		to, ok1 := readString8(r)
		subject, ok2 := readWide(r)
		text, ok3 := readWide(r)
		if !ok1 || !ok2 || !ok3 {
			gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(0) })
			return nil
		}
		status := h.send(me.Name, to, subject, text)
		gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(status) })
	case 2:
		h.mu.Lock()
		box := append([]*gram(nil), h.boxes[mine]...)
		h.mu.Unlock()
		gramReply(sess, 2, func(w *buffer.Writer) {
			w.WriteByte(2)
			w.Write7BitEncodedInt64(int64(len(box)))
			for _, g := range box {
				writeGramHeader(w, g)
			}
		})
	case 3:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		h.mu.Lock()
		var found *gram
		for _, g := range h.boxes[mine] {
			if g.id == uint32(id) {
				g.read = true
				found = g
			}
		}
		h.mu.Unlock()
		gramReply(sess, 3, func(w *buffer.Writer) {
			if found == nil {
				w.WriteByte(0)
				return
			}
			w.WriteByte(3)
			w.Write7BitEncodedInt64(id)
			writeWideString(w, found.text, 1024)
		})
	case 4:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		h.mu.Lock()
		status := byte(0)
		box := h.boxes[mine]
		for i, g := range box {
			if g.id == uint32(id) {
				h.boxes[mine] = append(box[:i:i], box[i+1:]...)
				status = 13
				break
			}
		}
		h.mu.Unlock()
		gramReply(sess, 4, func(w *buffer.Writer) { w.WriteByte(status) })
	}
	return nil
}

// send files a gram in to's box, and tells to (sub-op 5 and newMailText)
// when on line.
func (h *MailHandler) send(from, to, subject, text string) byte {
	if to == "" {
		return 9
	}
	if h.charStore != nil {
		if ok, err := h.charStore.NameExists(to); err != nil || !ok {
			return 9
		}
	}
	key := strings.ToLower(to)
	h.mu.Lock()
	box := h.boxes[key]
	if len(box) >= mailboxMax {
		h.mu.Unlock()
		return 10
	}
	unread := 0
	for _, g := range box {
		if !g.read && strings.EqualFold(g.from, from) {
			unread++
		}
	}
	if unread >= unreadPerSender {
		h.mu.Unlock()
		return 11
	}
	g := &gram{id: h.nextID, from: from, subject: subject, text: text, date: mailNow()}
	h.nextID++
	h.boxes[key] = append(box, g)
	h.mu.Unlock()
	log.Printf("Mail: %s -> %s: %q", from, to, subject)
	var target *entity.Player
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		if strings.EqualFold(p.Name, to) {
			target = p
		}
	})
	if target != nil {
		if s := h.gameWorld.FindSessionByEntityID(target.ServerID); s != nil {
			notifyNewMail(s)
		}
	}
	return 1
}
