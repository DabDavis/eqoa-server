package entity

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Casting held to what Sandstorm sent a druid of 23 (eqoa harvest,
// 2026-10-21): the bytes of its starts, refusals, heal figures and lockout
// ends, and the arithmetic of its lines (CHA 60, DEX 105). The spells here
// are made up but for those numbers.

type caster struct {
	w    *World
	sess *session.Session
	p    *Player
	npc  *NPC
	t0   time.Time
	me   uint32
	npcO uint32
}

func newCaster(t *testing.T) *caster {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Druid", Class: 7, Level: 23, HP: 500, MaxHP: 536, Power: 900, MaxPower: 1025,
		Str: 60, Sta: 103, Agi: 100, Dex: 105, Wis: 87, Int: 55, Cha: 60, Speed: 8.833333, X: 100, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1424A1D0)
	npc := &NPC{ServerID: 2002, Name: "Rotting Flesh", Level: 1, HP: 30, MaxHP: 30, X: 102, Y: 10, Z: 100}
	w.NPCs[npc.ServerID] = npc
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = 0x01312E98
	return &caster{w: w, sess: sess, p: p, npc: npc, t0: time.Unix(1_800_000_000, 0), me: 0x1424A1D0, npcO: 0x01312E98}
}

func (c *caster) at(d time.Duration) time.Time { return c.t0.Add(d) }

func (c *caster) tick(d time.Duration) []respawnDue {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	gates := c.w.castSystem(c.at(d))
	c.w.effectSystem(c.at(d))
	return gates
}

