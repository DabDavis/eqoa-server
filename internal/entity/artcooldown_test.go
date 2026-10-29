package entity

import (
	"testing"
	"time"
)

// artOf is a combat art: no cast time, cast at a creature.
func artOf(id int, name string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: 20, Scope: 1, Name: name, Recast: 10 * time.Second,
		PowerCost: 5, Range: 10, Def: ParseSpellText("Target, Decrease Hitpoints by 3")}
}

// The arts share the balance's cooldown (1.5 s in the seed rows): a second
// art inside it is not answered, one after it is; a spell with a cast time
// is not held by it.
func TestCombatArtsShareACooldown(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	c.w.balance = SeedBalance()
	if got := c.w.balance.ArtCooldown(c.p.Level); got != 1500*time.Millisecond {
		t.Fatalf("the seed's cooldown at %d: %v", c.p.Level, got)
	}
	kick, bash := artOf(801, "Kick"), artOf(802, "Bash")
	if !c.w.StartCast(c.sess, kick, c.npcO, c.at(0)) {
		t.Fatal("the first art was not begun")
	}
	c.tick(500 * time.Millisecond)
	if c.w.StartCast(c.sess, bash, c.npcO, c.at(1000*time.Millisecond)) {
		t.Error("a second art 1 s after the first began")
	}
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(1100*time.Millisecond)) {
		t.Error("a spell with a cast time was held by the arts' cooldown")
	}
	c.tick(4 * time.Second)
	if !c.w.StartCast(c.sess, bash, c.npcO, c.at(4*time.Second)) {
		t.Error("an art after the cooldown was not begun")
	}
}

// The control: with no balance rows there is no cooldown (the game before
// it): a second art 0.5 s after the first begins.
func TestNoArtCooldownWithoutTheBalance(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	if !c.w.StartCast(c.sess, artOf(801, "Kick"), c.npcO, c.at(0)) {
		t.Fatal("the first art was not begun")
	}
	c.tick(450 * time.Millisecond)
	if !c.w.StartCast(c.sess, artOf(802, "Bash"), c.npcO, c.at(500*time.Millisecond)) {
		t.Error("with no balance a second art was held")
	}
}
