package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// conLevelSwitch turns world.con_level on for one test, off again after it.
func conLevelSwitch(t *testing.T) {
	t.Helper()
	SetConLevel(true)
	t.Cleanup(func() { SetConLevel(false) })
}

// conNPC puts an NPC on s's channel slot and returns the id s knows it by.
func conNPC(c *chatWorld, s *session.Session, slot int, n *entity.NPC) uint32 {
	c.gw.NPCs[n.ServerID] = n
	vs := entity.GetOrCreateVisibility(s)
	id := uint32(200000 + slot)
	vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = n.ServerID, id
	return id
}

// The petition's ask (5130): with the switch on, /con, #con and a PS2's "/t
// con" tell the player alone its target's level, a creature's (its source '$'
// off) or a player's, in one yellow line. No target is said so. "/t con
// hello" stays a tell, and "/con" with more after it is no /con.
func TestConSaysTheTargetsLevel(t *testing.T) {
	conLevelSwitch(t)
	c := newChatWorld(t)
	s, p := c.join(5, "Indulgence", 100, 100, 0)
	p.Level = 20
	os, op := c.join(6, "Bystander", 101, 100, 0)
	op.Level = 33
	snake := conNPC(c, s, 1, &entity.NPC{ServerID: 701, Name: "a moss snake$", Level: 22, MaxHP: 40, HP: 40, X: 102, Y: 50, Z: 100})

	c.say(s, "/con")
	if got := heard(t, s); len(got) != 1 || got[0].text != conNoTargetLine {
		t.Fatalf("no target: told %q", texts(got))
	}
	s.Queue().GetPending()

	c.gw.SetTarget(p.ServerID, snake)
	for _, typed := range []string{"/con", "#con", "/CON", "/t con", "/tell con"} {
		c.say(s, typed)
		got := heard(t, s)
		if len(got) != 1 || got[0].op != protocol.OpcodeColoredChat || got[0].text != "a moss snake is level 22." || got[0].rgb != [3]byte{255, 255, 0} {
			t.Fatalf("%s: told %+v", typed, got)
		}
		if other := heard(t, os); len(other) != 0 {
			t.Fatalf("%s reached another player: %q", typed, texts(other))
		}
		s.Queue().GetPending()
	}

	c.gw.SetTarget(p.ServerID, c.gw.IDSeenBy(s, op))
	c.say(s, "/t con")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "Bystander is level 33." {
		t.Fatalf("a player's level: told %q", got)
	}
	s.Queue().GetPending()

	c.say(s, "/t con hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "con is not connected." {
		t.Fatalf("a tell read as /con: %q", got)
	}
	s.Queue().GetPending()
	c.say(s, "/con everything")
	for _, l := range heard(t, s) {
		if l.text == "a moss snake is level 22." {
			t.Fatal("a /con with more after it answered")
		}
	}
}

// The control: with world.con_level off (Sandstorm's game, every server
// before the petition) /con is no command: neither /con nor "/t con" tells a
// level, and no offer goes at world entry.
func TestConIsNothingWithTheSwitchOff(t *testing.T) {
	c := newChatWorld(t)
	s, p := c.join(5, "Indulgence", 100, 100, 0)
	c.gw.SetTarget(p.ServerID, conNPC(c, s, 1, &entity.NPC{ServerID: 701, Name: "a moss snake", Level: 22, MaxHP: 40, HP: 40}))
	c.say(s, "/con")
	c.say(s, "/t con")
	for _, l := range heard(t, s) {
		if l.text == "a moss snake is level 22." {
			t.Fatalf("told a level with the switch off: %q", texts(heard(t, s)))
		}
	}
	s.Queue().GetPending()
	SendConLevelOffer(s)
	if got := s.Queue().GetPending(); len(got) != 0 {
		t.Fatalf("an offer went with the switch off: %d messages", len(got))
	}
}

// The offer, on: OpcodeConLevelOffer and the layout, 1, nothing else.
func TestConLevelOffer(t *testing.T) {
	conLevelSwitch(t)
	c := newChatWorld(t)
	s, _ := c.join(5, "Indulgence", 100, 100, 0)
	SendConLevelOffer(s)
	got := s.Queue().GetPending()
	if len(got) != 1 {
		t.Fatalf("%d messages, want the offer", len(got))
	}
	want := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeConLevelOffer))
	want = append(want, conLevelLayout)
	if d := got[0].Data(); !bytes.Equal(d, want) {
		t.Fatalf("offer % x, want % x", d, want)
	}
}
