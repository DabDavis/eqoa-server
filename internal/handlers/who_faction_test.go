package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// 0x0E02 echoes the request's token as a zigzag; it was a raw byte, which
// broke from the 64th request.
func TestWhoListTokenIsZigzag(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	h := NewWhoHandler(c.sm)
	h.SetGameWorld(c.gw)
	for _, token := range []int64{1, 63, 64, 300} {
		req := make([]byte, 16)
		w := buffer.NewWriter(req)
		w.WriteByte(1)
		w.Write7BitEncodedInt64(token)
		before := len(alice.Queue().PeekPending())
		if err := h.HandleWhoList(alice, incoming(protocol.OpcodeWhoList, req[:w.Position()])); err != nil {
			t.Fatal(err)
		}
		msgs := alice.Queue().PeekPending()
		if len(msgs) != before+1 {
			t.Fatalf("token %d: no answer", token)
		}
		r := buffer.NewReader(msgs[len(msgs)-1].Data()[2:])
		one, _ := r.ReadByte()
		got, _ := r.Read7BitEncodedInt64()
		n, _ := r.Read7BitEncodedInt64()
		if one != 1 || got != token || n != 1 {
			t.Errorf("token %d: answered 1=%d token=%d count=%d", token, one, got, n)
		}
	}
}

// Every faction page is answered, the empty and the past-the-end too, with
// the total; the page is a zigzag.
func TestFactionPagesAnswerEveryRequest(t *testing.T) {
	read := func(b []byte) (token, page, pages, count int64) {
		r := buffer.NewReader(b[2:])
		if m, _ := r.ReadByte(); m != 0x77 {
			t.Fatalf("marker %02X", m)
		}
		token, _ = r.Read7BitEncodedInt64()
		page, _ = r.Read7BitEncodedInt64()
		pages, _ = r.Read7BitEncodedInt64()
		count, _ = r.Read7BitEncodedInt64()
		return
	}
	total := factionPageCount()
	if total != 34 {
		t.Fatalf("%d pages, want 34", total)
	}
	seen := 0
	for p := int64(0); p < total; p++ {
		token, page, pages, count := read(factionPagePayload(70, p))
		if token != 70 || page != p || pages != total {
			t.Errorf("page %d: token %d page %d pages %d", p, token, page, pages)
		}
		seen += int(count)
	}
	if seen != 331 {
		t.Errorf("%d factions over the pages, want the table's 331", seen)
	}
	if _, page, pages, count := read(factionPagePayload(70, 40)); page != 40 || pages != total || count != 0 {
		t.Errorf("past the end: page %d pages %d count %d, want an empty answer", page, pages, count)
	}
}
