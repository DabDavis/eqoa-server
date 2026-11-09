package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// bufferSpell is a made-up workbook spell of one of Buffer's shapes, with its icon and visual.
func bufferSpell(id, level, scope int, name, js string, fx uint32) CastSpell {
	sp := &database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: 0x41F0, CastTime: 3, Power: 300, Scope: scope,
		Recast: 4, SpellName: name, Icon: int64(id + 1000)}
	return SpellOfRow(sp, 0, "", js, fx)
}

// gmArmorJSON is the GM Armor's effects as the data step writes them: +500 AC on a stack type of its own (its id).
const gmArmorJSON = `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":18000,"stack":905120,` +
	`"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Buff AC",` +
	`"effects":[{"id":"GMAC","type":"Statistic Change","kind":"stat","stat":"ac","amount":500}]}`

// bufferFive is his five in their shapes (made-up figures): a heal with stats and its maximum (stack 7), an AC and
// HP buff (stack 1), a max HP and power buff for those round its caster (stack 3), an all-resist buff (stack 1, no
// stat shared with the AC buff), and the GM Armor's record.
func bufferFive() []CastSpell {
	return []CastSpell{
		bufferSpell(9301, 51, 1, "Test Avatar", `{"v":1,"scope":"target","harmful":false,"duration_s":1830,"stack":7,"level":51,"effects":[
			{"kind":"hp","amount":300},{"kind":"stat","stat":"str","amount":40},{"kind":"stat","stat":"maxhp","amount":300}]}`, 0x0A0A0001),
		bufferSpell(9302, 60, 1, "Test Order", `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":60,"effects":[
			{"kind":"stat","stat":"ac","amount":65},{"kind":"stat","stat":"maxhp","amount":65}]}`, 0x0A0A0002),
		bufferSpell(9303, 60, 0, "Test Heart", `{"v":1,"scope":"player_aoe","harmful":false,"duration_s":1800,"stack":3,"level":60,"radius":10,"effects":[
			{"kind":"stat","stat":"maxhp","amount":120},{"kind":"stat","stat":"maxpower","amount":120}]}`, 0x0A0A0003),
		bufferSpell(9304, 60, 1, "Test Ward", `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":60,"effects":[
			{"kind":"stat","stat":"fr","amount":85},{"kind":"stat","stat":"cr","amount":85},{"kind":"stat","stat":"lr","amount":85},
			{"kind":"stat","stat":"pr","amount":85},{"kind":"stat","stat":"dr","amount":85},{"kind":"stat","stat":"ar","amount":85}]}`, 0x0A0A0004),
		bufferSpell(905120, 60, 1, "GM Armor", gmArmorJSON, 0x1DAEB5B6),
	}
}

// newBufferRig is newCaster with its NPC made Buffer, 2 off the player and on its first slot.
func newBufferRig(t *testing.T) *caster {
	t.Helper()
	c := newCaster(t)
	c.npc.Name, c.npc.NPCType, c.npc.Level, c.npc.HP, c.npc.MaxHP, c.npc.Facing = BufferName, 0x80, 60, 5000, 5000, 1.5
	return c
}

// bufferTick runs his landings and the effects' ends at d.
func (c *caster) bufferTick(d time.Duration) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.bufferSystem(c.at(d))
	c.w.effectSystem(c.at(d))
}

func (c *caster) effectOf(id int) *ActiveEffect {
	for _, e := range c.p.Effects {
		if e.SpellID == id {
			return e
		}
	}
	return nil
}

// casts is the 0x00B1s among msgs: caster, target, cast time and visual.
func casts(msgs [][]byte) [][4]uint32 {
	var out [][4]uint32
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeCastingCombat || len(m) < 19 {
			continue
		}
		n := int(m[10])
		at := 11 + 4*n
		out = append(out, [4]uint32{binary.LittleEndian.Uint32(m[2:]), binary.LittleEndian.Uint32(m[6:]),
			binary.LittleEndian.Uint32(m[at:]), binary.LittleEndian.Uint32(m[at+4:])})
	}
	return out
}

