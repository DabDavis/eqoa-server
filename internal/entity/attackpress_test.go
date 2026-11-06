package entity

import (
	"testing"
	"time"
)

// A press starts no round before CLIENT's own swing timer would let the
// client swing (pressed: the server's copy, swingTimer, less pressSlack),
// so a client that sends 0x000F faster than the game does swings no faster
// than its weapon, though the limiter in front of it (handlers/security.go)
// now lets the game's own 2 s presses through: a round begun at 0 with a 3 s
// swing, a retarget (the fight ends, the client's timer goes on), then a
// press at 1 s on the creature again begins its round at 2.5 s, not at 1;
// and after a swing out of reach (0x00D8: the client's timer 2 s on) a press
// 2 s later, the game's own, begins its round at once. The control: the
// same early press with no timer kept (a fresh player) begins at once.
func TestAPressWaitsForTheClientsSwingTimer(t *testing.T) {
	f := newFight(t)
	swing := Swing{Kind: 0, Delay: 3 * time.Second, Reach: 4.5}
	f.p.AutoAttack = true // the button held
	at := func(d time.Duration) time.Time { return f.t0.Add(d) }
	if !f.w.EngageAttack(f.sess, f.npcID, swing, at(0)) {
		t.Fatal("not engaged")
	}
	f.tick(300 * time.Millisecond)
	if !f.p.RoundStart.Equal(at(0)) {
		t.Fatalf("the first round began at %v, want 0", f.p.RoundStart.Sub(f.t0))
	}
	f.w.StopAttackOnRetarget(f.sess, 0x0131_3012) // another target: the fight ends, the timer goes on
	if !f.w.EngageAttack(f.sess, f.npcID, swing, at(time.Second)) {
		t.Fatal("not engaged again")
	}
	for _, d := range []time.Duration{1300 * time.Millisecond, 2 * time.Second, 2400 * time.Millisecond} {
		f.tick(d)
		if !f.p.RoundStart.IsZero() {
			t.Fatalf("an early press's round began at %v (tick %v); want 2.5 s", f.p.RoundStart.Sub(f.t0), d)
		}
	}
	f.tick(2600 * time.Millisecond)
	if !f.p.RoundStart.Equal(at(2500 * time.Millisecond)) {
		t.Fatalf("the early press's round began at %v, want 2.5 s", f.p.RoundStart.Sub(f.t0))
	}

	// the game's own: a swing out of reach, then a press 2 s later
	f.w.SwingOutOfReach(f.sess, at(5500*time.Millisecond))
	f.w.StopAttackOnRetarget(f.sess, 0x0131_3012)
	if !f.w.EngageAttack(f.sess, f.npcID, swing, at(7500*time.Millisecond)) {
		t.Fatal("not engaged after the retry")
	}
	f.tick(7800 * time.Millisecond)
	if !f.p.RoundStart.Equal(at(7500 * time.Millisecond)) {
		t.Fatalf("the press 2 s after a swing out of reach began its round at %v, want 7.5 s", f.p.RoundStart.Sub(f.t0))
	}

	// the control: no timer kept
	g := newFight(t)
	g.p.AutoAttack = true
	if !g.w.EngageAttack(g.sess, g.npcID, swing, g.t0.Add(time.Second)) {
		t.Fatal("the control: not engaged")
	}
	g.tick(1300 * time.Millisecond)
	if !g.p.RoundStart.Equal(g.t0.Add(time.Second)) {
		t.Errorf("the control's round began at %v, want 1 s", g.p.RoundStart.Sub(g.t0))
	}
}
