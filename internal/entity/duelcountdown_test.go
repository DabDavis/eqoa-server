package entity

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// An accepted duel counts down in chat to its two, as SOE's server did
// (petition 1063): "Duel starts in 15 seconds." at the accept, the 10 and the
// 5 on their seconds, then "3", "2", "1", and "START" with the fight. Nothing
// of the duel lands before it (a swing at the opponent is not taken), the
// bystander hears none of it, and a tick that comes late says only the latest
// line it passed.
func TestADuelCountsDownBeforeItsFight(t *testing.T) {
	d := newDuelists(t)
	expect := func(when string, want ...string) {
		t.Helper()
		a, b, c := duelLines(duelDrain(d.sa)), duelLines(duelDrain(d.sb)), duelLines(duelDrain(d.sc))
		if !slices.Equal(a, want) || !slices.Equal(b, want) || len(c) != 0 {
			t.Fatalf("%s: A %q, B %q, the bystander %q; want %q", when, a, b, c, want)
		}
	}
	accept := func() *Duel {
		t.Helper()
		d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
		duel := d.w.duels.of[d.a.ServerID]
		for _, s := range []*session.Session{d.sa, d.sb, d.sc} {
			duelDrain(s)
		}
		d.w.DuelAnswer(d.sb, duel.ID, true, d.at(time.Second))
		expect("at the accept", "Duel starts in 15 seconds.")
		return duel
	}
	duel := accept()
	if id, on := d.w.DuelOf(d.a); id != duel.ID || on || duel.Phase != duelCountdown {
		t.Fatalf("counting down: duel %d, on %v, phase %d", id, on, duel.Phase)
	}
	swing := Swing{Kind: 0, Delay: 3 * time.Second, Reach: 4.5}
	if d.w.EngageAttack(d.sa, d.aSeesB, swing, d.at(2*time.Second)) {
		t.Fatal("a swing at the opponent was taken in the countdown")
	}
	for _, c := range []struct {
		at   time.Duration
		want []string
	}{
		{5900 * time.Millisecond, nil},
		{6 * time.Second, []string{"Duel starts in 10 seconds."}},
		{6100 * time.Millisecond, nil},
		{11 * time.Second, []string{"Duel starts in 5 seconds."}},
		{12 * time.Second, nil},
		{13 * time.Second, []string{"3"}},
		{14 * time.Second, []string{"2"}},
		{15 * time.Second, []string{"1"}},
		{15900 * time.Millisecond, nil},
	} {
		d.tick(c.at)
		expect(c.at.String(), c.want...)
	}
	if duel.Phase != duelCountdown {
		t.Fatalf("begun before its time: phase %d", duel.Phase)
	}
	d.tick(16 * time.Second)
	a, b := duelLines(duelDrain(d.sa)), duelLines(duelDrain(d.sb))
	if !slices.Equal(a, []string{"START", "The duel with Beta has begun."}) || !slices.Equal(b, []string{"START", "The duel with Alpha has begun."}) {
		t.Fatalf("at the end: A %q, B %q", a, b)
	}
	if duel.Phase != duelOn || !duel.Began.Equal(d.at(16*time.Second)) || duel.X != 102 {
		t.Fatalf("the fight: phase %d, began %v, centre %v", duel.Phase, duel.Began.Sub(d.t0), duel.X)
	}
	if !d.w.EngageAttack(d.sa, d.aSeesB, swing, d.at(17*time.Second)) {
		t.Fatal("a swing at the opponent was refused after START")
	}

	// a tick that comes late (2.5 s left) says only "3"
	d = newDuelists(t)
	accept()
	d.tick(13500 * time.Millisecond)
	expect("a late tick", "3")
	d.tick(20 * time.Second)
	if a := duelLines(duelDrain(d.sa)); !slices.Equal(a, []string{"START", "The duel with Beta has begun."}) {
		t.Fatalf("after a late tick: %q", a)
	}
}

// A duel called off in its countdown is no fight: a cancel, a duelist parted
// from the other past DuelArenaRadius, dead, or gone. Both are told why, the
// wager held at the accept goes back to each its own (a gone duelist's onto
// its row), nobody forfeits and the bystander hears nothing.
func TestADuelCalledOffInItsCountdown(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(d *duelists)
		line string
	}{
		{"Beta cancels", func(d *duelists) { d.w.DuelCancel(d.sb, d.at(5*time.Second)) }, "Beta has called off the duel."},
		{"Alpha cancels", func(d *duelists) { d.w.DuelCancel(d.sa, d.at(5*time.Second)) }, "Alpha has called off the duel."},
		{"Alpha walks off", func(d *duelists) { d.a.X += DuelArenaRadius + 10; d.tick(5 * time.Second) }, "The duel is off."},
		{"Alpha dies", func(d *duelists) { d.a.HP = 0; d.tick(5 * time.Second) }, "The duel is off."},
		{"Beta logs out", func(d *duelists) { d.w.RemovePlayer(d.b.ServerID); d.w.runDuelPurse() }, "Beta has called off the duel."},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := newDuelists(t)
			var ops []DuelPurseOp
			d.w.OnDuelPurse(func(op DuelPurseOp) { ops = append(ops, op) })
			d.a.Tunar, d.b.Tunar, d.a.CharServerID, d.b.CharServerID = 100, 100, 71, 72
			d.w.DuelChallenge(d.sa, 0, "Beta", 40, d.at(0))
			duel := d.w.duels.of[d.a.ServerID]
			d.w.DuelAnswer(d.sb, 0, true, d.at(time.Second))
			if duel.Phase != duelCountdown || d.a.Tunar != 60 || d.b.Tunar != 60 || len(ops) != 1 || !ops[0].Hold {
				t.Fatalf("accepted: phase %d, purses %d and %d, ops %+v", duel.Phase, d.a.Tunar, d.b.Tunar, ops)
			}
			for _, s := range []*session.Session{d.sa, d.sb, d.sc} {
				duelDrain(s)
			}
			c.do(d)
			if d.w.duels.of[d.a.ServerID] != nil || d.w.duels.of[d.b.ServerID] != nil {
				t.Fatal("the duel is still there")
			}
			if len(ops) != 2 || ops[1].Hold || ops[1].Moves[0].Tunar != 40 || ops[1].Moves[1].Tunar != 40 {
				t.Fatalf("the stakes back: ops %+v", ops)
			}
			if d.a.Tunar != 100 {
				t.Fatalf("Alpha's purse %d, want 100", d.a.Tunar)
			}
			want := []string{c.line, "Your wager of 40 tunar is returned."}
			if got := duelLines(duelDrain(d.sa)); !slices.Equal(got, want) {
				t.Fatalf("Alpha was told %q, want %q", got, want)
			}
			if c.name == "Beta logs out" {
				if m := ops[1].Moves[1]; m.P != nil || m.CharID != 72 {
					t.Fatalf("the gone Beta's stake: %+v, want onto its row (72)", m)
				}
			} else {
				if got := duelLines(duelDrain(d.sb)); !slices.Equal(got, want) || d.b.Tunar != 100 {
					t.Fatalf("Beta was told %q, purse %d", got, d.b.Tunar)
				}
			}
			if got := duelLines(duelDrain(d.sc)); len(got) != 0 {
				t.Fatalf("the bystander was told %q", got)
			}
		})
	}
}
