package handlers

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// face is the disposition, title, guild and token of the 0x0010s queued for s, the last.
func pvpFaces(s *session.Session) (faces []byte, title, guild string, token uint32) {
	fixed := func(b []byte) string {
		u := make([]uint16, 0, 64)
		for i := 0; i < 64; i++ {
			c := binary.LittleEndian.Uint16(b[2*i:])
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		return string(utf16.Decode(u))
	}
	for _, d := range queued(s, protocol.OpcodeClassicTargetInfo) {
		faces = append(faces, d[2])
		title, guild = fixed(d[5:]), fixed(d[5+128:])
		token = binary.LittleEndian.Uint32(d[5+256:])
	}
	return faces, title, guild, token
}

func hasText(ls []line, want string) bool {
	for _, l := range ls {
		if strings.Contains(l.text, want) {
			return true
		}
	}
	return false
}

// /pvp, #pvp and "/t pvp" (all a PS2 can send) are the flag; none is said aloud; "/t pvp hello" is
// a tell to someone named Pvp. With world.pvp off each says it is not available.
func TestPvPCommandWords(t *testing.T) {
	for _, word := range []string{"/pvp", "/PVP", "#pvp", "/t pvp", "/tell pvp"} {
		c := newChatWorld(t)
		s, p := c.join(1, "Alice", 100, 100, 0)
		other, _ := c.join(2, "Bob", 101, 100, 0)
		c.say(s, word)
		if p.PvPFlagged() {
			t.Fatalf("%s with world.pvp off put up a flag", word)
		}
		if !hasText(heard(t, s), "PvP is not available on this server.") {
			t.Fatalf("%s off: told %q", word, texts(heard(t, s)))
		}
		s.Queue().GetPending()
		c.gw.SetPvP(true)
		c.say(s, word)
		if !p.PvPFlagged() {
			t.Fatalf("%s put up no flag: told %q", word, texts(heard(t, s)))
		}
		if len(heard(t, other)) != 0 {
			t.Fatalf("%s reached another player: %q", word, texts(heard(t, other)))
		}
	}
	c := newChatWorld(t)
	c.gw.SetPvP(true)
	s, p := c.join(1, "Alice", 100, 100, 0)
	c.say(s, "/t pvp hello")
	if p.PvPFlagged() {
		t.Fatal("a tell with text put up a flag")
	}
}

// The face in the target window: red for a flagged player whoever looks, a player's blue otherwise;
// and it changes at once on a flag's change for everyone already targeting that player, with no
// re-target, its title, guild and token as a real reply's.
func TestPvPFace(t *testing.T) {
	c := newChatWorld(t)
	c.gw.SetPvP(true)
	th := NewTargetHandler(c.sm)
	th.SetGameWorld(c.gw)
	sa, pa := c.join(1, "Alice", 100, 100, 0)
	sb, _ := c.join(2, "Bob", 101, 100, 0)
	sc, pc := c.join(3, "Cara", 102, 100, 0)
	pa.GuildName = "Blades"
	idOf := func(s *session.Session, p *entity.Player) uint32 {
		vs := entity.GetOrCreateVisibility(s)
		for i, id := range vs.ChannelEntity {
			if id == 0 {
				vs.ChannelEntity[i], vs.ChannelObjectID[i] = p.ServerID, 0x18800+uint32(p.ServerID)
				return vs.ChannelObjectID[i]
			}
		}
		t.Fatal("no slot")
		return 0
	}
	aFromB, cFromB := idOf(sb, pa), idOf(sb, pc)
	aFromC := idOf(sc, pa)
	aim := func(s *session.Session, id, token uint32) []byte {
		t.Helper()
		s.Queue().GetPending()
		b := make([]byte, 8)
		binary.LittleEndian.PutUint32(b, token)
		binary.LittleEndian.PutUint32(b[4:], id)
		if err := th.HandleTarget(s, incoming(protocol.OpcodeClassicTargetInfo, b)); err != nil {
			t.Fatal(err)
		}
		f, _, _, _ := pvpFaces(s)
		return f
	}
	if f := aim(sb, aFromB, 0); len(f) != 1 || f[0] != entity.FaceAlly {
		t.Fatalf("an unflagged player's face %v", f)
	}
	if f := aim(sc, aFromC, 5); len(f) != 1 || f[0] != entity.FaceAlly {
		t.Fatalf("an unflagged player's face %v", f)
	}
	sb.Queue().GetPending()
	sc.Queue().GetPending()

	// Alice flags: told to Bob and Cara, who have her targeted, neither of them flagged
	c.say(sa, "/pvp")
	f, _, guild, _ := pvpFaces(sb)
	if len(f) != 1 || f[0] != entity.FaceHostile || guild != "Blades" {
		t.Fatalf("Bob was told %v, guild %q", f, guild)
	}
	f, _, _, token := pvpFaces(sc)
	if len(f) != 1 || f[0] != entity.FaceHostile || token != 5 {
		t.Fatalf("Cara was told %v, token %d", f, token)
	}
	if f, _, _, _ := pvpFaces(sa); len(f) != 0 {
		t.Fatalf("Alice, targeting no one, was told %v", f)
	}
	// looked at anew, by an unflagged viewer and a flagged one
	if f := aim(sb, aFromB, 0); len(f) != 1 || f[0] != entity.FaceHostile {
		t.Fatalf("flagged, an unflagged viewer's %v", f)
	}
	c.say(sb, "/pvp")
	if f := aim(sb, aFromB, 0); len(f) != 1 || f[0] != entity.FaceHostile {
		t.Fatalf("flagged, a flagged viewer's %v", f)
	}
	if f := aim(sb, cFromB, 0); len(f) != 1 || f[0] != entity.FaceAlly {
		t.Fatalf("unflagged, a flagged viewer's %v", f)
	}
	// Bob has moved on to Cara: Alice's flag coming off tells Cara alone
	sc.Queue().GetPending()
	sb.Queue().GetPending()
	pvpCommand(c.gw, sa, time.Now().Add(entity.PvPHoldTime+time.Second))
	if pa.PvPFlagged() {
		t.Fatalf("the flag did not come off: %q", texts(heard(t, sa)))
	}
	if f, _, _, _ := pvpFaces(sc); len(f) != 1 || f[0] != entity.FaceAlly {
		t.Fatalf("Cara was told %v on the flag's end", f)
	}
	if f, _, _, _ := pvpFaces(sb); len(f) != 0 {
		t.Fatalf("Bob, on Cara now, was told %v", f)
	}
}
