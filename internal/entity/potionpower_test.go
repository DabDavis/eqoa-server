package entity

import (
	"strings"
	"testing"
	"time"
)

// Petitions 1489 and 1581: a potion thrown by Use costs 10% of its thrower's
// most power (1,025: 103), and with less than that it is refused, told, nothing thrown;
// world.throw_power_pct sets the share (25: 256), 0 none.
func TestAThrowCostsPower(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100, 100
	c.p.Power = 150
	if r := c.use(t, thrown(), c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.p.Power != 47 || c.npc.HP != 80 {
		t.Errorf("after a throw: %d power, the creature at %d; want 47, 80", c.p.Power, c.npc.HP)
	}
	if r := c.use(t, thrown(), c.npcO, time.Second); r != ThrowNoPower {
		t.Errorf("47 power for a 103 throw: %q, want %q", r, ThrowNoPower)
	}
	if c.p.Power != 47 || c.npc.HP != 80 {
		t.Errorf("refused, yet %d power, the creature at %d", c.p.Power, c.npc.HP)
	}

	c = newCaster(t)
	c.w.SetThrowPowerPct(25)
	if r := c.use(t, thrown(), c.npcO, 0); r != "" || c.p.Power != 900-256 {
		t.Errorf("at 25%%: %q, %d power; want %d", r, c.p.Power, 900-256)
	}
	c = newCaster(t)
	c.w.SetThrowPowerPct(0)
	c.p.Power = 0
	if r := c.use(t, thrown(), c.npcO, 0); r != "" || c.p.Power != 0 || c.npc.HP != 10 {
		t.Errorf("at 0%%: %q, %d power, the creature at %d; want thrown, free", r, c.p.Power, c.npc.HP)
	}
}

// The ranged auto-attack's throw of a worn potion costs its power too, and
// short of it the throw is refused in red and the fight let go, nothing taken
// off the stack.
func TestAnAutoAttackThrowCostsPower(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100, 100
	c.p.Power = 150
	c.w.SetPotionEffects([]PotionEffect{thrown()})
	swing := Swing{Kind: 8, Delay: 3500 * time.Millisecond, Reach: 20, Potion: thrownID, PotionLeft: 3}
	if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
		t.Fatal("no fight")
	}
	c.p.AutoAttack = true
	round := func(d time.Duration) {
		c.w.mu.Lock()
		c.w.attackSystem(c.at(d))
		c.w.mu.Unlock()
	}
	round(250 * time.Millisecond)
	round(800 * time.Millisecond)
	if c.npc.HP != 80 || c.p.Power != 47 || c.p.Swing.PotionLeft != 2 {
		t.Fatalf("one throw: the creature at %d, %d power, %d left; want 80, 47, 2", c.npc.HP, c.p.Power, c.p.Swing.PotionLeft)
	}
	round(3750 * time.Millisecond)
	round(4300 * time.Millisecond)
	if c.npc.HP != 80 || c.p.Power != 47 || c.p.Swing.PotionLeft != 2 || c.p.AttackTarget != 0 {
		t.Errorf("short of power: the creature at %d, %d power, %d left, fighting %d; want 80, 47, 2, none",
			c.npc.HP, c.p.Power, c.p.Swing.PotionLeft, c.p.AttackTarget)
	}
	if red := redLines(c.sess); len(red) == 0 || red[len(red)-1] != ThrowNoPower {
		t.Errorf("not told: %q", red)
	}
}

// A draught's health against poison resist: none taken with a chance of 65%
// times PR over 315 (at most 315), else the whole of it. At PR 315 a roll
// under 6,500 (of 10,000) takes none, told, and at 6,500 the whole; at PR
// 100, 2,063; at PR 0 never.
func TestPoisonResistAgainstADraught(t *testing.T) {
	defer func(f func(int) int) { draughtIntn = f }(draughtIntn)
	for _, tc := range []struct {
		pr, roll int
		hp       int
	}{
		{315, 6499, 500}, {315, 6500, 440}, {400, 6499, 500}, {100, 2062, 500}, {100, 2063, 440}, {0, 0, 440},
	} {
		draughtIntn = func(int) int { return tc.roll }
		c := newCaster(t)
		c.p.PoisonR = tc.pr
		if r := c.use(t, draught(), 0, 0); r != "" {
			t.Fatalf("refused: %q", r)
		}
		if c.p.HP != tc.hp || c.p.Power != 950 {
			t.Errorf("PR %d, roll %d: %d HP, %d power; want %d, 950", tc.pr, tc.roll, c.p.HP, c.p.Power, tc.hp)
		}
		told := false
		for _, l := range redLines(c.sess) {
			told = told || strings.Contains(l, DraughtResisted)
		}
		if told != (tc.hp == 500) {
			t.Errorf("PR %d, roll %d: told %v", tc.pr, tc.roll, told)
		}
	}
}
