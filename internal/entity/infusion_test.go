package entity

import (
	"testing"
	"time"
)

// infusion is an alchemist's infusion (health into power, cast 2 s) with its
// line as tools/fix_balance_spells.py words it, or as it was.
func infusion(line string) CastSpell {
	return CastSpell{SpellID: 700532, DisplayIndex: 7, Level: 45, Scope: 0, Name: "Virulent Infusion",
		CastTime: 2 * time.Second, Range: 30, Def: ParseSpellText(line)}
}

// An infusion worded as a pact's price (tools/fix_balance_spells.py) costs
// its caster its hit points at the landing and gives its power every tick
// for 18 seconds; the control: as the importer worded it, "SELF Decrease HP
// by N" is a self line's own price, paid once and in full since jib's ruling
// (selfCostLocked; it was never paid before, a heal of a negative amount
// healing nothing), and Virulent's "Buff HP by 1000" heals 1,000.
func TestAnInfusionPaysItsPrice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP, c.p.Power, c.p.MaxPower = 2000, 3000, 100, 9000
	if !c.w.StartCast(c.sess, infusion("Self, Decrease Hitpoints by 1000 to Self, Increase Power per tic by 600, 18sec"), 0xFFFFFFFF, c.at(0)) {
		t.Fatal("not begun")
	}
	c.tick(2*time.Second + castLand)
	if c.p.HP != 1000 {
		t.Errorf("after the landing: %d HP, want 1000 (the price paid)", c.p.HP)
	}
	if c.p.Power != 700 {
		t.Errorf("after the landing: %d power, want 700 (its first tick)", c.p.Power)
	}
	c.tick(2*time.Second + castLand + 6*time.Second)
	c.tick(2*time.Second + castLand + 12*time.Second)
	if c.p.Power != 1900 {
		t.Errorf("after 12 s: %d power, want 1900 (three ticks)", c.p.Power)
	}

	for _, was := range []struct {
		line   string
		wantHP int
	}{
		{"SELF Decrease HP by 1000 and Increase Power per tic by 600 for 18 seconds", 1000},
		{"SELF Buff HP by 1000 and Power per tic by 600 for 18 seconds", 3000},
	} {
		c := newCaster(t)
		c.p.HP, c.p.MaxHP, c.p.Power, c.p.MaxPower = 2000, 3000, 100, 9000
		c.w.StartCast(c.sess, infusion(was.line), 0xFFFFFFFF, c.at(0))
		c.tick(2*time.Second + castLand)
		if c.p.HP != was.wantHP || c.p.Power != 700 {
			t.Errorf("the control %q: %d HP and %d power, want %d and 700", was.line, c.p.HP, c.p.Power, was.wantHP)
		}
	}
}