// sent drains the queue: each message whole, opcode first.
func (c *caster) sent() [][]byte {
	var out [][]byte
	for _, m := range c.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func opOfMsg(b []byte) protocol.GameOpcode { return protocol.GameOpcode(binary.LittleEndian.Uint16(b)) }

func wantOps(t *testing.T, msgs [][]byte, want ...protocol.GameOpcode) {
	t.Helper()
	if len(msgs) != len(want) {
		t.Fatalf("sent %d messages %x, want %v", len(msgs), msgs, want)
	}
	for i := range want {
		if opOfMsg(msgs[i]) != want[i] {
			t.Fatalf("message %d is %04X, want %v (all: %x)", i, uint16(opOfMsg(msgs[i])), want, msgs)
		}
	}
}

// A heal like Minor Healing: display index 0, cast 2, recast 1, cost 15.
func minorHeal() CastSpell {
	return CastSpell{SpellID: 457, DisplayIndex: 0, Scope: 1, Name: "Minor Mending", Icon: 0x828563AF,
		CastTime: 2 * time.Second, Recast: time.Second, PowerCost: 15, Range: 30, FX: 0xC2B700FA,
		Def: ParseSpellText("Target, Increase HP per tic by 20 ; Increase 25% based on CHA, Decrease Power by 1, 24sec")}
}

// A strike like Fiery Burst: display index 12, cast 2, recast 3, cost 21.
func strike() CastSpell {
	return CastSpell{SpellID: 650, DisplayIndex: 12, Scope: 1, Name: "Burst", Icon: 0x11111111,
		CastTime: 2 * time.Second, Recast: 3 * time.Second, PowerCost: 21, Range: 30, FX: 0xE78024D9,
		Def: ParseSpellText("Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX")}
}

// The start: 0x00CD then 0x00B1, in Sandstorm's bytes (its druid's ids), the
// cost taken at once. Then the landing 1.4 s after the cast time: 0x00DB of
// +35 (20 + 25% of CHA 60), byte for byte, and the effect on the channel.
// Then the lockout's end, 3300 ms from the start: 0x00CD with 0.
func TestCastIsSandstorms(t *testing.T) {
	c := newCaster(t)
	if !c.w.StartCast(c.sess, minorHeal(), c.me, c.at(0)) {
		t.Fatal("not begun")
	}
	msgs := c.sent()
	wantOps(t, msgs, protocol.OpcodeCooldownTimer, protocol.OpcodeCastingCombat)
	if w := unhex(t, "cd000000000000c833"); !bytes.Equal(msgs[0], w) {
		t.Errorf("0x00CD % X, want Sandstorm's % X", msgs[0], w)
	}
	if w := unhex(t, "b100d0a12414d0a1241400d0070000fa00b7c2"); !bytes.Equal(msgs[1], w) {
		t.Errorf("0x00B1 % X, want Sandstorm's % X", msgs[1], w)
	}
	if c.p.Power != 885 || !c.p.StatsDirty {
		t.Errorf("power %d (dirty %v), want 885 at once", c.p.Power, c.p.StatsDirty)
	}

	// the lockout's end (3.3 s) comes before the landing
	c.tick(3399 * time.Millisecond)
	if msgs := c.sent(); len(msgs) != 1 || opOfMsg(msgs[0]) != protocol.OpcodeCooldownTimer {
		t.Fatalf("by 3.399 s: %x, want the lockout's end alone", msgs)
	}
	c.tick(3400 * time.Millisecond)
	msgs = c.sent()
	wantOps(t, msgs, protocol.OpcodeDamageIndication)
	if w := unhex(t, "db00d0a1241446d0a12414"); !bytes.Equal(msgs[0], w) {
		t.Errorf("0x00DB % X, want Sandstorm's % X (+35)", msgs[0], w)
	}
	if c.p.HP != 535 || len(c.p.Buffs) != 1 || c.p.Buffs[0] != (BuffSlot{Icon: 0x828563AF, Name: "Minor Mending"}) {
		t.Errorf("HP %d, effects %+v", c.p.HP, c.p.Buffs)
	}
	// the next tick, 6 s on: +35 again (short of the maximum by 1)
	c.tick(9400 * time.Millisecond)
	if msgs := c.sent(); len(msgs) != 1 || !bytes.Equal(msgs[0], unhex(t, "db00d0a1241446d0a12414")) || c.p.HP != 536 {
		t.Errorf("second tick: %x, HP %d", msgs, c.p.HP)
	}
	// at full hit points a tick tells nothing
	c.tick(15400 * time.Millisecond)
	if msgs := c.sent(); len(msgs) != 0 {
		t.Errorf("a tick at full HP sent %x", msgs)
	}
	// 24 s after landing it wears off
	c.tick(27400 * time.Millisecond)
	if len(c.p.Effects) != 0 || len(c.p.Buffs) != 0 {
		t.Errorf("still on: %+v", c.p.Buffs)
	}
}

func TestCastLockoutEnds(t *testing.T) {
	c := newCaster(t)
	c.w.StartCast(c.sess, minorHeal(), c.me, c.at(0))
	c.sent()
	c.p.HP = c.p.MaxHP // no heal figure
	c.tick(3299 * time.Millisecond)
	for _, m := range c.sent() {
		if opOfMsg(m) == protocol.OpcodeCooldownTimer {
			t.Fatal("the lockout ended early")
		}
	}
	c.tick(3300 * time.Millisecond)
	msgs := c.sent()
	wantOps(t, msgs, protocol.OpcodeCooldownTimer)
	if w := unhex(t, "cd00000000000000"); !bytes.Equal(msgs[0], w) {
		t.Errorf("0x00CD % X, want Sandstorm's % X", msgs[0], w)
	}
}

// Sandstorm's five refusals, byte for byte, none taking power or starting a
// lockout.
func TestCastRefusalsAreSandstorms(t *testing.T) {
	cases := []struct {
		name   string
		spell  func() CastSpell
		target func(c *caster) uint32
		setup  func(c *caster)
		want   string
	}{
		{"No target.", strike, func(c *caster) uint32 { return 0xFFFFFFFF }, nil,
			"7b0a0a0000004e006f0020007400610072006700650074002e00ff3f3f"},
		{"Target is friendly.", strike, func(c *caster) uint32 { return c.me }, nil,
			"7b0a13000000540061007200670065007400200069007300200066007200690065006e0064006c0079002e00ff3f3f"},
		{"Target is not friendly.", minorHeal, func(c *caster) uint32 { return c.npcO }, nil,
			"7b0a1700000054006100720067006500740020006900730020006e006f007400200066007200690065006e0064006c0079002e00ff3f3f"},
		{"Out of range.", strike, func(c *caster) uint32 { return c.npcO }, func(c *caster) { c.npc.X = 155 },
			"7b0a0d0000004f007500740020006f0066002000720061006e00670065002e00ff3f3f"},
		{"Not enough power.", strike, func(c *caster) uint32 { return c.npcO }, func(c *caster) { c.p.Power = 20 },
			"7b0a110000004e006f007400200065006e006f00750067006800200070006f007700650072002e00ff3f3f"},
	}
	for _, tc := range cases {
		c := newCaster(t)
		if tc.setup != nil {
			tc.setup(c)
		}
		power := c.p.Power
		if c.w.StartCast(c.sess, tc.spell(), tc.target(c), c.at(0)) {
			t.Errorf("%s: begun", tc.name)
		}
		msgs := c.sent()
		if len(msgs) != 1 || !bytes.Equal(msgs[0], unhex(t, tc.want)) {
			t.Errorf("%s: sent %x\n want %s", tc.name, msgs, tc.want)
		}
		if c.p.Power != power || len(c.p.Lockouts) != 0 || c.p.Casting != nil {
			t.Errorf("%s: power %d, lockouts %v", tc.name, c.p.Power, c.p.Lockouts)
		}
	}
}

// A cast sent during another, or into its own lockout, is not answered.
func TestCastDuringACastIsNotAnswered(t *testing.T) {
	c := newCaster(t)
	c.w.StartCast(c.sess, minorHeal(), c.me, c.at(0))
	c.sent()
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(600*time.Millisecond)) || len(c.sent()) != 0 {
		t.Error("a cast 0.6 s into another was answered")
	}
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(2399*time.Millisecond)) || len(c.sent()) != 0 {
		t.Error("a cast within cast time + 0.4 s was answered")
	}
	// the other spell is free once the cast is over; the same one is not
	if c.w.StartCast(c.sess, minorHeal(), c.me, c.at(2500*time.Millisecond)) || len(c.sent()) != 0 {
		t.Error("a cast into its own lockout was answered")
	}
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(2500*time.Millisecond)) {
		t.Error("another spell after the cast was refused")
	}
}

