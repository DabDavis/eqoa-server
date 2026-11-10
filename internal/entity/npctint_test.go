package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// tintsSent is the body of every OpcodeNPCTints queued for s, in order.
func tintsSent(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNPCTints {
			out = append(out, d[2:])
		}
	}
	return out
}

// The message is a u16 count, then each tint's name (a u8 byte count and the
// key) and its u32 colour, little-endian; a name past the entity channel's 24
// bytes is cut to them; one that tints nothing, and a second that cuts to one
// already sent, are left out; an empty set is its count alone.
func TestEncodeNPCTints(t *testing.T) {
	got := EncodeNPCTints([]database.NPCTint{{Name: "yhafiran", Color: 0x558C50FF}})
	want := []byte{1, 0, 8, 'y', 'h', 'a', 'f', 'i', 'r', 'a', 'n', 0xFF, 0x50, 0x8C, 0x55}
	if !bytes.Equal(got, want) {
		t.Fatalf("Yhafiran: % x, want % x", got, want)
	}
	long := "an exceptionally long creature name"
	got = EncodeNPCTints([]database.NPCTint{
		{Name: long, Color: 0x80808080},
		{Name: long + " two", Color: 0x102030FF},
		{Name: "moss snake", Color: 0x80808000},
	})
	cut := long[:24]
	want = append(append([]byte{1, 0, 24}, cut...), 0x80, 0x80, 0x80, 0x80)
	if !bytes.Equal(got, want) {
		t.Fatalf("the cut: % x, want % x", got, want)
	}
	if got := EncodeNPCTints(nil); !bytes.Equal(got, []byte{0, 0}) {
		t.Fatalf("none: % x", got)
	}
	if TintWireName("ends in a space after the cut x") != "ends in a space after th" || TintWireName("Lady Vox$") != "lady vox" {
		t.Errorf("wire names: %q %q", TintWireName("ends in a space after the cut x"), TintWireName("Lady Vox$"))
	}
}

// The tints go to the clients that asked for them and no other: the session
// that answered 1 is sent the set (the tinted name in it, the untinted NPC's
// not), the session that never answered (a PS2) nothing, ever; the same set
// is not sent twice; a change goes at once; the table emptied, the client
// that held tints is sent the empty set; a client that asks while there are
// none is sent nothing until there are some.
func TestNPCTintsGoOnlyToClientsThatAsk(t *testing.T) {
	w := NewWorld()
	w.SessionManager = session.NewManager()
	at := func(n int) *session.Session {
		return w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(n)), Port: 6000 + n}, uint32(n), uint32(n), 1, 2, true)
	}
	ps2, van := at(1), at(2)
	van.SetNPCTints(true)
	if n := w.SetNPCTints([]database.NPCTint{{Name: "yhafiran", Color: 0x558C50FF}}); n != 1 {
		t.Fatalf("told %d clients, want 1", n)
	}
	sent := tintsSent(van)
	if len(sent) != 1 || !bytes.Contains(sent[0], []byte("yhafiran")) || bytes.Contains(sent[0], []byte("uzrathixius")) {
		t.Fatalf("the client that asked: %q", sent)
	}
	if len(tintsSent(ps2)) != 0 {
		t.Fatal("the PS2 was sent tints")
	}
	if w.RefreshNPCTints() != 0 || len(tintsSent(van)) != 1 {
		t.Fatal("the same set sent again")
	}
	w.SetNPCTints([]database.NPCTint{{Name: "uzrathixius", Color: 0x404040FF}, {Name: "yhafiran", Color: 0x558C50FF}})
	if sent := tintsSent(van); len(sent) != 2 || !bytes.Contains(sent[1], []byte("uzrathixius")) {
		t.Fatalf("after a change: %q", sent)
	}
	w.SetNPCTints(nil)
	if sent := tintsSent(van); len(sent) != 3 || !bytes.Equal(sent[2], []byte{0, 0}) {
		t.Fatalf("emptied: %q", sent)
	}
	late := at(3)
	late.SetNPCTints(true)
	if w.RefreshNPCTints(late) != 0 || len(tintsSent(late)) != 0 {
		t.Fatal("an empty set sent to a client that holds none")
	}
	w.SetNPCTints([]database.NPCTint{{Name: "yhafiran", Color: 0x558C50FF}})
	if len(tintsSent(late)) != 1 || len(tintsSent(van)) != 4 || len(tintsSent(ps2)) != 0 {
		t.Fatalf("the tints again: late %d, van %d, ps2 %d", len(tintsSent(late)), len(tintsSent(van)), len(tintsSent(ps2)))
	}
	// a client that stops is sent nothing more
	van.SetNPCTints(false)
	w.SetNPCTints([]database.NPCTint{{Name: "yhafiran", Color: 0x203020FF}})
	if len(tintsSent(van)) != 4 {
		t.Fatal("sent after it stopped")
	}
	if s := w.NPCTintSummary(); !strings.Contains(s, "yhafiran 203020FF") {
		t.Errorf("summary: %s", s)
	}
}
