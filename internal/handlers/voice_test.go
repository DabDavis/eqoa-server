package handlers

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/voice"
)

// opsQueued is the opcodes queued for s, in order.
func opsQueued(s *session.Session) []protocol.GameOpcode {
	var out []protocol.GameOpcode
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, protocol.GameOpcode(binary.LittleEndian.Uint16(d)))
		}
	}
	return out
}

// A server with voice offers it at the world entry (0x7E10, a u32 1), one
// without offers nothing; a client that never answers (a PS2) is sent
// nothing more however long it plays, and one that answers 1 gets its ICE
// servers and its roster.
func TestVoiceIsOfferedAndOnlySentToAClientThatAsks(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	srv, err := voice.New(voice.Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	tw.w.SetVoice(entity.VoiceSettings{SFU: srv})
	defer SetVoiceOffered(false)

	SetVoiceOffered(false)
	SendVoiceOffer(tw.s)
	if got := opsQueued(tw.s); len(got) != 0 {
		t.Fatalf("a server without voice offered: %v", got)
	}
	SetVoiceOffered(true)
	SendVoiceOffer(tw.s)
	msgs := tw.s.Queue().GetPending()
	if len(msgs) != 1 || protocol.GameOpcode(binary.LittleEndian.Uint16(msgs[0].Data())) != protocol.OpcodeVoiceOffer ||
		binary.LittleEndian.Uint32(msgs[0].Data()[2:]) != 1 {
		t.Fatalf("the offer: %d messages", len(msgs))
	}
	for i := 0; i < 50; i++ {
		tw.w.Tick(time.Now())
	}
	for _, op := range opsQueued(tw.s) {
		if op >= protocol.OpcodeVoiceOffer && op <= protocol.OpcodeVoiceRoomState {
			t.Fatalf("a client that never asked was sent %v", op)
		}
	}
	tw.s.Queue().GetPending()

	h := NewVoiceHandler(tw.w)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeVoiceOffer)
	msg.SetData(build(protocol.OpcodeVoiceOffer, 6, func(w *buffer.Writer) { w.WriteUint32(1) })[2:])
	if err := h.HandleVoiceOn(tw.s, msg); err != nil {
		t.Fatal(err)
	}
	got := map[protocol.GameOpcode]int{}
	for _, op := range opsQueued(tw.s) {
		got[op]++
	}
	if got[protocol.OpcodeVoiceConfig] != 1 || got[protocol.OpcodeVoiceRoster] != 1 {
		t.Fatalf("voice on: sent %v, want the ICE servers and the roster", got)
	}
}

// A room's name is refused for a word the chat masks or a root a
// character's name may not carry; fair names that hold such letters pass.
func TestProfaneRoomName(t *testing.T) {
	for _, n := range []string{"Fuck Club", "the ass room", "Shitlord Hall", "BITCHES"} {
		if !ProfaneRoomName(n) {
			t.Errorf("%q passed", n)
		}
	}
	for _, n := range []string{"Bass Players", "Assist Squad", "Hancock's Inn", "Cockatrice Hunters", "Freeport Docks"} {
		if ProfaneRoomName(n) {
			t.Errorf("%q refused", n)
		}
	}
}
