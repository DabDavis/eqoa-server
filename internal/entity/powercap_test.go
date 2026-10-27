package entity

import (
	"testing"
	"time"
)

// The ranger's Eye lines, as EQOA-Data has them (and Allakhazam's archived
// records of the game's, AK 1587/1747/1748/1749/1750), read whole: the WIS,
// a ceiling of 0 on the maximum power, the power a tick and 45 seconds. The
// controls: a raise of the maximum stays a raise, and a gate's price with
// its teleport in one clause is left to the gate (gates.go reads it).
func TestEyeLinesReadTheirCeiling(t *testing.T) {
	for _, c := range []struct {
		line     string
		wis, pot int
	}{
		{"Self, Increase WIS by 90, MaxPower to 0, Increase Power per tic by 15,45sec", 90, 15},
		{"Self, Increase WIS by 140, Set MaxPower to 0, Increase Power per tic by 20, 45sec", 140, 20},
		{"Self, Increase WIS by 190, Set MaxPower to 0, Increase Power per tic by 20, 45sec", 190, 20},
		{"Self, Increase WIS by 240, Set MaxPower to 0, Increase Power per tic by 25, 45sec", 240, 25},
		{"Self, Increase WIS by 290, Set MaxPower to 0, Increase Power per tic by 30, 45sec", 290, 30},
		{"Self, Increase WIS by 305, Set Maximum Power to 0, 45sec", 305, 0},
	} {
		d := ParseSpellText(c.line)
		var wis, pot, caps int
		top := -1
		for _, e := range d.Effects {
			switch {
			case e.Kind == EffectStat && e.Stat == StatWis:
				wis = e.Amount
			case e.Kind == EffectPowerTick:
				pot = e.Amount
			case e.Kind == EffectPowerCap:
				caps++
				top = e.Amount
			}
		}
		if wis != c.wis || pot != c.pot || caps != 1 || top != 0 || d.Duration != 45*time.Second || len(d.Unread) != 0 || d.Harmful {
			t.Errorf("%q: WIS %d, PoT %d, %d ceilings at %d, %v, unread %q, harmful %v", c.line, wis, pot, caps, top, d.Duration, d.Unread, d.Harmful)
		}
	}
	if d := ParseSpellText("Self, Caps Power at 1500, Increase WIS by 305, 45sec"); len(d.Effects) != 2 || d.Effects[0].Kind != EffectPowerCap || d.Effects[0].Amount != 1500 {
		t.Errorf("caps power at 1500: %+v", d.Effects)
	}
	if d := ParseSpellText("Self, Increase INT by 10, MaxPower by 50, 10min"); len(d.Effects) != 2 || d.Effects[1].Kind != EffectStat || d.Effects[1].Stat != StatMaxPower {
		t.Errorf("the control, a raise of the maximum: %+v", d.Effects)
	}
	d := ParseSpellText("Group, Set Maximum Power to 0 and Teleport to Melton")
	if len(d.Effects) != 0 || len(d.Unread) != 1 {
		t.Errorf("the control, a gate's price beside its teleport: %+v, unread %q", d.Effects, d.Unread)
	}
	if _, drains := gateLine(&d); !drains {
		t.Error("the gate's price is no longer read")
	}
	if g := ParseSpellText("Group, Set Maximum Power to 0, teleport to SBM"); !g.onlyPowerCaps() {
		t.Errorf("a gate's price alone: %+v", g.Effects)
	} else if _, drains := gateLine(&g); !drains {
		t.Error("the gate's price read as a ceiling no longer takes the caster's power")
	}
}

func eyeSpell(line string) CastSpell {
	return CastSpell{SpellID: 700342, DisplayIndex: 12, Scope: 0, Name: "Raven Eye", Icon: 0x1234,
		CastTime: 0, Recast: 300 * time.Second, Range: 30, Def: ParseSpellNamed("Raven Eye", line, "")}
}

