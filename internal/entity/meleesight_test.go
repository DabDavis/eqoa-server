package entity

import (
	"testing"
	"time"
)

// A player's melee hit on a creature needs him to see it (Live petition 5567:
// "able to attack thru tents at nasehir"): through a wall 1.2 from him, the
// creature 2 off, nothing lands, no hate, "No line of sight." in red once (not
// a line a hit), and the fight is kept; the wall gone, his next hits land. The
// control: the same swing with nothing between lands.
func TestAMeleeHitNeedsSight(t *testing.T) {
	swing := func(c *caster, from time.Duration) {
		c.p.AutoAttack = true
		for _, d := range []time.Duration{250, 800, 1500, 3000, 4500} {
			c.w.mu.Lock()
			c.w.attackSystem(c.at(from + d*time.Millisecond))
			c.w.mu.Unlock()
		}
	}
	engage := func(c *caster) {
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		c.npc.X = 102
		if !c.w.EngageAttack(c.sess, c.npcO, Swing{Kind: 5, Delay: 1500 * time.Millisecond, Reach: 5}, c.at(0)) {
			t.Fatal("no fight")
		}
	}

	c := newCaster(t)
	c.w.SetGround(sightGround{wallAt(101.2)})
	engage(c)
	swing(c, 0)
	if c.npc.HP != 100000 || len(c.npc.Threats) != 0 {
		t.Errorf("through a wall: the creature at %d, threats %v", c.npc.HP, c.npc.Threats)
	}
	if c.p.AttackTarget == 0 {
		t.Error("the fight was let go")
	}
	red := redLines(c.sess)
	if len(red) == 0 || red[len(red)-1] != castNotSeen {
		t.Errorf("not told: %q", red)
	}
	told := 0
	for _, l := range red {
		if l == castNotSeen {
			told++
		}
	}
	if told > 2 {
		t.Errorf("told %d times in under 5 s", told)
	}
	c.w.SetGround(sightGround{})
	swing(c, 5*time.Second)
	if c.npc.HP == 100000 {
		t.Error("the wall gone, no hit landed")
	}

	c = newCaster(t)
	c.w.SetGround(sightGround{})
	engage(c)
	swing(c, 0)
	if c.npc.HP == 100000 || len(c.npc.Threats) == 0 {
		t.Errorf("in sight: the creature at %d, threats %v", c.npc.HP, c.npc.Threats)
	}
	if red := redLines(c.sess); len(red) != 0 && red[len(red)-1] == castNotSeen {
		t.Errorf("told out of sight in plain sight: %q", red)
	}
}
