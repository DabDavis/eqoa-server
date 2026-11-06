package entity

import (
	"testing"
	"time"
)

// Petition 1112: "switching targets or un con'ing target results in combat
// breaking. you have to go into menus unequip weapon and reequip it. then damage
// resumes". CLIENT's ToggleAutoAttack sends its 0x000F only with its swing timer
// run out; a press on a new target within a swing's delay of the last sends
// nothing, and AutoAttackTick swings at it from the timer's end for good.

// A second creature for the rig's fight: its object id, as the client knows it.
const unsentOtherID uint32 = 0x0131_3012

var swing6s2 = Swing{Kind: 5, Delay: 6 * time.Second, Reach: 4.5}

func (f *fight) other() *NPC {
	npc := &NPC{ServerID: 2003, Name: "Second Crab", Level: 15, HP: 100000, MaxHP: 100000, X: 101, Y: 10, Z: 101}
	f.w.NPCs[npc.ServerID] = npc
	vs := GetOrCreateVisibility(f.sess)
	vs.ChannelEntity[1] = npc.ServerID
	vs.ChannelObjectID[1] = unsentOtherID
	return npc
}

// target is a 0x0010 as HandleTarget takes it: the old fight let go, the new one named.
func (f *fight) target(objectID uint32) {
	f.w.StopAttackOnRetarget(f.sess, objectID)
	f.w.SetTarget(f.p.ServerID, objectID)
}

// button is a 0x40's byte 36 at d.
func (f *fight) button(up bool, d time.Duration) {
	f.w.ApplyClientUpdate(f.p.ServerID, ClientUpdate{X: f.p.X, Y: f.p.Y, Z: f.p.Z, HasAutoAttack: true, AutoAttack: up, At: f.t0.Add(d)})
}

// A fight on one creature, 6 s rounds; the player changes target 3 s in and
// presses within the client's timer, so no 0x000F comes. Its client swings at
// the new creature at 6 s and every 6 s after: so does the server, on that beat.
// The old creature is hit no more. On the code before, nothing landed on the new
// creature, whatever came after.
func TestAPressWithinTheTimerFightsTheNewTarget(t *testing.T) {
	f := newFight(t)
	b := f.other()
	at := func(d time.Duration) time.Time { return f.t0.Add(d) }
	f.target(f.npcID)
	f.button(true, 0)
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s2, at(0)) {
		t.Fatal("not engaged")
	}
	f.tick(300 * time.Millisecond)
	f.tick(3 * time.Second)
	hp := f.npc.HP
	if hp == f.npc.MaxHP {
		t.Fatal("the first fight landed nothing: the test proves nothing")
	}

	f.target(unsentOtherID)
	if f.p.AttackTarget != 0 || f.p.AutoAttack {
		t.Fatalf("after the change of target: fighting %d, button %v; want none, down", f.p.AttackTarget, f.p.AutoAttack)
	}
	f.button(true, 3500*time.Millisecond) // the press: within the timer, no 0x000F
	for _, d := range []time.Duration{3600 * time.Millisecond, 5 * time.Second, 5900 * time.Millisecond} {
		f.tick(d)
		if f.p.AttackTarget != 0 {
			t.Fatalf("engaged at %v, ahead of the client's swing at 6 s", d)
		}
	}
	f.tick(6050 * time.Millisecond)
	if f.p.AttackTarget != b.ServerID || f.p.AttackObjectID != unsentOtherID {
		t.Fatalf("at the client's swing: fighting %d (%#x), want the new creature %d", f.p.AttackTarget, f.p.AttackObjectID, b.ServerID)
	}
	if !f.p.RoundStart.Equal(at(6*time.Second)) || !f.p.NextSwing.Equal(at(12*time.Second)) || !f.p.swingTimer.Equal(at(12*time.Second)) {
		t.Errorf("its round from %v, next %v, timer %v; want the client's beat, 6 s and 12 s", f.p.RoundStart.Sub(f.t0),
			f.p.NextSwing.Sub(f.t0), f.p.swingTimer.Sub(f.t0))
	}
	if b.HP != b.MaxHP {
		t.Errorf("hit before the round's first hit is due: %d of %d", b.HP, b.MaxHP)
	}
	f.tick(6600 * time.Millisecond)
	if b.HP >= b.MaxHP {
		t.Fatalf("the new creature took nothing by 6.6 s: %d of %d", b.HP, b.MaxHP)
	}
	f.tick(9 * time.Second)
	f.tick(12050 * time.Millisecond)
	if !f.p.RoundStart.Equal(at(12 * time.Second)) {
		t.Errorf("the next round from %v, want 12 s", f.p.RoundStart.Sub(f.t0))
	}
	if f.npc.HP != hp {
		t.Errorf("the old creature was hit after the change of target: %d, was %d", f.npc.HP, hp)
	}
}

