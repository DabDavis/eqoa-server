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

// The first 0x0014 offers the stat window's figures, a u8 1 (the layout)
// after the opcode, once (petition 2925).
func TestStatFiguresOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	var got [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeStatFigures {
			got = append(got, d)
		}
	}
	if want := []byte{0x84, 0x7E, 1}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
}

// The answer is the player's: 1 on, 0 off, a short one changes nothing, the
// registry routes 0x7E84 to it; an [OG] character's (its stat caps the
// game's) is taken as 0, and so is any under world.vanilla_ui.
func TestHandleStatFigures(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 29), Port: 6029}, 29, 29, 1, 2, true)
	p := &entity.Player{ServerID: 1029, Name: "Statter", HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewStatFiguresHandler(w)
	r := NewRegistry()
	h.RegisterHandlers(r)
	send := func(data []byte) {
		t.Helper()
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeStatFigures)
		m.SetData(data)
		if err := r.Handle(sess, m); err != nil {
			t.Fatal(err)
		}
	}
	send([]byte{1})
	if !p.StatFigures {
		t.Fatal("asked with 1: not on")
	}
	send(nil)
	if !p.StatFigures {
		t.Fatal("a short answer stopped it")
	}
	send([]byte{0})
	if p.StatFigures {
		t.Fatal("0: still on")
	}
	sess.SetOG(true)
	send([]byte{1})
	if p.StatFigures {
		t.Fatal("an [OG] character's answer taken")
	}
	sess.SetOG(false)
	vanillaUISwitch(t)
	send([]byte{1})
	if p.StatFigures {
		t.Fatal("world.vanilla_ui: an answer taken")
	}
}
