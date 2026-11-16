package entity

import (
	"testing"
	"time"
)

// Heirloom petition 2202 (Zerkziee, 2026-11-16, the Isle of Dread):
// "Megaladon fight doesnt seem right with range in the water I had to get on
// top of him to actually shoot him with a bow ability/attack". The shark's
// feet were under the beach the player stood on, so the lines to a man's head
// and waist over them ran into the sand, its back out of the water. A player's
// shot and harmful spell look at a big creature's body at its own heights
// (sight.go's seesCreatureLocked): from a beach 3.3 over its feet, 30 off, with
// the bank between, both land on a size-4 Megaladon (its body's edge 8.332, as
// models.txt has it). The controls: the same bank still hides a creature of a
// man's size, and a wall over the big body's head still hides it.
func TestABigCreatureUnderABankIsSeenByItsBody(t *testing.T) {
	const shark uint32 = 0x69842C02
	SetModelRadii(map[uint32]float32{shark: 8.332})
	t.Cleanup(func() { SetModelRadii(nil) })
	bank := sightBox{101, -10, 50, 128, 13.3, 150} // the beach, up to the player's feet
	rig := func(g sightGround, big bool) *caster {
		c := newCaster(t)
		c.w.SetGround(g)
		c.p.Y = 13.3
		c.npc.X, c.npc.Y = 130, 10
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		if big {
			c.npc.Name, c.npc.ModelID, c.npc.Size = "Megaladon", int64(shark), 4
		}
		return c
	}
	shoot := func(c *caster) bool {
		swing := Swing{Kind: swingRange1, Delay: 3500 * time.Millisecond, Reach: 35}
		if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(0)) {
			t.Fatal("no fight")
		}
		c.p.AutoAttack = true
		for _, d := range []time.Duration{250, 800} {
			c.w.mu.Lock()
			c.w.attackSystem(c.at(d * time.Millisecond))
			c.w.mu.Unlock()
		}
		return c.npc.HP != 100000
	}
	shot := CastSpell{SpellID: 9902, Name: "Test Burning Shot", Level: 1, Scope: 1, Range: 40, PowerCost: 50,
		Def: SpellDef{Harmful: true}}
	for _, k := range []struct {
		what string
		g    sightGround
		big  bool
		sees bool
	}{
		{"the Megaladon under the bank", sightGround{bank}, true, true},
		{"a man-sized creature under the bank", sightGround{bank}, false, false},
		{"the Megaladon behind a wall over its head", sightGround{bank, {115, -10, 50, 115.5, 16, 150}}, true, false},
		{"the Megaladon in the open", sightGround{}, true, true},
	} {
		if got := shoot(rig(k.g, k.big)); got != k.sees {
			t.Errorf("%s: a bow's shot landed %v, want %v", k.what, got, k.sees)
		}
		c := rig(k.g, k.big)
		if got := c.w.StartCast(c.sess, shot, c.npcO, c.at(0)); got != k.sees {
			t.Errorf("%s: a harmful art began %v, want %v", k.what, got, k.sees)
		}
	}
}
