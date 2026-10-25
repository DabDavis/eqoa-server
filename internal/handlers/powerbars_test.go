package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// powerOffers is every OpcodePowerBars queued for the session's client.
func powerOffers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodePowerBars {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers others' power, a u32 1 after the opcode, once.
func TestPowerBarsOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := powerOffers(sess), []byte{0x01, 0x7E, 1, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
}

// The client's answer is its player's: nonzero has the bars, 0 stops them;
// a short one or a session with no player changes nothing.
func TestHandlePowerBars(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 8), Port: 6008}, 8, 8, 1, 2, true)
	p := &entity.Player{ServerID: 1001, Name: "Asker"}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewPowerBarsHandler(w)
	answer := func(s *session.Session, data []byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePowerBars)
		m.SetData(data)
		if err := h.HandlePowerBars(s, m); err != nil {
			t.Fatal(err)
		}
	}
	answer(sess, []byte{1, 0, 0, 0})
	if !p.PowerBars {
		t.Fatal("asked with 1: no bars")
	}
	answer(sess, []byte{0, 0})
	if !p.PowerBars {
		t.Fatal("a short answer stopped them")
	}
	answer(sess, []byte{0, 0, 0, 0})
	if p.PowerBars {
		t.Fatal("asked with 0: still on")
	}
	nobody := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 6009}, 9, 9, 1, 2, true)
	answer(nobody, []byte{1, 0, 0, 0})
}
