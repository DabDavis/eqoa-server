package entity

import (
	"encoding/binary"
	"math"
	"net"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A server with the flag: the duel's fixture (Alpha and Beta a few steps apart, Gamma near,
// each seeing the others), the flag allowed. Gamma's view of A and B is the fixture's; A and
// B see Gamma too.
func newPvP(t *testing.T) *duelists {
	t.Helper()
	d := newDuelists(t)
	d.w.SetPvP(true)
	d.c.X, d.c.Z = 104, 102
	GetOrCreateVisibility(d.sa).ChannelEntity[1], GetOrCreateVisibility(d.sa).ChannelObjectID[1] = d.c.ServerID, 0x00018811
	GetOrCreateVisibility(d.sb).ChannelEntity[1], GetOrCreateVisibility(d.sb).ChannelObjectID[1] = d.c.ServerID, 0x00018812
	return d
}

const (
	aSeesC uint32 = 0x00018811
	bSeesC uint32 = 0x00018812
)

// pvpChat is the plain lines (0x0A7A) among msgs.
func pvpChat(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeClientMessage || len(m) < 6 {
			continue
		}
		n := int(binary.LittleEndian.Uint32(m[2:6]))
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(m[6+2*i:])
		}
		out = append(out, string(utf16.Decode(u)))
	}
	return out
}

func (d *duelists) flag(t *testing.T, s *session.Session, x time.Duration) {
	t.Helper()
	if d.w.PvPCommand(s, d.at(x)) == nil {
		t.Fatalf("/pvp changed nothing: %q", pvpChat(duelDrain(s)))
	}
	duelDrain(s)
}

// attackFor runs attackSystem from x for how long.
func (d *duelists) attackFor(from, to time.Duration) {
	for x := from; x <= to; x += 100 * time.Millisecond {
		d.w.mu.Lock()
		d.w.attackSystem(d.at(x))
		d.w.mu.Unlock()
	}
}

var pvpSwing = Swing{Kind: 1, Delay: 3200 * time.Millisecond, Reach: 4.5}

// With world.pvp off, /pvp says so, no flag goes up, nobody can be swung at and nothing on the
// channel changes: the server is as it was.
func TestPvPOffServerChangesNothing(t *testing.T) {
	d := newDuelists(t)
	d.w.mu.Lock()
	before := d.w.serializeEntityForChannel(d.sb, d.a.ServerID, d.bSeesA)
	d.w.mu.Unlock()
	if d.w.PvPCommand(d.sa, d.at(0)) != nil || d.a.PvPFlagged() {
		t.Fatal("a flag went up with world.pvp off")
	}
	if got := pvpChat(duelDrain(d.sa)); len(got) != 1 || got[0] != pvpOffServer {
		t.Fatalf("told %q", got)
	}
	d.a.pvp.on, d.b.pvp.on = false, false
	if d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(time.Second)) {
		t.Fatal("engaged a player")
	}
	if got := duelDrain(d.sa); len(got) != 0 {
		t.Fatalf("a press on a player said %x, as nothing did before", got)
	}
	if d.w.StartCast(d.sa, strike(), d.aSeesB, d.at(time.Second)) {
		t.Fatal("a strike at a player was cast")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != castFriendly {
		t.Fatalf("the refusal %q, want the old one", got)
	}
	d.w.mu.Lock()
	after := d.w.serializeEntityForChannel(d.sb, d.a.ServerID, d.bSeesA)
	d.w.mu.Unlock()
	if string(before) != string(after) || before[142] != 2 || before[157] != 0 {
		t.Fatalf("the channel changed: byte 142 %d, 157 %d", after[142], after[157])
	}
	if d.w.PlayerFace(d.a) != FaceAlly {
		t.Fatal("the face changed")
	}
}

