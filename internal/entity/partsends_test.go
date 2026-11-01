package entity

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A player's own hold, and the parts a cast or the bearer's own action ends
// (the user, 2026-11-01: "yes hold the tree form, yes only end the marked
// part"), against potionbody_test.go's bodyRig; the spells made up.

// lastSpeedSent is the run speed of the last 0x00F8 among the messages.
func lastSpeedSent(msgs [][]byte) (float32, bool) {
	var v float32
	ok := false
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodePlayerSpeed && len(m) >= 6 {
			v, ok = math.Float32frombits(binary.LittleEndian.Uint32(m[2:6])), true
		}
	}
	return v, ok
}

// a tree form's kind: a body, two stats, a hold on its own caster a hit ends
const treeJSON = `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
	{"kind":"illusion","sprite":"0x010C0B0A"},{"kind":"stat","stat":"sta","amount":100},{"kind":"stat","stat":"ac","amount":2500},
	{"kind":"speed","amount":-100,"absolute":true,"dispel_on_contact":true}]}`

// A hold of the player's own cast holds it (a run speed of 0 told, as a duel
// opponent's hold is), its form and stats on; a hit ends the hold alone, the
// run speed told again, the form and stats staying. On the code before, the
// hold held nothing.
func TestAHoldOfItsOwnCastHoldsThePlayer(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	tree := wbSpell(9911, 51, 0, "Test Tree Shape", treeJSON)
	if !c.w.StartCast(c.sess, tree, 0xFFFFFFFF, c.at(0)) {
		t.Fatal("not begun")
	}
	c.tick(tree.CastTime + castLand)
	if v, ok := lastSpeedSent(c.sent()); c.p.SpeedNow() != 0 || !ok || v != 0 {
		t.Fatalf("landed: speed %v, told %v %v; want held, 0 told", c.p.SpeedNow(), v, ok)
	}
	if m, _ := channelBody(c.p); m != testOgreMale || c.p.Mods.Sta != 100 || c.p.Mods.AC != 2500 {
		t.Fatalf("landed: body %#x, STA %+d, AC %+d", m, c.p.Mods.Sta, c.p.Mods.AC)
	}
	c.contactPlayer()
	if v, ok := lastSpeedSent(c.sent()); c.p.SpeedNow() != run || !ok || v != run {
		t.Errorf("after a hit: speed %v, told %v %v; want the run's %v", c.p.SpeedNow(), v, ok, run)
	}
	if m, _ := channelBody(c.p); m != testOgreMale || c.p.Mods.Sta != 100 || effectNamed(c.p.Effects, 9911) == nil {
		t.Errorf("after a hit: body %#x, STA %+d, on %v; want the form and its stats", m, c.p.Mods.Sta,
			effectNamed(c.p.Effects, 9911) != nil)
	}
}

// A barrier's hold, which the workbook does not mark to end at a hit, holds
// its caster for its time through a hit, then lets it go.
func TestABarriersHoldLastsItsTime(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	barrier := wbSpell(9912, 20, 0, "Test Clay Wall", `{"v":1,"scope":"self","target_type":"Self","duration_s":30,"effects":[
		{"kind":"mitigation","mitigation":{"pct":40,"max":2000}},{"kind":"speed","amount":-100,"absolute":true}]}`)
	c.land(t, barrier, 0xFFFFFFFF, 0)
	c.contactPlayer()
	if c.p.SpeedNow() != 0 {
		t.Fatalf("a hit let the barrier's hold go: %v", c.p.SpeedNow())
	}
	c.tick(barrier.CastTime + castLand + 31*time.Second)
	if v, ok := lastSpeedSent(c.sent()); c.p.SpeedNow() != run || !ok || v != run {
		t.Errorf("after its time: speed %v, told %v %v; want the run's %v", c.p.SpeedNow(), v, ok, run)
	}
}

// a spirit walk's kind: an invisibility a hit and a cast end, a speed, the
// invisible body
const walkJSON = `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
	{"kind":"invis","dispel_on_contact":true,"dispel_on_cast":true},{"kind":"speed"},{"kind":"illusion","sprite":"0x435EF663"}]}`

