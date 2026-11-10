package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A monk's bare hands roll its hand to hand (offence.go): nine tenths of the
// fit's line by level (petition 1455), rolled as a one-hander of that DMG in
// repair; with a weapon in hand
// a monk rolls the weapon (as a ranger, its archetype, does), and any other
// class's bare hands roll a weapon's 0 (the top of its STR alone, the floor
// 0.02 of it). The control: a monk's bare hands as they rolled before (no
// BareHands) are that roll of 0 too.
func TestAMonksBareHandsRollItsHandToHand(t *testing.T) {
	for level, want := range map[int]int{1: 6, 5: 44, 10: 92, 20: 188, 30: 284, 40: 379, 50: 475, 60: 571} {
		if got := handToHandDMG(level); got != want {
			t.Errorf("level %d: hand to hand DMG %d, want nine tenths of the fit's, %d", level, got, want)
		}
	}
	npc := &NPC{Level: 30}
	who := func(class int, g Gear) *Player {
		return &Player{Class: class, Level: 30, Str: 150, Dex: 150, Gear: g}
	}
	roll := func(p *Player) [3]int {
		f, l, h := playerMeleeRoll(p, npc)
		return [3]int{f, l, h}
	}
	bare := Gear{WeaponCond: 1, BareHands: true}
	monk := roll(who(classMonk, bare))
	if armed := roll(who(classMonk, Gear{WeaponCond: 1, WeaponDamage: 284})); monk != armed {
		t.Errorf("a level 30 monk's bare hands roll %v, a 284 one-hander %v", monk, armed)
	}
	sword := Gear{WeaponCond: 1, WeaponDamage: 100, WeaponSlot: 14}
	if got, ranger := roll(who(classMonk, sword)), roll(who(1, sword)); got == monk || got != ranger {
		t.Errorf("a monk with a weapon rolls %v, want its weapon's as a ranger's %v", got, ranger)
	}
	none := Gear{WeaponCond: 1}
	if r, zero := roll(who(classWarrior, bare)), roll(who(classWarrior, none)); r != zero || r[0] != 3 {
		t.Errorf("a warrior's bare hands roll %v, want a weapon's 0, %v (its floor 3)", r, zero)
	}
	if was := roll(who(classMonk, none)); was[0] != 3 || was == monk {
		t.Errorf("the control: a monk's bare hands as before roll %v, want a weapon's 0 (floor 3)", was)
	}
	// #stats says what it rolls on
	p := who(classMonk, bare)
	if s := meleeWeaponWords(p); !strings.Contains(s, "hand to hand of level 30, DMG 284") {
		t.Errorf("#stats: %q", s)
	}
	p.Gear = Gear{WeaponCond: 1, WeaponDamage: 400, WeaponSlot: 18}
	if s := meleeWeaponWords(p); !strings.HasPrefix(s, "ranged weapon DMG 400") {
		t.Errorf("#stats with a bow: %q", s)
	}
}

