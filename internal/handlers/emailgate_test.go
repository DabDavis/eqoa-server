package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

type verifiedFlags map[int]bool

func (f verifiedFlags) EmailVerified(id int) (bool, error) { return f[id], nil }

func chatLines(sess *session.Session, op protocol.GameOpcode) []string {
	var out []string
	for _, d := range queued(sess, op) {
		text, _ := readWideLine(d[2:])
		out = append(out, text)
	}
	return out
}

// Before the cutoff an unverified account is reminded at world entry and
// every interval while it plays; a verified one is told nothing. From the
// cutoff it is told why in red and ejected a moment later.
func TestTheEmailGateInTheWorld(t *testing.T) {
	c := newChatWorld(t)
	cut := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	gate := &accountmail.Gate{DB: verifiedFlags{2: true}, Cutoff: cut, Every: 30 * time.Minute, Page: "eqoaclassic.com/eqoa/password"}
	g := NewEmailGate(gate, c.sm)
	clock := cut.Add(-48 * time.Hour)
	g.Now = func() time.Time { return clock }
	var ejected []*session.Session
	var waited []time.Duration
	g.eject = func(s *session.Session) { ejected = append(ejected, s) }
	g.after = func(d time.Duration, f func()) { waited = append(waited, d); f() }
	c.gw.OnPlayerEntered(g.Entered)

	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	as.SetAccountID(1)
	bs.SetAccountID(2)
	as.SetInGame(true)
	bs.SetInGame(true)
	as.Queue().GetPending()
	bs.Queue().GetPending()
	c.gw.PlayerEntered(as)
	c.gw.PlayerEntered(bs)
	a := chatLines(as, protocol.OpcodeColoredChat)
	if len(a) != 1 || !strings.Contains(a[0], "eqoaclassic.com/eqoa/password") || !strings.Contains(a[0], "December 1, 2026") {
		t.Fatalf("Ann was told %q", a)
	}
	if b := chatLines(bs, protocol.OpcodeColoredChat); len(b) != 0 {
		t.Fatalf("Bob (verified) was told %q", b)
	}
	if len(ejected) != 0 {
		t.Fatal("ejected before the cutoff")
	}
	// within the interval: nothing; past it: again
	as.Queue().GetPending()
	clock = clock.Add(10 * time.Minute)
	if n := g.Remind(); n != 0 {
		t.Fatalf("reminded %d within the interval", n)
	}
	clock = clock.Add(21 * time.Minute)
	if n := g.Remind(); n != 1 || len(chatLines(as, protocol.OpcodeColoredChat)) != 1 {
		t.Fatalf("reminded %d past the interval", n)
	}
	// from the cutoff: told in red at world entry, then ejected
	clock = cut.Add(time.Minute)
	if n := g.Remind(); n != 0 {
		t.Fatalf("reminded %d past the cutoff", n)
	}
	as.Queue().GetPending()
	c.gw.PlayerEntered(as)
	red := chatLines(as, protocol.OpcodeErrorMessage)
	if len(red) != 1 || !strings.Contains(red[0], "verified e-mail") || !strings.Contains(red[0], "eqoaclassic.com/eqoa/password") {
		t.Fatalf("Ann was told %q", red)
	}
	if len(ejected) != 1 || ejected[0] != as || len(waited) != 1 || waited[0] != EmailGateEject {
		t.Fatalf("ejected %v after %v", ejected, waited)
	}
	bs.Queue().GetPending()
	c.gw.PlayerEntered(bs)
	if len(ejected) != 1 || len(chatLines(bs, protocol.OpcodeErrorMessage)) != 0 {
		t.Fatal("a verified account was refused")
	}
	if NewEmailGate(nil, c.sm) != nil {
		t.Fatal("a gate with no requirement")
	}
}
