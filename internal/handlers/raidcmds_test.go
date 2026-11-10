package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The raid's commands as typed (raidcmds.go; the GM Jibrish's petitions 1448, 1451,
// 1452, 1453): every spelling reaches its command, and a PS2's "/t <word>" stays a tell
// where a player of the name is in the world or the sender has nothing to answer.

// boxNames are the names of the invitation boxes (0x0773) queued for s.
func boxNames(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 14 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeGroupInviteReceived {
			n := int(binary.LittleEndian.Uint32(d[10:]))
			out = append(out, string(d[14:14+n]))
		}
	}
	return out
}

// assists are the ids 0x007B told s to target.
func assists(s *session.Session) []uint32 {
	var out []uint32
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) == 6 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeAssistTarget {
			out = append(out, binary.LittleEndian.Uint32(d[2:]))
		}
	}
	return out
}

func flushQueue(s *session.Session) { s.Queue().GetPending() }

func TestTheRaidsCommandsAreRoutedInEverySpelling(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	c.say(alice, "/raidinvite bob")
	if got := boxNames(alice); len(got) != 1 || got[0] != "The raid flag" {
		t.Fatalf("/raidinvite from no group: Alice's boxes %q", got)
	}
	flushQueue(alice)
	c.say(alice, "/t raidinvite Bob") // a PS2's
	if got := boxNames(alice); len(got) != 1 || got[0] != "The raid flag" {
		t.Fatalf("/t raidinvite: Alice's boxes %q", got)
	}
	if got := heard(t, bob); len(got) != 0 {
		t.Fatalf("Bob heard a tell: %+v", got)
	}
	// /invite and /t invite
	flushQueue(alice)
	c.say(alice, "#invite Bob")
	if got := boxNames(alice); len(got) != 1 || got[0] != "Your own group" {
		t.Fatalf("#invite: %q", got)
	}
	// /help, /t help, and a non-GM's #help
	for _, l := range []string{"/help", "/t help", "#help"} {
		flushQueue(alice)
		c.say(alice, l)
		got := texts(heard(t, alice))
		if len(got) < len(playerHelpLines) || got[0] != playerHelpLines[0] {
			t.Fatalf("%s: %q", l, got)
		}
	}
	// /target and /tar, by a start, within 100: the client's own target message
	vs := entity.GetOrCreateVisibility(alice)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = pb.ServerID, 0x00500002
	for _, l := range []string{"/target bo", "/tar BOB", "/t tar bob"} {
		flushQueue(alice)
		c.say(alice, l)
		if got := assists(alice); len(got) != 1 || got[0] != 0x00500002 {
			t.Fatalf("%s: 0x007B %x", l, got)
		}
	}
	flushQueue(alice)
	c.say(alice, "/target carl")
	if got := texts(heard(t, alice)); len(got) != 1 || got[0] != entity.NoTargetInRange {
		t.Fatalf("/target of no one: %q", got)
	}
	// /rsay out of a group
	flushQueue(alice)
	c.say(alice, "/rsay hello")
	if got := texts(heard(t, alice)); len(got) != 1 || got[0] != "You are not in a raid." {
		t.Fatalf("/rsay out of a group: %q", got)
	}
}

// /rsay is the group's chat, a raid's in its orange: a flagged group hears "tells the
// raid"; a plain group the group's line (the control).
func TestRsayIsTheRaidsChat(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	c.group(t, alice, pa, bob, pb)
	flushQueue(alice)
	flushQueue(bob)
	c.say(alice, "/rsay pull")
	if got := heard(t, bob); len(got) != 1 || got[0].text != "Alice tells the group: pull" {
		t.Fatalf("a plain group's /rsay: %+v", got)
	}
	c.say(alice, "/raidaoe") // the flag up: a raid's chat
	flushQueue(bob)
	c.say(alice, "/t rsay now")
	got := heard(t, bob)
	if len(got) != 1 || got[0].text != "Alice tells the raid: now" || got[0].rgb != raidChatRGB {
		t.Fatalf("a raid's /t rsay: %+v", got)
	}
}

