package handlers

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A header is written the way VIGramHeader's reader takes it: the id first,
// two words, the 8-bit sender, the wide subject, seven date bytes, read.
func TestGramHeaderLayout(t *testing.T) {
	g := &gram{id: 0x01020304, from: "Ann", subject: "Hi", date: time.Date(2026, 10, 19, 21, 5, 3, 0, time.UTC), read: true}
	buf := make([]byte, 128)
	w := buffer.NewWriter(buf)
	writeGramHeader(w, g)
	want := []byte{4, 3, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 'A', 'n', 'n', 2, 0, 0, 0, 'H', 0, 'i', 0, 0xEA, 0x07, 10, 19, 21, 5, 3, 1}
	if got := buf[:w.Position()]; !bytes.Equal(got, want) {
		t.Fatalf("header\n got %v\nwant %v", got, want)
	}
}

// The send statuses: no name and (with a character store) an unknown one
// are 9; a full box 10; ten unread from one sender 11, though another
// sender still gets through (the control).
func TestGramSendStatuses(t *testing.T) {
	h := NewMailHandler(entity.NewWorld(), nil)
	if s := h.send("Ann", "", "s", "t"); s != 9 {
		t.Fatalf("no name: %d", s)
	}
	for i := 0; i < unreadPerSender; i++ {
		if s := h.send("Ann", "Bob", "s", "t"); s != 1 {
			t.Fatalf("send %d: %d", i, s)
		}
	}
	if s := h.send("ann", "bob", "s", "t"); s != 11 {
		t.Fatalf("too many unread: %d", s)
	}
	if s := h.send("Cid", "Bob", "s", "t"); s != 1 {
		t.Fatalf("another sender: %d", s)
	}
	for _, g := range h.boxes["bob"] {
		g.read = true
	}
	for len(h.boxes["bob"]) < mailboxMax {
		h.send("Dee", "Bob", "s", "t")
		for _, g := range h.boxes["bob"] {
			g.read = true
		}
	}
	if s := h.send("Eve", "Bob", "s", "t"); s != 10 {
		t.Fatalf("full: %d", s)
	}
}

// A gram to a player in the world is announced: 0x0D41's sub-op 5 and
// "You have new mail." (CLIENT shows nothing for the sub-op alone; F3
// 20261021-001609). Grams waiting when a player enters the world are
// announced then; a box with only read grams is not (the control).
func TestNewMailIsAnnounced(t *testing.T) {
	c := newChatWorld(t)
	m := NewMailHandler(c.gw, nil)
	m.RegisterHandlers(NewRegistry())
	_, _ = c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	announced := func(s *session.Session) (sub5, line bool) {
		for _, msg := range s.Queue().GetPending() {
			d := msg.Data()
			if len(d) >= 3 && binary.LittleEndian.Uint16(d) == uint16(opcodeGramReply) && d[2] == 5 {
				sub5 = true
			}
			if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClientMessage {
				if text, _ := readWideLine(d[2:]); text == newMailText {
					line = true
				}
			}
		}
		return
	}
	bs.Queue().GetPending()
	if s := m.send("Ann", "Bob", "hi", "hello"); s != 1 {
		t.Fatalf("send: %d", s)
	}
	if sub5, line := announced(bs); !sub5 || !line {
		t.Fatalf("on arrival: sub-op 5 %v, line %v", sub5, line)
	}
	c.gw.PlayerEntered(bs)
	if sub5, line := announced(bs); !sub5 || !line {
		t.Fatalf("at entry with one unread: sub-op 5 %v, line %v", sub5, line)
	}
	for _, g := range m.boxes["bob"] {
		g.read = true
	}
	c.gw.PlayerEntered(bs)
	if sub5, line := announced(bs); sub5 || line {
		t.Fatalf("at entry with all read: sub-op 5 %v, line %v", sub5, line)
	}
}

// fakeNames is a character store's names, lower case.
type fakeNames map[string]bool

func (f fakeNames) NameExists(name string) (bool, error) { return f[strings.ToLower(name)], nil }

// gramRequestBytes is a 0x0D01 payload: the sub-op, the disc's envelope
// (06 41 0D sub) when disc, then the fields.
func gramRequestBytes(sub byte, disc bool, fields func(w *buffer.Writer)) []byte {
	buf := make([]byte, 8192)
	w := buffer.NewWriter(buf)
	w.WriteByte(sub)
	if disc {
		w.WriteBytes([]byte{0x06, 0x41, 0x0D, sub})
	}
	if fields != nil {
		fields(w)
	}
	return buf[:w.Position()]
}

// gramReplies is every 0x0D41 a session was sent, each after its opcode.
func gramReplies(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 3 && binary.LittleEndian.Uint16(d) == uint16(opcodeGramReply) {
			out = append(out, d[2:])
		}
	}
	return out
}

