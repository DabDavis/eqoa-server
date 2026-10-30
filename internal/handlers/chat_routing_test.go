package handlers

import (
	"encoding/binary"
	"net"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A small world of players, each with a session, to route chat through.
type chatWorld struct {
	t  *testing.T
	sm *session.Manager
	gw *entity.World
	h  *ChatHandler
}

func newChatWorld(t *testing.T) *chatWorld {
	sm := session.NewManager()
	gw := entity.NewWorld()
	gw.SessionManager = sm
	h := NewChatHandler(sm)
	h.SetGameWorld(gw)
	return &chatWorld{t: t, sm: sm, gw: gw, h: h}
}

// join puts a player named name at (x, z) in world w.
func (c *chatWorld) join(id int, name string, x, z float32, w uint8) (*session.Session, *entity.Player) {
	addr := &net.UDPAddr{IP: net.IPv4(203, 0, 113, byte(id)), Port: 5000 + id}
	sess := c.sm.CreateSession(addr, uint32(id), uint32(id), uint16(id), 1, true)
	p := c.gw.CreatePlayer(sess, &database.CharacterRecord{ServerID: uint32(id), CharName: name, X: x, Y: 50, Z: z})
	if p == nil {
		c.t.Fatalf("CreatePlayer(%s) failed", name)
	}
	p.World = w
	sess.SetCharacterInWorld(true)
	return sess, p
}

// wide is a chat line as 0x000B carries it: u32 count, UTF-16, u32 target.
func wide(text string, target uint32) []byte {
	u := utf16.Encode([]rune(text))
	b := make([]byte, 4+2*len(u)+4)
	binary.LittleEndian.PutUint32(b, uint32(len(u)))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[4+2*i:], c)
	}
	binary.LittleEndian.PutUint32(b[4+2*len(u):], target)
	return b
}

func incoming(op protocol.GameOpcode, data []byte) *protocol.Message {
	m := protocol.NewMessage(protocol.MessageTypeReliable, op)
	m.SetData(data)
	return m
}

// heard is what one session was sent: each message's opcode and its text.
type line struct {
	op   protocol.GameOpcode
	text string
	rgb  [3]byte
	sub  byte
}

func readWideLine(b []byte) (string, []byte) {
	if len(b) < 4 {
		return "", nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	if 4+2*n > len(b) {
		return "", nil
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[4+2*i:])
	}
	return string(utf16.Decode(u)), b[4+2*n:]
}

func heard(t *testing.T, s *session.Session) []line {
	t.Helper()
	var out []line
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		op := protocol.GameOpcode(binary.LittleEndian.Uint16(d))
		// forming a group (group's invitation and roster) is not chat
		if op == protocol.OpcodeCreateGroup || op == protocol.OpcodeGroupInviteReceived {
			continue
		}
		l := line{op: op}
		switch op {
		case protocol.OpcodeClientMessage, protocol.OpcodeErrorMessage:
			l.text, _ = readWideLine(d[2:])
		case protocol.OpcodeColoredChat:
			var rest []byte
			l.text, rest = readWideLine(d[2:])
			if len(rest) >= 3 {
				copy(l.rgb[:], rest)
			}
		case protocol.OpcodeGroupChat:
			l.text, _ = readWideLine(d[6:])
		case protocol.OpcodeGuildInfo:
			l.sub = d[2]
			l.text, _ = readWideLine(d[7:])
		case protocol.OpcodeReplyName:
			n := int(binary.LittleEndian.Uint32(d[2:]))
			l.text = string(d[6 : 6+n])
		}
		out = append(out, l)
	}
	return out
}

// group makes a and b a group the way clients do: a invites b, b accepts,
// each naming the other by the id its own session knows it by (heard skips
// the invitation and the roster this sends).
func (c *chatWorld) group(t *testing.T, sa *session.Session, pa *entity.Player, sb *session.Session, pb *entity.Player) {
	t.Helper()
	c.gw.GroupInvite(sa, c.gw.IDSeenBy(sa, pb))
	c.gw.GroupAccept(sb, c.gw.IDSeenBy(sb, pa))
	if len(c.gw.GroupMembers(pa)) != 2 {
		t.Fatalf("no group: %v", c.gw.GroupMembers(pa))
	}
}