// A talk's cast (petition 1390): each of his spells' 0x00B1 from him at the player, a second long, in its own
// visual, him left facing as he stood; nothing on the player before the second is up; at it, all five, each until
// five hours on (the records' own 30 minutes left as they were), their stats summed and Avatar's heal landed, their
// icons on the effects channel, and his line naming them; five hours of play on, all five gone.
func TestBufferLandsFiveForFiveHoursASecondOn(t *testing.T) {
	c := newBufferRig(t)
	c.p.HP = 100
	spells := bufferFive()
	line, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, spells, c.at(0))
	if !ok || line != BufferLineBegins {
		t.Fatalf("StartBuffer: %q %v", line, ok)
	}
	got := casts(c.sent())
	if len(got) != len(spells) {
		t.Fatalf("his casts: %x, want %d", got, len(spells))
	}
	for i, g := range got {
		if want := [4]uint32{c.npcO, c.me, 1000, spells[i].FX}; g != want {
			t.Errorf("cast %d: %x, want %x (from him at the player, a second, its visual)", i, g, want)
		}
	}
	if c.npc.Facing != 1.5 {
		t.Errorf("he turned: facing %v", c.npc.Facing)
	}
	for i, s := range bufferFive() {
		if spells[i].Def.Duration != s.Def.Duration {
			t.Errorf("%s's own duration was changed: %v, its record's %v", s.Name, spells[i].Def.Duration, s.Def.Duration)
		}
	}
	c.bufferTick(999 * time.Millisecond)
	if len(c.p.Effects) != 0 {
		t.Fatalf("landed before the second: %v", c.effectNames())
	}
	c.bufferTick(time.Second)
	// the heal goes on last (bufferLandLocked), the rest in his order
	if names := c.effectNames(); strings.Join(names, ",") != "Test Order,Test Heart,Test Ward,GM Armor,Test Avatar" {
		t.Fatalf("at the second: %v", names)
	}
	for _, e := range c.p.Effects {
		if want := c.at(time.Second + BufferLasts); !e.Until.Equal(want) {
			t.Errorf("%s until %v, want %v (five hours)", e.Name, e.Until, want)
		}
	}
	m := c.p.Mods
	if m.AC != 565 || m.MaxHP != 485 || m.MaxPower != 120 || m.Str != 40 || m.FR != 85 || m.AR != 85 || m.PR != 85 {
		t.Errorf("their stats: %+v", m)
	}
	if c.p.HP != 400 {
		t.Errorf("Avatar's heal: %d HP, want 400", c.p.HP)
	}
	if len(c.p.Buffs) != 5 || c.p.Buffs[3].Name != "GM Armor" || c.p.Buffs[4].Icon != 9301+1000 {
		t.Errorf("the effects channel's slots: %+v", c.p.Buffs)
	}
	said := pvpChat(c.sent())
	if want := "Buffer says, 'There you are, Druid: Test Avatar, Test Order, Test Heart, Test Ward and GM Armor, for five hours of play.'"; len(said) != 1 || said[0] != want {
		t.Errorf("his line: %q, want %q", said, want)
	}
	c.bufferTick(time.Second + BufferLasts - time.Millisecond)
	if len(c.p.Effects) != 5 {
		t.Errorf("a moment before five hours: %v", c.effectNames())
	}
	c.bufferTick(time.Second + BufferLasts)
	if len(c.p.Effects) != 0 || c.p.Mods != (StatMods{}) {
		t.Errorf("five hours on: %v, %+v", c.effectNames(), c.p.Mods)
	}
}

