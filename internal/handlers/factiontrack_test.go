package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// factionTrackOffers is every OpcodeFactionTrackOffer queued for the session.
func factionTrackOffers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeFactionTrackOffer {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the tracker, layout 1 after the opcode, once; on
// world.vanilla_ui none is offered.
func TestFactionTrackOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := factionTrackOffers(sess), []byte{0x86, 0x7E, 1}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
	SetVanillaUI(true)
	defer SetVanillaUI(false)
	sess = entitySlotsSession()
	if err := h.HandleCharacterInWorld(sess, in); err != nil {
		t.Fatal(err)
	}
	if got := factionTrackOffers(sess); len(got) != 0 {
		t.Fatalf("vanilla_ui offered %x", got)
	}
}

// wideAnswer is OpcodeFactionTrackAnswer's body for s.
func wideAnswer(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(u)))
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

// The answer is its player's: a faction's name in any case is followed, an
// empty one follows none, a cut-short one changes nothing; on world.vanilla_ui
// an answer is taken as none.
func TestHandleFactionTrack(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 59), Port: 6059}, 59, 59, 1, 2, true)
	p := &entity.Player{ServerID: 1059, Name: "Tracker"}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewFactionTrackHandler(w)
	send := func(data []byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeFactionTrackAnswer)
		m.SetData(data)
		if err := h.HandleFactionTrack(sess, m); err != nil {
			t.Fatal(err)
		}
	}
	send(wideAnswer("citizens of freeport"))
	if got := w.FactionTrack(sess); got != "citizens of freeport" {
		t.Fatalf("followed %q", got)
	}
	send([]byte{9, 0, 0})
	if got := w.FactionTrack(sess); got != "citizens of freeport" {
		t.Fatalf("a cut-short answer changed it to %q", got)
	}
	send(wideAnswer(""))
	if got := w.FactionTrack(sess); got != "" {
		t.Fatalf("an empty answer kept %q", got)
	}
	SetVanillaUI(true)
	defer SetVanillaUI(false)
	send(wideAnswer("Citizens of Freeport"))
	if got := w.FactionTrack(sess); got != "" {
		t.Fatalf("vanilla_ui followed %q", got)
	}
}