// mailWorld is three players in the world with a mail handler whose name
// store knows them and Dee (off line).
func mailWorld(t *testing.T) (*MailHandler, *session.Session, *session.Session, *session.Session) {
	c := newChatWorld(t)
	m := NewMailHandler(c.gw, nil)
	m.names = fakeNames{"ann": true, "bob": true, "cid": true, "dee": true}
	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	cs, _ := c.join(3, "Cid", 120, 120, 0)
	for _, s := range []*session.Session{as, bs, cs} {
		s.Queue().GetPending()
	}
	return m, as, bs, cs
}

// liveSend is the live client's send (SendGram): a zigzag count, the names,
// the subject, the text.
func liveSend(names []string, subject, text string) []byte {
	return gramRequestBytes(1, false, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(len(names)))
		for _, n := range names {
			writeString8(w, n, 1024)
		}
		writeWideString(w, subject, 1024)
		writeWideString(w, text, 1024)
	})
}

// The envelope is told by its bytes: the disc's 06 41 0D after the sub-op,
// on all four requests; a live send (a count and names) and a live open (an
// id) never begin so, whatever the count or the id.
func TestGramEnvelope(t *testing.T) {
	for sub := byte(1); sub <= 4; sub++ {
		if !gramEnvelope(gramRequestBytes(sub, true, nil)) {
			t.Fatalf("the disc's sub-op %d not seen", sub)
		}
	}
	for n := 0; n <= 25; n++ {
		names := make([]string, n)
		for i := range names {
			names[i] = "Abcdefghijklmnopqrstuvw"[:1+i%23]
		}
		if gramEnvelope(liveSend(names, "s", "t")) {
			t.Fatalf("a live send to %d names read as the disc's", n)
		}
	}
	for _, id := range []int64{0, 3, 1 << 20, -1} {
		d := gramRequestBytes(3, false, func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) })
		if gramEnvelope(d) {
			t.Fatalf("a live open of %d read as the disc's", id)
		}
	}
}

// A live send to three names puts a copy in each box, each copy holding all
// three names, and answers 1. The control: the disc's form, sent the same
// way, still answers its 1 and files one copy.
func TestGramLiveSendSeveral(t *testing.T) {
	m, as, bs, cs := mailWorld(t)
	if err := m.handleRequest(as, incoming(opcodeGramRequest, liveSend([]string{"Bob", "Cid", "Dee"}, "Hi", "Hello all"))); err != nil {
		t.Fatal(err)
	}
	if got := gramReplies(as); len(got) != 1 || !bytes.Equal(got[0], []byte{1, gramSent}) {
		t.Fatalf("reply % X", got)
	}
	for _, who := range []string{"bob", "cid", "dee"} {
		box := m.boxes[who]
		if len(box) != 1 || strings.Join(box[0].to, ";") != "Bob;Cid;Dee" || box[0].from != "Ann" || box[0].text != "Hello all" {
			t.Fatalf("%s's box: %+v", who, box)
		}
	}
	if m.boxes["bob"][0].id == m.boxes["cid"][0].id {
		t.Fatal("two copies with one id")
	}
	// the two on line were told
	for _, s := range []*session.Session{bs, cs} {
		if r := gramReplies(s); len(r) != 1 || r[0][0] != 5 {
			t.Fatalf("not told: % X", r)
		}
	}
	disc := gramRequestBytes(1, true, func(w *buffer.Writer) {
		writeString8(w, "Bob", 24)
		writeWideString(w, "Disc", 1024)
		writeWideString(w, "One name", 1024)
	})
	if err := m.handleRequest(as, incoming(opcodeGramRequest, disc)); err != nil {
		t.Fatal(err)
	}
	if got := gramReplies(as); len(got) != 1 || !bytes.Equal(got[0], []byte{1, gramSent}) {
		t.Fatalf("disc reply % X", got)
	}
	if box := m.boxes["bob"]; len(box) != 2 || strings.Join(box[1].to, ";") != "Bob" {
		t.Fatalf("disc copy: %+v", box)
	}
}

// A name no character has stops a live send: 9 and the unknown names, in
// the order given, and nobody gets it (the window stays to fix them). A
// repeated name is one name.
func TestGramLiveSendUnknown(t *testing.T) {
	m, as, _, _ := mailWorld(t)
	m.handleRequest(as, incoming(opcodeGramRequest, liveSend([]string{"Bob", "Zed", "bob", "Quux"}, "Hi", "x")))
	want := []byte{1, gramUnknown, 4}
	for _, n := range []string{"Zed", "Quux"} {
		want = append(want, byte(len(n)), 0, 0, 0)
		want = append(want, n...)
	}
	if got := gramReplies(as); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("reply\n got % X\nwant % X", got, want)
	}
	if len(m.boxes["bob"]) != 0 {
		t.Fatal("delivered despite an unknown name")
	}
	// the disc's one unknown name is its bare 9
	if s := m.send("Ann", "Zed", "s", "t"); s != gramUnknown {
		t.Fatalf("disc unknown: %d", s)
	}
}

