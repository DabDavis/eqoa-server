package gmweb

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A GM's npctint row is the game's at once (entity/npctint.go): the client in
// the world that draws tints is sent the set with the name in it, in any case
// the GM typed it, and again at a change of colour; the row deleted, the
// empty set. A row that names no one, or a colour past 32 bits, is refused
// and the clients are told nothing. The control: the PS2's session beside it
// is sent nothing through all of it.
func TestTintEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	online := func(n int) *session.Session {
		sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9200 + n}, uint32(n), uint32(n), 1, 2, true)
		sess.SetCharacterInWorld(true)
		return sess
	}
	van, ps2 := online(1), online(2)
	van.SetNPCTints(true)
	tints := func(s *session.Session) [][]byte {
		var out [][]byte
		for _, m := range s.Queue().PeekPending() {
			if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNPCTints {
				out = append(out, d[2:])
			}
		}
		return out
	}

	code, out := c.op("insert_row", map[string]any{"table": "npctint",
		"values": map[string]any{"name": "Yhafiran", "color": 0x558C50FF, "note": "the test's"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if got := tints(van); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 0, 8, 'y', 'h', 'a', 'f', 'i', 'r', 'a', 'n', 0xFF, 0x50, 0x8C, 0x55}) {
		t.Fatalf("after the insert: % x", got)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "1 client") {
		t.Errorf("live: %v", out["live"])
	}
	if w := f.world.NPCTints(); len(w) != 1 || w[0].Name != "yhafiran" {
		t.Errorf("the world holds %v", w)
	}

	if code, out := c.op("update_row", map[string]any{"table": "npctint", "key": map[string]any{"name": "Yhafiran"},
		"values": map[string]any{"color": 0x203020FF}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if got := tints(van); len(got) != 2 || !bytes.HasSuffix(got[1], []byte{0xFF, 0x20, 0x30, 0x20}) {
		t.Fatalf("after the update: % x", got)
	}

	for _, bad := range []map[string]any{{"name": "  ", "color": 0x558C50FF}, {"name": "Poxathixius", "color": float64(1 << 33)}} {
		if code, out := c.op("insert_row", map[string]any{"table": "npctint", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if len(tints(van)) != 2 {
		t.Errorf("a refused row told the clients something: %d messages", len(tints(van)))
	}

	if code, out := c.op("delete_row", map[string]any{"table": "npctint", "key": map[string]any{"name": "Yhafiran"}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if got := tints(van); len(got) != 3 || !bytes.Equal(got[2], []byte{0, 0}) {
		t.Fatalf("after the delete: % x", got)
	}
	if len(tints(ps2)) != 0 {
		t.Fatalf("the PS2 was sent %d tint messages", len(tints(ps2)))
	}
}