// A PS2's "/t <word>" is a tell where it is not a command: to a player of the name in
// the world (/t help), with nothing to answer (/t need, /t abandon), and /t raid with
// words; the same words as commands are the controls above.
func TestATellIsKeptWhereTheWordIsNoCommand(t *testing.T) {
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	help, _ := c.join(2, "Help", 25010, 16000, 0)
	need, _ := c.join(3, "Need", 25010, 16000, 0)
	raid, _ := c.join(4, "Raid", 25010, 16000, 0)
	c.h.raid = NewRaidHandler(c.gw, NewNPCWindowHandler(nil, nil))
	for _, tc := range []struct {
		line string
		to   *session.Session
		want string
	}{
		{"/t help me please", help, "Alice tells you, 'me please'"},
		{"/t need 3", need, "Alice tells you, '3'"},
		{"/t raid hello", raid, "Alice tells you, 'hello'"},
	} {
		flushQueue(tc.to)
		c.say(alice, tc.line)
		got := texts(heard(t, tc.to))
		found := false
		for _, g := range got {
			found = found || g == tc.want
		}
		if !found {
			t.Errorf("%q: %q heard %q", tc.line, tc.want, got)
		}
	}
	flushQueue(alice)
	c.say(alice, "/t abandon")
	if got := texts(heard(t, alice)); len(got) != 1 || !strings.Contains(got[0], "Tell Failed") && !strings.Contains(got[0], "not connected") {
		t.Errorf("/t abandon with no corpse held: %q", got)
	}
}

// A GM's #help is its own (the control: a player's above is the players').
func TestAGMsHelpIsItsOwn(t *testing.T) {
	c, gm, _, _, _ := gmWorld(t)
	c.say(gm, "#help")
	got := texts(heard(t, gm))
	if len(got) == 0 || got[0] == playerHelpLines[0] {
		t.Fatalf("a GM's #help: %q", got)
	}
	flushQueue(gm)
	c.say(gm, "/help")
	got = texts(heard(t, gm))
	if len(got) == 0 || got[0] != playerHelpLines[0] || got[len(got)-1] != "  GM commands: #help" {
		t.Fatalf("a GM's /help: %q", got)
	}
}

// raidOffers is every OpcodeRaidOffer queued for the session's client.
func raidOffers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeRaidOffer {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the raid's windows (u8 1), once; the second offers nothing more.
func TestTheRaidOfferAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got := raidOffers(sess); len(got) != 1 || string(got[0]) != "\x42\x7E\x01" {
		t.Fatalf("world entry offered %x", got)
	}
}

// 0x7E43 makes the player's client a window client (1) or not (0); 0x7E48's ops reach
// their commands: the loot rule (1), leave (2), the flag (6), the leader by target (8); an
// unknown op does nothing (the control).
func TestTheRaidMessagesReachTheRaid(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	h := NewRaidHandler(c.gw, NewNPCWindowHandler(nil, nil))
	r := NewRegistry()
	h.RegisterHandlers(r)
	feed := func(s *session.Session, op protocol.GameOpcode, data ...byte) {
		t.Helper()
		f, ok := r.Get(op)
		if !ok {
			t.Fatalf("no handler for %#x", uint16(op))
		}
		if err := f(s, incoming(op, data)); err != nil {
			t.Fatal(err)
		}
	}
	feed(alice, protocol.OpcodeRaidAnswer, 1)
	if !c.gw.RaidWindows(pa) {
		t.Fatal("0x7E43 1 did not take")
	}
	feed(alice, protocol.OpcodeRaidAnswer, 0)
	if c.gw.RaidWindows(pa) {
		t.Fatal("0x7E43 0 did not stop it")
	}
	c.group(t, alice, pa, bob, pb)
	feed(alice, protocol.OpcodeRaidCommand, 6) // the flag: a raid
	if !c.gw.GroupIsRaid(pa) {
		t.Fatal("op 6 raised no flag")
	}
	feed(alice, protocol.OpcodeRaidCommand, 99) // the control
	feed(alice, protocol.OpcodeRaidCommand, 1, byte(entity.LootNeedGreed))
	flushQueue(bob)
	id := c.gw.IDSeenBy(alice, pb)
	feed(alice, protocol.OpcodeRaidCommand, 8, byte(id), byte(id>>8), byte(id>>16), byte(id>>24))
	if m := c.gw.GroupMembers(pa); len(m) != 2 || m[0] != pb {
		t.Fatalf("op 8: the members %v", m)
	}
	if got := texts(heard(t, bob)); !raidSaid(got, "Alice makes Bob the raid's leader.") {
		t.Fatalf("Bob heard %q", got)
	}
	feed(bob, protocol.OpcodeRaidCommand, 2) // leave
	if len(c.gw.GroupMembers(pa)) != 0 {
		t.Fatal("op 2 left nothing")
	}
}
