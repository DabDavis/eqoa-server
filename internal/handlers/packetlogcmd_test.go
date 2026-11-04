package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/debuglog"
	"github.com/eqoa/server/internal/protocol"
)

// #packetlog is a GM's: a player's is an unknown command; the GM's says
// whether every packet's lines go to the log and turns them on and off.
func TestPacketLogIsAGMsSwitch(t *testing.T) {
	defer debuglog.SetPackets(debuglog.Packets())
	debuglog.SetPackets(false)
	c, gs, _, ps, _ := gmWorld(t)

	c.say(ps, "#packetlog on")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#packetlog".` {
		t.Fatalf("a player's #packetlog: %+v", got)
	}
	if debuglog.Packets() {
		t.Fatal("a player turned the packet log on")
	}

	gs.Queue().GetPending()
	c.say(gs, "/packetlog on")
	if got := texts(heard(t, gs)); !debuglog.Packets() || !slices.ContainsFunc(got, func(s string) bool {
		return s == "[GM] Packet log: on (#packetlog on|off: every packet's own lines, about 100 a second a player)"
	}) {
		t.Fatalf("the GM's /packetlog on: %q, on %v", got, debuglog.Packets())
	}
	gs.Queue().GetPending()
	c.say(gs, "#packetlog off")
	if got := texts(heard(t, gs)); debuglog.Packets() || len(got) != 1 {
		t.Fatalf("the GM's #packetlog off: %q, on %v", got, debuglog.Packets())
	}
	gs.Queue().GetPending()
	c.say(gs, "#packetlog loud")
	if got := texts(heard(t, gs)); len(got) != 1 || got[0] != "[GM] Usage: #packetlog [on|off]" {
		t.Fatalf("a bad word: %q", got)
	}
}
