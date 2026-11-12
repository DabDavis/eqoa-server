package handlers

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A raid group through the opcodes a client sends and its /raid (entity/raid.go,
// raidflag.go, raidroster.go; entity's tests have the rules): six in by invites and a
// flag, what each is sent, its chat and its loot lines. The control in each is a
// player beside them who answers nothing.

type raidGroupRig struct {
	c    *chatWorld
	h    *GroupHandler
	sess []*session.Session
	p    []*entity.Player
	out  *session.Session // a bystander, in say range
}

func u32Msg(op protocol.GameOpcode, v uint32) *protocol.Message {
	return groupMsg(op, u32b(v))
}

// newRaidGroup is the players: the first invites the next three and raises a raid
// flag for the rest, each accepting, through the handlers.
func newRaidGroup(t *testing.T, names ...string) *raidGroupRig {
	t.Helper()
	r := &raidGroupRig{c: newChatWorld(t)}
	r.h = NewGroupHandler(r.c.gw)
	for i, n := range names {
		s, p := r.c.join(i+1, n, 25000+float32(i), 16000, 0)
		s.SetPlayerObjectID(uint32(600000 + i))
		r.sess, r.p = append(r.sess, s), append(r.p, p)
	}
	r.out, _ = r.c.join(90, "Bystander", 25001, 16001, 0)
	for i := 1; i < len(r.p); i++ {
		if i < entity.GroupWindowSize {
			if err := r.h.handleInvite(r.sess[0], groupMsg(protocol.OpcodeGroupInvite, u32b(r.c.gw.IDSeenBy(r.sess[0], r.p[i])), str8b(r.p[i].Name))); err != nil {
				t.Fatal(err)
			}
		} else if i == entity.GroupWindowSize {
			r.c.say(r.sess[0], "/raid")
		}
		if err := r.h.handleAccept(r.sess[i], u32Msg(protocol.OpcodeAcceptGroupInvite, r.c.gw.IDSeenBy(r.sess[i], r.p[0]))); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// rostersSent reads each 0x0772 a session was queued as CLIENT's Msg0772 does (m_group
// holds four), the names of each, drained.
func rostersSent(t *testing.T, s *session.Session) (out [][]string) {
	t.Helper()
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeRemoveGroupMember:
			t.Errorf("0x069A % x: refused", d[2:])
		case protocol.OpcodeCreateGroup:
			count := int(binary.LittleEndian.Uint32(d[6:]))
			if count > entity.GroupWindowSize {
				t.Fatalf("a roster of %d: past CLIENT's m_group[4]", count)
			}
			b := d[10+4*count:]
			var names []string
			for i := 0; i < count; i++ {
				n := int(binary.LittleEndian.Uint32(b))
				names = append(names, string(b[4:4+n]))
				b = b[4+n:]
			}
			out = append(out, names)
		}
	}
	return out
}

// Six through the opcodes and a flag: nobody refused, one group, its leader first; no
// client is sent more than four, ever, in any roster on the way there; a bystander
// nothing.
func TestSixInvitedThroughTheOpcodes(t *testing.T) {
	names := []string{"Alda", "Bren", "Cael", "Dirn", "Esk", "Fenn"}
	r := newRaidGroup(t, names...)
	if got := r.c.gw.GroupMembers(r.p[0]); len(got) != 6 || got[0] != r.p[0] || got[5] != r.p[5] {
		t.Fatalf("the group: %v", got)
	}
	want := []string{"Alda Bren Cael Dirn", "Alda Bren Cael Dirn", "Alda Bren Cael Dirn", "Alda Bren Cael Dirn", "Alda Bren Cael Esk", "Alda Bren Cael Fenn"}
	for i, s := range r.sess {
		rosters := rostersSent(t, s)
		if len(rosters) == 0 {
			t.Fatalf("%s was sent no roster", names[i])
		}
		if got := strings.Join(rosters[len(rosters)-1], " "); got != want[i] {
			t.Errorf("%s's last roster %q, want %q", names[i], got, want[i])
		}
	}
	if got := rostersSent(t, r.out); len(got) != 0 {
		t.Errorf("a bystander's rosters %v", got)
	}
}

// Group chat reaches all six, those past each one's window too, as the raid's: "tells
// the raid", in orange (the user, 2026-11-08: "make it say tells the raid instead of
// group and make text color orange"); a bystander in say range hears none of it. The
// control (TestChatGroupModeReachesTheGroupOnly): a group of two tells the group.
func TestARaidGroupsChatReachesEveryone(t *testing.T) {
	r := newRaidGroup(t, "Alda", "Bren", "Cael", "Dirn", "Esk", "Fenn")
	for _, s := range append(append([]*session.Session(nil), r.sess...), r.out) {
		s.Queue().GetPending()
	}
	r.c.mode(r.sess[5], ChatModeGroup)
	r.c.say(r.sess[5], "pull now")
	for i, s := range r.sess {
		got := heard(t, s)
		if len(got) != 1 || got[0].op != protocol.OpcodeColoredChat || got[0].text != "Fenn tells the raid: pull now" || got[0].rgb != raidChatRGB {
			t.Errorf("member %d heard %+v", i, got)
		}
	}
	if got := heard(t, r.out); len(got) != 0 {
		t.Errorf("a bystander heard %+v", got)
	}
}

// A take from a corpse is told to the other five, by each one's Loot Messages option,
// through the World's own list; a bystander hears nothing. Anyone may loot a corpse
// (loot.go), so nothing else a loot does knows a group's size.
func TestARaidGroupHearsWhatIsLooted(t *testing.T) {
	r := newRaidGroup(t, "Alda", "Bren", "Cael", "Dirn", "Esk", "Fenn")
	for _, s := range append(append([]*session.Session(nil), r.sess...), r.out) {
		s.Queue().GetPending()
	}
	off := r.sess[2]
	opt := &FactionHandler{}
	if err := opt.HandleLootMessages(off, groupMsg(protocol.OpcodeLootMessages, []byte{1, 1})); err != nil {
		t.Fatal(err)
	}
	h := &NPCWindowHandler{}
	h.SetGameWorld(r.c.gw)
	h.tellGroupLooted(r.p[4], database.CharInventoryEntry{Pattern: database.ItemPatternRecord{ItemName: "a Rusty Sword"}}, 1)
	for i, s := range r.sess {
		got := heard(t, s)
		switch {
		case i == 4, s == off:
			if len(got) != 0 {
				t.Errorf("member %d heard %+v", i, got)
			}
		case len(got) != 1 || got[0].text != "Esk receives a Rusty Sword from the corpse.":
			t.Errorf("member %d heard %+v", i, got)
		}
	}
	if got := heard(t, r.out); len(got) != 0 {
		t.Errorf("a bystander heard %+v", got)
	}
}

// The flag by each spelling: /raid and #raid, in any case, and a PS2's /t raid, which is
// all it can send (entity/raidflag.go). The controls: a tell to someone is a tell, a line
// that only begins so is said.
func TestTheRaidFlagsSpellings(t *testing.T) {
	for _, tc := range []struct {
		line string
		flag bool
	}{
		{"/raid", true}, {"/RAID", true}, {"#raid", true}, {"/raid now", true},
		{"/t raid", true}, {"/tell raid", true}, {"/T RAID", true},
		{"/t raid hello", false}, {"/t Bren hi", false}, {"/raids", false}, {"raid", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			r := newRaidGroup(t, "Alda", "Bren")
			sb := r.sess[1]
			out := r.out
			out.Queue().GetPending()
			r.sess[0].Queue().GetPending()
			sb.Queue().GetPending()
			r.c.say(r.sess[0], tc.line)
			asked := false
			for _, m := range out.Queue().GetPending() {
				if protocol.GameOpcode(binary.LittleEndian.Uint16(m.Data())) == protocol.OpcodeGroupInviteReceived {
					asked = true
				}
			}
			if asked != tc.flag {
				t.Errorf("the bystander asked: %v, want %v", asked, tc.flag)
			}
		})
	}
}