// The bearer's own action ends its invisibility alone (the invisible body
// with it): the speed stays and runs to its time. On the code before, the
// action took the speed with it.
func TestAnActionEndsTheInvisibilityAlone(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	walk := wbSpell(9913, 30, 0, "Test Spirit Stroll", walkJSON)
	c.land(t, walk, 0xFFFFFFFF, 0)
	if !c.p.Invisible() || c.p.SpeedNow() <= run {
		t.Fatalf("landed: invisible %v, speed %v", c.p.Invisible(), c.p.SpeedNow())
	}
	c.w.BreakInvis(c.sess, "an action")
	if e := effectNamed(c.p.Effects, 9913); e == nil || c.p.Invisible() || c.p.SpeedNow() <= run {
		t.Fatalf("after an action: on %v, invisible %v, speed %v (run %v); want it on, seen, still fast",
			e != nil, c.p.Invisible(), c.p.SpeedNow(), run)
	}
	// a cast after: its invisibility ended already, nothing more ends
	c.w.mu.Lock()
	c.w.dispelOnCastLocked(c.p)
	c.w.mu.Unlock()
	if effectNamed(c.p.Effects, 9913) == nil || c.p.SpeedNow() <= run {
		t.Errorf("a cast after the action took what stays")
	}

	// a real body beside the invisibility (Shadow Walk's skeleton) stays
	d := bodyRig(t)
	shadow := wbSpell(9914, 30, 0, "Test Bone Stroll", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis"},{"kind":"illusion","sprite":"0x010C0B0A"}]}`)
	d.land(t, shadow, 0xFFFFFFFF, 0)
	d.w.BreakInvis(d.sess, "a swing")
	if m, _ := channelBody(d.p); d.p.Invisible() || m != testOgreMale {
		t.Errorf("after an action: invisible %v, body %#x; want seen, the illusion's body", d.p.Invisible(), m)
	}
}

// The controls: an invisibility alone ends whole at an action, as before;
// one beside an illusion that has no body (Gather Shadows') too.
func TestAnActionEndsAnInvisibilityAloneWhole(t *testing.T) {
	c := bodyRig(t)
	alone := wbSpell(9915, 30, 0, "Test Fade", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis"}]}`)
	c.land(t, alone, 0xFFFFFFFF, 0)
	c.w.BreakInvis(c.sess, "an action")
	if effectNamed(c.p.Effects, 9915) != nil {
		t.Error("an invisibility alone stayed through an action")
	}
	d := bodyRig(t)
	shadows := wbSpell(9916, 30, 0, "Test Gloom", `{"v":1,"scope":"self","target_type":"Self","duration_s":330,"effects":[
		{"kind":"invis"},{"kind":"illusion"}]}`)
	d.land(t, shadows, 0xFFFFFFFF, 0)
	d.w.BreakInvis(d.sess, "an action")
	if effectNamed(d.p.Effects, 9916) != nil {
		t.Error("an invisibility beside an illusion of no body stayed through an action")
	}
}

// A cast ends only the parts the workbook marks dispel_on_cast: a wolf
// form's speed, its form and stats staying; the control, an effect wholly
// marked, ends whole.
func TestACastEndsOnlyItsMarkedParts(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	wolf := wbSpell(9917, 30, 0, "Test Wolf Run", `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
		{"kind":"illusion","sprite":"0x010C0B0A"},{"kind":"stat","stat":"str","amount":29},{"kind":"speed","dispel_on_cast":true}]}`)
	c.land(t, wolf, 0xFFFFFFFF, 0)
	c.w.mu.Lock()
	c.w.dispelOnCastLocked(c.p)
	c.w.mu.Unlock()
	if m, _ := channelBody(c.p); effectNamed(c.p.Effects, 9917) == nil || c.p.SpeedNow() != run || m != testOgreMale ||
		c.p.Mods.Str != 29 {
		t.Errorf("after a cast: on %v, speed %v (run %v), body %#x, STR %+d; want the speed alone gone",
			effectNamed(c.p.Effects, 9917) != nil, c.p.SpeedNow(), run, m, c.p.Mods.Str)
	}
	d := bodyRig(t)
	all := wbSpell(9918, 30, 0, "Test Brief Haste", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"speed","dispel_on_cast":true},{"kind":"stat","stat":"agi","amount":10,"dispel_on_cast":true}]}`)
	d.land(t, all, 0xFFFFFFFF, 0)
	d.w.mu.Lock()
	d.w.dispelOnCastLocked(d.p)
	d.w.mu.Unlock()
	if effectNamed(d.p.Effects, 9918) != nil {
		t.Error("a wholly marked effect stayed through a cast")
	}
}

// A part both a cast and a hit end is taken off once: the cast ends it and
// its stat; the hit after ends the part only a hit ends, and not the first
// again; the part neither marks stays.
func TestAPartTwoEndsShareIsTakenOffOnce(t *testing.T) {
	c := bodyRig(t)
	boon := wbSpell(9919, 30, 0, "Test Twin Mark", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"stat","stat":"str","amount":10,"dispel_on_cast":true,"dispel_on_contact":true},
		{"kind":"stat","stat":"dex","amount":5,"dispel_on_contact":true},{"kind":"stat","stat":"agi","amount":7}]}`)
	c.land(t, boon, 0xFFFFFFFF, 0)
	c.w.mu.Lock()
	c.w.dispelOnCastLocked(c.p)
	c.w.mu.Unlock()
	if c.p.Mods.Str != 0 || c.p.Mods.Dex != 5 || c.p.Mods.Agi != 7 {
		t.Fatalf("after a cast: STR %+d DEX %+d AGI %+d; want 0, +5, +7", c.p.Mods.Str, c.p.Mods.Dex, c.p.Mods.Agi)
	}
	c.contactPlayer()
	if e := effectNamed(c.p.Effects, 9919); e == nil || c.p.Mods.Str != 0 || c.p.Mods.Dex != 0 || c.p.Mods.Agi != 7 {
		t.Errorf("after a hit: on %v, STR %+d (want 0, not taken twice), DEX %+d (want 0), AGI %+d (want +7)", e != nil,
			c.p.Mods.Str, c.p.Mods.Dex, c.p.Mods.Agi)
	}
}