// A strike lands as a hit of 26 + 8% of DEX 105 (34): 0x00DB from the
// caster, negative, and a 30 HP creature dies of it, the caster paid in XP.
func TestCastStrikeKills(t *testing.T) {
	c := newCaster(t)
	c.w.StartCast(c.sess, strike(), c.npcO, c.at(0))
	c.sent()
	c.tick(3400 * time.Millisecond)
	var hit []byte
	for _, m := range c.sent() {
		if opOfMsg(m) == protocol.OpcodeDamageIndication {
			hit = m
		}
	}
	// zigzag(-34) = 67 = 0x43
	if w := unhex(t, "db00d0a1241443982e3101"); !bytes.Equal(hit, w) {
		t.Errorf("0x00DB % X, want % X (-34)", hit, w)
	}
	if !c.npc.Dead || c.p.TotalXP == 0 {
		t.Errorf("dead %v, XP %d", c.npc.Dead, c.p.TotalXP)
	}
}

// What a buff adds, as Rockskin's did on Sandstorm (AC +150, max power
// +60, max HP +30) and as the stat channel then carries it; all of it gone
// when it wears off.
func TestCastBuffStats(t *testing.T) {
	c := newCaster(t)
	sp := CastSpell{SpellID: 900, DisplayIndex: 10, Scope: 1, Name: "Stoneskin", Icon: 0x727C44F8,
		CastTime: 4 * time.Second, Recast: 3 * time.Second, PowerCost: 120, Range: 30,
		Def: ParseSpellText("Target, Increase AC by 150, Max Power by 60, MaxHP by 30, HP by 30, 30min")}
	c.w.StartCast(c.sess, sp, c.me, c.at(0))
	c.tick(5400 * time.Millisecond)
	if c.p.MaxHPNow() != 566 || c.p.MaxPowerNow() != 1085 || c.p.StatNow(StatAC) != 150 || c.p.HP != 530 {
		t.Errorf("max HP %d, max power %d, AC %d, HP %d", c.p.MaxHPNow(), c.p.MaxPowerNow(), c.p.StatNow(StatAC), c.p.HP)
	}
	b := SerializeStatUpdate(c.p)
	if g := [3]uint32{binary.LittleEndian.Uint32(b[32:]), binary.LittleEndian.Uint32(b[40:]), binary.LittleEndian.Uint32(b[56:])}; g != [3]uint32{566, 1085, 150} {
		t.Errorf("stat channel max HP, max power, AC: %v", g)
	}
	if binary.LittleEndian.Uint32(b[148:]) != 536 {
		t.Error("the base max HP took the buff")
	}
	c.tick(5400*time.Millisecond + 30*time.Minute)
	if c.p.MaxHPNow() != 536 || c.p.StatNow(StatAC) != 0 || c.p.HP > 536 {
		t.Errorf("worn off: max HP %d, AC %d, HP %d", c.p.MaxHPNow(), c.p.StatNow(StatAC), c.p.HP)
	}
}

