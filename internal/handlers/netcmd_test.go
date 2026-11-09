package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// #net is a GM's. Alone it lists the sessions with a character, the one
// whose oldest unacknowledged message has waited longest first; with a name
// it reports that one whole and asks its client for its own counters
// (0x07E0). The control: a player's #net is an unknown command.
func TestNetIsAGMsReportOfTheStreams(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)

	c.say(ps, "#net")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#net".` {
		t.Fatalf("a player's #net: %+v", got)
	}
	ps.Queue().GetPending()

	// the player's client has had nothing acknowledged (its unknown command's
	// answer, then this, 20 s waiting)
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
	m.SetData([]byte{0xE0, 0x07})
	ps.Queue().AddWithSequence(m)
	ps.Queue().GetPending()
	m.Queued = time.Now().Add(-20 * time.Second)
	m.LastSent = m.Queued.Add(time.Second)

	c.say(gs, "#net")
	got := texts(heard(t, gs))
	if len(got) != 3 || !strings.Contains(got[0], "2 sessions with a character") ||
		!strings.HasPrefix(got[1], "[GM]   Player: 2 unacked (oldest 20.0s)") || !strings.HasPrefix(got[2], "[GM]   Gm: 0 unacked") {
		t.Fatalf("the GM's #net: %q", got)
	}
	gs.Queue().GetPending()

	c.say(gs, "/net player")
	got = texts(heard(t, gs))
	if len(got) != 4 || !strings.Contains(got[0], "Player (203.0.113.2:5002)") ||
		!strings.Contains(got[1], "2 unacked (seq 1-2, oldest 20.0s)") || !strings.Contains(got[3], "asked its client") {
		t.Fatalf("the GM's #net player: %q", got)
	}
	asked := false
	for _, m := range ps.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeServerReady {
			asked = true
		}
	}
	if !asked {
		t.Fatal("the player's client was not asked for its counters (0x07E0)")
	}
	gs.Queue().GetPending()

	c.say(gs, "#net nobody")
	if got := texts(heard(t, gs)); len(got) != 1 || !strings.Contains(got[0], `No session has a character named "nobody"`) {
		t.Fatalf("#net of no one: %q", got)
	}
}