// A live send some names did not get is 4: the names, as many reasons (10
// full, 11 too many unread from the sender) and whether anyone got it; one
// name alone too (a bare 10 reads "Unknown error." to the live client),
// then delivered 0. The disc's one name keeps its bare 10 and 11.
func TestGramLiveSendPartly(t *testing.T) {
	m, as, _, _ := mailWorld(t)
	for len(m.boxes["cid"]) < mailboxMax {
		m.send("Dee", "Cid", "s", "t")
		for _, g := range m.boxes["cid"] {
			g.read = true
		}
	}
	for i := 0; i < unreadPerSender; i++ {
		m.send("Ann", "Dee", "s", "t")
	}
	m.handleRequest(as, incoming(opcodeGramRequest, liveSend([]string{"Cid", "Bob", "Dee"}, "Hi", "x")))
	want := []byte{1, gramPartly, 4, 3, 0, 0, 0, 'C', 'i', 'd', 3, 0, 0, 0, 'D', 'e', 'e', 4, gramBoxFull, gramTooMany, 1}
	if got := gramReplies(as); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("reply\n got % X\nwant % X", got, want)
	}
	if len(m.boxes["bob"]) != 1 {
		t.Fatal("Bob did not get it")
	}
	m.handleRequest(as, incoming(opcodeGramRequest, liveSend([]string{"Cid"}, "Hi", "x")))
	want = []byte{1, gramPartly, 2, 3, 0, 0, 0, 'C', 'i', 'd', 2, gramBoxFull, 0}
	if got := gramReplies(as); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("one full box\n got % X\nwant % X", got, want)
	}
	if s := m.send("Ann", "Cid", "s", "t"); s != gramBoxFull {
		t.Fatalf("disc full: %d", s)
	}
	if s := m.send("Ann", "Dee", "s", "t"); s != gramTooMany {
		t.Fatalf("disc too many: %d", s)
	}
}

// Past gramRecipientsMax names a gram goes to the first ones, and the rest
// are names it did not go to, "Unknown error." (a reason neither 10 nor 11),
// unlooked-up; each copy's list is the names it went to.
func TestGramRecipientsCap(t *testing.T) {
	m, as, _, _ := mailWorld(t)
	names := fakeNames{}
	var to []string
	for i := 0; i < gramRecipientsMax+2; i++ {
		n := "P" + strings.Repeat("a", i+1)
		to = append(to, n)
		if i < gramRecipientsMax {
			names[strings.ToLower(n)] = true // the last two exist nowhere
		}
	}
	m.names = names
	m.handleRequest(as, incoming(opcodeGramRequest, liveSend(to, "Hi", "x")))
	got := gramReplies(as)
	if len(got) != 1 || got[0][1] != gramPartly {
		t.Fatalf("reply % X", got)
	}
	r := buffer.NewReader(got[0][2:])
	n, _ := r.Read7BitEncodedInt64()
	var failed []string
	for i := 0; i < int(n); i++ {
		s, _ := readString8(r)
		failed = append(failed, s)
	}
	k, _ := r.Read7BitEncodedInt64()
	reasons, _ := r.ReadBytes(int(k))
	delivered, _ := r.ReadByte()
	if strings.Join(failed, ",") != strings.Join(to[gramRecipientsMax:], ",") || !bytes.Equal(reasons, []byte{gramFailOther, gramFailOther}) || delivered != 1 {
		t.Fatalf("failed %v reasons %v delivered %d", failed, reasons, delivered)
	}
	box := m.boxes[strings.ToLower(to[0])]
	if len(box) != 1 || len(box[0].to) != gramRecipientsMax {
		t.Fatalf("first box %+v", box)
	}
}

// An open answers in its own form: the live one (no envelope) with the To
// line, the names joined with ";", before the text; the disc's the text
// alone. Both mark the gram read.
func TestGramOpenByForm(t *testing.T) {
	m, as, bs, _ := mailWorld(t)
	m.handleRequest(as, incoming(opcodeGramRequest, liveSend([]string{"Bob", "Cid"}, "Hi", "Body text")))
	id := int64(m.boxes["bob"][0].id)
	bs.Queue().GetPending()
	open := func(disc bool) []byte {
		m.handleRequest(bs, incoming(opcodeGramRequest, gramRequestBytes(3, disc, func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) })))
		got := gramReplies(bs)
		if len(got) != 1 {
			t.Fatalf("replies % X", got)
		}
		return got[0]
	}
	want := func(parts ...string) []byte {
		buf := make([]byte, 4096)
		w := buffer.NewWriter(buf)
		w.WriteByte(3)
		w.WriteByte(3)
		w.Write7BitEncodedInt64(id)
		for _, p := range parts {
			writeWideString(w, p, 1024)
		}
		return buf[:w.Position()]
	}
	if got := open(false); !bytes.Equal(got, want("Bob;Cid", "Body text")) {
		t.Fatalf("live open\n got % X\nwant % X", got, want("Bob;Cid", "Body text"))
	}
	if !m.boxes["bob"][0].read {
		t.Fatal("not marked read")
	}
	if got := open(true); !bytes.Equal(got, want("Body text")) {
		t.Fatalf("disc open\n got % X\nwant % X", got, want("Body text"))
	}
}
