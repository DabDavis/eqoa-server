package handlers

import (
	"slices"
	"strings"
	"testing"
)

// /afk, #afk and a PS2's "/t afk" turn the away tag over; "/afk <words>" puts
// it up with a message, which answers a tell once a minute to each teller; any
// other typed line ends it, and goes on as typed.
func TestAFKCommandsAndAutoReply(t *testing.T) {
	c := newChatWorld(t)
	sa, a := c.join(1, "Alice", 25000, 16000, 0)
	sb, b := c.join(2, "Bobby", 25050, 16000, 0)
	drain := func() {
		sa.Queue().GetPending()
		sb.Queue().GetPending()
	}

	for _, tc := range []struct {
		line string
		away bool
	}{
		{"/afk", true}, {"/afk", false},
		{"#afk on", true}, {"#AFK off", false},
		{"/t afk", true}, {"/t afk off", false},
		{"/t afk on", true}, {"/afk off", false},
	} {
		c.say(sa, tc.line)
		if a.AFK() != tc.away {
			t.Fatalf("%s: away %v, want %v", tc.line, a.AFK(), tc.away)
		}
		want := "Alice"
		if tc.away {
			want = "[AFK] Alice"
		}
		if a.ShownName() != want {
			t.Fatalf("%s: drawn %q", tc.line, a.ShownName())
		}
		if got := texts(heard(t, sb)); len(got) != 0 {
			t.Fatalf("%s: another heard %q", tc.line, got)
		}
		drain()
	}

	// a message: a tell is answered with it, once a minute (the rate is
	// entity.AFKReply's, tested there); the teller's tell still goes
	c.say(sa, "/afk back in 10")
	if !a.AFK() {
		t.Fatal("/afk <words> not away")
	}
	drain()
	c.say(sb, "/t Alice hello")
	got := texts(heard(t, sb))
	if !slices.Contains(got, "You told Alice, 'hello'") || !slices.Contains(got, "Alice is away: back in 10") {
		t.Fatalf("the teller heard %q", got)
	}
	if got := texts(heard(t, sa)); !slices.Contains(got, "Bobby tells you, 'hello'") {
		t.Fatalf("the away player heard %q", got)
	}
	drain()
	c.say(sb, "/t [AFK] Alice again") // its drawn name finds it
	got = texts(heard(t, sb))
	if !slices.Contains(got, "You told Alice, 'again'") || slices.Contains(got, "Alice is away: back in 10") {
		t.Fatalf("a second tell within the minute: %q", got)
	}
	if !a.AFK() {
		t.Fatal("a tell to it ended it")
	}
	drain()

	// any other line of its own ends it, and is said
	c.say(sa, "I'm back")
	if a.AFK() || a.ShownName() != "Alice" {
		t.Fatal("a line did not end it")
	}
	if got := texts(heard(t, sa)); !slices.Contains(got, "You are no longer away.") || !slices.Contains(got, "Alice: I'm back") {
		t.Fatalf("told %q", got)
	}
	drain()
	c.say(sb, "/t Alice hi")
	if got := texts(heard(t, sb)); slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "is away") }) {
		t.Fatalf("answered while back: %q", got)
	}
	drain()

	// "/t afk <words>" is a tell to a player named Afk when one is in the world
	sk, k := c.join(3, "Afk", 25060, 16000, 0)
	c.say(sb, "/t afk hello there")
	if b.AFK() {
		t.Fatal("a tell to Afk made the teller away")
	}
	if got := texts(heard(t, sk)); !slices.Contains(got, "Bobby tells you, 'hello there'") {
		t.Fatalf("Afk heard %q", got)
	}
	_ = k
	c.say(sb, "/t afk") // alone, still the command
	if !b.AFK() {
		t.Fatal("/t afk alone with a player named Afk in the world")
	}
}

// /help lists /afk.
func TestHelpListsAFK(t *testing.T) {
	if !strings.Contains(strings.Join(playerHelpLines, "\n"), "/afk") {
		t.Fatal("/help lacks /afk")
	}
}
