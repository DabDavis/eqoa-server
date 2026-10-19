package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Customer service, as CLIENT's VICSClient drives it (decomp
// modules/client/src/vicsclient.cpp, vicsrecords.cpp; the windows
// viwndcsrequest.cpp, viwndcsinbox.cpp, viwndreadcs.cpp: Support's two
// entries, the request form and the inbox).
//
// A request is message 0x0AA1: a sub-op byte, then its fields. Every reply is
// 0x07B0: the sub-op, a status byte (0 is success; StatusToError maps the
// rest to the codes the client shows), then the rest.
//
//	0 request  in: zigzag category, zigzag subcategory, wide text
//	1 reply    in: zigzag id, wide text; the client asks for the text again
//	2 inbox    out: status, u32 (unread by the client), zigzag count, each header
//	3 open     in: zigzag id; out: status, wide text
//	4 delete   in: zigzag id; out: status, zigzag id
//	5 (out only, nothing after): something new
//	6 lists    out: status, zigzag count, each category
//
// A header: u32 id, the date (u16 year, month, day, hour, minute, second),
// u32 category, u32 subcategory, an unread byte. A category: zigzag id, a
// hidden byte, a wide name (128 bytes), zigzag count, each subcategory
// (zigzag id, a hidden byte, a wide name).
//
// The tickets live in memory, keyed by the player's name; a new request is
// acknowledged with a message in its own ticket, and a reply is added to the
// ticket's text. Gone on a restart.

const (
	opcodeCSRequest protocol.GameOpcode = 0x0AA1
	opcodeCSReply   protocol.GameOpcode = 0x07B0

	csInboxMax = 100 // the client keeps 100 rows, and refuses a request past them
)

type csSub struct {
	id     int64
	hidden bool
	name   string
}

type csCategory struct {
	id     int64
	hidden bool
	name   string
	subs   []csSub
}

// csCategories are what the request form offers (a hidden one is listed but
// not offered).
var csCategories = []csCategory{
	{id: 1, name: "Bug Report", subs: []csSub{{id: 10, name: "Crash"}, {id: 11, name: "Graphics"}, {id: 12, name: "Quest"}}},
	{id: 2, name: "Stuck", subs: []csSub{{id: 20, name: "In terrain"}, {id: 21, name: "Lost in a zone"}}},
	{id: 3, name: "Player Issue", subs: []csSub{{id: 30, name: "Harassment"}, {id: 31, name: "Other", hidden: true}}},
	{id: 9, name: "Internal", hidden: true, subs: []csSub{{id: 90, name: "Notes"}}},
}

type csTicket struct {
	id          uint32
	date        time.Time
	category    int64
	subcategory int64
	unread      bool
	text        string
}

// SupportHandler keeps every player's tickets.
type SupportHandler struct {
	gameWorld *entity.World

	mu      sync.Mutex
	tickets map[string][]*csTicket
	nextID  uint32
}

func NewSupportHandler(gw *entity.World) *SupportHandler {
	return &SupportHandler{gameWorld: gw, tickets: map[string][]*csTicket{}, nextID: 1001}
}

func (h *SupportHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeCSRequest, h.handleRequest)
	log.Printf("Support: registered handlers")
}

// csReply queues 0x07B0 with sub and status, then fill.
func csReply(sess *session.Session, sub, status byte, fill func(w *buffer.Writer)) {
	queueReliable(sess, opcodeCSReply, build(opcodeCSReply, 32768, func(w *buffer.Writer) {
		w.WriteByte(sub)
		w.WriteByte(status)
		if fill != nil {
			fill(w)
		}
	}))
}