// /pvp: on, and off only once a minute has passed since it went on and since the last attack given
// or taken, the player told how long is left; a second /pvp at 30 s changes nothing.
func TestPvPFlagTimes(t *testing.T) {
	d := newPvP(t)
	if p := d.w.PvPCommand(d.sa, d.at(0)); p != d.a || !d.a.PvPFlagged() {
		t.Fatalf("/pvp gave %v, flagged %v", p, d.a.PvPFlagged())
	}
	if got := pvpChat(duelDrain(d.sa)); len(got) != 2 || got[0] != pvpLineOn || got[1] != pvpLineOnHow {
		t.Fatalf("told %q", got)
	}
	if d.w.PvPCommand(d.sa, d.at(30*time.Second)) != nil || !d.a.PvPFlagged() {
		t.Fatal("came off at 30 s")
	}
	if got := pvpChat(duelDrain(d.sa)); len(got) != 1 || got[0] != "Your PvP flag stays on for 30 more seconds." {
		t.Fatalf("told %q", got)
	}
	if d.w.PvPCommand(d.sa, d.at(59*time.Second+500*time.Millisecond)) != nil {
		t.Fatal("came off at 59.5 s")
	}
	if got := pvpChat(duelDrain(d.sa)); got[0] != "Your PvP flag stays on for 1 more second." {
		t.Fatalf("told %q", got)
	}
	if p := d.w.PvPCommand(d.sa, d.at(60*time.Second)); p != d.a || d.a.PvPFlagged() {
		t.Fatal("still on at 60 s")
	}
	if got := pvpChat(duelDrain(d.sa)); len(got) != 1 || got[0] != pvpLineOff {
		t.Fatalf("told %q", got)
	}

	// an attack given holds the giver, one taken the taker, each a minute from it
	d.flag(t, d.sa, 100*time.Second)
	d.flag(t, d.sb, 100*time.Second)
	fight := d.w.pvpFightLocked(d.a, d.b)
	d.w.mu.Lock()
	d.w.duelHurtLocked(fight, d.a, d.a.ServerID, d.b, 10, d.at(150*time.Second), numberMelee)
	d.w.mu.Unlock()
	for _, s := range []*session.Session{d.sa, d.sb} {
		duelDrain(s)
		if d.w.PvPCommand(s, d.at(160*time.Second)) != nil {
			t.Fatal("a flag came off at 60 s after it went on, 10 s after an attack")
		}
		if got := pvpChat(duelDrain(s)); len(got) != 1 || got[0] != "Your PvP flag stays on for 50 more seconds." {
			t.Fatalf("told %q", got)
		}
		if d.w.PvPCommand(s, d.at(209*time.Second)) != nil {
			t.Fatal("a flag came off 59 s after the attack")
		}
		duelDrain(s)
	}
	if d.w.PvPCommand(d.sa, d.at(210*time.Second)) != d.a || d.w.PvPCommand(d.sb, d.at(210*time.Second)) != d.b {
		t.Fatal("a flag stayed on 60 s after the attack")
	}
}

// Melee: only two flagged players, either way round; the refused are told why, every 5 s at most.
func TestPvPMeleeNeedsBothFlags(t *testing.T) {
	d := newPvP(t)
	if d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(0)) || d.a.AttackTarget != 0 {
		t.Fatal("unflagged at unflagged engaged")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || !strings.Contains(got[0], "You are not flagged") {
		t.Fatalf("told %q", got)
	}
	d.flag(t, d.sb, time.Second)
	if d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(2*time.Second)) {
		t.Fatal("unflagged at flagged engaged")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 0 {
		t.Fatalf("told again within 5 s: %q", got)
	}
	if d.w.EngageAttack(d.sb, d.bSeesA, pvpSwing, d.at(7*time.Second)) {
		t.Fatal("flagged at unflagged engaged")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Alpha is not flagged for PvP." {
		t.Fatalf("told %q", got)
	}
	d.flag(t, d.sa, 8*time.Second)
	if !d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(9*time.Second)) || d.a.AttackTarget != d.b.ServerID {
		t.Fatal("flagged at flagged not engaged")
	}
}

// A flagged player's rounds on a foe land as a duel's: each hit half the roll on its level, told to
// both, nothing to any creature; the flags held. Taking one's flag off (a minute on) ends the fight
// at the next round.
func TestPvPMeleeLandsAtTheDuelShare(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.a.Gear.WeaponDamage, d.a.Gear.WeaponCond = 40, 1
	if !d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(time.Second)) {
		t.Fatal("not engaged")
	}
	d.a.AutoAttack = true
	floor, low, high := playerMeleeRoll(d.a, duelStandIn(d.b.Level))
	lo, hi := duelHalf(max(floor, low)), duelHalf(max(floor, high))
	before := d.b.HP
	d.attackFor(time.Second, 8*time.Second)
	hits := ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication)
	told := ofOp(duelDrain(d.sb), protocol.OpcodeDamageIndication)
	if len(hits) < 2 || len(told) != len(hits) {
		t.Fatalf("%d hits told to the attacker, %d to the target", len(hits), len(told))
	}
	taken := before - d.b.HP
	if taken < len(hits)*lo || taken > len(hits)*hi {
		t.Fatalf("%d hits took %d, want each within the halved roll %d..%d", len(hits), taken, lo, hi)
	}
	if d.a.TotalXP != 0 || d.a.pvp.hold.Before(d.at(time.Second+PvPHoldTime)) || d.b.pvp.hold.Before(d.at(time.Second+PvPHoldTime)) {
		t.Fatalf("XP %d, holds %v %v", d.a.TotalXP, d.a.pvp.hold, d.b.pvp.hold)
	}
	if d.w.PvPCommand(d.sb, d.at(9*time.Second)) != nil {
		t.Fatal("came off in a fight")
	}
	d.b.pvp.hold = d.at(9 * time.Second)
	if d.w.PvPCommand(d.sb, d.at(10*time.Second)) != d.b {
		t.Fatal("did not come off")
	}
	d.attackFor(10*time.Second, 11*time.Second)
	if d.a.AttackTarget != 0 {
		t.Fatal("still engaged on one with no flag")
	}
}

