package entity

import (
	"testing"
	"time"
)

// Petition 1581 (the GM Jibrish): a ranged weapon's shot costs exactly 10% of
// its shooter's most power (1,025: 103), paid as it is loosed in reach; short
// of it the shot is refused in red, nothing landed, the fight let go. A melee
// swing costs none; world.ranged_power_pct sets the share, 0 none; a thrown
// potion costs 10% too now.
func TestARangedShotCostsPower(t *testing.T) {
	shoot := func(c *caster, kind int) {
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		swing := Swing{Kind: kind, Delay: 3500 * time.Millisecond, Reach: 20}
		if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
			t.Fatal("no fight")
		}
		c.p.AutoAttack = true
		for _, d := range []time.Duration{250, 800, 3750, 4300} {
			c.w.mu.Lock()
			c.w.attackSystem(c.at(d * time.Millisecond))
			c.w.mu.Unlock()
		}
	}
	c := newCaster(t)
	c.p.Power = 900
	shoot(c, swingRange4)
	if c.p.Power != 900-2*103 || c.npc.HP == 100000 {
		t.Errorf("two shots: %d power, the creature at %d; want %d, hurt", c.p.Power, c.npc.HP, 900-2*103)
	}

	c = newCaster(t)
	c.p.Power = 150
	shoot(c, swingRange1)
	if c.p.Power != 150-103 || c.p.AttackTarget != 0 {
		t.Errorf("one shot then short: %d power, fighting %d; want %d, none", c.p.Power, c.p.AttackTarget, 150-103)
	}
	if red := redLines(c.sess); len(red) == 0 || red[len(red)-1] != ShotNoPower {
		t.Errorf("not told: %q", red)
	}

	c = newCaster(t)
	c.p.Power = 50
	shoot(c, swingRange1)
	if c.p.Power != 50 || c.npc.HP != 100000 || c.p.AttackTarget != 0 {
		t.Errorf("no power: %d power, the creature at %d, fighting %d; want 50, untouched, none", c.p.Power, c.npc.HP, c.p.AttackTarget)
	}

	c = newCaster(t)
	c.p.Power = 50
	shoot(c, 5) // a melee swing
	if c.p.Power != 50 || c.npc.HP == 100000 {
		t.Errorf("melee: %d power, the creature at %d; want 50, hurt", c.p.Power, c.npc.HP)
	}

	c = newCaster(t)
	c.w.SetRangedPowerPct(0)
	c.p.Power = 0
	shoot(c, swingRange1)
	if c.p.Power != 0 || c.npc.HP == 100000 {
		t.Errorf("at 0%%: %d power, the creature at %d; want 0, hurt", c.p.Power, c.npc.HP)
	}
	c = newCaster(t)
	c.w.SetRangedPowerPct(25)
	shoot(c, swingRange1)
	if c.p.Power != 900-256*2 {
		t.Errorf("at 25%%: %d power, want %d", c.p.Power, 900-256*2)
	}
}