// The commonest way there: a kill, the next creature targeted and pressed before
// the timer runs out.
func TestAPressWithinTheTimerAfterAKillFights(t *testing.T) {
	f := newFight(t)
	b := f.other()
	f.target(f.npcID)
	f.button(true, 0)
	f.npc.HP = 1
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s2, f.t0) {
		t.Fatal("not engaged")
	}
	f.tick(300 * time.Millisecond)
	f.tick(700 * time.Millisecond)
	if !f.npc.Dead || f.p.AttackTarget != 0 {
		t.Fatalf("the kill: dead %v, fighting %d", f.npc.Dead, f.p.AttackTarget)
	}
	f.target(unsentOtherID)
	f.button(true, 1200*time.Millisecond)
	f.tick(2 * time.Second)
	if f.p.AttackTarget != 0 {
		t.Fatal("engaged ahead of the client's swing")
	}
	f.tick(6050 * time.Millisecond)
	f.tick(6600 * time.Millisecond)
	if b.HP >= b.MaxHP {
		t.Errorf("the next creature took nothing: fighting %d, %d of %d", f.p.AttackTarget, b.HP, b.MaxHP)
	}
}

// The button down again before the client's swing is a tap: AutoAttackTick's own
// test, and nothing starts. The next press, the timer run out, brings its 0x000F
// and begins as ever. The control for all of it: a target it may not fight, by
// every test a 0x000F's goes by, begins nothing with the button up.
func TestATapAndWhatMayNotBeFoughtStartNothing(t *testing.T) {
	f := newFight(t)
	b := f.other()
	f.target(f.npcID)
	f.button(true, 0)
	f.w.EngageAttack(f.sess, f.npcID, swing6s2, f.t0)
	f.tick(300 * time.Millisecond)
	f.target(unsentOtherID)
	f.button(true, 2*time.Second)
	f.button(false, 3*time.Second)
	f.tick(7 * time.Second)
	if f.p.AttackTarget != 0 || b.HP != b.MaxHP {
		t.Fatalf("a tap: fighting %d, %d of %d", f.p.AttackTarget, b.HP, b.MaxHP)
	}
	f.button(true, 7*time.Second)
	if !f.w.EngageAttack(f.sess, unsentOtherID, swing6s2, f.t0.Add(7*time.Second)) {
		t.Fatal("the 0x000F: not engaged")
	}
	if !f.p.NextSwing.Equal(f.t0.Add(7 * time.Second)) {
		t.Errorf("the 0x000F's round from %v, want its press at 7 s", f.p.NextSwing.Sub(f.t0))
	}

	for name, spoil := range map[string]func(n *NPC){
		"dead":         func(n *NPC) { n.Dead = true },
		"unattackable": func(n *NPC) { n.NPCType |= 0x80 },
		"a pet":        func(n *NPC) { n.Owner = 1001 },
	} {
		g := newFight(t)
		c := g.other()
		spoil(c)
		g.w.HandsChanged(g.sess, swing6s2)
		g.target(unsentOtherID)
		g.button(true, time.Second)
		g.tick(10 * time.Second)
		if g.p.AttackTarget != 0 {
			t.Errorf("%s: engaged %d", name, g.p.AttackTarget)
		}
	}
	h := newFight(t)
	h.w.HandsChanged(h.sess, swing6s2)
	h.target(0xFFFFFFFF)
	h.p.AutoAttack = true
	h.tick(10 * time.Second)
	if h.p.AttackTarget != 0 {
		t.Errorf("no target: engaged %d", h.p.AttackTarget)
	}
}

