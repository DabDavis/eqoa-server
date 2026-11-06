package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A hit ends only the parts of an effect the workbook marks dispel_on_contact
// (the user, 2026-11-01: "yes only end the marked part"); an effect whose every
// lasting part is marked ends whole. Against potionbody_test.go's bodyRig (a
// druid with a race's two bodies, made-up ids); the spells made up.

// contactPlayer is a creature's hit on the rig's player as systems.go makes
// it: what ends at a hit ends.
func (c *caster) contactPlayer() {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.contactPlayerLocked(c.p)
}

// ownHit is the rig's player's own swing at a creature as playerRoundHitsLocked
// makes it: a run speed that ends at contact ends.
func (c *caster) ownHit() {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.ownHitLocked(c.p, "a swing")
}

func effectNamed(es []*ActiveEffect, id int) *ActiveEffect {
	for _, e := range es {
		if e.SpellID == id {
			return e
		}
	}
	return nil
}

// sentSpeed reports a 0x00F8 among the messages sent.
func sentSpeed(msgs [][]byte) bool {
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodePlayerSpeed {
			return true
		}
	}
	return false
}

// a wolf form's kind: a body, two stats, a speed marked to end at contact
const wolfJSON = `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
	{"kind":"illusion","sprite":"0x010C0B0A"},{"kind":"stat","stat":"str","amount":29},{"kind":"stat","stat":"agi","amount":29},
	{"kind":"speed","dispel_on_contact":true}]}`

// A hit taken ends no speed part (petition 1109). Its bearer's own hit ends it
// alone: the run speed falls back (0x00F8 told), the stats and the body stay,
// the effect stays on the effects list and runs to its time, ending at no hit
// more. Before 2026-11-01 a hit took the whole effect, body and stats with it.
func TestAnOwnHitEndsTheSpeedPartTheStatsAndFormStay(t *testing.T) {
	c := bodyRig(t)
	wolf := wbSpell(9901, 30, 0, "Test Wolf Shape", wolfJSON)
	run := c.p.SpeedNow()
	c.land(t, wolf, 0xFFFFFFFF, 0)
	if e := effectNamed(c.p.Effects, 9901); e == nil || !e.Speed || c.p.SpeedNow() <= run || c.p.Mods.Str != 29 {
		t.Fatalf("landed: %+v, speed %v (run %v), STR %+d", e, c.p.SpeedNow(), run, c.p.Mods.Str)
	}
	c.sent()
	c.contactPlayer()
	if e := effectNamed(c.p.Effects, 9901); e == nil || !e.Speed || c.p.SpeedNow() <= run || sentSpeed(c.sent()) {
		t.Fatalf("after a hit taken: %+v, speed %v (run %v); want it all on, nothing told", e, c.p.SpeedNow(), run)
	}
	c.ownHit()
	e := effectNamed(c.p.Effects, 9901)
	if e == nil {
		t.Fatal("its own hit ended the whole effect")
	}
	if told := sentSpeed(c.sent()); e.Speed || c.p.SpeedNow() != run || !told {
		t.Errorf("the speed part: on %v, speed %v (want the run's %v), told %v", e.Speed, c.p.SpeedNow(), run, told)
	}
	if m, _ := channelBody(c.p); m != testOgreMale || c.p.Mods.Str != 29 || c.p.Mods.Agi != 29 || len(c.p.Buffs) != 1 {
		t.Errorf("the rest: body %#x, STR %+d, AGI %+d, buffs %d; want the ogre's, +29, +29 and the effect shown",
			m, c.p.Mods.Str, c.p.Mods.Agi, len(c.p.Buffs))
	}
	c.ownHit()
	c.contactPlayer()
	if effectNamed(c.p.Effects, 9901) == nil {
		t.Error("a second hit ended what stays")
	}
	c.tick(31 * time.Minute)
	if effectNamed(c.p.Effects, 9901) != nil {
		t.Error("what stays outlived its time")
	}
}

