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
		{nil, 1, 100, true},
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
		{"/roll", "Alice rolls 100 (1 to 100)."},
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

// The L2 menu's "Roll 0-99" is the bare opcode 0x00C8 (quick command 23): a
// roll of 0 to 99, heard as /roll's is. The control: the chat handler
// registers it, so the game's dispatcher reaches it.
func TestTheL2MenusRollIsZeroToNinetyNine(t *testing.T) {
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
		if len(got) != 1 || got[0].text != "Alice rolls 99 (0 to 99)." || got[0].rgb != rollRGB {
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