// A press with the timer run out, as the server follows it, brings a 0x000F: it
// is waited for, and it starts the round from its own press. A timer the server
// lost sight of (a restart's) leaves none to come, so a button up that long
// with none is a press too, from now: the kept client swinging on is fought for.
func TestARunOutTimerWaitsForTheZeroXF(t *testing.T) {
	f := newFight(t)
	b := f.other()
	f.w.HandsChanged(f.sess, swing6s2) // the hands, as a restart's resume reads them
	f.target(unsentOtherID)
	f.button(true, 10*time.Second)
	f.tick(10100 * time.Millisecond)
	if f.p.AttackTarget != 0 {
		t.Fatal("engaged with its 0x000F still on the way")
	}
	f.tick(10600 * time.Millisecond)
	if f.p.AttackTarget != b.ServerID {
		t.Fatalf("after the wait: fighting %d, want the creature", f.p.AttackTarget)
	}
	if !f.p.RoundStart.Equal(f.t0.Add(10600 * time.Millisecond)) {
		t.Errorf("its round from %v, want the tick at 10.6 s", f.p.RoundStart.Sub(f.t0))
	}
	f.tick(11200 * time.Millisecond)
	if b.HP >= b.MaxHP {
		t.Errorf("the creature took nothing: %d of %d", b.HP, b.MaxHP)
	}

	g := newFight(t)
	g.other()
	g.w.HandsChanged(g.sess, swing6s2)
	g.target(unsentOtherID)
	g.button(true, 10*time.Second)
	if !g.w.EngageAttack(g.sess, unsentOtherID, swing6s2, g.t0.Add(10050*time.Millisecond)) {
		t.Fatal("the 0x000F: not engaged")
	}
	g.tick(10100 * time.Millisecond)
	if !g.p.NextSwing.Equal(g.t0.Add(10050 * time.Millisecond)) {
		t.Errorf("the 0x000F's round from %v, want its press", g.p.NextSwing.Sub(g.t0))
	}
}

// What the press swings with is the hands': a change of them out of a fight
// counts (HandsChanged), and one a fight left waiting for its next round (the
// kill came first) does too. The client's look out of reach moves its timer with
// no fight on as with one, so the round waits for its next look.
func TestAnUnsentPressSwingsTheHandsOnTheClientsTimer(t *testing.T) {
	bow := Swing{Kind: 11, Delay: 5 * time.Second, Reach: 30}
	f := newFight(t)
	f.other()
	f.w.HandsChanged(f.sess, swing6s2)
	f.w.HandsChanged(f.sess, bow)
	f.target(unsentOtherID)
	f.p.buttonUpAt, f.p.AutoAttack = f.t0, true
	f.tick(time.Second)
	if f.p.AttackTarget == 0 || f.p.Swing != bow {
		t.Fatalf("fighting %d with %+v; want the bow", f.p.AttackTarget, f.p.Swing)
	}

	g := newFight(t)
	g.target(g.npcID)
	g.button(true, 0)
	g.w.EngageAttack(g.sess, g.npcID, swing6s2, g.t0)
	g.w.HandsChanged(g.sess, bow)
	g.w.mu.Lock()
	g.p.disengage()
	g.w.mu.Unlock()
	if g.p.Swing != bow {
		t.Errorf("after a fight left with new hands: %+v, want the bow", g.p.Swing)
	}

	h := newFight(t)
	c := h.other()
	h.target(h.npcID)
	h.button(true, 0)
	h.w.EngageAttack(h.sess, h.npcID, swing6s2, h.t0)
	h.tick(300 * time.Millisecond)
	h.target(unsentOtherID)
	h.button(true, 3*time.Second)
	h.w.SwingOutOfReach(h.sess, h.t0.Add(6*time.Second))
	h.tick(6100 * time.Millisecond)
	if h.p.AttackTarget != 0 {
		t.Fatal("engaged with the client's swing out of reach")
	}
	h.tick(8050 * time.Millisecond)
	if h.p.AttackTarget != c.ServerID || !h.p.RoundStart.Equal(h.t0.Add(8*time.Second)) {
		t.Errorf("at its next look: fighting %d from %v; want the creature from 8 s", h.p.AttackTarget, h.p.RoundStart.Sub(h.t0))
	}
}