// Spells: a harmful single-target spell lands on a foe at the duel's share and a damage over time
// halved each tick, held, never kept over a logout, off when the flag comes off; refused saying why
// at a player who is no foe; an area spell refused at a foe, spared by its splash.
func TestPvPSpells(t *testing.T) {
	d := newPvP(t)
	if d.w.StartCast(d.sb, strike(), d.bSeesA, d.at(0)) {
		t.Fatal("a strike at a player with no flags was cast")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || !strings.Contains(got[0], "You are not flagged") {
		t.Fatalf("the refusal %q", got)
	}
	d.flag(t, d.sb, 0)
	if d.w.StartCast(d.sb, strike(), d.bSeesA, d.at(time.Second)) {
		t.Fatal("a strike at an unflagged player was cast")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Alpha is not flagged for PvP." {
		t.Fatalf("the refusal %q", got)
	}
	d.flag(t, d.sa, time.Second)
	before := d.a.HP
	sp := strike()
	sp.SpellID = 651
	if !d.w.StartCast(d.sb, sp, d.bSeesA, d.at(2*time.Second)) {
		t.Fatalf("a strike at a foe was refused: %q", duelLines(duelDrain(d.sb)))
	}
	if !d.b.pvp.hold.Equal(d.at(2*time.Second + PvPHoldTime)) {
		t.Fatalf("the cast did not hold its caster: %v", d.b.pvp.hold)
	}
	land := d.at(2*time.Second + sp.CastTime + castLand)
	d.w.mu.Lock()
	d.w.castSystem(land)
	d.w.mu.Unlock()
	if taken := before - d.a.HP; taken != 17 {
		t.Fatalf("the strike took %d, want 33 halved, 17", taken)
	}
	if !d.a.pvp.hold.Equal(land.Add(PvPHoldTime)) {
		t.Fatalf("the landing did not hold its target: %v", d.a.pvp.hold)
	}
	if len(ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication)) != 1 || len(ofOp(duelDrain(d.sb), protocol.OpcodeDamageIndication)) != 1 {
		t.Fatal("not told to both")
	}

	dot := CastSpell{SpellID: 700, DisplayIndex: 4, Scope: 1, Name: "Searing", Icon: 0x22222222, CastTime: time.Second,
		Recast: time.Second, Range: 30, FX: 0xE78024D9, Def: ParseSpellText("Target, Decrease HP per tic by 20, 18sec")}
	land = d.at(5 * time.Second)
	if !d.w.StartCast(d.sb, dot, d.bSeesA, land.Add(-dot.CastTime-castLand)) {
		t.Fatalf("the damage over time was refused: %q", duelLines(duelDrain(d.sb)))
	}
	before = d.a.HP
	d.w.mu.Lock()
	d.w.castSystem(land)
	d.w.mu.Unlock()
	if len(d.a.Effects) != 1 || d.a.Effects[0].duelFoe != d.b.ServerID {
		t.Fatalf("the effects on A %+v", d.a.Effects)
	}
	if taken := before - d.a.HP; taken != 10 {
		t.Fatalf("the first tick took %d, want 20 halved", taken)
	}
	if rows := savedEffects(d.a, land); len(rows) != 0 {
		t.Fatalf("kept over a logout: %+v", rows)
	}
	// a tick holds both from it
	at := land.Add(effectTick)
	d.w.mu.Lock()
	d.w.tickEffectLocked(d.a, d.a.Effects[0], at)
	d.w.mu.Unlock()
	if !d.a.pvp.hold.Equal(at.Add(PvPHoldTime)) || !d.b.pvp.hold.Equal(at.Add(PvPHoldTime)) {
		t.Fatalf("a tick held %v and %v, want %v", d.a.pvp.hold, d.b.pvp.hold, at.Add(PvPHoldTime))
	}
	// the flag off: what foes put on it goes
	d.a.pvp.hold = at
	if d.w.PvPCommand(d.sa, at.Add(time.Second)) != d.a || len(d.a.Effects) != 0 {
		t.Fatalf("flag off: effects %+v", d.a.Effects)
	}

	// an area spell at a foe lands on it at the PvP share (the user, 2026-11-08: "neo cant use aoe
	// in pvp, allow aoes to only hit players in pvp mode when pvp"); Gamma beside it, not flagged,
	// is untouched
	d.flag(t, d.sa, 10*time.Minute)
	storm := pvpStorm(t)
	duelDrain(d.sb)
	cast := d.at(10 * time.Minute)
	beforeA, beforeC := d.a.HP, d.c.HP
	if !d.w.StartCast(d.sb, storm, d.bSeesA, cast) {
		t.Fatalf("an area spell at a foe was refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(cast.Add(storm.CastTime + castLand))
	d.w.mu.Unlock()
	if taken := beforeA - d.a.HP; taken != 105 || d.c.HP != beforeC {
		t.Fatalf("the storm at A took %d (want 210 halved), Gamma's HP %d (was %d)", taken, d.c.HP, beforeC)
	}
}

// pvpStorm is an area spell: 210 on its target, 90 on the others round it.
func pvpStorm(t *testing.T) CastSpell {
	t.Helper()
	const line = "Target, Decrease Hitpoints by 210 aoe Decrease Hitpoints by 90"
	storm := CastSpell{SpellID: 701, DisplayIndex: 5, Scope: 1, Name: "Storm", Range: 30, Def: ParseSpellText(line)}
	storm.Area = ReadSpellArea(storm.Name, line, storm.Def)
	if storm.Area.Centre == AreaNone {
		t.Fatalf("the test's storm reads as no area: %+v", storm.Area)
	}
	return storm
}

// An area spell's splash reaches a flagged caster's foes at the PvP share, and no other player (the
// user, 2026-11-08: "neo cant use aoe in pvp, allow aoes to only hit players in pvp mode when pvp"):
// Beta's storm at a creature beside Alpha hits the creature and Alpha, its foe, for the splash's 90
// halved; Gamma beside them, not flagged, takes nothing. The controls: grouped with Beta, Alpha is no
// foe and is spared; with Beta not flagged, its splash spares Alpha.
func TestPvPAreaSplashesFoesAlone(t *testing.T) {
	run := func(flagB, grouped bool) (alpha, gamma, rat int) {
		d := newPvP(t)
		d.flag(t, d.sa, 0)
		if flagB {
			d.flag(t, d.sb, 0)
		}
		if grouped {
			d.w.groups = map[uint32]*Group{9: {ID: 9, Members: []EntityID{d.a.ServerID, d.b.ServerID}}}
			d.a.GroupID, d.b.GroupID = 9, 9
		}
		npc := &NPC{ServerID: 2002, Name: "rat", Level: 1, HP: 5000, MaxHP: 5000, X: 101, Y: 10, Z: 100}
		d.w.NPCs[npc.ServerID] = npc
		d.w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
		vs := GetOrCreateVisibility(d.sb)
		vs.ChannelEntity[2], vs.ChannelObjectID[2] = npc.ServerID, 0x00018822
		storm := pvpStorm(t)
		a0, c0 := d.a.HP, d.c.HP
		cast := d.at(2 * time.Minute)
		if !d.w.StartCast(d.sb, storm, 0x00018822, cast) {
			t.Fatalf("the storm at the rat was refused: %q", duelLines(duelDrain(d.sb)))
		}
		d.w.mu.Lock()
		d.w.castSystem(cast.Add(storm.CastTime + castLand))
		d.w.mu.Unlock()
		return a0 - d.a.HP, c0 - d.c.HP, 5000 - npc.HP
	}
	if a, c, r := run(true, false); a != 45 || c != 0 || r <= 0 {
		t.Errorf("foes: Alpha took %d (want 90 halved), Gamma %d, the rat %d", a, c, r)
	}
	if a, c, _ := run(true, true); a != 0 || c != 0 {
		t.Errorf("grouped: Alpha took %d, Gamma %d; want nothing", a, c)
	}
	if a, c, _ := run(false, false); a != 0 || c != 0 {
		t.Errorf("Beta not flagged: Alpha took %d, Gamma %d; want nothing", a, c)
	}
}

// A hit the victim cannot take is a death, at no cost: no debt, no experience, its flag staying;
// the killer lets go; it returns by the respawn's way, and the two can fight again.
func TestPvPKill(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.b.Level, d.b.TotalXP = 32, 12345
	d.b.Debt = 7
	d.a.AttackTarget, d.a.AttackObjectID = d.b.ServerID, d.aSeesB
	d.b.HP = 5
	now := d.at(time.Second)
	d.w.mu.Lock()
	over := d.w.duelHurtLocked(d.w.pvpFightLocked(d.a, d.b), d.a, d.a.ServerID, d.b, 500, now, numberMelee)
	d.w.mu.Unlock()
	if !over || !d.b.IsDead() || !d.b.respawnPending() {
		t.Fatalf("a killing hit: over %v, HP %d, respawn %v", over, d.b.HP, d.b.respawnPending())
	}
	if d.b.Debt != 7 || d.b.TotalXP != 12345 || d.a.TotalXP != 0 {
		t.Fatalf("debt %d, the victim's XP %d, the killer's XP %d", d.b.Debt, d.b.TotalXP, d.a.TotalXP)
	}
	if d.a.AttackTarget != 0 || !d.b.PvPFlagged() {
		t.Fatalf("the killer's target %d; the victim's flag %v", d.a.AttackTarget, d.b.PvPFlagged())
	}
	gotB := duelDrain(d.sb)
	if got := duelLines(gotB); len(got) != 1 || got[0] != "Alpha has slain you." {
		t.Fatalf("the victim was told %q", got)
	}
	if len(ofOp(gotB, protocol.OpcodeCharacterDied)) != 1 {
		t.Fatalf("no 0x005E: %x", gotB)
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "You have slain Beta." {
		t.Fatalf("the killer was told %q", got)
	}
	// dead it is no foe: a player on the channel, and no fight
	d.w.mu.Lock()
	if b := d.w.serializeEntityForChannel(d.sa, d.b.ServerID, d.aSeesB)[142]; b != 2 {
		t.Errorf("the dead foe's byte 142 is %d", b)
	}
	if d.w.pvpFightLocked(d.a, d.b) != nil {
		t.Error("a fight with the dead")
	}
	due := d.w.playerRespawnSystem(now.Add(PlayerRespawnDelay))
	d.w.mu.Unlock()
	if len(due) != 1 || d.b.HP != d.b.MaxHP || d.b.Debt != 7 {
		t.Fatalf("respawn: %d due, HP %d, debt %d", len(due), d.b.HP, d.b.Debt)
	}
	if !d.a.pvpFoe(d.b) {
		t.Fatal("back, still flagged: no foe")
	}
}

// A group's flagged members can't hit each other, by melee or spell, and stay players to each
// other; a third flagged player can hit both and be hit. Leaving the group makes them foes.
func TestPvPGroupAndAThird(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.flag(t, d.sc, 0)
	d.a.GroupID, d.b.GroupID = 7, 7
	if d.w.EngageAttack(d.sa, d.aSeesB, pvpSwing, d.at(time.Second)) {
		t.Fatal("a group member engaged another")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "Beta is in your group." {
		t.Fatalf("told %q", got)
	}
	if d.w.StartCast(d.sa, strike(), d.aSeesB, d.at(time.Second)) {
		t.Fatal("a strike at a group member was cast")
	}
	duelDrain(d.sa)
	d.w.mu.Lock()
	view := func(s *session.Session, p *Player, obj uint32) []byte {
		return d.w.serializeEntityForChannel(s, p.ServerID, obj)
	}
	if b := view(d.sa, d.b, d.aSeesB); b[142] != 2 || b[157] != 1 {
		t.Errorf("a group member: byte 142 %d, 157 %d", b[142], b[157])
	}
	if b := view(d.sc, d.a, d.cSeesA); b[142] != pvpFoeByte {
		t.Errorf("the third sees A with %d", b[142])
	}
	if b := view(d.sa, d.c, aSeesC); b[142] != pvpFoeByte {
		t.Errorf("A sees the third with %d", b[142])
	}
	d.w.mu.Unlock()
	for _, c := range []struct {
		s   *session.Session
		obj uint32
	}{{d.sc, d.cSeesA}, {d.sc, d.cSeesB}, {d.sa, aSeesC}, {d.sb, bSeesC}} {
		if !d.w.EngageAttack(c.s, c.obj, pvpSwing, d.at(2*time.Second)) {
			t.Fatalf("a third and a member did not engage (%x)", c.obj)
		}
	}
	d.a.GroupID, d.b.GroupID = 0, 0
	if !d.a.pvpFoe(d.b) {
		t.Fatal("out of the group, no foes")
	}
}

// Who sees what, on the channel: byte 157 green to everyone, the player's own channel too; byte 142
// the foe's 4 only to a flagged viewer who may hit it. Both change at the next serialize, in the disc's
// look and the live's, the moment a flag does.
func TestPvPViews(t *testing.T) {
	d := newPvP(t)
	bytes := func(s *session.Session, p *Player, obj uint32) (byte, byte) {
		d.w.mu.Lock()
		defer d.w.mu.Unlock()
		b := d.w.serializeEntityForChannel(s, p.ServerID, obj)
		return b[142], b[157]
	}
	want := func(tag string, s *session.Session, p *Player, obj uint32, b142, b157 byte) {
		t.Helper()
		if got, got2 := bytes(s, p, obj); got != b142 || got2 != b157 {
			t.Errorf("%s: bytes 142, 157: %d, %d, want %d, %d", tag, got, got2, b142, b157)
		}
	}
	want("nobody flagged", d.sb, d.a, d.bSeesA, 2, 0)
	d.flag(t, d.sa, 0)
	want("flagged, an unflagged viewer", d.sb, d.a, d.bSeesA, 2, 1)
	want("an unflagged one, a flagged viewer", d.sa, d.b, d.aSeesB, 2, 0)
	if own := SerializePlayerUpdate(1, d.a); own[142] != 2 || own[157] != 1 {
		t.Errorf("its own channel: %d, %d", own[142], own[157])
	}
	d.flag(t, d.sb, 0)
	want("a foe", d.sb, d.a, d.bSeesA, pvpFoeByte, 1)
	want("a foe", d.sa, d.b, d.aSeesB, pvpFoeByte, 1)
	want("flagged, the unflagged third", d.sc, d.a, d.cSeesA, 2, 1)
	if d.w.PvPCommand(d.sa, d.at(2*time.Minute)) != d.a {
		t.Fatal("did not come off")
	}
	duelDrain(d.sa)
	want("flag off", d.sb, d.a, d.bSeesA, 2, 0)
	want("a flagged one, its flag off", d.sa, d.b, d.aSeesB, 2, 1)
	if d.w.PlayerFace(d.a) != FaceAlly || d.w.PlayerFace(d.b) != FaceHostile {
		t.Errorf("the faces: %d, %d", d.w.PlayerFace(d.a), d.w.PlayerFace(d.b))
	}

	// the live look carries both at its own places (150-200 are the disc's 111-161)
	d.flag(t, d.sa, 3*time.Minute)
	d.sb.SetLiveLook(true)
	d.w.mu.Lock()
	b := d.w.serializeEntityForChannel(d.sb, d.a.ServerID, d.bSeesA)
	d.w.mu.Unlock()
	if len(b) != LiveEntityStateSize || b[142+39] != pvpFoeByte || b[157+39] != 1 {
		t.Errorf("the live look: %d bytes, %d, %d", len(b), b[142+39], b[157+39])
	}
}

// TargetersOf is everyone with that player as its target, itself too, by the id each knows it by.
func TestPvPTargetersOf(t *testing.T) {
	d := newPvP(t)
	d.b.TargetObjectID, d.b.targetToken = d.bSeesA, 0
	d.c.TargetObjectID, d.c.targetToken = d.cSeesB, 9
	d.a.TargetObjectID = d.sa.PlayerObjectID()
	got := d.w.TargetersOf(d.a)
	if len(got) != 2 {
		t.Fatalf("%d targeting A: %+v", len(got), got)
	}
	seen := map[*Player]uint32{}
	for _, v := range got {
		seen[v.Viewer] = v.ObjectID
	}
	if seen[d.b] != d.bSeesA || seen[d.a] != d.sa.PlayerObjectID() {
		t.Fatalf("%+v", seen)
	}
	got = d.w.TargetersOf(d.b)
	if len(got) != 1 || got[0].Viewer != d.c || got[0].Token != 9 {
		t.Fatalf("targeting B: %+v", got)
	}
}

// A keep restart carries the flag and what is left of its time; a server since turned off drops it.
func TestPvPKeep(t *testing.T) {
	d := newPvP(t)
	now := time.Now()
	d.a.pvp = pvpFlag{on: true, hold: now.Add(42 * time.Second)}
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.PvP || k.PvPHoldMS != 42000 {
		t.Fatalf("kept %+v", k)
	}
	if k2, _ := d.w.KeepPlayer(d.sb, now); k2.PvP || k2.PvPHoldMS != 0 {
		t.Fatalf("the unflagged kept %+v", k2)
	}
	d.a.pvp = pvpFlag{}
	later := now.Add(5 * time.Second)
	if !d.w.RestoreKeptPlayer(d.sa, k, later) {
		t.Fatal("not restored")
	}
	if !d.a.PvPFlagged() || !d.a.pvp.hold.Equal(later.Add(42*time.Second)) {
		t.Fatalf("restored %+v", d.a.pvp)
	}
	d.w.SetPvP(false)
	d.a.pvp = pvpFlag{}
	d.w.RestoreKeptPlayer(d.sa, k, later)
	if d.a.PvPFlagged() {
		t.Fatal("restored on a server with it off")
	}
}

// The flag goes on through a takeover, the same Player, and the new client is told; a hold spent
// there holds as ever.
func TestPvPTakeover(t *testing.T) {
	d := newPvP(t)
	d.a.CharServerID = 77
	d.flag(t, d.sa, 0)
	hold := d.a.pvp.hold
	s2 := d.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 20), Port: 7020}, 20, 20, 1, 2, true)
	s2.SetCharacterInWorld(true)
	s2.SetPlayerObjectID(0x1424A1E0)
	p, _ := d.w.TakeOverPlayer(s2, 77)
	if p != d.a || !p.PvPFlagged() || !p.pvp.hold.Equal(hold) {
		t.Fatalf("taken over: %v, flag %+v", p == d.a, p.pvp)
	}
	duelDrain(s2)
	d.w.SendTakenOver(s2, d.at(time.Second))
	if got := pvpChat(duelDrain(s2)); len(got) != 1 || got[0] != pvpLineStill {
		t.Fatalf("told %q", got)
	}
}

// A logout ends the flag (a new login is a new Player) and takes off what it put on foes; a foe's
// melee on it is let go.
func TestPvPLogout(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.a.AttackTarget, d.a.AttackObjectID = d.b.ServerID, d.aSeesB
	d.a.Effects = append(d.a.Effects, &ActiveEffect{SpellID: 900, Caster: d.b.ServerID, duelFoe: d.b.ServerID, HPTick: -5, Until: d.at(time.Hour), NextTick: d.at(0)})
	d.w.mu.Lock()
	d.w.leavingLocked(d.b)
	delete(d.w.Players, d.b.ServerID)
	d.w.mu.Unlock()
	if len(d.a.Effects) != 0 {
		t.Fatalf("what the one gone put on it: %+v", d.a.Effects)
	}
	d.attackFor(time.Second, time.Second)
	if d.a.AttackTarget != 0 {
		t.Fatal("still engaged on one gone")
	}
}

// A pet: told to, or by its stance (aggressive what its owner swings at, defensive what struck its
// owner), it goes for a foe and every hit is its owner's PvP hit; passive it waits; never on one who
// is no foe. It stays a pet to the foe (byte 142 3), and a shield's answer leaves it at a hit point,
// yielding.
func TestPvPPet(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	pet := &NPC{ServerID: 4004, Name: "Pet", Level: 30, HP: 400, MaxHP: 400, Owner: d.a.ServerID, X: 101, Y: 10, Z: 101}
	d.w.NPCs[pet.ServerID] = pet
	if d.w.pets == nil {
		d.w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: d.a, npc: pet, stance: petDefensive, last: d.at(time.Second)}
	d.w.pets[d.a.ServerID] = ps
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	if b := d.w.serializeEntityForChannel(d.sb, pet.ServerID, 0x00018877)[142]; b != 3 {
		t.Errorf("the foe sees the pet with byte 142 = %d, want a pet's 3", b)
	}
	if d.w.pvpPetLocked(ps, 0.1, d.at(2*time.Second)) || ps.pvpTarget != 0 {
		t.Fatal("a defensive pet went before its owner was struck")
	}
	d.w.duelHurtLocked(d.w.pvpFightLocked(d.b, d.a), d.b, d.b.ServerID, d.a, 10, d.at(2*time.Second), numberMelee)
	before := d.b.HP
	for x := 2 * time.Second; x < 6*time.Second; x += 100 * time.Millisecond {
		if !d.w.pvpPetLocked(ps, 0.1, d.at(x)) {
			t.Fatalf("a defensive pet did not fight at %v", x)
		}
	}
	if ps.pvpTarget != d.b.ServerID || d.b.HP >= before {
		t.Fatalf("its target %d; B's HP %d of %d", ps.pvpTarget, d.b.HP, before)
	}
	if !d.a.pvp.hold.After(d.at(6 * time.Second)) {
		t.Fatal("a pet's hit did not hold its owner")
	}
	// a back off lets go, for its hold
	ps.pvpTarget, ps.heldUntil = 0, d.at(7*time.Minute)
	if d.w.pvpPetLocked(ps, 0.1, d.at(7*time.Second)) {
		t.Fatal("it fought through a back off")
	}
	ps.heldUntil = time.Time{}
	// passive: only told to
	ps.stance = petPassive
	if d.w.pvpPetLocked(ps, 0.1, d.at(8*time.Second)) {
		t.Fatal("a passive pet went for a foe")
	}
	if !d.w.pvpPetCommandLocked(ps, d.b) || !d.w.pvpPetLocked(ps, 0.1, d.at(8*time.Second)) {
		t.Fatal("told to, it did not go")
	}
	if d.w.pvpPetCommandLocked(ps, d.c) {
		t.Fatal("told to go for an unflagged player")
	}
	// a shield's answer: a hit point, and it yields
	fight := d.w.pvpFightLocked(d.b, d.a)
	d.w.duelHurtPetLocked(fight, d.b, pet, 5000, d.at(9*time.Second), numberMelee)
	if pet.HP != 1 || pet.Dead || ps.pvpTarget != 0 || !ps.heldUntil.Equal(d.at(9*time.Second+pvpPetYields)) {
		t.Fatalf("HP %d, dead %v, target %d, held %v", pet.HP, pet.Dead, ps.pvpTarget, ps.heldUntil)
	}
}

// A duel between two flagged players stays a duel: nobody dies of it. A third flagged player's hit on
// a flagged duelist is a PvP hit, and a death there ends the duel as a creature's would.
func TestPvPAndADuel(t *testing.T) {
	d := newPvP(t)
	duel := d.start(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.flag(t, d.sc, 0)
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	if f := d.w.fightBetweenLocked(d.a, d.b); f != duel {
		t.Fatalf("their fight %+v, want their duel", f)
	}
	if f := d.w.fightBetweenLocked(d.c, d.a); f == nil || !f.pvp {
		t.Fatalf("the third's fight %+v", f)
	}
	d.b.HP = 3
	if !d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 5000, d.at(time.Second), numberMelee) || d.b.HP != 1 || d.b.IsDead() {
		t.Fatalf("a duel's last hit left %d", d.b.HP)
	}
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("the duel did not end")
	}
	// again, and the third kills one of it
	d.w.duels.add(duel)
	duel.Phase = duelOn
	d.b.HP = 3
	d.w.duelHurtLocked(d.w.pvpFightLocked(d.c, d.b), d.c, d.c.ServerID, d.b, 5000, d.at(2*time.Second), numberMelee)
	if !d.b.IsDead() {
		t.Fatalf("a third's PvP hit left %d", d.b.HP)
	}
	d.w.duelSystem(d.at(3 * time.Second))
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("a duelist dead: the duel goes on")
	}
}