func (c *chatWorld) send(s *session.Session, op protocol.GameOpcode, data []byte) {
	c.t.Helper()
	var err error
	switch op {
	case protocol.OpcodeClientSayChat:
		err = c.h.HandleSayChat(s, incoming(op, data))
	case protocol.OpcodeClientShout:
		err = c.h.HandleShoutChat(s, incoming(op, data))
	case protocol.OpcodeChangeChatMode:
		err = c.h.HandleChangeChatMode(s, incoming(op, data))
	case protocol.OpcodeRollRequest:
		err = c.h.HandleRollRequest(s, incoming(op, data))
	}
	if err != nil {
		c.t.Fatal(err)
	}
}

func (c *chatWorld) mode(s *session.Session, m byte) {
	c.send(s, protocol.OpcodeChangeChatMode, []byte{m})
}

func (c *chatWorld) say(s *session.Session, text string) {
	c.send(s, protocol.OpcodeClientSayChat, wide(text, 0xFFFFFFFF))
}

func texts(ls []line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.text)
	}
	return out
}

// Say: the room hears it, in the sender's world, and nobody past 200.
func TestChatSayReachesTheRoomOnly(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	near, _ := c.join(2, "Near", 25050, 16000, 0)
	far, _ := c.join(3, "Far", 25500, 16000, 0)
	elsewhere, _ := c.join(4, "Elsewhere", 25000, 16000, 1)

	c.say(alice, "hello")
	for _, s := range []*session.Session{alice, near} {
		if got := heard(t, s); len(got) != 1 || got[0].op != protocol.OpcodeClientMessage || got[0].text != "Alice: hello" {
			t.Errorf("a listener in range heard %+v", got)
		}
	}
	for _, s := range []*session.Session{far, elsewhere} {
		if got := heard(t, s); len(got) != 0 {
			t.Errorf("out of range or in another world, heard %+v", got)
		}
	}
}

// The bug: group chat (mode 2) went to the room as a say.
func TestChatGroupModeReachesTheGroupOnly(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 27000, 16000, 0)  // far away: group chat has no range
	carl, _ := c.join(3, "Carl", 25010, 16000, 0) // in say range, not in the group
	c.group(t, alice, pa, bob, pb)

	c.mode(alice, ChatModeGroup)
	c.say(alice, "pull now")
	for _, s := range []*session.Session{alice, bob} {
		got := heard(t, s)
		if len(got) != 1 || got[0].op != protocol.OpcodeGroupChat || got[0].text != "Alice tells the group: pull now" {
			t.Errorf("a member heard %+v", got)
		}
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard the group: %+v", got)
	}
}

func TestChatGroupModeWithoutAGroup(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)
	c.mode(alice, ChatModeGroup)
	c.say(alice, "anyone?")
	if got := heard(t, alice); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != "You are not in a group." {
		t.Errorf("the sender got %+v, want word 629 as an error", got)
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard %+v", got)
	}
}

// Guild mode (and 0x0578 sub 12, through RouteGuildChat): members only, as
// 0x0468 sub 5.
func TestChatGuildModeReachesTheGuildOnly(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 27000, 16000, 1)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)
	g, err := c.gw.CreateGuild("Testers", pa)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.gw.JoinGuild(g, pb); err != nil {
		t.Fatal(err)
	}

	c.mode(alice, ChatModeGuild)
	c.say(alice, "guild only")
	for _, s := range []*session.Session{alice, bob} {
		got := heard(t, s)
		if len(got) != 1 || got[0].op != protocol.OpcodeGuildInfo || got[0].sub != 5 || got[0].text != "Alice: guild only" {
			t.Errorf("a member heard %+v", got)
		}
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard the guild: %+v", got)
	}
}

// Shout: everyone, from 0x0C01 sub 2 and from 0x000B in mode 3; sub 1 is
// not a line.
func TestChatShout(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	far, _ := c.join(2, "Far", 2000, 3000, 0)
	c.send(alice, protocol.OpcodeClientShout, append([]byte{2}, wide("wts sword", 0)[:4+2*9]...))
	c.send(alice, protocol.OpcodeClientShout, []byte{1, 1})
	c.mode(alice, ChatModeShout)
	c.say(alice, "lfg")
	for _, s := range []*session.Session{alice, far} {
		got := heard(t, s)
		want := []string{"Alice shouts: wts sword", "Alice shouts: lfg"}
		if len(got) != 2 || got[0].text != want[0] || got[1].text != want[1] || got[0].rgb != [3]byte{0x3F, 0xFF, 0x3F} {
			t.Errorf("heard %+v, want %q in (3F, FF, 3F)", got, want)
		}
	}
}