// A change of hands in a fight (HandsChanged) is the next round's: the round
// in hand lands as it was swung (Attack4, four hits), the next on the
// client's beat swings what is held now (a bow's Range4: one hit, 5 s, its
// reach of 25, so a crab 20 off is hit) and the one after on its delay. The
// control: without it the fight swings its engagement's Attack4 on, and the
// crab 20 off is out of its reach. A new engagement's swing is the hands'
// then, a change before it forgotten; hands put back as they swing leave
// nothing waiting.
func TestHandsChangedSwingsFromTheNextRound(t *testing.T) {
	bow := Swing{Kind: 11, Delay: 5 * time.Second, Reach: 25}
	for _, changed := range []bool{true, false} {
		f := newFight(t)
		f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
		f.p.AutoAttack = true
		at := func(d time.Duration) int { f.tick(d); return len(f.damage(t)) }
		at(500 * time.Millisecond)
		at(1000 * time.Millisecond)
		if changed {
			f.w.HandsChanged(f.sess, bow)
		}
		if n := at(2400 * time.Millisecond); n != 4 {
			t.Fatalf("changed %v: the round in hand landed %d hits, want its four", changed, n)
		}
		f.npc.X = f.p.X + 20 // a bow's reach, not a blade's
		n6, n7, n11 := at(6500*time.Millisecond), at(7000*time.Millisecond), at(11500*time.Millisecond)
		if changed && (n6 != 5 || n7 != 5 || n11 != 6 || f.p.Swing != bow) {
			t.Errorf("the bow's rounds: %d at 6.5 s, %d at 7 s, %d at 11.5 s (want 5, 5, 6), swing %+v", n6, n7, n11, f.p.Swing)
		}
		if !changed && (n6 != 4 || n11 != 4 || f.p.Swing != swing6s) {
			t.Errorf("the control: %d hits by 11.5 s, swing %+v; want the blades' rounds out of reach", n11, f.p.Swing)
		}
	}
	f := newFight(t)
	f.w.HandsChanged(f.sess, bow) // no fight: nothing to do
	if f.p.swingNew {
		t.Error("a change of hands out of a fight was kept")
	}
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.w.HandsChanged(f.sess, bow)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0.Add(time.Second))
	if f.p.swingNew || f.p.Swing != swing6s {
		t.Errorf("a new engagement kept the change before it: %+v", f.p.Swing)
	}
	// the hands back as they swing (or armour changed): nothing waits
	f.w.HandsChanged(f.sess, bow)
	f.w.HandsChanged(f.sess, swing6s)
	if f.p.swingNew {
		t.Error("the hands put back left the bow's swing waiting")
	}
}

// CastWantsHands is a cast its required equipment alone refuses: a ranger's
// arrow art with its blades in hand, all else fine. Not when anything else
// refuses or holds it (out of range, out of power, locked out, the arts'
// cooldown, another cast on, dead), when the hands already fit, or for a
// spell that names nothing; and it changes nothing: StartCast then refuses it
// for the weapon as before.
func TestCastWantsHandsOnlyWhenTheEquipmentAloneRefuses(t *testing.T) {
	arrow := wbSpell(1141, 5, 1, "Test Arrow", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"req_equipment":"Range",
		"effects":[{"kind":"hp","amount":-10}]}`)
	art := arrow
	art.CastTime = 0 // an art: no cast time, at a creature
	plain := wbSpell(1142, 5, 1, "Test Bolt", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"effects":[{"kind":"hp","amount":-10}]}`)
	blades := func(c *caster) { c.p.PrimaryType, c.p.SecondaryType = 1, 1 }
	for _, tc := range []struct {
		name  string
		spell CastSpell
		set   func(c *caster)
		want  bool
	}{
		{"blades in hand", arrow, blades, true},
		{"an art, blades in hand", art, blades, true},
		{"a bow in hand", arrow, func(c *caster) { c.p.SecondaryType = 7 }, false},
		{"no requirement", plain, blades, false},
		{"out of range", arrow, func(c *caster) { blades(c); c.npc.X = c.p.X + 100 }, false},
		{"out of power", arrow, func(c *caster) { blades(c); c.p.Power = 0 }, false},
		{"locked out", arrow, func(c *caster) {
			blades(c)
			c.p.Lockouts = append(c.p.Lockouts, castLockout{spellID: arrow.SpellID, until: c.at(10 * time.Second)})
		}, false},
		{"the arts' cooldown", art, func(c *caster) { blades(c); c.p.artReady = c.at(time.Second) }, false},
		{"another cast on", arrow, func(c *caster) {
			blades(c)
			c.p.Casting = &castState{spell: plain, ends: c.at(2 * time.Second), castEnds: c.at(time.Second)}
		}, false},
		{"dead", arrow, func(c *caster) { blades(c); c.p.HP = 0 }, false},
	} {
		c := newCaster(t)
		tc.set(c)
		if got := c.w.CastWantsHands(c.sess, tc.spell, c.npcO, c.at(0)); got != tc.want {
			t.Errorf("%s: wants hands %v, want %v", tc.name, got, tc.want)
		}
		if len(c.sent()) != 0 {
			t.Errorf("%s: the check sent something", tc.name)
		}
	}
	c := newCaster(t)
	blades(c)
	power := c.p.Power
	if !c.w.CastWantsHands(c.sess, arrow, c.npcO, c.at(0)) || c.p.Power != power || c.p.Casting != nil || len(c.p.Lockouts) != 0 {
		t.Fatal("the check changed the caster")
	}
	if c.w.StartCast(c.sess, arrow, c.npcO, c.at(0)) {
		t.Fatal("begun with blades in hand")
	}
	told := false
	for _, m := range c.sent() {
		told = told || strings.Contains(string(m), "w\x00e\x00a\x00p\x00o\x00n")
	}
	if !told {
		t.Error("the weapon refusal not told")
	}
}