// A flagged player killed falls for everyone (byte 58 CLIENT's dying state, seen only while it lies
// dead), and at its return every client that had it on a slot makes it anew under a new id; an
// unflagged player's death is as it was.
func TestPvPDeathIsSeen(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	view := func(s *session.Session, p *Player, obj uint32) []byte {
		return d.w.serializeEntityForChannel(s, p.ServerID, obj)
	}
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	if b := view(d.sa, d.b, d.aSeesB); b[58] == pvpDyingState {
		t.Fatalf("alive, byte 58 is %d", b[58])
	}
	d.b.HP = 3
	now := d.at(time.Second)
	d.w.duelHurtLocked(d.w.pvpFightLocked(d.a, d.b), d.a, d.a.ServerID, d.b, 5000, now, numberMelee)
	if !d.b.IsDead() {
		t.Fatal("not dead")
	}
	for _, c := range []struct {
		s   *session.Session
		obj uint32
	}{{d.sa, d.aSeesB}, {d.sc, d.cSeesB}} {
		if b := view(c.s, d.b, c.obj); b[58] != pvpDyingState || b[142] != 2 {
			t.Errorf("dead: bytes 58, 142 %d, %d", b[58], b[142])
		}
	}
	// the control: a player with no flag, dead, is as any dead player was
	d.c.HP, d.c.DiedAt = 0, now
	if b := view(d.sa, d.c, aSeesC); b[58] == pvpDyingState {
		t.Errorf("an unflagged player's death changed byte 58: %d", b[58])
	}
	d.c.HP, d.c.DiedAt = 100, time.Time{}

	due := d.w.playerRespawnSystem(now.Add(PlayerRespawnDelay))
	if len(due) != 1 || d.b.HP != d.b.MaxHP {
		t.Fatalf("respawn: %d due, HP %d", len(due), d.b.HP)
	}
	for _, s := range []*session.Session{d.sa, d.sc} {
		for i, id := range GetOrCreateVisibility(s).ChannelEntity {
			if id == d.b.ServerID {
				t.Errorf("slot %d still holds it after its return", i)
			}
		}
	}
	// and only its slots: the other it sees stays
	if GetOrCreateVisibility(d.sc).ChannelEntity[0] != d.a.ServerID {
		t.Error("another's slot was taken")
	}
}