// A self spell lands on the caster whatever was targeted: Sandstorm
// answered one sent with no target and one sent at a creature so, and the
// 0x00B1's target is the caster.
func TestCastSelfScope(t *testing.T) {
	for _, target := range []uint32{0xFFFFFFFF, 0x01312E98} {
		c := newCaster(t)
		sp := CastSpell{SpellID: 901, DisplayIndex: 9, Scope: 0, Name: "Insight", CastTime: 4 * time.Second, Recast: 3 * time.Second,
			PowerCost: 10, Range: 30, FX: 0xD17B032D, Def: ParseSpellText("Self, Increase WIS by 12, 30min")}
		if !c.w.StartCast(c.sess, sp, target, c.at(0)) {
			t.Fatalf("target %#x: not begun", target)
		}
		msgs := c.sent()
		if w := unhex(t, "b100d0a12414d0a1241400a00f00002d037bd1"); len(msgs) != 2 || !bytes.Equal(msgs[1], w) {
			t.Errorf("target %#x: %x, want Sandstorm's %x", target, msgs, w)
		}
		c.tick(5400 * time.Millisecond)
		if c.p.StatNow(StatWis) != 99 {
			t.Errorf("WIS %d", c.p.StatNow(StatWis))
		}
	}
}

// A movement buff makes the run speed 1.3 times (8.83 -> 11.48 on
// Sandstorm) by 0x00F8, and back when it wears off.
func TestCastSpeed(t *testing.T) {
	c := newCaster(t)
	sp := CastSpell{SpellID: 902, DisplayIndex: 3, Scope: 1, Name: "Wolf Spirit", CastTime: 2 * time.Second, Recast: time.Second,
		PowerCost: 30, Range: 30, Def: ParseSpellText("Target, Sow, 30min")}
	c.w.StartCast(c.sess, sp, c.me, c.at(0))
	c.sent()
	c.tick(3400 * time.Millisecond)
	speed := func() []float32 {
		var out []float32
		for _, m := range c.sent() {
			if opOfMsg(m) == protocol.OpcodePlayerSpeed {
				out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(m[2:])))
			}
		}
		return out
	}
	if s := speed(); len(s) != 1 || s[0] < 11.48 || s[0] > 11.49 {
		t.Errorf("0x00F8 %v, want 11.48", s)
	}
	c.tick(3400*time.Millisecond + 30*time.Minute)
	if s := speed(); len(s) != 1 || s[0] != c.p.Speed {
		t.Errorf("worn off: 0x00F8 %v", s)
	}
}

// Return Home lands as the caster's gate (the hook moves it, unlocked).
func TestCastGate(t *testing.T) {
	c := newCaster(t)
	sp := CastSpell{SpellID: 1623, DisplayIndex: 1, Scope: 0, Name: "Go Home", CastTime: 60 * time.Second, Recast: 5 * time.Second,
		PowerCost: 25, Range: 4.5, Def: ParseSpellText("Returns you to last bind point")}
	c.w.StartCast(c.sess, sp, 0xFFFFFFFF, c.at(0))
	if g := c.tick(61 * time.Second); len(g) != 0 {
		t.Fatal("home before it landed")
	}
	if g := c.tick(61400 * time.Millisecond); len(g) != 1 || g[0].p != c.p || g[0].sess != c.sess {
		t.Errorf("gates %+v", g)
	}
}

// A per-tick line with no duration ticks for 24 s.
func TestCastTickWithoutDuration(t *testing.T) {
	c := newCaster(t)
	sp := minorHeal()
	sp.Def = ParseSpellText("Group, Increase HP per tic by 10 ; Increase 10% based on CHA")
	sp.Scope = 2
	c.p.HP = 400
	c.w.StartCast(c.sess, sp, 0xFFFFFFFF, c.at(0))
	c.tick(3400 * time.Millisecond)
	if c.p.HP != 416 || len(c.p.Effects) != 1 || !c.p.Effects[0].Until.Equal(c.at(27400*time.Millisecond)) {
		t.Errorf("HP %d, effects %+v", c.p.HP, c.p.Effects)
	}
}

// With no visual recorded, the stand-in of the spell's kind.
func TestCastVisualStandIns(t *testing.T) {
	heal, hit, buff := minorHeal(), strike(), CastSpell{Def: ParseSpellText("Self, Increase WIS by 2, 30min")}
	heal.FX, hit.FX = 0, 0
	if heal.visual() != fxHeal || hit.visual() != fxDamage || buff.visual() != fxBuff {
		t.Errorf("%08X %08X %08X", heal.visual(), hit.visual(), buff.visual())
	}
}

// A death ends the cast and takes every spell's effect off.
func TestCastDeathClears(t *testing.T) {
	c := newCaster(t)
	c.w.StartCast(c.sess, minorHeal(), c.me, c.at(0))
	c.tick(3400 * time.Millisecond)
	c.w.mu.Lock()
	c.w.playerDiedLocked(nil, c.p, c.at(4*time.Second))
	c.w.mu.Unlock()
	if len(c.p.Effects) != 0 || len(c.p.Buffs) != 0 || c.p.Casting != nil {
		t.Errorf("after death: %+v %+v", c.p.Effects, c.p.Buffs)
	}
}