// Kept over a logout: an effect saved before its invisibility ended keeps
// its parts (an action after the login ends the invisibility alone); one
// whose invisibility ended before the logout comes back without it.
func TestPartsKeptOverALogout(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	walk := wbSpell(9913, 30, 0, "Test Spirit Stroll", walkJSON)
	c.land(t, walk, 0xFFFFFFFF, 0)
	rows := savedEffects(c.p, c.at(time.Minute))
	d := bodyRig(t)
	if d.w.RestoreEffects(d.sess, rows, d.at(0)) != 1 || !d.p.Invisible() {
		t.Fatalf("restored: invisible %v", d.p.Invisible())
	}
	d.w.BreakInvis(d.sess, "an action")
	if effectNamed(d.p.Effects, 9913) == nil || d.p.Invisible() || d.p.SpeedNow() <= run {
		t.Fatalf("an action after the login: on %v, invisible %v, speed %v", effectNamed(d.p.Effects, 9913) != nil,
			d.p.Invisible(), d.p.SpeedNow())
	}
	rows = savedEffects(d.p, d.at(time.Minute))
	f := bodyRig(t)
	f.w.RestoreEffects(f.sess, rows, f.at(0))
	if e := effectNamed(f.p.Effects, 9913); e == nil || f.p.Invisible() || f.p.SpeedNow() <= run || e.Rules.DispelOnCast ||
		e.Rules.DispelOnContact {
		t.Errorf("after the logout: on %v, invisible %v, speed %v; ends at a cast or hit %v", e != nil, f.p.Invisible(),
			f.p.SpeedNow(), e != nil && (e.Rules.DispelOnCast || e.Rules.DispelOnContact))
	}
}

// A pet's swing ends its invisibility alone: what else lasts on it stays.
func TestAPetsSwingEndsItsInvisibilityAlone(t *testing.T) {
	pet := &NPC{ServerID: 7001, Name: "Test Pet"}
	pet.Effects = []*ActiveEffect{{SpellID: 9920, Name: "Test Pet Veil", Invis: true, Mods: StatMods{AC: 50},
		parts: []*effectPart{{Invis: true, Invisibility: true}, {Mods: StatMods{AC: 50}}}}}
	breakNPCInvis(pet)
	if len(pet.Effects) != 1 || pet.invisible() || pet.Effects[0].Mods.AC != 50 {
		t.Errorf("after its swing: %d effects, invisible %v; want the AC staying", len(pet.Effects), pet.invisible())
	}
}