// A duel's melee takes up a change of hands as a fight with a creature does:
// the round in hand lands as it was swung (four hits), the next swings what
// is held now (one hit, on the client's beat). The control: with no change
// the next round is four hits again.
func TestADuelsSwingFollowsTheHands(t *testing.T) {
	four := Swing{Kind: 5, Delay: 6 * time.Second, Reach: 4.5}
	one := Swing{Kind: 0, Delay: 3 * time.Second, Reach: 4.5}
	for _, changed := range []bool{true, false} {
		d := newDuelists(t)
		d.start(t)
		d.a.Gear.WeaponDamage, d.a.Gear.WeaponCond = 40, 1
		if !d.w.EngageAttack(d.sa, d.aSeesB, four, d.at(2*time.Second)) {
			t.Fatal("not engaged")
		}
		d.a.AutoAttack = true
		run := func(from, to time.Duration) int {
			for x := from; x <= to; x += 100 * time.Millisecond {
				d.w.mu.Lock()
				d.w.attackSystem(d.at(x))
				d.w.mu.Unlock()
			}
			return len(ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication))
		}
		n := run(2*time.Second, 3100*time.Millisecond)
		if changed {
			d.w.HandsChanged(d.sa, one)
		}
		if n += run(3200*time.Millisecond, 7900*time.Millisecond); n != 4 {
			t.Errorf("changed %v: the round in hand landed %d hits, want its four", changed, n)
		}
		want, swing := 1, one
		if !changed {
			want, swing = 4, four // 8.5, 9, 9.7 and 10.4 s
		}
		if n := run(8*time.Second, 10900*time.Millisecond); n != want || d.a.Swing != swing {
			t.Errorf("changed %v: the next round %d hits by 10.9 s, swing %+v; want %d", changed, n, d.a.Swing, want)
		}
	}
}