// The GM Armor stacks with every AC buff: Test Skin (stack 1, +AC +max HP, rank 50) is taken off by his Test Order
// (its line, rank 60: the control, the line rule at work on his other spells) and the GM Armor neither takes it off
// nor is taken off; a buff of a line read from its text (+AC alone) and one of another stack type (the self AC line,
// stack 2) stay beside it, every AC summed. After his: Test Skin again is refused under Test Order, and a stronger
// one of their line takes Test Order's place; the GM Armor stays through both.
func TestBufferGMArmorStacksWithEveryACBuff(t *testing.T) {
	c := newBufferRig(t)
	skin := bufferSpell(9311, 50, 1, "Test Skin", `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":50,"effects":[
		{"kind":"stat","stat":"ac","amount":100},{"kind":"stat","stat":"maxhp","amount":10}]}`, 0)
	self := bufferSpell(9312, 45, 0, "Test Steel", `{"v":1,"scope":"self","harmful":false,"duration_s":600,"stack":2,"level":45,"effects":[
		{"kind":"stat","stat":"ac","amount":248}]}`, 0)
	text := CastSpell{SpellID: 9313, Name: "Test Rock", Scope: 0, Level: 30, Def: ParseSpellNamed("Test Rock", "Self, Increase AC by 30, 30min", "")}
	mightier := bufferSpell(9314, 61, 1, "Test Mightier Order", `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":61,"effects":[
		{"kind":"stat","stat":"ac","amount":700},{"kind":"stat","stat":"maxhp","amount":700}]}`, 0)
	for _, s := range []CastSpell{skin, self, text} {
		if !c.w.LandOnSelf(c.sess, s, c.at(0)) {
			t.Fatalf("%s did not land", s.Name)
		}
	}
	if c.p.Mods.AC != 378 {
		t.Fatalf("before: AC +%d, want 378", c.p.Mods.AC)
	}
	if _, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(time.Minute)); !ok {
		t.Fatal("not begun")
	}
	c.bufferTick(time.Minute + time.Second)
	if c.effectOf(9311) != nil || c.effectOf(9302) == nil {
		t.Errorf("the control: Test Order (stack 1, rank 60) did not take Test Skin's (its line, rank 50) place: %v", c.effectNames())
	}
	if c.effectOf(905120) == nil || c.effectOf(9312) == nil || c.effectOf(9313) == nil {
		t.Fatalf("the GM Armor beside the others: %v", c.effectNames())
	}
	if c.p.Mods.AC != 65+500+248+30 {
		t.Errorf("AC +%d, want %d", c.p.Mods.AC, 65+500+248+30)
	}
	c.w.LandOnSelf(c.sess, skin, c.at(2*time.Minute))
	if c.effectOf(9311) != nil || c.effectOf(9302) == nil || c.effectOf(905120) == nil {
		t.Errorf("Test Skin after his, under Test Order: %v", c.effectNames())
	}
	c.w.LandOnSelf(c.sess, mightier, c.at(3*time.Minute))
	if c.effectOf(9302) != nil || c.effectOf(9314) == nil || c.effectOf(905120) == nil {
		t.Errorf("a stronger one of Test Order's line: %v (in Test Order's place, the GM Armor kept)", c.effectNames())
	}
	if c.p.Mods.AC != 700+500+248+30 {
		t.Errorf("AC +%d, want %d", c.p.Mods.AC, 700+500+248+30)
	}
	if e := c.effectOf(905120); e == nil || e.Line != "stack905120 player +AC" {
		t.Errorf("the GM Armor's line: %+v", e)
	}
}

// A second talk with his on renews each to its whole five hours, in its own old one's place (five effects, not
// ten), and a renewal at full health and power leaves both full (each old one comes off before its new one goes on,
// which holds them under the lower maximum of that moment); a renewal 200 short gets its heal whole (the heal goes on
// last: before another's renewal, that renewal would take 120 of it back). A talk while his cast is on its way is let
// go, nothing sent; the next lands once.
func TestBufferSecondTalkRenews(t *testing.T) {
	c := newBufferRig(t)
	if _, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0)); !ok {
		t.Fatal("not begun")
	}
	c.bufferTick(time.Second)
	c.p.HP = c.p.MaxHPNow() - 200
	c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(time.Minute))
	c.bufferTick(time.Minute + time.Second)
	if c.p.HP != c.p.MaxHPNow() {
		t.Errorf("a renewal 200 short: %d/%d HP, want the heal whole", c.p.HP, c.p.MaxHPNow())
	}
	two := 2 * time.Hour
	c.bufferTick(two)
	c.p.HP, c.p.Power = c.p.MaxHPNow(), c.p.MaxPowerNow()
	if _, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(two)); !ok {
		t.Fatal("the second talk not begun")
	}
	c.sent()
	if line, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(two+500*time.Millisecond)); ok || line != "" || len(c.sent()) != 0 {
		t.Errorf("a talk while his cast is on its way: %q %v", line, ok)
	}
	c.bufferTick(two + time.Second)
	if len(c.p.Effects) != 5 || len(c.p.Buffs) != 5 {
		t.Fatalf("renewed: %v, %d slots", c.effectNames(), len(c.p.Buffs))
	}
	for _, e := range c.p.Effects {
		if want := c.at(two + time.Second + BufferLasts); !e.Until.Equal(want) {
			t.Errorf("%s until %v, want %v", e.Name, e.Until, want)
		}
	}
	if m := c.p.Mods; m.AC != 565 || m.MaxHP != 485 {
		t.Errorf("renewed, not stacked on themselves: %+v", m)
	}
	if c.p.HP != c.p.MaxHPNow() || c.p.Power != c.p.MaxPowerNow() {
		t.Errorf("renewed at full: %d/%d HP, %d/%d power", c.p.HP, c.p.MaxHPNow(), c.p.Power, c.p.MaxPowerNow())
	}
	c.bufferTick(two + 2*time.Second)
	if n := len(pvpChat(c.sent())); n != 1 {
		t.Errorf("%d lines from the renewal, want 1", n)
	}
}

