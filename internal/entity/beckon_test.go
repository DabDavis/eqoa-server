package entity

import (
	"testing"
	"time"
)

// Arcane Beckon has no line in the database: its default (ours) raises the
// caster's CHA by 15 for 30 minutes; a GM's line in spelleffect comes
// first. The control: the empty line alone does nothing.
func TestArcaneBeckonRaisesCharisma(t *testing.T) {
	desc := "Fills you with an arcane presence, increasing charisma."
	d := ParseSpellNamed("Arcane Beckon", "", desc)
	if len(d.Effects) != 1 || d.Effects[0].Kind != EffectStat || d.Effects[0].Stat != StatCha || d.Effects[0].Amount != 15 ||
		d.Duration != 30*time.Minute || d.Target != TargetSelf || d.Harmful {
		t.Fatalf("Arcane Beckon: %+v", d)
	}
	if gm := ParseSpellNamed("Arcane Beckon", "Self, Increase CHA by 40, 1hr", desc); gm.Effects[0].Amount != 40 || gm.Duration != time.Hour {
		t.Errorf("a GM's line: %+v", gm)
	}
	if bare := ParseSpell("", desc); len(bare.Effects) != 0 {
		t.Error("the control: an empty line did something")
	}

	c := newCaster(t)
	beckon := CastSpell{SpellID: 520, DisplayIndex: 7, Scope: 0, Name: "Arcane Beckon", Icon: 0x9C8D3B60,
		CastTime: time.Second, Recast: 30 * time.Second, PowerCost: 45, Def: d}
	c.land(t, beckon, 0xFFFFFFFF, 0)
	if c.p.StatNow(StatCha) != 75 {
		t.Errorf("CHA %d under Arcane Beckon, want 60 + 15", c.p.StatNow(StatCha))
	}
}
