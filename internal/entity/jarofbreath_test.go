package entity

import (
	"testing"
	"time"
)

// The Heirloom Jar of Breath's use (handlers/jarofbreath.go: its itemeffect
// row, as tools/jar_of_breath_20261111.py writes it) against the cast tests'
// world (newCaster), under water by the client's walk flags.
func jarOfBreath() PotionEffect {
	return NewPotionEffect(715070, "Heirloom Jar of Breath", "Self, Enduring Breath, 4hr", "Enduring Breath", 1533256884, "", 0, 0)
}

// The line is drunk, not thrown, a breath of four hours.
func TestJarOfBreathLineReadsAsFourHoursOfBreath(t *testing.T) {
	pe := jarOfBreath()
	if pe.Thrown() || pe.duration() != 4*time.Hour || len(pe.Def.Effects) != 1 || pe.Def.Effects[0].Kind != EffectBreath ||
		pe.Def.Harmful || len(pe.Def.Unread) != 0 {
		t.Fatalf("read as thrown %v, %v, effects %+v, harmful %v, unread %q", pe.Thrown(), pe.duration(), pe.Def.Effects,
			pe.Def.Harmful, pe.Def.Unread)
	}
	// the bards' ballads' and Hold Breath's lines read the same now
	if d := ParseSpellText("Group, Enduring Breath, Radius 30, 90sec"); !d.hasBreath() || d.Duration != 90*time.Second {
		t.Errorf("a ballad's line: %+v", d)
	}
}

func (d *SpellDef) hasBreath() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectBreath {
			return true
		}
	}
	return false
}

// Used, its user loses no breath under water for four hours (the control
// without it loses a step in 6 s), then breath goes as before; kept over a
// logout with its rule; ended by death.
func TestJarOfBreathHoldsBreathUnderWaterForFourHours(t *testing.T) {
	c := newCaster(t)
	p := c.p
	p.Breath, p.WalkFlags, p.HasWalkFlags = BreathFull, 0x0004, true // under water
	under := func(at time.Duration, seconds int) {
		t.Helper()
		for range seconds * 10 / 6 { // a breath call every 0.6 s (TickCount%6)
			c.w.TickCount = 6
			c.w.breathSystem(c.at(at))
		}
		if !p.Underwater {
			t.Fatal("not under water")
		}
	}

	// the control: no jar, a step lost in 6 s
	under(0, 6)
	if p.Breath != BreathFull-BreathStep {
		t.Fatalf("without the jar: breath %d after 6 s, want %d", p.Breath, BreathFull-BreathStep)
	}
	p.Breath = BreathFull

	if r := c.use(t, jarOfBreath(), 0xFFFFFFFF, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	under(time.Second, 120) // two minutes, past the 48 s of air
	if p.Breath != BreathFull || p.HP != 500 {
		t.Fatalf("with the jar, 2 min under: breath %d, HP %d; want %d, 500", p.Breath, p.HP, BreathFull)
	}
	c.potionTick(4*time.Hour - time.Minute)
	under(4*time.Hour-time.Minute, 30)
	if !p.holdsBreath() || p.Breath != BreathFull {
		t.Fatalf("a minute before four hours: holds %v, breath %d", p.holdsBreath(), p.Breath)
	}

	// kept over a logout, its rule with it
	rows := savedEffects(p, c.at(4*time.Hour-time.Minute))
	if len(rows) != 1 || rows[0].LeftMS < 59_000 || rows[0].LeftMS > 61_000 {
		t.Fatalf("kept rows %+v, want one with a minute left", rows)
	}
	p.Effects, p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, time.Now()); n != 1 || !p.holdsBreath() {
		t.Fatalf("restored %d, holds %v; want 1, true", n, p.holdsBreath())
	}
	p.Effects, p.Buffs = nil, nil
	c.use(t, jarOfBreath(), 0xFFFFFFFF, 0)

	// its four hours end
	c.potionTick(4*time.Hour + time.Second)
	if p.holdsBreath() {
		t.Fatal("still held past four hours")
	}
	under(4*time.Hour+time.Second, 6)
	if p.Breath != BreathFull-BreathStep {
		t.Fatalf("past four hours: breath %d after 6 s, want %d", p.Breath, BreathFull-BreathStep)
	}

	// death ends it
	c.use(t, jarOfBreath(), 0xFFFFFFFF, 5*time.Hour)
	if !p.holdsBreath() {
		t.Fatal("used again: no breath held")
	}
	c.w.mu.Lock()
	c.w.clearSpellsLocked(p) // playerDiedLocked's
	c.w.mu.Unlock()
	if p.holdsBreath() {
		t.Fatal("held after death")
	}
}