// Only the speaker: a player beside it gets nothing. He refuses the dead and one flagged for PvP in a line, and a
// cast finds nobody when its speaker has gone past his reach, or left the world, by its landing.
func TestBufferTheSpeakerAlone(t *testing.T) {
	c := newBufferRig(t)
	other := &Player{ServerID: 1002, Name: "Neighbour", Class: 9, Level: 40, HP: 500, MaxHP: 500, X: 101, Y: 10, Z: 100}
	c.w.Players[other.ServerID] = other
	c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0))
	c.bufferTick(time.Second)
	if len(c.p.Effects) != 5 || len(other.Effects) != 0 {
		t.Errorf("the speaker %v, beside it %d effects", c.effectNames(), len(other.Effects))
	}

	for _, k := range []struct {
		name string
		set  func(*Player)
		want string
	}{
		{"dead", func(p *Player) { p.HP = 0 }, BufferLineDead},
		{"flagged for PvP", func(p *Player) { p.pvp.on = true }, BufferLinePvP},
	} {
		c := newBufferRig(t)
		k.set(c.p)
		if line, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0)); ok || line != k.want || len(casts(c.sent())) != 0 {
			t.Errorf("%s: %q %v", k.name, line, ok)
		}
	}

	c = newBufferRig(t)
	c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0))
	c.p.X += bufferReach + 5 // ran off within the second
	c.bufferTick(time.Second)
	if len(c.p.Effects) != 0 {
		t.Errorf("past his reach: %v", c.effectNames())
	}
	c = newBufferRig(t)
	c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0))
	delete(c.w.Players, c.p.ServerID) // logged out within the second
	c.bufferTick(time.Second)
	if len(c.p.Effects) != 0 {
		t.Errorf("gone: %v", c.effectNames())
	}
}

// His line names what took: a spell under a stronger one of its line is not taken (its bearer told in red, the
// line rule's), and when none takes he says so.
func TestBufferLineNamesWhatTook(t *testing.T) {
	c := newBufferRig(t)
	strong := bufferSpell(9321, 61, 1, "Test Mightier Order", `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":1,"level":61,"effects":[
		{"kind":"stat","stat":"ac","amount":700},{"kind":"stat","stat":"maxhp","amount":700}]}`, 0)
	c.w.LandOnSelf(c.sess, strong, c.at(0))
	c.sent()
	c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0))
	c.bufferTick(time.Second)
	msgs := c.sent()
	if want := "Buffer says, 'There you are, Druid: Test Avatar, Test Heart, Test Ward and GM Armor, for five hours of play.'"; strings.Join(pvpChat(msgs), "|") != want {
		t.Errorf("his line: %q, want %q", pvpChat(msgs), want)
	}
	if c.effectOf(9302) != nil || c.effectOf(9321) == nil {
		t.Errorf("Test Order under a stronger one of its line: %v", c.effectNames())
	}

	c = newBufferRig(t)
	only := []CastSpell{bufferFive()[1]}
	c.w.LandOnSelf(c.sess, strong, c.at(0))
	c.w.StartBuffer(c.sess, c.npc.ServerID, only, c.at(0))
	c.sent()
	c.bufferTick(time.Second)
	if said := pvpChat(c.sent()); len(said) != 1 || said[0] != "Buffer says, 'You already carry stronger blessings than mine, Druid.'" {
		t.Errorf("none took: %q", said)
	}
}
