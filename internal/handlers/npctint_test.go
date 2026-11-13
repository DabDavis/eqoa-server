package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// queuedOf is every message of op queued for the session's client, opcode
// first.
func queuedOf(sess *session.Session, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == op {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the NPC tints, a u8 2 (the newest layout: the glows
// since petition 1893) after the opcode, once, and sends no tint: only an answer does; the second 0x0014
// offers nothing more.
func TestNPCTintsOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queuedOf(sess, protocol.OpcodeNPCTintsOffer), []byte{0x3E, 0x7E, 2}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
	if sess.NPCTints() || len(queuedOf(sess, protocol.OpcodeNPCTints)) != 0 {
		t.Fatal("tints before any answer")
	}
}

// A client that answers 1 is sent the tints at once: Yhafiran's, by his name's
// key, and not the untinted twin's; a u32's 1 the same by its first byte; an
// answer of 0 stops them, and the next change goes to it no more. A PS2's
// session in the same world, which never answers, is sent nothing through all
// of it. The answer comes through the registry as 0x7E3F.
func TestNPCTintsAnswered(t *testing.T) {
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	w.SetNPCTints([]database.NPCTint{{Name: "yhafiran", Color: 0x558C50FF}})
	van := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 40), Port: 6040}, 40, 40, 1, 2, true)
	ps2 := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 41), Port: 6041}, 41, 41, 1, 2, true)
	r := NewRegistry()
	NewNPCTintsHandler(w).RegisterHandlers(r)
	answer := func(data ...byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNPCTintsAnswer)
		m.SetData(data)
		if err := r.Handle(van, m); err != nil {
			t.Fatal(err)
		}
	}
	answer(1)
	got := queuedOf(van, protocol.OpcodeNPCTints)
	want := []byte{0x40, 0x7E, 1, 0, 8, 'y', 'h', 'a', 'f', 'i', 'r', 'a', 'n', 0xFF, 0x50, 0x8C, 0x55}
	if !van.NPCTints() || len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("answered 1: % x, want % x", got, want)
	}
	// the same answer again: it dropped what it held, so it is sent them again
	answer(1, 0, 0, 0)
	if len(queuedOf(van, protocol.OpcodeNPCTints)) != 2 {
		t.Fatal("a u32 1: not sent them again")
	}
	answer()
	if !van.NPCTints() {
		t.Fatal("an empty answer stopped them")
	}
	answer(0)
	w.SetNPCTints([]database.NPCTint{{Name: "uzrathixius", Color: 0x404040FF}})
	if van.NPCTints() || len(queuedOf(van, protocol.OpcodeNPCTints)) != 2 {
		t.Fatal("answered 0: still sent them")
	}
	if len(queuedOf(ps2, protocol.OpcodeNPCTints)) != 0 || ps2.NPCTints() {
		t.Fatal("the PS2 was sent tints")
	}
}

// The GM Jibrish's petition 1893 (Mr. Moogie's whole body in a soft red glow): a client that answers 2 is sent
// layout 2, each name's colour then its glow, a name of a glow alone in it; one that answers 1 (a vanilla before
// the glow, which answers 1 to any offer) the tints alone as before, the glow's name left out; an answer past
// the newest is the newest.
func TestNPCTintsLayoutTwoCarriesTheGlow(t *testing.T) {
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	w.SetNPCTints([]database.NPCTint{{Name: "mr. moogie", Color: 0xFFFFFFFF, Glow: 0xFF606099}, {Name: "yhafiran", Color: 0x558C50FF}})
	glow := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 50), Port: 6050}, 50, 50, 1, 2, true)
	old := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 51), Port: 6051}, 51, 51, 1, 2, true)
	far := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 52), Port: 6052}, 52, 52, 1, 2, true)
	r := NewRegistry()
	NewNPCTintsHandler(w).RegisterHandlers(r)
	answer := func(s *session.Session, data ...byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNPCTintsAnswer)
		m.SetData(data)
		if err := r.Handle(s, m); err != nil {
			t.Fatal(err)
		}
	}
	answer(glow, 2)
	answer(old, 1)
	answer(far, 9)
	moogie := append([]byte{10}, "mr. moogie"...)
	yh := append([]byte{8}, "yhafiran"...)
	want2 := append(append(append(append([]byte{0x40, 0x7E, 2, 0}, moogie...), 0xFF, 0xFF, 0xFF, 0xFF, 0x99, 0x60, 0x60, 0xFF), yh...),
		0xFF, 0x50, 0x8C, 0x55, 0, 0, 0, 0)
	want1 := append(append([]byte{0x40, 0x7E, 1, 0}, yh...), 0xFF, 0x50, 0x8C, 0x55)
	if got := queuedOf(glow, protocol.OpcodeNPCTints); len(got) != 1 || !bytes.Equal(got[0], want2) || glow.NPCTintsLayout() != 2 {
		t.Fatalf("answered 2: % x, want % x", got, want2)
	}
	if got := queuedOf(old, protocol.OpcodeNPCTints); len(got) != 1 || !bytes.Equal(got[0], want1) || old.NPCTintsLayout() != 1 {
		t.Fatalf("answered 1: % x, want % x (the glow left out)", got, want1)
	}
	if far.NPCTintsLayout() != 2 {
		t.Errorf("answered 9: layout %d, want the newest, 2", far.NPCTintsLayout())
	}
}
