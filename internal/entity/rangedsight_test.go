package entity

import (
	"testing"
	"time"
)

// A ranged shot needs its shooter to see its target (Hoofbite's petition
// 1924: Tinael Windstream pulled through a wall with a bow): with a wall
// between them nothing lands, no hate, no power paid, "You cannot see your
// target." in red, the fight let go; a throw the same. The controls: the same
// shot with nothing between lands, and a melee swing is not held to it.
func TestARangedShotNeedsSight(t *testing.T) {
	shoot := func(c *caster, kind int) {
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		c.npc.X = 115 // 15 off, a bow's reach
		swing := Swing{Kind: kind, Delay: 3500 * time.Millisecond, Reach: 20}
		if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
			t.Fatal("no fight")
		}
		c.p.AutoAttack = true
		for _, d := range []time.Duration{250, 800} {
			c.w.mu.Lock()
			c.w.attackSystem(c.at(d * time.Millisecond))
			c.w.mu.Unlock()
		}
	}
	wall := sightGround{wallAt(108)}

	c := newCaster(t)
	c.w.SetGround(wall)
	c.p.Power = 900
	shoot(c, swingRange1)
	if c.npc.HP != 100000 || c.p.Power != 900 || len(c.npc.Threats) != 0 || c.p.AttackTarget != 0 {
		t.Errorf("through a wall: the creature at %d, %d power, threats %v, fighting %d", c.npc.HP, c.p.Power, c.npc.Threats, c.p.AttackTarget)
	}
	if red := redLines(c.sess); len(red) == 0 || red[len(red)-1] != castNotSeen {
		t.Errorf("not told: %q", red)
	}

	c = newCaster(t)
	c.w.SetGround(sightGround{})
	c.p.Power = 900
	shoot(c, swingRange1)
	if c.npc.HP == 100000 || len(c.npc.Threats) == 0 {
		t.Errorf("in sight: the creature at %d, threats %v", c.npc.HP, c.npc.Threats)
	}

	c = newCaster(t)
	c.w.SetGround(sightGround{wallAt(100.8)})
	c.npc.X = 102
	swing := Swing{Kind: 5, Delay: 3500 * time.Millisecond, Reach: 20}
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0))
	c.p.AutoAttack = true
	for _, d := range []time.Duration{250, 800} {
		c.w.mu.Lock()
		c.w.attackSystem(c.at(d * time.Millisecond))
		c.w.mu.Unlock()
	}
	if c.npc.HP == 100000 {
		t.Error("a melee swing was held to sight")
	}
}

// A harmful spell at a creature its caster cannot see is refused before it
// begins (the owner, 2026-11-13: "casters that cast from behind a barrier ...
// still drains mana and visually casts but no damage"): no cast, no power
// spent, "You cannot see your target." in red. The control: with nothing
// between, the same spell begins.
func TestAHarmfulSpellNeedsSightToBegin(t *testing.T) {
	nuke := CastSpell{SpellID: 9901, Name: "Test Nuke", Level: 1, Scope: 1, Range: 30, PowerCost: 50, CastTime: 2 * time.Second,
		Def: SpellDef{Harmful: true}}
	c := newCaster(t)
	c.w.SetGround(sightGround{wallAt(101)})
	if c.w.StartCast(c.sess, nuke, c.npcO, c.at(0)) {
		t.Fatal("a nuke through a wall began")
	}
	if c.p.Power != 900 {
		t.Errorf("power spent: %d", c.p.Power)
	}
	if red := redLines(c.sess); len(red) == 0 || red[len(red)-1] != castNotSeen {
		t.Errorf("not told: %q", red)
	}
	c = newCaster(t)
	c.w.SetGround(sightGround{})
	if !c.w.StartCast(c.sess, nuke, c.npcO, c.at(0)) {
		t.Fatal("the control: a nuke in sight did not begin")
	}
}
