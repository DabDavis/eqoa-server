package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
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