// A cast that wants a ranged weapon hands its caster to the hands-back hook
// once it has landed, with the lock let go (runHandsBack), when the caster
// stands within its melee reach of the cast's target: 4.5 off a creature of
// no known body, and 4.5 with the allowance of 3 off a big one (reach.go),
// whether this cast's own switch took up the bow (HandsBack) or an earlier
// one did (a pull at range: the hook's memory decides). A self cast hands it
// over when its own switch took up the bow (borrowed), else by its reach to
// the creature it fights: in melee yes, at range or out of a fight no. Not
// before the landing, not at range (the bow kept), not for a cast that wants
// no ranged weapon, not with another cast on at the landing or begun before
// the hook runs. The controls: the first case, the big creature's distance
// off one of no body, and the self cast's own switch.
func TestACastsHandsGoBackAtItsLandingInMeleeReach(t *testing.T) {
	withBodies(t)
	arrow := CastSpell{SpellID: 1151, DisplayIndex: 11, Scope: 1, Name: "Test Arrow", Range: 30,
		Def: ParseSpellText("Target, Decrease Hitpoints by 1"), HandsBack: true}
	arrow.Def.ReqEquip = "Range"
	eye := CastSpell{SpellID: 1152, DisplayIndex: 12, Scope: 0, Name: "Test Eye",
		Def: ParseSpellText("Self, Increase DEX by 5, 1min"), HandsBack: true}
	eye.Def.ReqEquip = "Range"
	hum := CastSpell{SpellID: 1153, DisplayIndex: 13, Scope: 0, Name: "Test Hum", CastTime: 2 * time.Second,
		Def: ParseSpellText("Self, Increase STR by 1, 1min")}
	blade := arrow
	blade.SpellID, blade.Name, blade.Def.ReqEquip = 1154, "Test Edge", "1HS,2HS"
	inHand := func(s CastSpell) CastSpell { s.HandsBack = false; return s } // no switch of its own: the bow in hand
	for _, tc := range []struct {
		name  string
		spell CastSpell
		set   func(c *caster)
		cast  func(c *caster) // after the spell has begun
		late  func(c *caster) // after its landing tick, before the hook runs
		want  int
	}{
		{name: "in melee reach", spell: arrow, want: 1},
		{name: "in melee reach, the bow an earlier switch's", spell: inHand(arrow), want: 1},
		{name: "an art that wants no ranged weapon, though marked", spell: blade, set: func(c *caster) { c.p.PrimaryType, c.p.SecondaryType = 1, 1 }},
		{name: "at range", spell: arrow, set: func(c *caster) { c.npc.X = c.p.X + 20 }},
		{name: "at range, the bow an earlier switch's", spell: inHand(arrow), set: func(c *caster) { c.npc.X = c.p.X + 20 }},
		{name: "a big creature at 6.5", spell: arrow, set: func(c *caster) { c.npc.ModelID, c.npc.X = testGiant, c.p.X+6.5 }, want: 1},
		{name: "a creature of no known body at 6.5", spell: arrow, set: func(c *caster) { c.npc.X = c.p.X + 6.5 }},
		{name: "a self cast its switch took up the bow for", spell: eye, set: func(c *caster) { c.npc.X = c.p.X + 40 }, want: 1},
		{name: "a self cast, fighting in melee", spell: inHand(eye), set: func(c *caster) { c.p.AttackTarget = c.npc.ServerID }, want: 1},
		{name: "a self cast, fighting at range", spell: inHand(eye), set: func(c *caster) {
			c.p.AttackTarget, c.npc.X = c.npc.ServerID, c.p.X+20
		}},
		{name: "a self cast out of a fight", spell: inHand(eye)},
		{name: "another cast on at the landing", spell: arrow, cast: func(c *caster) {
			if !c.w.StartCast(c.sess, hum, 0xFFFFFFFF, c.at(500*time.Millisecond)) {
				t.Fatal("the second cast did not begin")
			}
		}},
		{name: "a cast begun before the hook", spell: arrow, late: func(c *caster) {
			c.w.mu.Lock()
			c.p.Casting = &castState{spell: hum, ends: c.at(time.Hour), lands: c.at(time.Hour)}
			c.w.mu.Unlock()
		}},
	} {
		c := newCaster(t)
		c.p.SecondaryType = 7 // the bow the switch took up, alone in the hands
		var calls []string
		c.w.OnHandsBack(func(s *session.Session, p *Player) {
			if s != c.sess {
				t.Errorf("%s: the hook was handed another session", tc.name)
			}
			calls = append(calls, p.Name)
		})
		if tc.set != nil {
			tc.set(c)
		}
		target := c.npcO
		if tc.spell.Scope == 0 {
			target = 0xFFFFFFFF
		}
		if !c.w.StartCast(c.sess, tc.spell, target, c.at(0)) {
			t.Fatalf("%s: %q did not begin: %x", tc.name, tc.spell.Name, c.sent())
		}
		if tc.cast != nil {
			tc.cast(c)
		}
		c.tick(castLand - 100*time.Millisecond)
		c.w.runHandsBack()
		if len(calls) != 0 {
			t.Errorf("%s: the hands went back before the landing", tc.name)
		}
		c.tick(castLand)
		if tc.late != nil {
			tc.late(c)
		}
		c.w.runHandsBack()
		c.w.runHandsBack() // handed once
		if len(calls) != tc.want {
			t.Errorf("%s: the hook ran %d times, want %d", tc.name, len(calls), tc.want)
		}
	}
}
