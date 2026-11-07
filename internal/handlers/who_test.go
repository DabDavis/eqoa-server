package handlers

import (
	"strings"
	"testing"
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
