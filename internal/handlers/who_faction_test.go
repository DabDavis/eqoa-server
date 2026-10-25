package handlers

import (
	"strings"
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
	list := (&FactionHandler{}).standings(nil)
	total := factionPageCount(list)
	if total != 34 {
		t.Fatalf("%d pages, want 34", total)
	}
	seen := 0
	for p := int64(0); p < total; p++ {
		token, page, pages, count := read(factionPagePayload(list, 70, p))
		if token != 70 || page != p || pages != total {
			t.Errorf("page %d: token %d page %d pages %d", p, token, page, pages)
		}
		seen += int(count)
	}
	if seen != 331 {
		t.Errorf("%d factions over the pages, want the table's 331", seen)
	}
	if _, page, pages, count := read(factionPagePayload(list, 70, 40)); page != 40 || pages != total || count != 0 {
		t.Errorf("past the end: page %d pages %d count %d, want an empty answer", page, pages, count)
	}
}

// The window's pages are the character's own standings: a standing moved off
// the start is listed with its new number, in the window's order.
func TestFactionPagesAreTheCharactersOwn(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	if _, _, err := c.gw.SetFaction(alice, "Citizens of Freeport", -24); err != nil {
		t.Fatal(err)
	}
	h := NewFactionHandler(c.sm)
	h.SetGameWorld(c.gw)
	list := h.standings(alice)
	at := -1
	for i, f := range list {
		if f.Name == "Citizens of Freeport" {
			at = i
			if f.Standing != -24 {
				t.Errorf("listed at %d", f.Standing)
			}
		}
	}
	// among the capture's -1 to -99 standings: after Verthok the Executioner (-1)
	if at < 0 || list[at-1].Standing < -24 || list[at+1].Standing > -24 {
		t.Fatalf("Citizens of Freeport at %d of %d, between %v and %v", at, len(list), list[at-1], list[at+1])
	}
	req := make([]byte, 16)
	w := buffer.NewWriter(req)
	w.WriteByte(0x66)
	w.Write7BitEncodedInt64(3)
	w.Write7BitEncodedInt64(int64(at / factionsPerPage))
	if err := h.HandleClientFaction(alice, incoming(protocol.OpcodeClientFaction, req[:w.Position()])); err != nil {
		t.Fatal(err)
	}
	msgs := alice.Queue().PeekPending()
	page := msgs[len(msgs)-1].Data()
	if got := string(page); !containsWide(page, "Citizens of Freeport") {
		t.Errorf("page %d does not list it: %x", at/factionsPerPage, got)
	}
	// Bob, who killed nothing, still has the start's 1
	bob, _ := c.join(2, "Bob", 25000, 16000, 0)
	for _, f := range h.standings(bob) {
		if f.Name == "Citizens of Freeport" && f.Standing != 1 {
			t.Errorf("Bob's standing %d, want the start's 1", f.Standing)
		}
	}
}

// containsWide reports whether b holds s as UTF-16LE.
func containsWide(b []byte, s string) bool {
	w := make([]byte, 0, 2*len(s))
	for _, r := range s {
		w = append(w, byte(r), byte(r>>8))
	}
	return strings.Contains(string(b), string(w))
}

// 0x00C9's first byte is the Faction Messages option: 1 hides the lines, 0
// shows them again.
func TestFactionMessagesOption(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	h := NewFactionHandler(c.sm)
	h.SetGameWorld(c.gw)
	h.HandleFactionMessages(alice, incoming(protocol.OpcodeFactionMessages, []byte{1, 0}))
	if !alice.FactionMessagesOff() {
		t.Error("1 did not hide them")
	}
	h.HandleFactionMessages(alice, incoming(protocol.OpcodeFactionMessages, []byte{0, 1}))
	if alice.FactionMessagesOff() {
		t.Error("0 did not show them")
	}
}
