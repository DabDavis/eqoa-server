package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// namedLevels is levels with each account's username, as database.SQLiteAccountStore gives both.
type namedLevels struct {
	levels
	names map[int]string
}

func (n namedLevels) Username(id int) (string, error) { return n.names[id], nil }

// noclipMessages are the 0x7E7As queued on s: each its u32.
func noclipMessages(s *session.Session) []uint32 {
	var out []uint32
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 6 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNoclip {
			out = append(out, binary.LittleEndian.Uint32(d[2:]))
		}
	}
	return out
}

// The GM Jibrish's petition 1904: /noclip, "Just for jibrish and dabs account", "default turns on /fly", a toggle.
// A GM of world.noclip_accounts (any case) turns it over: on, it flies (0x7E1B 1) and is told noclip (0x7E7A 1);
// again, both off. #fly off ends it. A GM of another account is told it is not theirs and nothing changes (the
// control); a player is answered as an unknown command; a client that cannot fly is told so, and nothing is sent.
func TestNoclipIsTheNamedGMsToggle(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	c.h.SetAccessLevels(namedLevels{levels: levels{1: 1, 2: 0, 3: 2}, names: map[int]string{1: "Dab", 2: "player", 3: "othergm"}})
	SetNoclipAccounts([]string{" dab ", "jibrish"})
	t.Cleanup(func() { SetNoclipAccounts(nil) })
	gs.SetFlyClient(true)
	ps.SetFlyClient(true)

	c.say(gs, "/noclip")
	if !gp.Flying || !gp.Noclip {
		t.Fatalf("/noclip: flying %v, noclip %v; want both on", gp.Flying, gp.Noclip)
	}
	if f, n := flyMessages(gs), noclipMessages(gs); len(f) != 1 || f[0] != 1 || len(n) != 1 || n[0] != 1 {
		t.Fatalf("the client was told fly %v, noclip %v; want 1 and 1", f, n)
	}
	saysFly(t, gs, noclipOnLine)
	gs.Queue().GetPending()

	c.say(gs, "#noclip")
	if gp.Flying || gp.Noclip {
		t.Fatalf("/noclip again: flying %v, noclip %v; want both off", gp.Flying, gp.Noclip)
	}
	if f, n := flyMessages(gs), noclipMessages(gs); len(f) != 1 || f[0] != 0 || len(n) != 1 || n[0] != 0 {
		t.Fatalf("the client was told fly %v, noclip %v; want 0 and 0", f, n)
	}
	gs.Queue().GetPending()

	// #fly off ends it
	c.say(gs, "/noclip on")
	gs.Queue().GetPending()
	c.say(gs, "#fly off")
	if gp.Flying || gp.Noclip {
		t.Fatalf("#fly off: flying %v, noclip %v", gp.Flying, gp.Noclip)
	}
	if n := noclipMessages(gs); len(n) != 1 || n[0] != 0 {
		t.Fatalf("#fly off told noclip %v, want one 0", n)
	}
	gs.Queue().GetPending()

	// another GM's account: not theirs
	os, op := c.join(3, "OtherGm", 25020, 15020, 0)
	os.SetAccountID(3)
	os.SetFlyClient(true)
	c.say(os, "/noclip")
	saysFly(t, os, noclipNotYours)
	if op.Flying || op.Noclip || len(noclipMessages(os)) != 0 || len(flyMessages(os)) != 0 {
		t.Fatal("another GM's /noclip did something")
	}
	// a player: an unknown command
	c.say(ps, "/noclip")
	if got := heard(t, ps); len(got) != 1 || got[0].text != `Unrecognized command "/noclip".` {
		t.Fatalf("a player's /noclip: %+v", got)
	}
	if pp.Noclip || pp.Flying {
		t.Fatal("a player's /noclip did something")
	}
	// a client that cannot fly
	gs.SetFlyClient(false)
	c.say(gs, "/noclip")
	saysFly(t, gs, noclipCannot)
	if gp.Noclip || len(noclipMessages(gs)) != 0 {
		t.Fatal("a client that cannot fly was put in noclip")
	}
}
