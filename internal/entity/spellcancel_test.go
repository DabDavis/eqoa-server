package entity

import (
	"testing"
	"time"
)

// Take a Shield Break (board 875): its line "Self, Cancel Sacred Shield, Cancel
// Divine Shield" is read whole, helping, and cast with Sacred Shield on it takes
// the shield off: AC back, no effect left. The control: cast with no
// shield on, a harmful effect on the paladin stays (it is no cure).
func TestTakeAShieldBreakCancelsTheShields(t *testing.T) {
	brk := CastSpell{SpellID: 901008, DisplayIndex: 2, Scope: 0, Name: "Take a Shield Break", Range: 30,
		Def: ParseSpellNamed("Take a Shield Break", "Self, Cancel Sacred Shield, Cancel Divine Shield", "Cancels the Sacred Shield and/or Divine Shield abilities")}
	if d := brk.Def; len(d.Unread) != 0 || d.Harmful || len(d.Effects) != 2 || d.Effects[0].Kind != EffectCancel ||
		d.Effects[0].Spell != "sacred shield" || d.Effects[1].Spell != "divine shield" {
		t.Fatalf("the line read %+v", d)
	}
	shield := CastSpell{SpellID: 700010, DisplayIndex: 1, Scope: 0, Name: "Sacred Shield", Range: 30,
		Def: ParseSpellNamed("Sacred Shield", "Self, Increase AC by 40, Decrease Movement Speed by 30, 5min", "")}

	c := newCaster(t)
	ac, speed := c.p.StatNow(StatAC), c.p.SpeedNow()
	if !c.w.StartCast(c.sess, shield, c.me, c.at(0)) {
		t.Fatal("the shield was not cast")
	}
	c.tick(time.Second)
	if c.p.StatNow(StatAC) != ac+40 || len(c.p.Effects) != 1 {
		t.Fatalf("with the shield: AC %d (was %d), speed %v (was %v), %d effects", c.p.StatNow(StatAC), ac, c.p.SpeedNow(), speed, len(c.p.Effects))
	}
	if !c.w.StartCast(c.sess, brk, c.me, c.at(10*time.Second)) {
		t.Fatal("the break was not cast")
	}
	c.tick(11 * time.Second)
	if c.p.StatNow(StatAC) != ac || c.p.SpeedNow() != speed || len(c.p.Effects) != 0 {
		t.Errorf("after the break: AC %d (want %d), speed %v (want %v), %d effects", c.p.StatNow(StatAC), ac, c.p.SpeedNow(), speed, len(c.p.Effects))
	}

	// the control: no shield on, a harmful effect is not cured
	c = newCaster(t)
	c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: 5555, Name: "Poison Bolt", HPTick: -10, Rules: EffectRules{Harm: true},
		Until: c.at(time.Minute), NextTick: c.at(time.Minute)})
	if !c.w.StartCast(c.sess, brk, c.me, c.at(0)) {
		t.Fatal("the control: the break was not cast")
	}
	c.tick(time.Second)
	if len(c.p.Effects) != 1 || c.p.Effects[0].SpellID != 5555 {
		t.Errorf("the control: effects %+v", c.p.Effects)
	}
}
