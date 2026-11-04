package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// #ticks is a GM's: a player's is an unknown command; the GM's says there is
// no report before the tick timer's first minute, then shows it, a line a part.
func TestTicksIsAGMsReport(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)

	c.say(ps, "#ticks")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#ticks".` {
		t.Fatalf("a player's #ticks: %+v", got)
	}

	c.say(gs, "/ticks")
	if got := texts(heard(t, gs)); len(got) != 1 || !strings.Contains(got[0], "No tick report yet") {
		t.Fatalf("the GM's /ticks before a report: %q", got)
	}

	base := time.Unix(1_800_000_000, 0)
	for i := 0; i <= 600; i++ {
		start := base.Add(time.Duration(i) * 100 * time.Millisecond)
		c.gw.TickTurn(start, start.Add(2*time.Millisecond), start.Add(3*time.Millisecond), 100*time.Millisecond)
	}
	gs.Queue().GetPending()
	c.say(gs, "#ticks")
	got := texts(heard(t, gs))
	if len(got) < 4 || !strings.HasPrefix(got[0], "[GM] 601 turns in 60.0 s") {
		t.Fatalf("the GM's #ticks: %q", got)
	}
}
