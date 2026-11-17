package entity

import (
	"testing"
	"time"
)

// hotSpell is a heal over time of so much a tick (CHA 60 adds nothing at 0%),
// cast 2 s, landing at cast + castLand.
func hotSpell(id, perTick int, name string) CastSpell {
	sp := minorHeal()
	sp.SpellID, sp.DisplayIndex, sp.Name, sp.Recast = id, id%40, name, 0
	sp.Def = ParseSpellText("Target, Increase HP per tic by " + itoaPad(perTick) + ", 24sec")
	return sp
}

// healed runs the world to d and reports the hit points gained since the
// last call.
func (c *caster) healed(d time.Duration, last *int) int {
	c.tick(d)
	got := c.p.HP - *last
	*last = c.p.HP
	return got
}

// jib's account (F3 20261025-222354), with the regular tick 2 s before the
// landing: the first tick at once, the next at the regular tick, four in
// all, none after.
func TestHoTFirstTickAtOnceThenTheRegularTick(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100, 2000
	land := 2*time.Second + castLand
	c.w.regenAt = c.at(land - 2*time.Second) // the landing is 2 s into a tick
	sp := hotSpell(901, 30, "Mending")
	if !c.w.StartCast(c.sess, sp, c.me, c.at(0)) {
		t.Fatal("not begun")
	}
	last := c.p.HP
	for _, step := range []struct {
		at   time.Duration
		want int
	}{
		{land - time.Millisecond, 0},
		{land, 30}, // at once
		{land + 4*time.Second - time.Millisecond, 0}, // not 6 s after the landing...
		{land + 4*time.Second, 30},                   // ...but at the regular tick
		{land + 10*time.Second, 30},                  // and every regular tick
		{land + 16*time.Second, 30},                  // the fourth
		{land + 22*time.Second, 0},                   // none after: four in all
	} {
		if got := c.healed(step.at, &last); got != step.want {
			t.Fatalf("at %v healed %d, want %d", step.at, got, step.want)
		}
	}
	if len(c.p.Effects) != 1 || c.p.Effects[0].Ticks != 0 {
		t.Fatalf("effects %+v", c.p.Effects)
	}
	c.tick(land + 24*time.Second)
	if len(c.p.Effects) != 0 {
		t.Errorf("not worn off at 24 s: %+v", c.p.Effects)
	}
}

// A weaker heal over a stronger one does nothing.
func TestHoTWeakerOverStrongerDoesNothing(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100, 2000
	strong := &ActiveEffect{SpellID: 902, Name: "Big", HPTick: 80, Ticks: 3,
		Until: c.at(20 * time.Second), NextTick: c.at(5 * time.Second)}
	c.p.Effects = []*ActiveEffect{strong}
	c.w.StartCast(c.sess, hotSpell(903, 30, "Small"), c.me, c.at(0))
	c.tick(3400 * time.Millisecond)
	if c.p.HP != 100 || len(c.p.Effects) != 1 || c.p.Effects[0] != strong {
		t.Fatalf("HP %d, effects %+v", c.p.HP, c.p.Effects)
	}
	c.tick(5 * time.Second) // the stronger one's own tick goes on
	if c.p.HP != 180 {
		t.Errorf("HP %d after the stronger one's tick, want 180", c.p.HP)
	}
}

// A stronger heal over a weaker one with ticks left: none at once, the next
// regular tick is the stronger's, and it has all four.
func TestHoTStrongerOverWeakerTakesItsNextTick(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100, 2000
	c.w.regenAt = c.at(-1 * time.Second) // regular ticks at 5, 11, 17, 23, 29 s
	weak := &ActiveEffect{SpellID: 904, Name: "Small", HPTick: 30, Ticks: 2,
		Until: c.at(15 * time.Second), NextTick: c.at(5 * time.Second)}
	c.p.Effects = []*ActiveEffect{weak}
	c.p.Buffs = []BuffSlot{{Name: "Small"}}
	c.w.StartCast(c.sess, hotSpell(905, 70, "Big"), c.me, c.at(0))
	last := c.p.HP
	if got := c.healed(3400*time.Millisecond, &last); got != 0 {
		t.Fatalf("healed %d at the landing, want none", got)
	}
	if len(c.p.Effects) != 1 || c.p.Effects[0].SpellID != 905 || len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Mending" && c.p.Buffs[0].Name != "Big" {
		t.Fatalf("effects %+v, buffs %+v", c.p.Effects, c.p.Buffs)
	}
	for _, step := range []struct {
		at   time.Duration
		want int
	}{
		{5 * time.Second, 70}, {11 * time.Second, 70}, {17 * time.Second, 70}, {23 * time.Second, 70},
		{29 * time.Second, 0},
	} {
		if got := c.healed(step.at, &last); got != step.want {
			t.Fatalf("at %v healed %d, want %d", step.at, got, step.want)
		}
	}
}