// a rogue's hide's kind: unseen, and slowed while hidden, each part ending at
// a hit or a cast
const hideJSON = `{"v":1,"scope":"self","target_type":"Self","duration_s":300,"effects":[
	{"kind":"hide","dispel_on_cast":true,"dispel_on_contact":true},
	{"kind":"speed","amount":-30,"dispel_on_cast":true,"dispel_on_contact":true}]}`

// A hide's slow ends with the hiding at its bearer's action, though the
// workbook marks it to end only at a hit or a cast (the user, 2026-11-01:
// "end the slow"): the whole hide ends and the run speed is told again. On
// the code before, the slow stayed to its time. The controls: a hit and a
// cast still end it whole; another invisibility's speed stays at an action,
// and so does a slow beside an invisibility that is not a hide (the rule is
// the hide's alone).
func TestAHidesSlowEndsWithTheHiding(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	hide := wbSpell(9921, 1, 0, "Test Lurk", hideJSON)
	c.land(t, hide, 0xFFFFFFFF, 0)
	if !c.p.Invisible() || c.p.SpeedNow() >= run {
		t.Fatalf("landed: invisible %v, speed %v (run %v); want hidden and slowed", c.p.Invisible(), c.p.SpeedNow(), run)
	}
	c.w.BreakInvis(c.sess, "a swing")
	if v, ok := lastSpeedSent(c.sent()); effectNamed(c.p.Effects, 9921) != nil || c.p.Invisible() || c.p.SpeedNow() != run ||
		!ok || v != run {
		t.Errorf("after an action: on %v, invisible %v, speed %v, told %v %v; want it gone, the run's %v told",
			effectNamed(c.p.Effects, 9921) != nil, c.p.Invisible(), c.p.SpeedNow(), v, ok, run)
	}

	for _, end := range []struct {
		why string
		do  func(*caster)
	}{
		{"a hit", func(d *caster) { d.contactPlayer() }},
		{"a cast", func(d *caster) { d.w.mu.Lock(); d.w.dispelOnCastLocked(d.p); d.w.mu.Unlock() }},
	} {
		d := bodyRig(t)
		d.land(t, hide, 0xFFFFFFFF, 0)
		end.do(d)
		if v, ok := lastSpeedSent(d.sent()); effectNamed(d.p.Effects, 9921) != nil || d.p.Invisible() || d.p.SpeedNow() != run ||
			!ok || v != run {
			t.Errorf("after %s: on %v, invisible %v, speed %v, told %v %v; want it gone, the run's %v told", end.why,
				effectNamed(d.p.Effects, 9921) != nil, d.p.Invisible(), d.p.SpeedNow(), v, ok, run)
		}
	}

	f := bodyRig(t)
	vanish := wbSpell(9922, 30, 0, "Test Quick Fade", `{"v":1,"scope":"self","target_type":"Self","duration_s":300,"effects":[
		{"kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true},
		{"kind":"invis","dispel_on_cast":true,"dispel_on_contact":true}]}`)
	f.land(t, vanish, 0xFFFFFFFF, 0)
	f.w.BreakInvis(f.sess, "a swing")
	if effectNamed(f.p.Effects, 9922) == nil || f.p.Invisible() || f.p.SpeedNow() <= run {
		t.Errorf("another invisibility's speed after an action: on %v, invisible %v, speed %v (run %v); want it on, seen, fast",
			effectNamed(f.p.Effects, 9922) != nil, f.p.Invisible(), f.p.SpeedNow(), run)
	}
	g := bodyRig(t)
	veil := wbSpell(9923, 30, 0, "Test Heavy Veil", `{"v":1,"scope":"self","target_type":"Self","duration_s":300,"effects":[
		{"kind":"invis","dispel_on_cast":true,"dispel_on_contact":true},
		{"kind":"speed","amount":-30,"dispel_on_cast":true,"dispel_on_contact":true}]}`)
	g.land(t, veil, 0xFFFFFFFF, 0)
	g.w.BreakInvis(g.sess, "a swing")
	if effectNamed(g.p.Effects, 9923) == nil || g.p.Invisible() || g.p.SpeedNow() >= run {
		t.Errorf("an invisibility's slow, not a hide's, after an action: on %v, invisible %v, speed %v (run %v); want it on, "+
			"seen, slowed", effectNamed(g.p.Effects, 9923) != nil, g.p.Invisible(), g.p.SpeedNow(), run)
	}
}
