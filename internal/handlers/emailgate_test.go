package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/accountmail"
	"github.com/eqoa/server/internal/config"
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

// Before the cutoff an unverified account is reminded once, at its login's
// world entry; a verified one is told nothing. From the
// cutoff the login refuses it (tcp.EmailRefusalStatus, the handshake's
// SetEmailRefusal) and the world says nothing.
func TestTheEmailGateInTheWorld(t *testing.T) {
	c := newChatWorld(t)
	cut := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	gate, err := accountmail.NewGate(verifiedFlags{2: true}, config.MailConfig{
		Accounts:        true,
		RequireVerified: true,
		RequireAfter:    cut.Format(time.DateOnly),
		BaseURL:         "https://eqoaclassic.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	g := NewEmailGate(gate, c.sm)
	clock := cut.Add(-48 * time.Hour)
	g.Now = func() time.Time { return clock }
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
	if len(a) != 1 || !strings.Contains(a[0], "https://eqoaclassic.com/eqoa/password#email") || !strings.Contains(a[0], "December 1, 2026") {
		t.Fatalf("Ann was told %q", a)
	}
	if boxes := dialogueBoxes(as); len(boxes) != 1 || boxes[0] != a[0] {
		t.Fatalf("Ann saw %q in the popup, want %q", boxes, a)
	}
	if b := chatLines(bs, protocol.OpcodeColoredChat); len(b) != 0 {
		t.Fatalf("Bob (verified) was told %q", b)
	}
	// once a login (petition 5281): nothing on the timer, nothing at a
	// later world entry of the same session
	as.Queue().GetPending()
	clock = clock.Add(10 * time.Minute)
	if n := g.Remind(); n != 0 {
		t.Fatalf("reminded %d within the interval", n)
	}
	clock = clock.Add(21 * time.Minute)
	if n := g.Remind(); n != 0 || len(chatLines(as, protocol.OpcodeColoredChat)) != 0 {
		t.Fatalf("reminded %d past the interval", n)
	}
	c.gw.PlayerEntered(as)
	if len(chatLines(as, protocol.OpcodeColoredChat)) != 0 {
		t.Fatal("told again at a later world entry")
	}
	// from the cutoff: no reminder, nothing at world entry (the login refuses)
	clock = cut.Add(time.Minute)
	if n := g.Remind(); n != 0 {
		t.Fatalf("reminded %d past the cutoff", n)
	}
	as.Queue().GetPending()
	c.gw.PlayerEntered(as)
	if len(chatLines(as, protocol.OpcodeErrorMessage)) != 0 || len(chatLines(as, protocol.OpcodeColoredChat)) != 0 {
		t.Fatal("the world spoke past the cutoff")
	}
	if gate.Refusal(1, clock) == "" || gate.Refusal(2, clock) != "" {
		t.Fatal("the gate's refusal")
	}
	if NewEmailGate(nil, c.sm) != nil {
		t.Fatal("a gate with no requirement")
	}
}
