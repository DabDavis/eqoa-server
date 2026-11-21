package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// purseSent is the purse of the last 0x0052 among sess's pending messages, and
// the chat lines; it drains the queue.
func purseSent(t *testing.T, s *session.Session) (purse int, sent bool, lines []string) {
	t.Helper()
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodePlayerTunar:
			r := buffer.NewReader(d[2:])
			v, err := r.Read7BitEncodedInt64()
			if err != nil {
				t.Fatalf("0x0052 % x: %v", d, err)
			}
			purse, sent = int(v), true
		case protocol.OpcodeClientMessage:
			lines = append(lines, chatText(d))
		}
	}
	return purse, sent, lines
}

// Half the purse, floored, at the share; none of an empty purse, none at 0%.
func TestADeathTakesItsShareOfThePurse(t *testing.T) {
	for _, c := range []struct{ purse, pct, want int }{
		{1001, 50, 500}, {1000, 50, 500}, {1, 50, 0}, {3, 50, 1}, {0, 50, 0}, {-5, 50, 0},
		{1000, 0, 0}, {1000, 100, 1000}, {1000, 250, 1000}, {2_000_000_001, 50, 1_000_000_000},
	} {
		if got := deathTunarOf(c.purse, c.pct); got != c.want {
			t.Errorf("%d at %d%%: %d, want %d", c.purse, c.pct, got, c.want)
		}
	}
	t.Cleanup(func() { SetDeathTunarPct(nil) })
	for _, c := range []struct {
		set  *int
		want int
	}{{nil, 50}, {intPtr(0), 0}, {intPtr(30), 30}, {intPtr(-1), 0}, {intPtr(150), 100}} {
		SetDeathTunarPct(c.set)
		if DeathTunarPct != c.want {
			t.Errorf("world.death_tunar_pct %v: %d, want %d", c.set, DeathTunarPct, c.want)
		}
	}
}

func intPtr(n int) *int { return &n }

// A creature's kill (work board #1762) takes half the carried tunar, the bank
// untouched: the client told its purse (0x0052) and "You lost N tunar.", the
// purse written through the OnPurse hook. The controls: the same death at
// world.death_tunar_pct 0 and of an empty purse takes and says nothing.
func TestAPvEDeathTakesHalfTheCarriedTunar(t *testing.T) {
	t.Cleanup(func() { SetDeathTunarPct(nil) })
	f := newDeathFixture(t)
	saved := make(chan [2]int, 4)
	f.w.OnPurse(func(id uint32, tunar int) { saved <- [2]int{int(id), tunar} })
	f.p.CharServerID, f.p.Tunar, f.p.BankTunar = 4242, 1001, 777
	f.kill(t)
	if !f.p.IsDead() {
		t.Fatal("not dead")
	}
	if f.p.Tunar != 501 || f.p.BankTunar != 777 {
		t.Fatalf("purse %d, bank %d; want 501, 777", f.p.Tunar, f.p.BankTunar)
	}
	purse, sent, lines := purseSent(t, f.sess)
	if !sent || purse != 501 {
		t.Errorf("0x0052 sent %v, purse %d; want 501", sent, purse)
	}
	said := false
	for _, l := range lines {
		said = said || l == "You lost 500 tunar."
	}
	if !said {
		t.Errorf("lines %q, want \"You lost 500 tunar.\"", lines)
	}
	select {
	case got := <-saved:
		if got != [2]int{4242, 501} {
			t.Errorf("saved %v, want character 4242's purse 501", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("the purse was not saved")
	}

	// the controls
	for _, c := range []struct {
		name         string
		pct          *int
		purse        int
		wantPurse    int
		wantAnything bool
	}{
		{"world.death_tunar_pct 0", intPtr(0), 1001, 1001, false},
		{"an empty purse", nil, 0, 0, false},
		{"one tunar", nil, 1, 1, false},
	} {
		SetDeathTunarPct(c.pct)
		g := newDeathFixture(t)
		g.p.Tunar, g.p.BankTunar = c.purse, 777
		g.kill(t)
		_, sent, lines := purseSent(t, g.sess)
		if g.p.Tunar != c.wantPurse || g.p.BankTunar != 777 || sent {
			t.Errorf("%s: purse %d, bank %d, 0x0052 %v; want %d, 777, none", c.name, g.p.Tunar, g.p.BankTunar, sent, c.wantPurse)
		}
		for _, l := range lines {
			if len(l) > 8 && l[:8] == "You lost" {
				t.Errorf("%s: told %q", c.name, l)
			}
		}
	}
}

// A PvP death (a duel's too: pvpDeathLocked) costs no tunar, as it costs no
// debt; nor does a GM's #testmode. The control: the same player killed by a
// GM's KILL (a death as any other) loses half.
func TestAPvPDeathTakesNoTunar(t *testing.T) {
	f := newDeathFixture(t)
	f.p.HP, f.p.Tunar = 47, 1000
	f.other.Tunar = 1000
	now := time.Unix(1_800_000_000, 0)
	f.w.mu.Lock()
	f.w.pvpDeathLocked(f.other, f.p, now)
	f.w.mu.Unlock()
	if !f.p.IsDead() || f.p.Tunar != 1000 || f.p.Debt != 0 {
		t.Fatalf("a PvP death: dead %v, purse %d, debt %d; want dead, 1000, 0", f.p.IsDead(), f.p.Tunar, f.p.Debt)
	}

	g := newDeathFixture(t)
	g.p.HP, g.p.Tunar, g.p.GMTestMode = 47, 1000, true
	g.w.mu.Lock()
	g.w.playerDiedLocked(g.sess, g.p, now)
	g.w.mu.Unlock()
	if g.p.IsDead() || g.p.Tunar != 1000 {
		t.Errorf("#testmode: purse %d, want 1000", g.p.Tunar)
	}

	h := newDeathFixture(t)
	h.p.HP, h.p.Tunar = 47, 1000
	if !h.w.KillPlayerNow(h.sess, h.p.ServerID) || h.p.Tunar != 500 {
		t.Fatalf("the control: a GM's KILL left a purse of %d, want 500", h.p.Tunar)
	}
}

// A Hardcore character's fall takes nothing (the character is gone), and a
// death in a capture-the-flag match takes nothing, a creature's or the
// world's (lava) included. The control: the same fighter out of a match loses
// half.
func TestNoTunarLostInAHardcoreFallOrAnArena(t *testing.T) {
	SetHardcoreOn(true)
	t.Cleanup(func() { SetHardcoreOn(false) })
	f := newDeathFixture(t)
	f.p.Hardcore, f.p.Tunar = true, 1000
	f.kill(t)
	if !f.p.Fallen || f.p.Tunar != 1000 {
		t.Errorf("a Hardcore fall: fallen %v, purse %d; want fallen, 1000", f.p.Fallen, f.p.Tunar)
	}

	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	a.Tunar = 1000
	r.w.mu.Lock()
	r.w.playerDiedLocked(r.sessOf(a), a, r.at(r.live()+time.Second))
	r.w.mu.Unlock()
	if !a.IsDead() || a.Tunar != 1000 {
		t.Errorf("a match's death: dead %v, purse %d; want dead, 1000", a.IsDead(), a.Tunar)
	}

	c := newCTF(t, 1)
	b := c.ps[0]
	b.Tunar = 1000
	c.w.mu.Lock()
	c.w.playerDiedLocked(c.sessOf(b), b, c.at(time.Second))
	c.w.mu.Unlock()
	if b.Tunar != 500 {
		t.Fatalf("the control: out of a match a purse of %d, want 500", b.Tunar)
	}
}