// A hit ends an invisibility part alone: the bearer is seen again, its
// illusion's body stays.
func TestAHitEndsTheInvisPartTheIllusionStays(t *testing.T) {
	c := bodyRig(t)
	veil := wbSpell(9902, 30, 0, "Test Veil Shape", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis","dispel_on_contact":true},{"kind":"illusion","sprite":"0x010C0B0A"}]}`)
	c.land(t, veil, 0xFFFFFFFF, 0)
	if !c.p.Invisible() {
		t.Fatal("not invisible")
	}
	c.contactPlayer()
	if effectNamed(c.p.Effects, 9902) == nil || c.p.Invisible() {
		t.Fatalf("after a hit: on %v, invisible %v; want it on, seen", effectNamed(c.p.Effects, 9902) != nil, c.p.Invisible())
	}
	if m, _ := channelBody(c.p); m != testOgreMale {
		t.Errorf("the body %#x, want the illusion's %#x", m, testOgreMale)
	}
}

// An illusion part the hit ends takes the body with it (Spirit's Boon's:
// invisibility and its body marked, its speed not); the speed stays.
func TestAHitEndsTheIllusionPartTheSpeedStays(t *testing.T) {
	c := bodyRig(t)
	boon := wbSpell(9903, 30, 0, "Test Boon", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"illusion","sprite":"0x010C0B0A","dispel_on_contact":true},{"kind":"speed"}]}`)
	run := c.p.SpeedNow()
	c.land(t, boon, 0xFFFFFFFF, 0)
	if m, _ := channelBody(c.p); m != testOgreMale {
		t.Fatalf("landed as %#x", m)
	}
	c.contactPlayer()
	if m, _ := channelBody(c.p); m != uint32(c.p.ModelID) || c.p.SpeedNow() <= run || effectNamed(c.p.Effects, 9903) == nil {
		t.Errorf("after a hit: body %#x (want its own), speed %v (run %v), on %v", m, c.p.SpeedNow(), run,
			effectNamed(c.p.Effects, 9903) != nil)
	}
}

// Invisibility is one part here: a spell whose Invisible part a hit ends loses
// its invisible body with it (Elemental Veil: the workbook marks its Invisible,
// not its Alter Appearance to the invisible body), so marked whole it ends
// whole; beside a speed not marked (Spirit Walk's), the speed stays.
func TestTheInvisibleBodyEndsWithItsInvisiblePart(t *testing.T) {
	veil := wbSpell(9904, 30, 0, "Test Elemental Mist", `{"v":1,"scope":"self","target_type":"Self","duration_s":330,"effects":[
		{"kind":"invis","dispel_on_contact":true},{"kind":"illusion","sprite":"0x435EF663"}]}`)
	for _, e := range veil.Def.Effects {
		if e.Kind == EffectInvis && !e.EndsOnContact {
			t.Fatalf("an invisibility part not marked: %+v", veil.Def.Effects)
		}
	}
	c := bodyRig(t)
	c.land(t, veil, 0xFFFFFFFF, 0)
	c.contactPlayer()
	if effectNamed(c.p.Effects, 9904) != nil || c.p.Invisible() {
		t.Errorf("after a hit: on %v, invisible %v; want it ended whole", effectNamed(c.p.Effects, 9904) != nil, c.p.Invisible())
	}

	walk := wbSpell(9905, 30, 0, "Test Spirit Stroll", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis","dispel_on_contact":true},{"kind":"speed"},{"kind":"illusion","sprite":"0x435EF663"}]}`)
	d := bodyRig(t)
	run := d.p.SpeedNow()
	d.land(t, walk, 0xFFFFFFFF, 0)
	d.contactPlayer()
	if effectNamed(d.p.Effects, 9905) == nil || d.p.Invisible() || d.p.SpeedNow() <= run {
		t.Errorf("after a hit: on %v, invisible %v, speed %v (run %v); want it on, seen, still fast",
			effectNamed(d.p.Effects, 9905) != nil, d.p.Invisible(), d.p.SpeedNow(), run)
	}
}

// The control: an effect whose every lasting part is marked ends whole at a
// hit, as before; one whose marked part is its only lasting one beside an
// instant hit ends whole too (Arcane Shock's snare).
func TestAWhollyMarkedEffectEndsWhole(t *testing.T) {
	c := bodyRig(t)
	both := wbSpell(9906, 30, 0, "Test Strong Veil", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis","dispel_on_contact":true},{"kind":"stat","stat":"str","amount":10,"dispel_on_contact":true}]}`)
	c.land(t, both, 0xFFFFFFFF, 0)
	c.contactPlayer()
	if effectNamed(c.p.Effects, 9906) != nil {
		t.Error("a wholly marked effect stayed through a hit")
	}
	shock := wbSpell(9907, 30, 1, "Test Shock", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":10,
		"effects":[{"kind":"hp","amount":-20},{"kind":"speed","amount":-40,"dispel_on_contact":true}]}`)
	ae := lasting(&castState{spell: shock}, c.p, c.at(0))
	if ae == nil || ae.parts != nil {
		t.Errorf("an instant hit beside the marked snare: split %+v, want none (ends whole)", ae)
	}
}

// Kept over a logout: a mixed effect saved before any hit keeps its parts
// and its split, so a hit taken after the login still leaves its run speed and
// its bearer's own hit ends that part alone; a part ended before the logout
// does not come back.
func TestContactPartsKeptOverALogout(t *testing.T) {
	c := bodyRig(t)
	wolf := wbSpell(9901, 30, 0, "Test Wolf Shape", wolfJSON)
	run := c.p.SpeedNow()
	c.land(t, wolf, 0xFFFFFFFF, 0)
	rows := savedEffects(c.p, c.at(time.Minute))

	d := bodyRig(t)
	if d.w.RestoreEffects(d.sess, rows, d.at(0)) != 1 {
		t.Fatalf("restored: %+v", d.p.Effects)
	}
	if d.p.SpeedNow() <= run {
		t.Fatal("restored without its speed")
	}
	d.contactPlayer()
	if d.p.SpeedNow() <= run {
		t.Fatal("a hit taken after the login ended the run speed")
	}
	d.ownHit()
	e := effectNamed(d.p.Effects, 9901)
	if e == nil || e.Speed || d.p.Mods.Str != 29 {
		t.Fatalf("its own hit after the login: on %v, speed part %v, STR %+d; want the speed part alone gone", e != nil,
			e != nil && e.Speed, d.p.Mods.Str)
	}

	// the speed ended, then a logout: it stays ended
	rows = savedEffects(d.p, d.at(time.Minute))
	f := bodyRig(t)
	f.w.RestoreEffects(f.sess, rows, f.at(0))
	g := effectNamed(f.p.Effects, 9901)
	if g == nil || g.Speed || f.p.SpeedNow() != run || g.Rules.DispelOnContact || f.p.Mods.Str != 29 {
		t.Errorf("after the logout: on %v, speed %v (run %v), ends at a hit %v, STR %+d", g != nil, f.p.SpeedNow(), run,
			g != nil && g.Rules.DispelOnContact, f.p.Mods.Str)
	}
	if m, _ := channelBody(f.p); m != testOgreMale {
		t.Errorf("its body after the logout %#x", m)
	}
}