// The bug: a tell (a PS2 sends the whole "/t Name text" line as 0x000B) was
// said to the room. Now the named player alone gets it, with 0x07A3 so their
// /r answers, and the sender an echo.
func TestChatTellReachesTheNamedPlayerOnly(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	bob, _ := c.join(2, "Bob", 3000, 3000, 2)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)

	for _, typed := range []string{"/t bob meet at the gate", "/TELL Bob  meet at the gate"} {
		c.say(alice, typed)
	}
	got := heard(t, bob)
	if len(got) != 4 {
		t.Fatalf("Bob got %+v, want two (reply name, tell) pairs", got)
	}
	for i := 0; i < 4; i += 2 {
		if got[i].op != protocol.OpcodeReplyName || got[i].text != "Alice" {
			t.Errorf("Bob's message %d: %+v, want 0x07A3 Alice", i, got[i])
		}
		if got[i+1].op != protocol.OpcodeColoredChat || got[i+1].text != "Alice tells you, 'meet at the gate'" {
			t.Errorf("Bob's message %d: %+v", i+1, got[i+1])
		}
	}
	echo := heard(t, alice)
	if len(echo) != 2 || echo[0].text != "You told Bob, 'meet at the gate'" {
		t.Errorf("Alice's echo: %+v", echo)
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard the tell: %+v", texts(got))
	}
}

// /r is the client's: it sends "/t <the 0x07A3 name> text", a tell like any.
func TestChatReplyIsATell(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	bob, _ := c.join(2, "Bob", 25010, 16000, 0)
	c.say(alice, "/t Bob hi")
	c.say(bob, "/t Alice hi back") // what Bob's /r sends after the 0x07A3 above
	got := heard(t, alice)
	if len(got) != 3 || got[1].op != protocol.OpcodeReplyName || got[1].text != "Bob" || got[2].text != "Bob tells you, 'hi back'" {
		t.Errorf("Alice got %+v", got)
	}
}

func TestChatTellToNobody(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)
	c.say(alice, "/t Nobody secret")
	c.say(alice, "/t")
	got := heard(t, alice)
	if len(got) != 2 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != "Nobody is not connected." || got[1].text != "Tell Failed" {
		t.Errorf("the sender got %+v", got)
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard %+v", texts(got))
	}
}

// A /command the server does not know is answered to the sender, not said.
func TestChatUnknownCommandIsNotSaid(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)
	c.say(alice, "/tel Bob my password is hunter2")
	if got := heard(t, alice); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "/tel".` {
		t.Errorf("the sender got %+v", got)
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard %+v", texts(got))
	}
}

// Whisper (mode 1) goes to the sender's target, never to the room.
func TestChatWhisperGoesToTheTarget(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	carl, _ := c.join(3, "Carl", 25010, 16000, 0)
	c.mode(alice, ChatModeWhisper)
	c.say(alice, "psst") // no target
	if got := heard(t, alice); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage {
		t.Errorf("the sender got %+v", got)
	}
	if got := heard(t, carl); len(got) != 0 {
		t.Errorf("a bystander heard %+v", texts(got))
	}
}

// The mode is the sender's own: one player's 0x000E does not move another's.
func TestChatModeIsPerPlayer(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	c.group(t, alice, pa, bob, pb)
	c.mode(alice, ChatModeGroup)
	c.say(bob, "plain say")
	if got := heard(t, alice); len(got) != 1 || got[0].op != protocol.OpcodeClientMessage || got[0].text != "Bob: plain say" {
		t.Errorf("Alice heard %+v", got)
	}
	c.mode(alice, ChatModeSay)
	c.say(alice, "back to say")
	if got := heard(t, bob); len(got) != 2 || got[1].op != protocol.OpcodeClientMessage {
		t.Errorf("Bob heard %+v", got)
	}
}
