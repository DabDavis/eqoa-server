package handlers

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
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

// A server with voice offers it at the world entry (0x7E06, a u32 1), one
// without offers nothing; a client that never answers (a PS2) is sent
// nothing more however long it plays, and one that answers 1 gets its
// credentials and its peers.
func TestVoiceIsOfferedAndOnlySentToAClientThatAsks(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	tw.w.SetVoice(entity.VoiceSettings{TurnURLs: []string{"turn:203.0.113.5:3479?transport=udp"}, Secret: "a-test-secret-only"})
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
		if op == protocol.OpcodeVoiceConfig || op == protocol.OpcodeVoicePeers || op == protocol.OpcodeVoiceSignal {
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
	got := opsQueued(tw.s)
	if len(got) != 2 || got[0] != protocol.OpcodeVoiceConfig || got[1] != protocol.OpcodeVoicePeers {
		t.Fatalf("voice on: sent %v, want the config and the peers", got)
	}
}
