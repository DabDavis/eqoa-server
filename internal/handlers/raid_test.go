package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A raid group through the opcodes a client sends (entity/raid.go; entity/raid_test.go
// has the rules): six invited and accepted, what each is sent, its chat and its loot
// lines. The control in each is a player beside them in no group.

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

// newRaidGroup is six players, the first inviting each of the rest, each accepting,
// through the handlers.
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
		if err := r.h.handleInvite(r.sess[0], groupMsg(protocol.OpcodeGroupInvite, u32b(r.c.gw.IDSeenBy(r.sess[0], r.p[i])), str8b(r.p[i].Name))); err != nil {
			t.Fatal(err)
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

// Six through the opcodes: nobody refused, one group, its leader first; no client is
// sent more than four, ever, in any roster on the way there; a bystander nothing.
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

// Group chat reaches all six, those past each one's window too; a bystander in say
// range hears none of it.
func TestARaidGroupsChatReachesEveryone(t *testing.T) {
	r := newRaidGroup(t, "Alda", "Bren", "Cael", "Dirn", "Esk", "Fenn")
	for _, s := range append(append([]*session.Session(nil), r.sess...), r.out) {
		s.Queue().GetPending()
	}
	r.c.mode(r.sess[5], ChatModeGroup)
	r.c.say(r.sess[5], "pull now")
	for i, s := range r.sess {
		got := heard(t, s)
		if len(got) != 1 || got[0].op != protocol.OpcodeGroupChat || got[0].text != "Fenn tells the group: pull now" {
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
	h.tellGroupLooted(r.p[4], "a Rusty Sword", 1)
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
