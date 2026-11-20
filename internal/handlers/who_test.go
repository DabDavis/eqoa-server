package handlers

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The user, 2026-11-07: "lets allow anyone to use /who". A player who is no GM gets everyone in
// the world by name, level and class, in name order, and how many, never where they stand; so do
// #who and a PS2's "/t who". It says nothing to anyone else.
func TestAnyoneMayUseWho(t *testing.T) {
	for _, cmd := range []string{"/who", "#who", "/WHO all", "/t who"} {
		c := newChatWorld(t)
		bob, _ := c.join(2, "bob", 25050, 16000, 0)
		alice, _ := c.join(1, "Alice", 25000, 16000, 0)
		zed, _ := c.join(3, "Zed", 25500, 16000, 1)
		c.say(alice, cmd)
		got := texts(heard(t, alice))
		if len(got) != 4 || !strings.HasPrefix(got[0], "Alice: level ") || !strings.HasPrefix(got[1], "bob: level ") ||
			!strings.HasPrefix(got[2], "Zed: level ") || got[3] != "3 players are in the world." {
			t.Errorf("%q: %q", cmd, got)
		}
		for _, l := range got {
			if strings.Contains(l, "(") || strings.Contains(l, "[GM]") {
				t.Errorf("%q: a place or a GM line: %q", cmd, l)
			}
		}
		if h := heard(t, bob); len(h) != 0 {
			t.Errorf("%q: bob heard %+v", cmd, h)
		}
		if h := heard(t, zed); len(h) != 0 {
			t.Errorf("%q: zed heard %+v", cmd, h)
		}
	}
}

// A tell to a GM by its name as it is drawn ("[GM] Name") goes to that GM; the control, a
// plain name, is as it was.
func TestATellReadsAGMsTag(t *testing.T) {
	for line, want := range map[string][2]string{
		"/t [GM] Jinkles hello there": {"Jinkles", "hello there"},
		"/tell [gm] Jinkles hi":       {"Jinkles", "hi"},
		"/t Jinkles hi":               {"Jinkles", "hi"},
	} {
		name, text, ok := parseTell(line)
		if !ok || name != want[0] || text != want[1] {
			t.Errorf("%q: %q %q %v", line, name, text, ok)
		}
	}
}

