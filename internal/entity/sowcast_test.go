package entity

import (
	"testing"
	"time"
)

// a lone run speed a cast and a hit end, as Spirit Circle's (petition 1067)
const circleJSON = `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
	{"kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true}]}`

// A run speed the workbook ends at a cast ends only at a harmful one
// (petition 1067: "SoW is being removed when casting buffs and beneficial
// spells. At least when applied from Spirit Circle"): a beneficial cast of
// the player's own keeps it, a harmful cast ends it, the run speed told
// again. Through StartCast, as the game casts. On the code before, the
// beneficial cast ended it.
func TestABeneficialCastKeepsARunSpeed(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	circle := wbSpell(9930, 40, 0, "Test Wolf Ring", circleJSON)
	c.land(t, circle, 0xFFFFFFFF, 0)
	if effectNamed(c.p.Effects, 9930) == nil || c.p.SpeedNow() <= run {
		t.Fatalf("landed: on %v, speed %v (run %v)", effectNamed(c.p.Effects, 9930) != nil, c.p.SpeedNow(), run)
	}
	mend := wbSpell(9931, 30, 0, "Test Mending", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"effects":[
		{"kind":"hp","amount":50}]}`)
	c.land(t, mend, 0xFFFFFFFF, 3*time.Second)
	if effectNamed(c.p.Effects, 9930) == nil || c.p.SpeedNow() <= run {
		t.Fatalf("after a beneficial cast: on %v, speed %v (run %v); want it on, fast", effectNamed(c.p.Effects, 9930) != nil,
			c.p.SpeedNow(), run)
	}
	spark := wbSpell(9932, 30, 0, "Test Spark", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"effects":[
		{"kind":"hp","amount":-5}]}`)
	if !spark.Def.Harmful {
		t.Fatal("the spark is not harmful: the test proves nothing")
	}
	c.sent()
	if !c.w.StartCast(c.sess, spark, c.npcO, c.at(6*time.Second)) {
		t.Fatalf("the spark did not begin: %x", c.sent())
	}
	if v, ok := lastSpeedSent(c.sent()); effectNamed(c.p.Effects, 9930) != nil || c.p.SpeedNow() != run || !ok || v != run {
		t.Errorf("after a harmful cast: on %v, speed %v, told %v %v; want it gone, the run's %v told",
			effectNamed(c.p.Effects, 9930) != nil, c.p.SpeedNow(), v, ok, run)
	}
}

// What else a cast ends still ends at any: a beneficial cast takes a haste's
// stat a cast ends and leaves its speed, the effect staying; a harmful cast
// ends the rest of it. A wolf form's speed stays through a beneficial cast
// with its form and stats, and a harmful one ends the speed alone. A hit taken
// leaves a lone run speed, and its bearer's own hit ends it whole (petition
// 1109).
func TestABeneficialCastEndsAllButTheRunSpeed(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	haste := wbSpell(9933, 30, 0, "Test Brief Haste", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"speed","dispel_on_cast":true},{"kind":"stat","stat":"agi","amount":10,"dispel_on_cast":true}]}`)
	c.land(t, haste, 0xFFFFFFFF, 0)
	c.w.mu.Lock()
	c.w.dispelOnCastLocked(c.p, false)
	c.w.mu.Unlock()
	if effectNamed(c.p.Effects, 9933) == nil || c.p.SpeedNow() <= run || c.p.Mods.Agi != 0 {
		t.Fatalf("after a beneficial cast: on %v, speed %v (run %v), AGI %+d; want on, fast, the AGI gone",
			effectNamed(c.p.Effects, 9933) != nil, c.p.SpeedNow(), run, c.p.Mods.Agi)
	}
	c.w.mu.Lock()
	c.w.dispelOnCastLocked(c.p, true)
	c.w.mu.Unlock()
	if effectNamed(c.p.Effects, 9933) != nil || c.p.SpeedNow() != run {
		t.Fatalf("after a harmful cast: on %v, speed %v; want it gone", effectNamed(c.p.Effects, 9933) != nil, c.p.SpeedNow())
	}

	d := bodyRig(t)
	wolf := wbSpell(9934, 30, 0, "Test Wolf Run", `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
		{"kind":"illusion","sprite":"0x010C0B0A"},{"kind":"stat","stat":"str","amount":29},{"kind":"speed","dispel_on_cast":true}]}`)
	d.land(t, wolf, 0xFFFFFFFF, 0)
	d.w.mu.Lock()
	d.w.dispelOnCastLocked(d.p, false)
	d.w.mu.Unlock()
	if m, _ := channelBody(d.p); effectNamed(d.p.Effects, 9934) == nil || d.p.SpeedNow() <= run || m != testOgreMale || d.p.Mods.Str != 29 {
		t.Fatalf("a wolf form after a beneficial cast: speed %v, body %#x, STR %+d; want all of it", d.p.SpeedNow(), m, d.p.Mods.Str)
	}
	d.w.mu.Lock()
	d.w.dispelOnCastLocked(d.p, true)
	d.w.mu.Unlock()
	if m, _ := channelBody(d.p); effectNamed(d.p.Effects, 9934) == nil || d.p.SpeedNow() != run || m != testOgreMale || d.p.Mods.Str != 29 {
		t.Errorf("a wolf form after a harmful cast: speed %v, body %#x, STR %+d; want the speed alone gone", d.p.SpeedNow(), m, d.p.Mods.Str)
	}

	f := bodyRig(t)
	f.land(t, wbSpell(9930, 40, 0, "Test Wolf Ring", circleJSON), 0xFFFFFFFF, 0)
	f.contactPlayer()
	if effectNamed(f.p.Effects, 9930) == nil || f.p.SpeedNow() <= run {
		t.Errorf("a lone run speed after a hit taken: on %v, speed %v; want it on, fast", effectNamed(f.p.Effects, 9930) != nil, f.p.SpeedNow())
	}
	f.ownHit()
	if effectNamed(f.p.Effects, 9930) != nil || f.p.SpeedNow() != run {
		t.Errorf("a lone run speed after its own hit: on %v, speed %v; want it gone", effectNamed(f.p.Effects, 9930) != nil, f.p.SpeedNow())
	}
}
