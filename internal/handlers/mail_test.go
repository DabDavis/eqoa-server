package handlers

import (
	"bytes"
	"encoding/binary"
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
