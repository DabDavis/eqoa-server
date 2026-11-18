package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

func TestRollRange(t *testing.T) {
	for _, c := range []struct {
		args   []string
		lo, hi int
		ok     bool
	}{
		{nil, 0, 100, true},
		{[]string{"20"}, 1, 20, true},
		{[]string{"5", "50"}, 5, 50, true},
		{[]string{"50", "5"}, 5, 50, true},
		{[]string{"0", "1"}, 0, 1, true},
		{[]string{"0"}, 0, 0, false},
		{[]string{"-3"}, 0, 0, false},
		{[]string{"ten"}, 0, 0, false},
		{[]string{"1000000001"}, 0, 0, false},
		{[]string{"1", "2", "3"}, 0, 0, false},
	} {
		lo, hi, ok := rollRange(c.args)
		if lo != c.lo || hi != c.hi || ok != c.ok {
			t.Errorf("rollRange(%q) = %d, %d, %v; want %d, %d, %v", c.args, lo, hi, ok, c.lo, c.hi, c.ok)
		}
	}
}

// A roll is heard as a say is (the roller and the room, nobody past 200 or
// in another world), in gold; #roll is a PS2 player's and needs no GM; a
// bad number answers the roller alone.
func TestRollIsHeardInSayRange(t *testing.T) {
	old := rollDice
	rollDice = func(n int) int { return n - 1 } // the most every time
	defer func() { rollDice = old }()
	room := func() (*chatWorld, *session.Session, *session.Session, *session.Session, *session.Session) {
		c := newChatWorld(t)
		alice, _ := c.join(1, "Alice", 25000, 16000, 0)
		near, _ := c.join(2, "Near", 25050, 16000, 0)
		far, _ := c.join(3, "Far", 25500, 16000, 0)
		elsewhere, _ := c.join(4, "Elsewhere", 25000, 16000, 1)
		return c, alice, near, far, elsewhere
	}
	for _, cmd := range []struct{ text, want string }{
		{"/roll", "Alice rolls 100 (0 to 100)."},
		{"/random 20", "Alice rolls 20 (1 to 20)."},
		{"#roll 50 5", "Alice rolls 50 (5 to 50)."},
	} {
		c, alice, near, far, elsewhere := room()
		c.say(alice, cmd.text)
		for _, s := range []*session.Session{alice, near} {
			got := heard(t, s)
			if len(got) != 1 || got[0].op != protocol.OpcodeColoredChat || got[0].text != cmd.want || got[0].rgb != rollRGB {
				t.Errorf("%q: a listener in range heard %+v", cmd.text, got)
			}
		}
		for _, s := range []*session.Session{far, elsewhere} {
			if got := heard(t, s); len(got) != 0 {
				t.Errorf("%q: out of range or in another world, heard %+v", cmd.text, got)
			}
		}
	}
	c, alice, near, _, _ := room()
	c.say(alice, "/roll many")
	if got := heard(t, alice); len(got) != 1 || got[0].text == "" {
		t.Errorf("a bad roll: the roller heard %+v", got)
	}
	if got := heard(t, near); len(got) != 0 {
		t.Errorf("a bad roll was heard by the room: %+v", got)
	}
}

// The menu's roll is the bare opcode 0x00C8 (quick command 23): a roll of 0 to
// 100, the same as a plain /roll's (petition 1107), heard as /roll's is. The control: the chat handler
// registers it, so the game's dispatcher reaches it.
func TestTheMenusRollIsZeroToAHundred(t *testing.T) {
	old := rollDice
	rollDice = func(n int) int { return n - 1 } // the most every time
	defer func() { rollDice = old }()
	c := newChatWorld(t)
	alice, _ := c.join(1, "Alice", 25000, 16000, 0)
	near, _ := c.join(2, "Near", 25050, 16000, 0)
	far, _ := c.join(3, "Far", 25500, 16000, 0)
	c.send(alice, protocol.OpcodeRollRequest, nil)
	for _, s := range []*session.Session{alice, near} {
		got := heard(t, s)
		if len(got) != 1 || got[0].text != "Alice rolls 100 (0 to 100)." || got[0].rgb != rollRGB {
			t.Errorf("in range heard %+v", got)
		}
	}
	if got := heard(t, far); len(got) != 0 {
		t.Errorf("out of range heard %+v", got)
	}
	r := NewRegistry()
	c.h.RegisterHandlers(r)
	if _, ok := r.Get(protocol.OpcodeRollRequest); !ok {
		t.Error("0x00C8 is not registered: the menu's roll reaches no handler")
	}
}

// The owner, 2026-11-18: "rolls from players are not showing for the whole raid". A /roll in
// a raid is heard by every member of it wherever it is, dead or alive, another of its groups
// (raidsubgroups) or another world; the room hears it as before, and a plain group's far
// member does not (the control: a group's roll is its say range's).
func TestARaidHearsItsMembersRolls(t *testing.T) {
	old := rollDice
	rollDice = func(n int) int { return n - 1 }
	defer func() { rollDice = old }()
	r := newRaidGroup(t, "Ann", "Bob", "Cal", "Dee", "Eve", "Fay")
	far, _ := r.c.join(91, "Stranger", 26000, 16000, 0)
	r.p[4].World = 1               // Eve: at her bind in another world
	r.p[5].X, r.p[5].HP = 27000, 0 // Fay: far off, dead
	r.p[1].X = 26500               // Bob: far off in the same world
	for _, s := range append(append([]*session.Session{}, r.sess...), r.out, far) {
		s.Queue().GetPending()
	}
	r.c.say(r.sess[0], "/roll")
	want := "Ann rolls 100 (0 to 100)."
	for i, s := range r.sess {
		got := heard(t, s)
		if len(got) != 1 || got[0].text != want {
			t.Errorf("%s heard %+v", r.p[i].Name, got)
		}
	}
	if got := heard(t, r.out); len(got) != 1 || got[0].text != want {
		t.Errorf("the room's bystander heard %+v", got)
	}
	if got := heard(t, far); len(got) != 0 {
		t.Errorf("a stranger far off heard %+v", got)
	}

	// the control: a plain group
	c := newChatWorld(t)
	sa, pa := c.join(1, "Ann", 25000, 16000, 0)
	sb, pb := c.join(2, "Bob", 25000, 16000, 0)
	c.group(t, sa, pa, sb, pb)
	pb.X = 26500
	sa.Queue().GetPending()
	sb.Queue().GetPending()
	c.say(sa, "/roll")
	if got := heard(t, sb); len(got) != 0 {
		t.Errorf("a plain group's far member heard %+v", got)
	}
}