// Without a regen tick yet the ticks keep Sandstorm's rhythm (six seconds
// from the landing), and a recast of the same heal replaces it with no tick
// at once.
func TestHoTRecastReplacesWithoutATickAtOnce(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100, 2000
	sp := hotSpell(906, 30, "Mending")
	c.w.StartCast(c.sess, sp, c.me, c.at(0))
	last := c.p.HP
	if got := c.healed(3400*time.Millisecond, &last); got != 30 {
		t.Fatalf("first landing healed %d", got)
	}
	c.w.StartCast(c.sess, sp, c.me, c.at(4*time.Second))
	if got := c.healed(7400*time.Millisecond, &last); got != 0 {
		t.Fatalf("the recast's landing healed %d, want none", got)
	}
	if got := c.healed(9400*time.Millisecond, &last); got != 30 {
		t.Fatalf("the next tick healed %d", got)
	}
	if e := c.p.Effects; len(e) != 1 || e[0].Ticks != 3 {
		t.Fatalf("effects %+v, want the recast with three ticks left", e)
	}
}

// A heal over time's ticks left are kept over a logout.
func TestHoTTicksAreKept(t *testing.T) {
	c := newCaster(t)
	c.p.Effects = []*ActiveEffect{{SpellID: 907, Name: "Mending", HPTick: 30, Ticks: 2,
		Until: c.at(20 * time.Second), NextTick: c.at(5 * time.Second)}}
	rows, _ := c.w.SavedEffects(c.p.ServerID, c.at(0))
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(time.Minute)); n != 1 || c.p.Effects[0].Ticks != 2 {
		t.Fatalf("restored %d: %+v", n, c.p.Effects)
	}
}

// Courage on a player at full health: the maximum rises by 40 and so do its
// hit points (the heal lands after the raise), for 30 minutes.
func TestCourageHealsWhatItRaises(t *testing.T) {
	c := newCaster(t)
	c.p.HP = c.p.MaxHPNow()
	before := c.p.HP
	sp := strike()
	sp.SpellID, sp.Name = 237, "Courage"
	sp.Def = ParseSpellNamed("Courage", "Target, Increase Maximum HP by 40, Increase AC by 40", "")
	c.land(t, sp, c.me, 0)
	if c.p.MaxHPNow() != before+40 || c.p.HP != before+40 || c.p.Mods.AC != 40 {
		t.Fatalf("max %d, HP %d, AC mod %d; want max and HP %d", c.p.MaxHPNow(), c.p.HP, c.p.Mods.AC, before+40)
	}
	if e := c.p.Effects; len(e) != 1 || !e[0].Until.Equal(c.at(2*time.Second+castLand+30*time.Minute)) {
		t.Fatalf("effects %+v", e)
	}
}

// The regen tick late (Heirloom petition 2221, Marr's Blessing: "double tick
// a lot ... stop ticking a while before it wears off"): it falls every 60th
// world tick, here 6.3 s apart as when world ticks run long, and a heal
// over time's tick due just before it was set from the regenAt before it.
// Every tick at least half a tick after the last, all of them landed, the
// last within a tick of the end. The control: the old rule (next from the
// late regenAt alone) ticks twice within a second.
func TestHoTNoDoubleTickWhenTheRegenTickIsLate(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100, 100000
	sp := hotSpell(904, 30, "Blessing")
	sp.Def = ParseSpellText("Target, Increase HP per tic by 30, 2min")
	land := 2*time.Second + castLand
	c.w.regenAt = c.at(land - 2*time.Second)
	if !c.w.StartCast(c.sess, sp, c.me, c.at(0)) {
		t.Fatal("not begun")
	}
	last := c.p.HP
	var heals []time.Duration
	nextRegen := land + 4*time.Second + 300*time.Millisecond // 6.3 s after the last
	for at := 100 * time.Millisecond; at <= land+130*time.Second; at += 100 * time.Millisecond {
		if got := c.healed(at, &last); got > 0 {
			for ; got > 0; got -= 30 {
				heals = append(heals, at)
			}
		}
		if at >= nextRegen { // regenSystem runs after effectSystem
			c.w.regenAt = c.at(at)
			nextRegen = at + 6300*time.Millisecond
		}
	}
	if len(heals) != 20 {
		t.Errorf("%d ticks, want 20: %v", len(heals), heals)
	}
	for i := 1; i < len(heals); i++ {
		if gap := heals[i] - heals[i-1]; gap < 3*time.Second {
			t.Fatalf("ticks %d and %d %v apart: %v", i-1, i, gap, heals)
		}
	}
	if end := land + 2*time.Minute; len(heals) > 0 && heals[len(heals)-1] < end-RegenInterval-time.Second {
		t.Errorf("the last tick at %v, the heal ends at %v", heals[len(heals)-1], end)
	}
}