// The offer is 0x7E18 with a u32 2 (the records may carry their groups); 0x7E19 is the
// client's answer: 1 the roster as before, 2 with the groups, 0 off.
func TestTheRaidRosterOfferAndAnswer(t *testing.T) {
	r := newRaidGroup(t, "Alda", "Bren")
	r.sess[0].Queue().GetPending()
	SendRaidRosterOffer(r.sess[0])
	msgs := r.sess[0].Queue().GetPending()
	if len(msgs) != 1 || !bytes.Equal(msgs[0].Data(), []byte{0x18, 0x7E, 2, 0, 0, 0}) {
		t.Fatalf("the offer: %v", msgs)
	}
	if err := r.h.handleRaidRosterAnswer(r.sess[0], u32Msg(protocol.OpcodeRaidRosterAnswer, 1)); err != nil {
		t.Fatal(err)
	}
	if !r.p[0].RaidRoster || r.p[0].RaidRosterGroups {
		t.Error("answered 1: not on, or with groups")
	}
	if err := r.h.handleRaidRosterAnswer(r.sess[0], u32Msg(protocol.OpcodeRaidRosterAnswer, 2)); err != nil {
		t.Fatal(err)
	}
	if !r.p[0].RaidRoster || !r.p[0].RaidRosterGroups {
		t.Error("answered 2: not on with groups")
	}
	if err := r.h.handleRaidRosterAnswer(r.sess[0], u32Msg(protocol.OpcodeRaidRosterAnswer, 0)); err != nil {
		t.Fatal(err)
	}
	if r.p[0].RaidRoster {
		t.Error("answered 0: still on")
	}
}