// askWho sends sess's who window request (0x0E01 1, token) and returns what it was sent for it.
func askWho(t *testing.T, c *chatWorld, sess *session.Session, token int64) [][]byte {
	t.Helper()
	h := NewWhoHandler(c.sm)
	h.SetGameWorld(c.gw)
	req := make([]byte, 16)
	w := buffer.NewWriter(req)
	w.WriteByte(1)
	w.Write7BitEncodedInt64(token)
	before := len(sess.Queue().PeekPending())
	if err := h.HandleWhoList(sess, incoming(protocol.OpcodeWhoList, req[:w.Position()])); err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, m := range sess.Queue().PeekPending()[before:] {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// whoNames reads a 0x0E02's rows' names.
func whoNames(t *testing.T, d []byte) []string {
	t.Helper()
	if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeWhoListResponse {
		t.Fatalf("not 0x0E02: % X", d[:2])
	}
	r := buffer.NewReader(d[2:])
	r.ReadByte()
	r.Read7BitEncodedInt64()
	n, _ := r.Read7BitEncodedInt64()
	var names []string
	for i := int64(0); i < n; i++ {
		r.ReadByte()
		r.ReadByte()
		r.ReadByte()
		ln, _ := r.ReadUint32()
		b, _ := r.ReadBytes(int(ln))
		r.ReadUint32()
		names = append(names, string(b))
	}
	return names
}

// The owner, 2026-11-20: "put players online in who menu in game". CLIENT's who window draws
// nothing the server fills but its rows (VIWndWhoList::OnDrawSelf), so the count comes as a
// system line (0x0A7A, which every client draws in its chat, a PS2's in the five lines under
// the window) after the list, the list itself the bytes a retail PS2 was always sent.
func TestWhoWindowSaysHowManyAreOnline(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	_, pb := c.join(2, "Bob", 25050, 16000, 0)
	_, pz := c.join(3, "Zed", 25500, 16000, 1)
	pa.Race, pa.Class, pa.Level = 1, 13, 20
	pb.Race, pb.Class, pb.Level = 4, 0, 7
	pz.Race, pz.Class, pz.Level = 9, 6, 60

	got := askWho(t, c, alice, 70)
	if len(got) != 2 {
		t.Fatalf("%d messages, want the list and the line", len(got))
	}
	// the list as it always was: u8 1, the token and the count as zigzags, then each row
	// (u8 race, class, level, the name, the id the asker knows the player by), the
	// world's order
	var rows []whoEntry
	for _, p := range []*entity.Player{pa, pb, pz} {
		rows = append(rows, whoEntry{p.Race, p.Class, p.Level, p.Name, c.gw.IDSeenBy(alice, p)})
	}
	order := whoNames(t, got[0])
	sort.Slice(rows, func(i, j int) bool { return whoIndex(order, rows[i].Name) < whoIndex(order, rows[j].Name) })
	want := make([]byte, 256)
	w := buffer.NewWriter(want)
	w.WriteUint16(uint16(protocol.OpcodeWhoListResponse))
	w.WriteByte(1)
	w.Write7BitEncodedInt64(70)
	w.Write7BitEncodedInt64(3)
	for _, r := range rows {
		w.WriteByte(byte(entity.WireRace(r.Race)))
		w.WriteByte(byte(r.Class))
		w.WriteByte(byte(r.Level))
		w.WriteUint32(uint32(len(r.Name)))
		w.WriteBytes([]byte(r.Name))
		w.WriteUint32(r.ID)
	}
	if !bytes.Equal(got[0], want[:w.Position()]) {
		t.Errorf("the list changed:\n got % X\nwant % X", got[0], want[:w.Position()])
	}
	if op := protocol.GameOpcode(binary.LittleEndian.Uint16(got[1])); op != protocol.OpcodeClientMessage {
		t.Fatalf("the count came as %04X, want a system line", uint16(op))
	}
	if line, _ := readWideLine(got[1][2:]); line != "3 players online." {
		t.Errorf("count line %q", line)
	}
	if !bytes.Equal(got[1], buildClientMessage("3 players online.").Data()) {
		t.Errorf("not 0x0A7A's own bytes: % X", got[1])
	}

	// one alone is "player"
	c2 := newChatWorld(t)
	solo, _ := c2.join(1, "Solo", 25000, 16000, 0)
	if line, _ := readWideLine(askWho(t, c2, solo, 1)[1][2:]); line != "1 player online." {
		t.Errorf("alone: %q", line)
	}
}

func whoIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// A GM hidden by Toggle Invis is neither a row nor counted in another player's who window or
// /who; the GM itself and other GMs see every player, and one who is not hidden is shown.
func TestWhoHidesAHiddenGM(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	gm, pg := c.join(2, "Watcher", 25050, 16000, 0)
	other, po := c.join(3, "Keeper", 25500, 16000, 0)
	pg.GM, po.GM = true, true
	pg.GMHidden = true

	line := func(d []byte) string { s, _ := readWideLine(d[2:]); return s }
	got := askWho(t, c, alice, 1)
	if names := whoNames(t, got[0]); whoIndex(names, "Watcher") >= 0 || len(names) != 2 {
		t.Errorf("a player's list: %q", names)
	}
	if l := line(got[1]); l != "2 players online." {
		t.Errorf("a player's count: %q", l)
	}
	for _, s := range []*session.Session{gm, other} {
		got := askWho(t, c, s, 1)
		if names := whoNames(t, got[0]); len(names) != 3 {
			t.Errorf("a GM's list: %q", names)
		}
		if l := line(got[1]); l != "3 players online." {
			t.Errorf("a GM's count: %q", l)
		}
	}

	c.say(alice, "/who")
	for _, l := range texts(heard(t, alice)) {
		if strings.Contains(l, "Watcher") {
			t.Errorf("/who showed the hidden GM: %q", l)
		}
	}
	if h := texts(heard(t, alice)); len(h) == 0 || h[len(h)-1] != "2 players are in the world." {
		t.Errorf("/who: %q", h)
	}

	pg.GMHidden = false
	if l := line(askWho(t, c, alice, 2)[1]); l != "3 players online." {
		t.Errorf("seen again: %q", l)
	}
}