// An alchemist's thrown potion at a PvP foe lands at the PvP share (petition 1248, the GM Jibrish:
// "Alchemist potions are only hitting for one on pvp characters, they should be hitting for 50% of
// their full damage like everything else"): thrown from the pack, its 200 lands as 100; worn and
// thrown in a round, the same, one potion used. The control: at a player not flagged, refused.
func TestPvPThrownPotionsLandAtTheShare(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sb, 0)
	flask := NewPotionEffect(9301, "Volatile Flask", "Target, Decrease Hitpoints by 200", "Volatile Flask", 0, "", 0, 0)
	if !flask.Thrown() {
		t.Fatalf("the flask is not thrown: %+v", flask.Def)
	}
	d.w.NotePotionEffect(flask)
	now := d.at(time.Minute)
	if why, err := d.w.UsePotion(d.sb, flask, d.bSeesA, now); err != nil || why == "" {
		t.Fatalf("the control, A not flagged: %q %v", why, err)
	}
	d.flag(t, d.sa, time.Minute)
	before := d.a.HP
	if why, err := d.w.UsePotion(d.sb, flask, d.bSeesA, d.at(2*time.Minute)); err != nil || why != "" {
		t.Fatalf("a throw at a foe: %q %v", why, err)
	}
	if taken := before - d.a.HP; taken != 100 {
		t.Errorf("a throw at a foe took %d, want 200 halved", taken)
	}
	d.b.Swing.Potion, d.b.Swing.PotionLeft = 9301, 2
	before = d.a.HP
	d.w.mu.Lock()
	d.w.thrownPotionAtFoeLocked(d.b, d.a, d.at(3*time.Minute))
	d.w.mu.Unlock()
	if taken := before - d.a.HP; taken != 100 || d.b.Swing.PotionLeft != 1 {
		t.Errorf("a worn one thrown in a round took %d (want 100), %d left (want 1)", taken, d.b.Swing.PotionLeft)
	}
}