func writeCSHeader(w *buffer.Writer, t *csTicket) {
	w.WriteUint32(t.id)
	w.WriteUint16(uint16(t.date.Year()))
	w.WriteByte(byte(t.date.Month()))
	w.WriteByte(byte(t.date.Day()))
	w.WriteByte(byte(t.date.Hour()))
	w.WriteByte(byte(t.date.Minute()))
	w.WriteByte(byte(t.date.Second()))
	w.WriteUint32(uint32(t.category))
	w.WriteUint32(uint32(t.subcategory))
	if t.unread {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func writeCSCategories(w *buffer.Writer) {
	flag := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	w.Write7BitEncodedInt64(int64(len(csCategories)))
	for _, c := range csCategories {
		w.Write7BitEncodedInt64(c.id)
		w.WriteByte(flag(c.hidden))
		writeWideString(w, c.name, 128)
		w.Write7BitEncodedInt64(int64(len(c.subs)))
		for _, s := range c.subs {
			w.Write7BitEncodedInt64(s.id)
			w.WriteByte(flag(s.hidden))
			writeWideString(w, s.name, 128)
		}
	}
}

// csNames is a ticket's category and subcategory, for its text.
func csNames(cat, sub int64) (string, string) {
	for _, c := range csCategories {
		if c.id == cat {
			for _, s := range c.subs {
				if s.id == sub {
					return c.name, s.name
				}
			}
			return c.name, ""
		}
	}
	return "", ""
}

func (h *SupportHandler) handleRequest(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil || h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	if me == nil {
		return nil
	}
	mine := strings.ToLower(me.Name)
	find := func(id int64) (int, *csTicket) {
		for i, t := range h.tickets[mine] {
			if int64(t.id) == id {
				return i, t
			}
		}
		return -1, nil
	}
	switch sub {
	case 0:
		cat, err1 := r.Read7BitEncodedInt64()
		subcat, err2 := r.Read7BitEncodedInt64()
		text, ok := readWide(r)
		if err1 != nil || err2 != nil || !ok {
			csReply(sess, 0, 1, nil)
			return nil
		}
		h.mu.Lock()
		if len(h.tickets[mine]) >= csInboxMax {
			h.mu.Unlock()
			csReply(sess, 0, 2, nil)
			return nil
		}
		c, s := csNames(cat, subcat)
		t := &csTicket{id: h.nextID, date: mailNow(), category: cat, subcategory: subcat, unread: true,
			text: fmt.Sprintf("Your request (%s / %s) has been received:\n\n%s", c, s, text)}
		h.nextID++
		h.tickets[mine] = append(h.tickets[mine], t)
		h.mu.Unlock()
		log.Printf("Support: %s filed ticket %d (%d/%d): %q", me.Name, t.id, cat, subcat, text)
		csReply(sess, 0, 0, nil)
	case 1:
		id, err := r.Read7BitEncodedInt64()
		text, ok := readWide(r)
		if err != nil || !ok {
			csReply(sess, 1, 1, nil)
			return nil
		}
		h.mu.Lock()
		_, t := find(id)
		if t != nil {
			t.text += "\n\n" + me.Name + ": " + text
		}
		h.mu.Unlock()
		if t == nil {
			csReply(sess, 1, 3, nil)
			return nil
		}
		log.Printf("Support: %s replied to ticket %d", me.Name, id)
		csReply(sess, 1, 0, nil)
	case 2:
		h.mu.Lock()
		list := append([]*csTicket(nil), h.tickets[mine]...)
		h.mu.Unlock()
		csReply(sess, 2, 0, func(w *buffer.Writer) {
			w.WriteUint32(0)
			w.Write7BitEncodedInt64(int64(len(list)))
			for _, t := range list {
				writeCSHeader(w, t)
			}
		})
	case 3:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		h.mu.Lock()
		_, t := find(id)
		text := ""
		if t != nil {
			t.unread = false
			text = t.text
		}
		h.mu.Unlock()
		if t == nil {
			csReply(sess, 3, 3, nil)
			return nil
		}
		csReply(sess, 3, 0, func(w *buffer.Writer) { writeWideString(w, text, 16384) })
	case 4:
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return nil
		}
		h.mu.Lock()
		i, t := find(id)
		if t != nil {
			list := h.tickets[mine]
			h.tickets[mine] = append(list[:i:i], list[i+1:]...)
		}
		h.mu.Unlock()
		status := byte(0)
		if t == nil {
			status = 3
		}
		csReply(sess, 4, status, func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) })
	case 6:
		csReply(sess, 6, 0, writeCSCategories)
	}
	return nil
}
