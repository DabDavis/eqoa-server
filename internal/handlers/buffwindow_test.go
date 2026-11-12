package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The first 0x0014 offers the buff window, a u8 1 after the opcode, once (petition 1761).
func TestBuffWindowOfferedAtWorldEntry(t *testing.T) {
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
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeBuffWindow {
			got = append(got, d)
		}
	}
	if want := []byte{0x57, 0x7E, 1}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
}

// The answer is the player's (a short one changes nothing); a removal takes a beneficial
// effect off, and a short one nothing.
func TestHandleBuffWindowAndRemove(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 28), Port: 6028}, 28, 28, 1, 2, true)
	p := &entity.Player{ServerID: 1028, Name: "Clicker", HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewBuffWindowHandler(w)
	send := func(op protocol.GameOpcode, data []byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, op)
		m.SetData(data)
		var err error
		if op == protocol.OpcodeBuffWindow {
			err = h.HandleBuffWindow(sess, m)
		} else {
			err = h.HandleBuffRemove(sess, m)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	send(protocol.OpcodeBuffWindow, []byte{1})
	if !p.BuffWindow {
		t.Fatal("asked with 1: not on")
	}
	send(protocol.OpcodeBuffWindow, nil)
	if !p.BuffWindow {
		t.Fatal("a short answer stopped it")
	}
	p.Effects = []*entity.ActiveEffect{{SpellID: 42, Name: "Strength", Icon: 7, Until: time.Now().Add(time.Hour), Mods: entity.StatMods{Str: 10}}}
	send(protocol.OpcodeBuffRemove, []byte{0, 42, 0, 0, 0, 7, 0, 0})
	if len(p.Effects) != 1 {
		t.Fatal("a short removal took it off")
	}
	send(protocol.OpcodeBuffRemove, []byte{0, 42, 0, 0, 0, 7, 0, 0, 0})
	if len(p.Effects) != 0 {
		t.Fatal("the removal left it on")
	}
}
