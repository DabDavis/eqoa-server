package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

func entitySlotsSession() *session.Session {
	return session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 6007}, 7, 7, 1, 2, true)
}

// offers is every OpcodeEntityView queued for the session's client.
func offers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeEntityView {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the server's most entity slots and view distance,
// two u32s after the opcode; a server keeping CLIENT's 24 within 300 offers
// nothing, one keeping either past them offers both.
func TestEntityViewOfferedAtWorldEntry(t *testing.T) {
	defer SetMaxEntitySlots(0)
	defer SetMaxViewDistance(0)
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	entry := func() [][]byte {
		sess := entitySlotsSession()
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
		// a second 0x0014 (a respawn's) offers nothing again: the session
		// keeps what its client asked
		h.HandleCharacterInWorld(sess, in)
		return offers(sess)
	}

	SetMaxEntitySlots(0)
	SetMaxViewDistance(0)
	if got, want := entry(), []byte{0x00, 0x7E, 204, 0, 0, 0, 0xE8, 0x03, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
	SetMaxViewDistance(300)
	if got, want := entry(), []byte{0x00, 0x7E, 204, 0, 0, 0, 0x2C, 0x01, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("max_view_distance 300 offered %x, want %x", got, want)
	}
	SetMaxEntitySlots(24)
	if got := entry(); len(got) != 0 {
		t.Fatalf("24 within 300 offered %x", got)
	}
	SetMaxViewDistance(600)
	if got, want := entry(), []byte{0x00, 0x7E, 24, 0, 0, 0, 0x58, 0x02, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("24 within 600 offered %x, want %x", got, want)
	}
}

// The client's numbers are the session's, within 24 and 300 and the server's
// most; a message with the slots alone leaves the distance.
func TestHandleEntityView(t *testing.T) {
	defer SetMaxEntitySlots(0)
	defer SetMaxViewDistance(0)
	h := NewEntityViewHandler()
	ask := func(sess *session.Session, data []byte) int {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeEntityView)
		m.SetData(data)
		if err := h.HandleEntityView(sess, m); err != nil {
			t.Fatal(err)
		}
		return sess.Connection().EntitySlots()
	}
	u32 := func(n uint32) []byte { return binary.LittleEndian.AppendUint32(nil, n) }

	for _, c := range []struct {
		max  int
		ask  uint32
		want int
	}{
		{0, 100, 100},
		{0, 204, 204},
		{0, 1000, 204},
		{0, 0xFFFFFFFF, 204},
		{0, 10, 24},
		{64, 100, 64},
		{24, 100, 24},
		{300, 300, 204},
	} {
		SetMaxEntitySlots(c.max)
		if got := ask(entitySlotsSession(), u32(c.ask)); got != c.want {
			t.Errorf("server's most %d, asked %d: %d slots, want %d", c.max, c.ask, got, c.want)
		}
	}

	SetMaxEntitySlots(0)
	sess := entitySlotsSession()
	ask(sess, u32(128))
	if got := ask(sess, []byte{1, 2}); got != 128 {
		t.Fatalf("a short 0x7E00 changed the slots to %d", got)
	}
	if got := ask(sess, u32(40)); got != 40 {
		t.Fatalf("asked 40 after 128: %d", got)
	}

	view := func(max int, asked uint32) int {
		SetMaxViewDistance(max)
		sess := entitySlotsSession()
		ask(sess, append(u32(128), u32(asked)...))
		return sess.Connection().ViewDistance()
	}
	for _, c := range []struct {
		max  int
		ask  uint32
		want int
	}{
		{0, 600, 600},
		{0, 1000, 1000},
		{0, 5000, 1000},
		{0, 100, 300},
		{0, 0, 300},
		{500, 800, 500},
		{300, 800, 300},
	} {
		if got := view(c.max, c.ask); got != c.want {
			t.Errorf("server's farthest %d, asked %d: looked for to %d, want %d", c.max, c.ask, got, c.want)
		}
	}
	SetMaxViewDistance(0)
	if got := sess.Connection().ViewDistance(); got != session.ViewDistance {
		t.Fatalf("slots alone: looked for to %d, want the default %d", got, session.ViewDistance)
	}
}
