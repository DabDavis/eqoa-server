package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// numberTagsOffers is every OpcodeNumberTagsOffer queued for the session's
// client.
func numberTagsOffers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNumberTagsOffer {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the number tags, a u8 2 (the layout) after the
// opcode, once; the second 0x0014 offers nothing more.
func TestNumberTagsOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := numberTagsOffers(sess), []byte{0x38, 0x7E, 2}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
	if sess.NumberTags() {
		t.Fatal("tagged before any answer")
	}
}

// The client's answer is its session's: a u8 1 has its figures tagged, 0
// stops them, a u32's 1 and 0 the same by their first byte; an empty one
// changes nothing.
func TestHandleNumberTags(t *testing.T) {
	sess := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 38), Port: 6038}, 38, 38, 1, 2, true)
	h := NewNumberTagsHandler()
	answer := func(data []byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNumberTagsAnswer)
		m.SetData(data)
		if err := h.HandleNumberTags(sess, m); err != nil {
			t.Fatal(err)
		}
	}
	answer([]byte{1})
	if !sess.NumberTags() || sess.NumberTagsLayout() != 1 {
		t.Fatalf("answered 1 (an older vanilla): layout %d", sess.NumberTagsLayout())
	}
	answer([]byte{2})
	if sess.NumberTagsLayout() != 2 {
		t.Fatalf("answered 2: layout %d", sess.NumberTagsLayout())
	}
	answer([]byte{9})
	if sess.NumberTagsLayout() != 2 {
		t.Fatalf("answered a later layout: %d, want the newest told (2)", sess.NumberTagsLayout())
	}
	answer([]byte{1})
	if sess.NumberTagsLayout() != 1 {
		t.Fatalf("answered 1 again: layout %d", sess.NumberTagsLayout())
	}
	answer(nil)
	if !sess.NumberTags() {
		t.Fatal("an empty answer stopped them")
	}
	answer([]byte{0})
	if sess.NumberTags() {
		t.Fatal("answered 0: still tagged")
	}
	answer([]byte{1, 0, 0, 0})
	if !sess.NumberTags() {
		t.Fatal("a u32 1: not tagged")
	}
	answer([]byte{0, 0, 0, 0})
	if sess.NumberTags() {
		t.Fatal("a u32 0: still tagged")
	}
	// the registry routes 0x7E39 to it
	r := NewRegistry()
	h.RegisterHandlers(r)
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNumberTagsAnswer)
	m.SetData([]byte{1})
	if err := r.Handle(sess, m); err != nil || !sess.NumberTags() {
		t.Fatalf("through the registry: %v, tagged %v", err, sess.NumberTags())
	}
}