// The user, 2026-11-09: "ranger bow only hitting for 90 in pvp mode on". A
// bow's swing on a foe is its shot as on a creature (playerShotHit: two
// arrows from Range4, each rangedShare times a roll), then the PvP share;
// it was one melee roll on the bow, halved. The control is
// TestPvPMeleeLandsAtTheDuelShare: a melee swing's hit as it was.
func TestPvPBowShootsAsOnACreature(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.a.Gear.WeaponDamage, d.a.Gear.WeaponCond = 400, 1
	bow := Swing{Kind: swingRange4, Delay: 3200 * time.Millisecond, Reach: 30}
	if !d.w.EngageAttack(d.sa, d.aSeesB, bow, d.at(time.Second)) {
		t.Fatal("not engaged")
	}
	d.a.AutoAttack = true
	floor, low, high := playerMeleeRoll(d.a, duelStandIn(d.b.Level))
	arrow := func(roll int) int { return int(math.Round(float64(max(floor, roll)) * rangedShare)) }
	lo, hi := duelHalf(2*arrow(low)), duelHalf(2*arrow(high))
	d.b.HP, d.b.MaxHP = 1_000_000, 1_000_000
	before := d.b.HP
	d.attackFor(time.Second, 8*time.Second)
	hits := ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication)
	if len(hits) < 2 {
		t.Fatalf("%d shots told", len(hits))
	}
	taken := before - d.b.HP
	if taken < len(hits)*lo || taken > len(hits)*hi {
		t.Fatalf("%d shots took %d, want each within the halved two-arrow shot %d..%d (a melee roll halved: %d..%d)",
			len(hits), taken, lo, hi, duelHalf(max(floor, low)), duelHalf(max(floor, high)))
	}
}