// Raven Eye on its caster: its maximum power is 0 while the Eye lasts, the
// power it had gone and the ticks' power held under the ceiling, its WIS
// raised; at the end the maximum is its own again and the power is where
// the Eye left it, to regenerate. The control: the same line without the
// ceiling raises the maximum by its WIS and takes no power.
func TestEyeHoldsMaximumPowerDown(t *testing.T) {
	c := newCaster(t)
	own, wis := c.p.MaxPowerNow(), c.p.StatNow(StatWis)
	c.land(t, eyeSpell("Self, Increase WIS by 140, Set MaxPower to 0, Increase Power per tic by 20, 45sec"), c.me, 0)
	if c.p.MaxPowerNow() != 0 || c.p.Power != 0 || c.p.StatNow(StatWis) != wis+140 {
		t.Fatalf("under the Eye: power %d of %d, WIS %d", c.p.Power, c.p.MaxPowerNow(), c.p.StatNow(StatWis))
	}
	c.tick(30 * time.Second) // ticks of the Eye's 20 power each
	if c.p.Power != 0 || c.p.MaxPowerNow() != 0 {
		t.Errorf("30 s in: power %d of %d", c.p.Power, c.p.MaxPowerNow())
	}
	c.tick(47 * time.Second) // landed at 1.4 s, gone 45 s after
	if len(c.p.Effects) != 0 || c.p.MaxPowerNow() != own || c.p.Power != 0 || c.p.StatNow(StatWis) != wis {
		t.Errorf("after the Eye: %d effects, power %d of %d (own %d), WIS %d", len(c.p.Effects), c.p.Power, c.p.MaxPowerNow(), own, c.p.StatNow(StatWis))
	}

	// the control
	d := newCaster(t)
	d.land(t, eyeSpell("Self, Increase WIS by 140, Increase Power per tic by 20, 45sec"), d.me, 0)
	if d.p.MaxPowerNow() <= own || d.p.Power < 900 {
		t.Errorf("the control: power %d of %d, own maximum %d", d.p.Power, d.p.MaxPowerNow(), own)
	}
}

// Two ceilings: the lower holds (an Eye of 0 under an epic's 1500).
func TestLowerPowerCeilingHolds(t *testing.T) {
	c := newCaster(t)
	c.p.Power, c.p.MaxPower = 3000, 3630
	epic := eyeSpell("Self, Increase WIS by 305, Set Maximum Power to 1500, 45sec")
	epic.SpellID, epic.Name = 701145, "Gelledyn's Perfect Vision"
	c.land(t, epic, c.me, 0)
	if c.p.MaxPowerNow() != 1500 || c.p.Power != 1500 {
		t.Fatalf("under the epic: power %d of %d", c.p.Power, c.p.MaxPowerNow())
	}
	c.land(t, eyeSpell("Self, Increase WIS by 140, Set MaxPower to 0, Increase Power per tic by 20, 45sec"), c.me, time.Second)
	if c.p.MaxPowerNow() != 0 || c.p.Power != 0 {
		t.Errorf("under both: power %d of %d", c.p.Power, c.p.MaxPowerNow())
	}
}

// A ceiling lasting no time takes the power down to it, and the maximum is
// left alone.
func TestInstantPowerCeilingTakesThePower(t *testing.T) {
	c := newCaster(t)
	own := c.p.MaxPowerNow()
	spell := CastSpell{SpellID: 9001, Scope: 0, Name: "Price", Range: 30, Def: ParseSpellNamed("Price", "Self, Set Maximum Power to 100", "")}
	c.land(t, spell, c.me, 0)
	if c.p.Power != 100 || c.p.MaxPowerNow() != own || len(c.p.Effects) != 0 {
		t.Errorf("power %d of %d (own %d), %d effects", c.p.Power, c.p.MaxPowerNow(), own, len(c.p.Effects))
	}
}

// The ceiling is kept over a logout with what is left of the Eye, and holds
// the power down again at the login.
func TestPowerCeilingKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.land(t, eyeSpell("Self, Increase WIS by 140, Set MaxPower to 0, Increase Power per tic by 20, 45sec"), c.me, 0)
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(10*time.Second))
	if !ok || len(rows) != 1 {
		t.Fatalf("kept %d (%v)", len(rows), ok)
	}
	d := newCaster(t)
	if n := d.w.RestoreEffects(d.sess, rows, d.at(time.Hour)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if d.p.MaxPowerNow() != 0 || d.p.Power != 0 {
		t.Errorf("after the login: power %d of %d", d.p.Power, d.p.MaxPowerNow())
	}
}
