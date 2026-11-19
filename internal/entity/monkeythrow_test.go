package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The monkeys' snowballs (monkeythrow.go; the owner, 2026-11-16: "make
// [a player]'s monkeys throw snowballs at players").

// wide is s as a client message carries it (UTF-16LE).
func wide(s string) []byte {
	var b []byte
	for _, c := range s {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

// saidTo reports whether sess was sent text since the last look, and empties
// its queue.
func saidTo(sess *session.Session, text string) bool {
	found := false
	for _, m := range sess.Queue().GetPending() {
		if bytes.Contains(m.Data(), wide(text)) {
			found = true
		}
	}
	sess.Queue().Clear()
	return found
}

// A try comes monkeyThrowMin to monkeyThrowMax after the last, both ends
// reached.
func TestMonkeyThrowCadence(t *testing.T) {
	old := monkeyPick
	t.Cleanup(func() { monkeyPick = old })
	now := time.Unix(1_800_000_000, 0)
	monkeyPick = func(n int) int { return 0 }
	if got := monkeyNextThrow(now).Sub(now); got != monkeyThrowMin {
		t.Errorf("the soonest: %v", got)
	}
	monkeyPick = func(n int) int { return n - 1 }
	if got := monkeyNextThrow(now).Sub(now); got != monkeyThrowMax {
		t.Errorf("the latest: %v", got)
	}
	monkeyPick = old
	for i := 0; i < 200; i++ {
		if d := monkeyNextThrow(now).Sub(now); d < monkeyThrowMin || d > monkeyThrowMax {
			t.Fatalf("a try %v on", d)
		}
	}
	if monkeyThrowMin != 20*time.Second || monkeyThrowMax != 40*time.Second || monkeyThrowGap != 30*time.Second {
		t.Error("the owner's coordinator's figures: 20-40 s a monkey, 30 s a player")
	}
}

// One monkey and one other player beside its owner: no throw before its time,
// one by monkeyThrowMax, at that player; 1 off its hit points and no more,
// never below 1; told to it ("<monkey> hits you with a snowball!") and to the
// owner; no PvP flag on either, no fight, no hate.
func TestAMonkeyHitsAPlayerWithASnowball(t *testing.T) {
	r := newMercRig(t)
	zsess, z := r.player("Zed", 0, 0)
	r.w.UseMonkeyBarrel(r.sess, r.at(0))
	ms := r.w.monkeys[r.p.ServerID][0]
	r.sess.Queue().Clear()
	zsess.Queue().Clear()
	top := z.MaxHPNow()
	z.HP = top
	hp := func(at time.Duration) int { r.monkeyTick(at); return z.HP }
	if got := hp(monkeyThrowMin - time.Second); got != top {
		t.Fatalf("a throw before monkeyThrowMin: HP %d of %d", got, top)
	}
	for d := monkeyThrowMin; d <= monkeyThrowMax && z.HP == top; d += 100 * time.Millisecond {
		r.monkeyTick(d)
	}
	if z.HP != top-1 {
		t.Fatalf("by monkeyThrowMax: HP %d, want %d", z.HP, top-1)
	}
	if !saidTo(zsess, ms.npc.Name+" hits you with a snowball!") {
		t.Error("Zed was not told")
	}
	if !saidTo(r.sess, ms.npc.Name+" hits Zed with a snowball!") {
		t.Error("the owner was not told")
	}
	if z.pvp.on || r.p.pvp.on || z.AttackTarget != 0 || !z.LastCombatTime.IsZero() || ms.npc.CurrentEnemy != 0 || len(ms.npc.Threats) != 0 {
		t.Errorf("a fight came of it: pvp %v/%v, target %d, combat %v, the monkey's enemy %d threats %d",
			z.pvp.on, r.p.pvp.on, z.AttackTarget, z.LastCombatTime, ms.npc.CurrentEnemy, len(ms.npc.Threats))
	}
	// at 1 HP it takes nothing
	z.HP = 1
	r.w.mu.Lock()
	r.w.monkeyThrowLocked(r.p, ms.npc, z, r.at(time.Hour))
	r.w.mu.Unlock()
	if z.HP != 1 {
		t.Errorf("a snowball at 1 HP left %d", z.HP)
	}
}

// Whom a monkey may hit: another player, alive, near, in sight, in its
// owner's world, not fighting, not invisible or hidden, not hit within the
// gap; its owner and NPCs never. Each in turn is made the one ruled out.
func TestMonkeyTargets(t *testing.T) {
	r := newMercRig(t)
	_, z := r.player("Zed", 0, 0)
	r.w.UseMonkeyBarrel(r.sess, r.at(0))
	n := r.w.monkeys[r.p.ServerID][0].npc
	crab := r.creature("Desert Crab", 28, 3)
	now := r.at(time.Minute)
	target := func() *Player {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return r.w.monkeyTargetLocked(r.p, n, now)
	}
	if got := target(); got != z {
		t.Fatalf("the control: %v, want Zed", got)
	}
	cases := []struct {
		name       string
		set, unset func()
	}{
		{"dead", func() { z.HP = 0 }, func() { z.HP = z.MaxHPNow() }},
		{"far", func() { z.X += SnowballRange + 5 }, func() { z.X -= SnowballRange + 5 }},
		{"another world", func() { z.World = 2 }, func() { z.World = 0 }},
		{"attacking", func() { z.AttackTarget, z.AutoAttack = crab.ServerID, true }, func() { z.AttackTarget, z.AutoAttack = 0, false }},
		{"hit lately", func() { z.LastCombatTime = now.Add(-5 * time.Second) }, func() { z.LastCombatTime = time.Time{} }},
		{"flagged for PvP", func() { z.pvp.on = true }, func() { z.pvp.on = false }},
		{"GM hidden", func() { z.GMHidden = true }, func() { z.GMHidden = false }},
		{"hit by a monkey within the gap", func() {
			r.w.monkeyThrown = map[EntityID]map[EntityID]time.Time{r.p.ServerID: {z.ServerID: now.Add(-monkeyThrowGap + time.Second)}}
		}, func() { r.w.monkeyThrown = nil }},
	}
	for _, c := range cases {
		c.set()
		if got := target(); got != nil {
			t.Errorf("%s: %q picked", c.name, got.Name)
		}
		c.unset()
	}
	// the gap gone by, it may be hit again
	r.w.monkeyThrown = map[EntityID]map[EntityID]time.Time{r.p.ServerID: {z.ServerID: now.Add(-monkeyThrowGap)}}
	if got := target(); got != z {
		t.Errorf("after the gap: %v", got)
	}
	// alone with its owner and a creature: no one
	r.w.RemovePlayer(z.ServerID)
	if got := target(); got != nil {
		t.Errorf("with only its owner and a crab: %q", got.Name)
	}
	hp := crab.HP
	for d := time.Duration(0); d <= 2*monkeyThrowMax; d += 500 * time.Millisecond {
		r.monkeyTick(d)
	}
	if crab.HP != hp || r.p.HP != r.p.MaxHPNow() && r.p.HP != 0 {
		t.Errorf("a snowball at an NPC or the owner: crab %d/%d, owner %d", crab.HP, hp, r.p.HP)
	}
}

// Five monkeys and one other player: in two minutes it is hit at most once
// every monkeyThrowGap, by all five together; a second player near is hit
// too (the gap is a player's). While their owner fights none throws.
func TestMonkeysThrowAtAPlayerOnceAGap(t *testing.T) {
	r := newMercRig(t)
	_, z := r.player("Zed", 0, 0)
	_, y := r.player("Yara", 0, 0)
	for i := 0; i < MonkeysMax; i++ {
		r.w.UseMonkeyBarrel(r.sess, r.at(0))
	}
	z.HP, y.HP = z.MaxHPNow(), y.MaxHPNow()
	var hitsZ, hitsY []time.Duration
	lastZ, lastY := z.HP, y.HP
	for d := time.Duration(0); d <= 2*time.Minute; d += 100 * time.Millisecond {
		r.monkeyTick(d)
		if z.HP != lastZ {
			hitsZ, lastZ = append(hitsZ, d), z.HP
		}
		if y.HP != lastY {
			hitsY, lastY = append(hitsY, d), y.HP
		}
	}
	if len(hitsZ) < 2 || len(hitsZ) > 5 || len(hitsY) < 2 {
		t.Fatalf("in two minutes: Zed hit at %v, Yara at %v", hitsZ, hitsY)
	}
	for _, hits := range [][]time.Duration{hitsZ, hitsY} {
		for i := 1; i < len(hits); i++ {
			if hits[i]-hits[i-1] < monkeyThrowGap {
				t.Errorf("two snowballs %v apart: %v", hits[i]-hits[i-1], hits)
			}
		}
	}

	// their owner fighting: nothing for two minutes
	r.w.monkeyThrown = nil
	r.p.AttackTarget, r.p.AutoAttack = 99, true
	lastZ, lastY = z.HP, y.HP
	for d := 3 * time.Minute; d <= 5*time.Minute; d += 100 * time.Millisecond {
		r.monkeyTick(d)
	}
	if z.HP != lastZ || y.HP != lastY {
		t.Errorf("a throw while their owner fights: Zed %d -> %d, Yara %d -> %d", lastZ, z.HP, lastY, y.HP)
	}
}
