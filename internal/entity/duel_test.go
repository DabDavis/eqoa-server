package entity

import (
	"bytes"
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

// Two players a few steps apart, each seeing the other on its first entity
// channel, and a bystander who sees both.
type duelists struct {
	w          *World
	sa, sb, sc *session.Session
	a, b, c    *Player
	aSeesB     uint32 // B's object id in A's session
	bSeesA     uint32
	cSeesA     uint32
	cSeesB     uint32
	t0         time.Time
}

func newDuelists(t *testing.T) *duelists {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	mk := func(i byte, p *Player, obj uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, i), Port: 7000 + int(i)}, uint32(i), uint32(i), 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(obj)
		return s
	}
	a := &Player{ServerID: 500000001, Name: "Alpha", Race: 6, Class: 0, Level: 30, HP: 1500, MaxHP: 1500, Power: 300, MaxPower: 300,
		Str: 120, Sta: 110, Agi: 90, Dex: 95, Wis: 60, Int: 60, Cha: 60, Speed: 8.833333, X: 100, Y: 10, Z: 100, GuildName: "Blades"}
	b := &Player{ServerID: 500000002, Name: "Beta", Race: 0, Class: 13, Level: 32, HP: 900, MaxHP: 900, Power: 1200, MaxPower: 1200,
		Str: 60, Sta: 80, Agi: 80, Dex: 90, Wis: 60, Int: 140, Cha: 60, Speed: 8.833333, X: 104, Y: 10, Z: 100}
	c := &Player{ServerID: 500000003, Name: "Gamma", Level: 10, HP: 200, MaxHP: 200, Speed: 8.833333, X: 120, Y: 10, Z: 110}
	d := &duelists{w: w, a: a, b: b, c: c, t0: time.Unix(1_800_000_000, 0),
		aSeesB: 0x00018801, bSeesA: 0x00018802, cSeesA: 0x00018803, cSeesB: 0x00018804}
	d.sa = mk(8, a, 0x1424A1D0)
	d.sb = mk(9, b, 0x1424A1D1)
	d.sc = mk(10, c, 0x1424A1D2)
	see := func(s *session.Session, ids ...any) {
		vs := GetOrCreateVisibility(s)
		for i := 0; i < len(ids); i += 2 {
			vs.ChannelEntity[i/2] = ids[i].(EntityID)
			vs.ChannelObjectID[i/2] = ids[i+1].(uint32)
		}
	}
	see(d.sa, b.ServerID, d.aSeesB)
	see(d.sb, a.ServerID, d.bSeesA)
	see(d.sc, a.ServerID, d.cSeesA, b.ServerID, d.cSeesB)
	return d
}

func (d *duelists) at(x time.Duration) time.Time { return d.t0.Add(x) }

// drain is every message queued for s since the last drain, opcode first.
func duelDrain(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// lines is the text of the coloured lines (0x0A7B) among msgs.
func duelLines(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeColoredChat || len(m) < 6 {
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

func ofOp(msgs [][]byte, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range msgs {
		if opOfMsg(m) == op {
			out = append(out, m)
		}
	}
	return out
}

func (d *duelists) tick(x time.Duration) {
	d.w.mu.Lock()
	d.w.duelSystem(d.at(x))
	d.w.mu.Unlock()
	d.w.runDuelPurse() // as Tick does after its lock
}

// start has Alpha challenge Beta (at its target) and Beta accept by the box.
func (d *duelists) start(t *testing.T) *Duel {
	t.Helper()
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	if duel == nil {
		t.Fatalf("no invitation: A was told %q", duelLines(duelDrain(d.sa)))
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(time.Second))
	if duel.Phase != duelOn {
		t.Fatalf("accepted, phase %d; B was told %q", duel.Phase, duelLines(duelDrain(d.sb)))
	}
	duelDrain(d.sa)
	duelDrain(d.sb)
	duelDrain(d.sc)
	return duel
}

// A challenge at the target: the challenger told, the challenged a line and
// the box (0x0034, the duel's box id, the live words, Accept and Decline);
// Decline tells the challenger the live word with the name and ends it.
func TestDuelChallengeAndDecline(t *testing.T) {
	d := newDuelists(t)
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "You challenge Beta to a duel." {
		t.Fatalf("the challenger was told %q", got)
	}
	duel := d.w.duels.of[d.b.ServerID]
	if duel == nil || duel.A != d.a.ServerID || duel.Phase != duelAsked {
		t.Fatalf("the invitation: %+v", duel)
	}
	got := duelDrain(d.sb)
	if l := duelLines(got); len(l) != 1 || l[0] != "Alpha challenges you to a duel." {
		t.Fatalf("the challenged was told %q", l)
	}
	boxes := ofOp(got, protocol.OpcodeQuestPopupMulti)
	if want := BuildDuelBox(DuelBoxID(duel.ID), "Alpha challenges you to a duel.", []string{"Accept", "Decline"}); len(boxes) != 1 || !bytes.Equal(boxes[0], want) {
		t.Fatalf("the box % X, want % X", boxes, want)
	}
	if !IsDuelBox(DuelBoxID(duel.ID)) || IsDuelBox(0x51000001) || IsDuelBox(0x52000001) || IsDuelBox(0) {
		t.Fatal("a duel box is told from the quests', the triggers' and an NPC's")
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 255, d.at(time.Second)) // a box closed: no answer
	if d.w.duels.of[d.b.ServerID] == nil {
		t.Fatal("a closed box declined it")
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 1, d.at(2*time.Second))
	if d.w.duels.of[d.a.ServerID] != nil || d.w.duels.of[d.b.ServerID] != nil || len(d.w.duels.byID) != 0 {
		t.Fatal("declined, the duel is still kept")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "Beta declined your duel invitation." {
		t.Fatalf("the challenger was told %q", got)
	}
}

// The rules of a challenge: not oneself (the live word, in the live red),
// not an NPC nor nothing ("Invalid target."), not someone already asked or
// fighting ("%s is in a duel."), not from afar, nor twice within DuelAskAgain.
func TestDuelChallengeRules(t *testing.T) {
	d := newDuelists(t)
	refused := func(s *session.Session, want string) {
		t.Helper()
		got := duelDrain(s)
		if l := duelLines(got); len(l) != 1 || l[0] != want {
			t.Fatalf("told %q, want %q", l, want)
		}
		if m := ofOp(got, protocol.OpcodeColoredChat)[0]; !bytes.Equal(m[len(m)-3:], duelRed[:]) {
			t.Fatalf("a refusal in % X, want the live red", m[len(m)-3:])
		}
	}
	d.w.DuelChallenge(d.sa, d.sa.PlayerObjectID(), "", 0, d.at(0))
	refused(d.sa, "You cannot duel yourself.")
	d.w.DuelChallenge(d.sa, 0, "alpha", 0, d.at(0))
	refused(d.sa, "You cannot duel yourself.")
	d.w.DuelChallenge(d.sa, 0xFFFFFFFF, "", 0, d.at(0))
	refused(d.sa, "Invalid target.")
	npc := &NPC{ServerID: 3003, Name: "Rat", Level: 1, HP: 10, MaxHP: 10, X: 101, Y: 10, Z: 100}
	d.w.NPCs[npc.ServerID] = npc
	GetOrCreateVisibility(d.sa).ChannelEntity[1], GetOrCreateVisibility(d.sa).ChannelObjectID[1] = npc.ServerID, 0x00018899
	d.w.DuelChallenge(d.sa, 0x00018899, "", 0, d.at(0))
	refused(d.sa, "Invalid target.")
	d.w.DuelChallenge(d.sa, 0, "Nobody", 0, d.at(0))
	refused(d.sa, "Nobody is not connected.")

	d.c.X, d.c.Z = 100+DuelStartRange+1, 100
	d.w.DuelChallenge(d.sa, 0, "gamma", 0, d.at(0))
	refused(d.sa, "Gamma is too far away to duel.")
	d.c.X, d.c.Z = 120, 110

	d.w.DuelChallenge(d.sa, 0, "Beta", 0, d.at(0)) // by name, any case
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.w.DuelChallenge(d.sc, d.cSeesB, "", 0, d.at(time.Second))
	refused(d.sc, "Beta is in a duel.")
	d.w.DuelChallenge(d.sc, d.cSeesA, "", 0, d.at(time.Second))
	refused(d.sc, "Alpha is in a duel.")
	d.w.DuelChallenge(d.sa, 0, "Gamma", 0, d.at(time.Second))
	refused(d.sa, "You are already in a duel.")

	d.w.DuelCancel(d.sa, d.at(2*time.Second)) // withdrawn
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Alpha has withdrawn the duel challenge." {
		t.Fatalf("the challenged was told %q", got)
	}
	duelDrain(d.sa)
	d.w.DuelChallenge(d.sa, 0, "Gamma", 0, d.at(3*time.Second))
	refused(d.sa, "You must wait a moment before challenging again.")
	d.w.DuelChallenge(d.sa, 0, "Gamma", 0, d.at(2*time.Second+DuelAskAgain))
	if d.w.duels.of[d.c.ServerID] == nil {
		t.Fatalf("after DuelAskAgain the challenge was refused: %q", duelLines(duelDrain(d.sa)))
	}
}

// An invitation stands DuelInviteTime: then it lapses, both told the live
// word, and a late Accept is told it too; the control: one second before, it
// stands.
func TestDuelInvitationExpires(t *testing.T) {
	d := newDuelists(t)
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.tick(DuelInviteTime - time.Second)
	if d.w.duels.byID[duel.ID] == nil {
		t.Fatal("lapsed before its time")
	}
	d.tick(DuelInviteTime + time.Millisecond)
	if d.w.duels.byID[duel.ID] != nil {
		t.Fatal("still standing after its time")
	}
	for _, s := range []*session.Session{d.sa, d.sb} {
		if got := duelLines(duelDrain(s)); len(got) != 1 || got[0] != "The duel invitation has expired." {
			t.Fatalf("told %q at the lapse", got)
		}
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(DuelInviteTime+time.Second))
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "The duel invitation has expired." || d.w.duels.of[d.b.ServerID] != nil {
		t.Fatalf("a late accept: told %q", got)
	}
}

// Accepted, the fight begins where they stand: both told, its centre halfway.
// Each sees the other as a creature (byte 142 0) and nobody else does: the
// bystander sees two players (2); a client with the live window sees a
// player too. At the end both are players to each other again.
func TestDuelCreatureViewToTheOpponentOnly(t *testing.T) {
	d := newDuelists(t)
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.w.DuelAnswer(d.sb, duel.ID, true, d.at(time.Second))
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "The duel with Beta has begun." {
		t.Fatalf("A was told %q", got)
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "The duel with Alpha has begun." {
		t.Fatalf("B was told %q", got)
	}
	if duel.X != 102 || duel.Z != 100 || duel.World != 0 {
		t.Fatalf("the centre (%v, %v) world %d, want halfway (102, 100)", duel.X, duel.Z, duel.World)
	}
	byte142 := func(s *session.Session, id EntityID, obj uint32) byte {
		d.w.mu.Lock()
		defer d.w.mu.Unlock()
		return d.w.serializeEntityForChannel(s, id, obj)[142]
	}
	if got := byte142(d.sa, d.b.ServerID, d.aSeesB); got != 0 {
		t.Errorf("A sees B with byte 142 = %d, want a creature's 0", got)
	}
	if got := byte142(d.sb, d.a.ServerID, d.bSeesA); got != 0 {
		t.Errorf("B sees A with byte 142 = %d, want 0", got)
	}
	if a, b := byte142(d.sc, d.a.ServerID, d.cSeesA), byte142(d.sc, d.b.ServerID, d.cSeesB); a != 2 || b != 2 {
		t.Errorf("the bystander sees %d and %d, want two players' 2", a, b)
	}
	d.b.DuelWindow = true
	if got := byte142(d.sb, d.a.ServerID, d.bSeesA); got != 2 {
		t.Errorf("a live window sees its opponent with %d, want a player's 2", got)
	}
	d.b.DuelWindow = false
	d.w.DuelCancel(d.sb, d.at(2*time.Second))
	if a, b := byte142(d.sa, d.b.ServerID, d.aSeesB), byte142(d.sb, d.a.ServerID, d.bSeesA); a != 2 || b != 2 {
		t.Errorf("after the duel: %d and %d, want 2", a, b)
	}
}

// A duel's hit lands at half, never under 1, and one the loser cannot take
// leaves it DuelLoserHP: the duel over, no death, both told who won and so
// is the bystander within DuelHeardWithin; out of it, a player is not told.
func TestDuelHitHalvedAndNoDeath(t *testing.T) {
	for _, c := range []struct{ n, want int }{{100, 50}, {101, 51}, {1, 1}, {0, 1}, {3, 2}} {
		if got := duelHalf(c.n); got != c.want {
			t.Errorf("duelHalf(%d) = %d, want %d", c.n, got, c.want)
		}
	}
	d := newDuelists(t)
	duel := d.start(t)
	d.w.mu.Lock()
	over := d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 300, d.at(2*time.Second))
	d.w.mu.Unlock()
	if over || d.b.HP != 750 {
		t.Fatalf("300 on Beta: over %v, HP %d, want 900 - 150", over, d.b.HP)
	}
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication); len(got) != 1 || !bytes.Equal(got[0], damageMsg(t, d.sa.PlayerObjectID(), -150, d.aSeesB)) {
		t.Fatalf("A was told % X", got)
	}
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDamageIndication); len(got) != 1 || !bytes.Equal(got[0], damageMsg(t, d.bSeesA, -150, d.sb.PlayerObjectID())) {
		t.Fatalf("B was told % X", got)
	}
	far := &Player{ServerID: 500000004, Name: "Delta", HP: 10, MaxHP: 10, X: 102 + DuelHeardWithin + 1, Y: 10, Z: 100}
	sd := d.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 11), Port: 7011}, 11, 11, 1, 2, true)
	d.w.Players[far.ServerID] = far
	sd.SetECSEntity(far)
	sd.SetCharacterInWorld(true)

	d.w.mu.Lock()
	over = d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 5000, d.at(3*time.Second))
	d.w.mu.Unlock()
	if !over || d.b.HP != DuelLoserHP || d.b.IsDead() || d.b.DiedAt != (time.Time{}) || d.a.TotalXP != 0 {
		t.Fatalf("the last hit: over %v, HP %d, dead %v, XP %d: the loser stays at %d", over, d.b.HP, d.b.IsDead(), d.a.TotalXP, DuelLoserHP)
	}
	if d.w.duels.of[d.a.ServerID] != nil || d.w.duels.of[d.b.ServerID] != nil {
		t.Fatal("the duel is still kept")
	}
	for _, s := range []*session.Session{d.sa, d.sb, d.sc} {
		if got := duelLines(duelDrain(s)); len(got) != 1 || got[0] != "Alpha has defeated Beta in a duel." {
			t.Fatalf("told %q at the end", got)
		}
	}
	if got := duelLines(duelDrain(sd)); len(got) != 0 {
		t.Fatalf("a player out of earshot was told %q", got)
	}
}

// Melee in a duel: a 0x000F at the opponent engages it (out of a duel it does
// not), the hits come in the swing's rounds, each the roll on a creature of
// the opponent's level halved; the control: the same 0x000F with no duel
// engages nothing.
func TestDuelMeleeRounds(t *testing.T) {
	d := newDuelists(t)
	swing := Swing{Kind: 1, Delay: 3200 * time.Millisecond, Reach: 4.5}
	if d.w.EngageAttack(d.sa, d.aSeesB, swing, d.at(0)) {
		t.Fatal("engaged a player with no duel")
	}
	duel := d.start(t)
	_ = duel
	d.a.Gear.WeaponDamage, d.a.Gear.WeaponCond = 40, 1
	if !d.w.EngageAttack(d.sa, d.aSeesB, swing, d.at(2*time.Second)) || d.a.AttackTarget != d.b.ServerID {
		t.Fatalf("engaged %v, target %d", d.a.AttackTarget == d.b.ServerID, d.a.AttackTarget)
	}
	d.a.AutoAttack = true
	floor, low, high := playerMeleeRoll(d.a, duelStandIn(d.b.Level))
	lo, hi := duelHalf(max(floor, low)), duelHalf(max(floor, high))
	before := d.b.HP
	for x := 2 * time.Second; x <= 9*time.Second; x += 100 * time.Millisecond {
		d.w.mu.Lock()
		d.w.attackSystem(d.at(x))
		d.w.mu.Unlock()
	}
	hits := ofOp(duelDrain(d.sa), protocol.OpcodeDamageIndication)
	if len(hits) < 2 {
		t.Fatalf("%d hits in 7 s of a 3.2 s swing", len(hits))
	}
	taken := before - d.b.HP
	if taken < len(hits)*lo || taken > len(hits)*hi {
		t.Fatalf("%d hits took %d, want each within the halved roll %d..%d", len(hits), taken, lo, hi)
	}
	if d.b.IsDead() || d.a.TotalXP != 0 {
		t.Fatalf("dead %v, XP %d", d.b.IsDead(), d.a.TotalXP)
	}
}

// A harmful spell at the opponent is cast and lands at half (out of a duel it
// is refused as before); what lasts of it, a damage over time, ticks at half,
// comes off at the duel's end and is never kept over a logout.
func TestDuelSpells(t *testing.T) {
	d := newDuelists(t)
	if d.w.StartCast(d.sb, strike(), d.bSeesA, d.at(0)) {
		t.Fatal("a strike at a player was cast with no duel")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != castFriendly {
		t.Fatalf("the refusal %q", got)
	}
	d.start(t)
	before := d.a.HP
	sp := strike()
	sp.SpellID = 651
	if !d.w.StartCast(d.sb, sp, d.bSeesA, d.at(2*time.Second)) {
		t.Fatalf("a strike at the opponent was refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(d.at(2*time.Second + sp.CastTime + castLand))
	d.w.mu.Unlock()
	// Burst: 26 and 8% of DEX 90 (7), 33, halved 17
	if taken := before - d.a.HP; taken != 17 {
		t.Fatalf("the strike took %d, want 33 halved, 17", taken)
	}

	dot := CastSpell{SpellID: 700, DisplayIndex: 4, Scope: 1, Name: "Searing", Icon: 0x22222222, CastTime: time.Second,
		Recast: time.Second, Range: 30, FX: 0xE78024D9, Def: ParseSpellText("Target, Decrease HP per tic by 20, 18sec")}
	if !dot.Def.Harmful {
		t.Fatalf("the test's damage over time reads as not harmful: %+v", dot.Def)
	}
	land := d.at(5 * time.Second)
	if !d.w.StartCast(d.sb, dot, d.bSeesA, land.Add(-dot.CastTime-castLand)) {
		t.Fatalf("the damage over time was refused: %q", duelLines(duelDrain(d.sb)))
	}
	before = d.a.HP
	d.w.mu.Lock()
	d.w.castSystem(land)
	d.w.mu.Unlock()
	if len(d.a.Effects) != 1 || d.a.Effects[0].duelFoe != d.b.ServerID {
		t.Fatalf("the effects on A %+v, want the duel's", d.a.Effects)
	}
	if taken := before - d.a.HP; taken != 10 {
		t.Fatalf("the first tick took %d, want 20 halved", taken)
	}
	if rows := savedEffects(d.a, land); len(rows) != 0 {
		t.Fatalf("a duel's effect is kept over a logout: %+v", rows)
	}
	d.w.DuelCancel(d.sa, land.Add(time.Second))
	if len(d.a.Effects) != 0 {
		t.Fatalf("the duel over, A still carries %+v", d.a.Effects)
	}
	before = d.a.HP
	d.w.mu.Lock()
	d.w.effectSystem(land.Add(13 * time.Second))
	d.w.mu.Unlock()
	if d.a.HP != before {
		t.Fatalf("a tick after the duel took %d", before-d.a.HP)
	}
}

// The fight ends when one leaves the arena (the other wins, by forfeit), when
// one logs out, and after DuelMaxTime as a draw; the control: standing at
// the arena's edge, it goes on.
func TestDuelEnds(t *testing.T) {
	d := newDuelists(t)
	d.start(t)
	d.a.X = 102 + DuelArenaRadius - 1
	d.tick(5 * time.Second)
	if d.w.duels.of[d.a.ServerID] == nil {
		t.Fatal("ended inside the arena")
	}
	d.a.X = 102 + DuelArenaRadius + 1
	d.tick(6 * time.Second)
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("still on outside the arena")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Alpha forfeits the duel to Beta." {
		t.Fatalf("told %q", got)
	}

	d = newDuelists(t)
	d.start(t)
	d.w.RemovePlayer(d.b.ServerID)
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("still on after the opponent left")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "Beta forfeits the duel to Alpha." {
		t.Fatalf("told %q", got)
	}

	d = newDuelists(t)
	d.start(t)
	d.tick(time.Second + DuelMaxTime)
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("still on after DuelMaxTime")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "The duel between Alpha and Beta ends in a draw." {
		t.Fatalf("told %q", got)
	}

	// a new login of the same character is not the duelist
	d = newDuelists(t)
	d.start(t)
	again := *d.b
	d.w.Players[d.b.ServerID] = &again
	d.tick(2 * time.Second)
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("still on after a new login of the opponent")
	}
}

// "Cancel duel?" is the live word on a box with Yes and No; Yes surrenders,
// No and a closed box do nothing.
func TestDuelCancelBox(t *testing.T) {
	d := newDuelists(t)
	d.w.DuelAskCancel(d.sa)
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "You are not in a duel." {
		t.Fatalf("no duel: told %q", got)
	}
	d.start(t)
	d.w.DuelAskCancel(d.sa)
	boxes := ofOp(duelDrain(d.sa), protocol.OpcodeQuestPopupMulti)
	if want := BuildDuelBox(DuelCancelBox, "Cancel duel?", []string{"Yes", "No"}); len(boxes) != 1 || !bytes.Equal(boxes[0], want) {
		t.Fatalf("the box % X, want % X", boxes, want)
	}
	d.w.DuelBoxAnswer(d.sa, DuelCancelBox, 1, d.at(2*time.Second))
	d.w.DuelBoxAnswer(d.sa, DuelCancelBox, 255, d.at(2*time.Second))
	if d.w.duels.of[d.a.ServerID] == nil {
		t.Fatal("No ended it")
	}
	d.w.DuelBoxAnswer(d.sa, DuelCancelBox, 0, d.at(3*time.Second))
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("Yes did not end it")
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Alpha forfeits the duel to Beta." {
		t.Fatalf("told %q", got)
	}
}

// A client with the live window gets the live game's own messages, byte for
// byte as eqoa-live-1500's CLIENT reads them: the challenger its duel id
// (sub 1), the challenged the invitation (sub 2: race, class, level, name,
// guild, title), at the start the challenger sub 6, both sub 7 (on, set, the
// opponent's id as each knows it, no pet) and sub 8 (the centre, the world),
// and sub 7 all off at the end; no boxes for it, and its opponent stays a
// player to it.
func TestDuelLiveWindowMessages(t *testing.T) {
	d := newDuelists(t)
	d.a.DuelWindow, d.b.DuelWindow = true, true
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	id := duel.ID
	le := func(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
	op := func(sub byte) []byte { return append([]byte{0x02, 0x15, sub}, le(id)...) }
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], op(1)) {
		t.Fatalf("the challenger's % X, want % X", got, op(1))
	}
	gotB := duelDrain(d.sb)
	want := op(2)
	want = append(want, 6, 0, 30)                                                         // race, class, level
	want = append(append(want, le(5)...), "Alpha"...)                                     // name
	want = append(append(want, le(6)...), 'B', 0, 'l', 0, 'a', 0, 'd', 0, 'e', 0, 's', 0) // guild
	want = append(want, le(0)...)                                                         // title: none
	if got := ofOp(gotB, protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the invitation % X, want % X", got, want)
	}
	if len(ofOp(gotB, protocol.OpcodeQuestPopupMulti)) != 0 || len(duelLines(gotB)) != 0 {
		t.Fatalf("a live window was sent a box or a line too: %x", gotB)
	}
	d.w.DuelAnswer(d.sb, id, true, d.at(time.Second))
	// accepted between two live windows: the wager window opens (sub 13) and
	// is zeroed (sub 12: the live 13 keeps the last wager's tunar and
	// accepts), the challenger's queue declined first (sub 6)
	zero := append(op(12), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 3 || !bytes.Equal(got[0], op(6)) || !bytes.Equal(got[1], op(13)) || !bytes.Equal(got[2], zero) {
		t.Fatalf("the challenger at the accept % X", got)
	}
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDuel); len(got) != 2 || !bytes.Equal(got[0], op(13)) || !bytes.Equal(got[1], zero) {
		t.Fatalf("the challenged at the accept % X", got)
	}
	var held []DuelPurseOp
	d.w.OnDuelPurse(func(op DuelPurseOp) { held = append(held, op) })
	d.a.Tunar, d.b.Tunar = 500, 30
	tunar := func(mine, theirs uint32, ma, ta byte) []byte {
		return append(append(append(op(12), le(mine)...), le(theirs)...), ma, ta)
	}
	accepts := func(ma, ta byte) []byte { return append(op(15), 1, ma, ta) }
	d.w.DuelWagerTunar(d.sa, 50)
	d.w.DuelWagerTunar(d.sb, 1000000) // at most what it carries
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 2 || !bytes.Equal(got[0], tunar(50, 0, 0, 0)) || !bytes.Equal(got[1], tunar(50, 30, 0, 0)) {
		t.Fatalf("the challenger's wager lines % X", got)
	}
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDuel); len(got) != 2 || !bytes.Equal(got[0], tunar(0, 50, 0, 0)) || !bytes.Equal(got[1], tunar(30, 50, 0, 0)) {
		t.Fatalf("the challenged's wager lines % X", got)
	}
	d.w.DuelWagerItem(d.sa)
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "Only tunar may be wagered on a duel here." {
		t.Fatalf("an item staked: told %q", got)
	}
	d.w.DuelWagerAccept(d.sa, true, d.at(2*time.Second))
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], accepts(1, 0)) {
		t.Fatalf("A's accept to A % X", got)
	}
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], accepts(0, 1)) {
		t.Fatalf("A's accept to B % X", got)
	}
	d.w.DuelWagerTunar(d.sa, 60) // once accepted the live client sends none: dropped
	if got := duelDrain(d.sa); len(got) != 0 || duel.Stake[0] != 50 {
		t.Fatalf("a stake after the accept: % X, stake %d", got, duel.Stake[0])
	}
	if len(held) != 0 || d.a.Tunar != 500 {
		t.Fatalf("held before both accepted: %+v, purse %d", held, d.a.Tunar)
	}
	d.w.DuelWagerAccept(d.sb, true, d.at(3*time.Second))
	if d.a.Tunar != 450 || d.b.Tunar != 0 || len(held) != 1 || !held[0].Hold || len(held[0].Moves) != 2 ||
		held[0].Moves[0].Tunar != 50 || held[0].Moves[0].PurseNow != 450 || held[0].Moves[1].Tunar != 30 || held[0].Moves[1].PurseNow != 0 {
		t.Fatalf("both accepted: purses %d and %d, held %+v", d.a.Tunar, d.b.Tunar, held)
	}
	state := func(on byte, foe uint32) []byte {
		return append(append(append(op(7), on, on), le(foe)...), le(0xFFFFFFFF)...)
	}
	place := append(append(append(append(op(8), le(math.Float32bits(102))...), le(math.Float32bits(10))...), le(math.Float32bits(100))...), 0)
	gotA := ofOp(duelDrain(d.sa), protocol.OpcodeDuel)
	if len(gotA) != 3 || !bytes.Equal(gotA[0], accepts(1, 1)) || !bytes.Equal(gotA[1], state(1, d.aSeesB)) || !bytes.Equal(gotA[2], place) {
		t.Fatalf("the challenger at the start % X", gotA)
	}
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDuel); len(got) != 3 || !bytes.Equal(got[0], accepts(1, 1)) || !bytes.Equal(got[1], state(1, d.bSeesA)) || !bytes.Equal(got[2], place) {
		t.Fatalf("the challenged at the start % X", got)
	}
	d.w.mu.Lock()
	if b := d.w.serializeEntityForChannel(d.sa, d.b.ServerID, d.aSeesB)[142]; b != 2 {
		t.Errorf("a live window sees its opponent with byte 142 = %d, want 2", b)
	}
	d.w.mu.Unlock()
	d.tick(2 * time.Second)
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 0 {
		t.Fatalf("sub 7 again with nothing changed: % X", got)
	}
	GetOrCreateVisibility(d.sa).ChannelObjectID[0] = 0x00018855 // B out of view and back
	d.tick(3 * time.Second)
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], state(1, 0x00018855)) {
		t.Fatalf("a new id for the opponent: % X", got)
	}
	d.w.DuelCancel(d.sb, d.at(4*time.Second))
	off := append(append(append(op(7), 0, 0), le(0xFFFFFFFF)...), le(0xFFFFFFFF)...)
	endA := duelDrain(d.sa)
	if got := ofOp(endA, protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], off) {
		t.Fatalf("the end % X, want % X", got, off)
	}
	// Beta surrendered: the pot, 80, to Alpha
	if d.a.Tunar != 530 || d.b.Tunar != 0 || len(held) != 2 || held[1].Hold || held[1].Moves[0].Tunar != 80 || held[1].Moves[0].PurseNow != 530 || held[1].Moves[1].Tunar != 0 {
		t.Fatalf("the payment: purses %d and %d, ops %+v", d.a.Tunar, d.b.Tunar, held)
	}
	if got := duelLines(endA); len(got) != 2 || got[1] != "You win the wager: 80 tunar." {
		t.Fatalf("the winner was told %q", got)
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 2 || got[1] != "You lose your wager of 30 tunar." {
		t.Fatalf("the loser was told %q", got)
	}
}

// The live window's lapse and declines: the challenged sub 3 (its window
// says the live word and closes), the challenger sub 5 with the name for a
// decline and sub 4 for a lapse.
func TestDuelLiveWindowDeclines(t *testing.T) {
	d := newDuelists(t)
	d.a.DuelWindow, d.b.DuelWindow = true, true
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	id := d.w.duels.of[d.a.ServerID].ID
	duelDrain(d.sa)
	duelDrain(d.sb)
	le := func(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
	d.w.DuelAnswer(d.sb, id, false, d.at(time.Second))
	want := append(append(append([]byte{0x02, 0x15, 5}, le(id)...), le(4)...), "Beta"...)
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the decline % X, want % X", got, want)
	}
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(10*time.Second))
	id = d.w.duels.of[d.a.ServerID].ID
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.tick(10*time.Second + DuelInviteTime + time.Millisecond)
	if got := ofOp(duelDrain(d.sb), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], append([]byte{0x02, 0x15, 3}, le(id)...)) {
		t.Fatalf("the challenged at the lapse % X", got)
	}
	if got := ofOp(duelDrain(d.sa), protocol.OpcodeDuel); len(got) != 1 || !bytes.Equal(got[0], append([]byte{0x02, 0x15, 4}, le(id)...)) {
		t.Fatalf("the challenger at the lapse % X", got)
	}
}

// A duelist's pet: defensive, it joins once the opponent has struck its
// owner, and its hits on the opponent land at half; the opponent's hits on it
// land at half too, and one it cannot take leaves it at a hit point, out of
// the duel, its owner told. The pet is shown to the opponent as a creature
// while it is in the duel.
func TestDuelPet(t *testing.T) {
	d := newDuelists(t)
	duel := d.start(t)
	pet := &NPC{ServerID: 4004, Name: "Pet", Level: 30, HP: 400, MaxHP: 400, Owner: d.a.ServerID, X: 101, Y: 10, Z: 101}
	d.w.NPCs[pet.ServerID] = pet
	if d.w.pets == nil {
		d.w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: d.a, npc: pet, stance: petDefensive, last: d.at(time.Second)}
	d.w.pets[d.a.ServerID] = ps
	GetOrCreateVisibility(d.sb).ChannelEntity[1], GetOrCreateVisibility(d.sb).ChannelObjectID[1] = pet.ServerID, 0x00018877
	d.w.mu.Lock()
	if b := d.w.serializeEntityForChannel(d.sb, pet.ServerID, 0x00018877)[142]; b != 0 {
		t.Errorf("the opponent sees the pet with byte 142 = %d, want a creature's 0", b)
	}
	if !d.w.duelFoeLocked(d.b, pet.ServerID) || d.w.duelFoeLocked(d.c, pet.ServerID) {
		t.Error("the pet is the opponent's foe and nobody else's")
	}
	if d.w.duelPetLocked(ps, 0.1, d.at(2*time.Second)) {
		t.Fatal("a defensive pet joined before its owner was struck")
	}
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 10, d.at(2*time.Second))
	before := d.b.HP
	for x := 2 * time.Second; x < 6*time.Second; x += 100 * time.Millisecond {
		d.w.duelPetLocked(ps, 0.1, d.at(x))
	}
	if !duel.pets[d.a.ServerID] || d.b.HP >= before {
		t.Fatalf("joined %v, the opponent's HP %d of %d", duel.pets[d.a.ServerID], d.b.HP, before)
	}
	r := creatureRoll(pet.Level)
	if hit := before - d.b.HP; hit > duelHalf(int(math.Round(r.Top-targetDefence(duelStandIn(d.b.Level)))))*4 {
		t.Fatalf("the pet took %d off, more than its halved roll allows", hit)
	}
	d.w.duelHurtPetLocked(duel, d.b, pet, 100, d.at(6*time.Second))
	if pet.HP != 350 {
		t.Fatalf("100 on the pet left %d, want 400 - 50", pet.HP)
	}
	duelDrain(d.sa)
	d.w.duelHurtPetLocked(duel, d.b, pet, 5000, d.at(7*time.Second))
	if pet.HP != 1 || pet.Dead || !duel.petOut[d.a.ServerID] {
		t.Fatalf("the pet's last hit: HP %d, dead %v, out %v", pet.HP, pet.Dead, duel.petOut[d.a.ServerID])
	}
	if d.w.duelFoeLocked(d.b, pet.ServerID) || d.w.duelPetLocked(ps, 0.1, d.at(8*time.Second)) {
		t.Fatal("a pet out of the duel is still in it")
	}
	if b := d.w.serializeEntityForChannel(d.sb, pet.ServerID, 0x00018877)[142]; b != 3 {
		t.Errorf("a pet out of the duel is shown with %d, want a pet's 3", b)
	}
	d.w.mu.Unlock()
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || !strings.Contains(got[0], "pet yields") {
		t.Fatalf("the owner was told %q", got)
	}
}

// A wager named in the challenge (a client without the live window): the box
// says it, both stake it as the fight begins, and the winner is paid both;
// a draw gives each its own back; a challenger or a challenged short of it is
// refused, nothing taken.
func TestDuelWagerByChallenge(t *testing.T) {
	d := newDuelists(t)
	var ops []DuelPurseOp
	d.w.OnDuelPurse(func(op DuelPurseOp) { ops = append(ops, op) })
	d.a.Tunar, d.b.Tunar = 1000, 150
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 2000, d.at(0))
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "You do not have 2000 tunar to wager." {
		t.Fatalf("a wager above the purse: told %q", got)
	}
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 200, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	got := duelDrain(d.sb)
	want := BuildDuelBox(DuelBoxID(duel.ID), "Alpha challenges you to a duel for 200 tunar.", []string{"Accept", "Decline"})
	if boxes := ofOp(got, protocol.OpcodeQuestPopupMulti); len(boxes) != 1 || !bytes.Equal(boxes[0], want) {
		t.Fatalf("the box % X", boxes)
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(time.Second))
	if d.w.duels.of[d.a.ServerID] != nil || d.a.Tunar != 1000 || d.b.Tunar != 150 || len(ops) != 0 {
		t.Fatalf("Beta has 150 of 200: duel %v, purses %d and %d, ops %+v", d.w.duels.of[d.a.ServerID], d.a.Tunar, d.b.Tunar, ops)
	}
	if got := duelLines(duelDrain(d.sb)); len(got) != 1 || got[0] != "Beta does not have the 200 tunar wagered." {
		t.Fatalf("told %q", got)
	}

	d.b.Tunar = 500
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 200, d.at(10*time.Second))
	duel = d.w.duels.of[d.a.ServerID]
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(11*time.Second))
	if duel.Phase != duelOn || d.a.Tunar != 800 || d.b.Tunar != 300 || len(ops) != 1 || !ops[0].Hold {
		t.Fatalf("begun: phase %d, purses %d and %d, ops %+v", duel.Phase, d.a.Tunar, d.b.Tunar, ops)
	}
	d.w.mu.Lock()
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 1_000_000, d.at(12*time.Second))
	d.w.mu.Unlock()
	d.w.runDuelPurse() // the tick's, after its lock
	if d.a.Tunar != 800 || d.b.Tunar != 700 || len(ops) != 2 || ops[1].Hold || ops[1].Moves[0].Tunar != 0 || ops[1].Moves[1].Tunar != 400 {
		t.Fatalf("Beta won: purses %d and %d, ops %+v", d.a.Tunar, d.b.Tunar, ops)
	}

	d = newDuelists(t)
	ops = nil
	d.w.OnDuelPurse(func(op DuelPurseOp) { ops = append(ops, op) })
	d.a.Tunar, d.b.Tunar = 100, 100
	d.w.DuelChallenge(d.sa, 0, "Beta", 60, d.at(0))
	duel = d.w.duels.of[d.a.ServerID]
	d.w.DuelAnswer(d.sb, 0, true, d.at(time.Second))
	d.tick(time.Second + DuelMaxTime)
	if d.a.Tunar != 100 || d.b.Tunar != 100 || len(ops) != 2 || ops[1].Moves[0].Tunar != 60 || ops[1].Moves[1].Tunar != 60 {
		t.Fatalf("a draw: purses %d and %d, ops %+v", d.a.Tunar, d.b.Tunar, ops)
	}
	if got := duelLines(duelDrain(d.sa)); len(got) < 1 || got[len(got)-1] != "Your wager of 60 tunar is returned." {
		t.Fatalf("a draw: told %q", got)
	}

	// a duelist who logs out mid-duel forfeits its stake, paid onto the
	// winner; the leaver's own row is paid nothing
	d = newDuelists(t)
	ops = nil
	d.w.OnDuelPurse(func(op DuelPurseOp) { ops = append(ops, op) })
	d.a.Tunar, d.b.Tunar, d.a.CharServerID, d.b.CharServerID = 100, 100, 71, 72
	d.w.DuelChallenge(d.sa, 0, "Beta", 50, d.at(0))
	d.w.DuelAnswer(d.sb, 0, true, d.at(time.Second))
	d.w.RemovePlayer(d.b.ServerID)
	d.w.runDuelPurse()
	if d.a.Tunar != 150 || len(ops) != 2 || ops[1].Moves[0].Tunar != 100 || ops[1].Moves[1].P != nil || ops[1].Moves[1].CharID != 72 || ops[1].Moves[1].Tunar != 0 {
		t.Fatalf("after the logout: purse %d, ops %+v", d.a.Tunar, ops)
	}
}

// The live client's sub 13 leaves the last wager's tunar and accepts, so a
// wager window opens with a sub 12 of zeroes after it, on both sides (the
// second duel of a login opened with both accepts on: lane-duelwnd24's rig).
func TestDuelWagerOpensZeroed(t *testing.T) {
	d := newDuelists(t)
	d.a.DuelWindow, d.b.DuelWindow = true, true
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	id := d.w.duels.of[d.a.ServerID].ID
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.w.DuelAnswer(d.sb, id, true, d.at(time.Second))
	open13 := append([]byte{0x02, 0x15, 13}, binary.LittleEndian.AppendUint32(nil, id)...)
	zero12 := append(append([]byte{0x02, 0x15, 12}, binary.LittleEndian.AppendUint32(nil, id)...),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	for _, s := range []*session.Session{d.sa, d.sb} {
		m := ofOp(duelDrain(s), protocol.OpcodeDuel)
		at := -1
		for i, b := range m {
			if bytes.Equal(b, open13) {
				at = i
			}
		}
		if at < 0 || at+1 >= len(m) || !bytes.Equal(m[at+1], zero12) {
			t.Fatalf("the wager's opening % X", m)
		}
	}
}

// Two live windows: a duel called off in its wager window closes both (sub
// 14), says why, and holds nothing; the wager window lasts DuelWagerTime.
func TestDuelWagerWindowCalledOff(t *testing.T) {
	d := newDuelists(t)
	d.a.DuelWindow, d.b.DuelWindow = true, true
	d.a.Tunar, d.b.Tunar = 100, 100
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	id := d.w.duels.of[d.a.ServerID].ID
	d.w.DuelAnswer(d.sb, id, true, d.at(time.Second))
	d.w.DuelWagerTunar(d.sa, 40)
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.w.DuelCancel(d.sa, d.at(2*time.Second))
	close14 := append([]byte{0x02, 0x15, 14}, binary.LittleEndian.AppendUint32(nil, id)...)
	// then the duel's state ended, so the live client's Quit surrenders nothing
	end7 := append(append([]byte{0x02, 0x15, 7}, binary.LittleEndian.AppendUint32(nil, id)...),
		0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
	for _, s := range []*session.Session{d.sa, d.sb} {
		got := duelDrain(s)
		if m := ofOp(got, protocol.OpcodeDuel); len(m) != 2 || !bytes.Equal(m[0], close14) || !bytes.Equal(m[1], end7) {
			t.Fatalf("the window's close % X", m)
		}
		if l := duelLines(got); len(l) != 1 || l[0] != "Alpha has called off the duel." {
			t.Fatalf("told %q", l)
		}
	}
	if d.w.duels.of[d.a.ServerID] != nil || d.a.Tunar != 100 {
		t.Fatalf("called off: duel %v, purse %d", d.w.duels.of[d.a.ServerID], d.a.Tunar)
	}

	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(10*time.Second))
	id = d.w.duels.of[d.a.ServerID].ID
	d.w.DuelAnswer(d.sb, id, true, d.at(11*time.Second))
	d.tick(11*time.Second + DuelWagerTime - time.Second)
	if d.w.duels.of[d.a.ServerID] == nil {
		t.Fatal("the wager window closed before its time")
	}
	d.tick(11*time.Second + DuelWagerTime + time.Millisecond)
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("the wager window outlived its time")
	}
}

// A duel opponent's hold stops a player and its slow halves the run, both
// told by 0x00F8 as they land and as the duel's end takes them off; the
// control: the same flags on an effect that is not a duel's change nothing
// (no other harm lands on a player).
func TestDuelHoldAndSlow(t *testing.T) {
	d := newDuelists(t)
	d.start(t)
	run := d.a.SpeedNow()
	d.w.mu.Lock()
	d.w.addEffectLocked(d.a, &ActiveEffect{SpellID: 801, Name: "Snare", Until: d.at(time.Minute), Snare: true, duelFoe: d.b.ServerID}, d.at(2*time.Second))
	slowed := d.a.SpeedNow()
	d.w.addEffectLocked(d.a, &ActiveEffect{SpellID: 802, Name: "Root", Until: d.at(time.Minute), Root: true, duelFoe: d.b.ServerID}, d.at(2*time.Second))
	held := d.a.SpeedNow()
	d.w.addEffectLocked(d.c, &ActiveEffect{SpellID: 803, Name: "Odd", Until: d.at(time.Minute), Root: true}, d.at(2*time.Second))
	d.w.mu.Unlock()
	if slowed != run*duelSnare || held != 0 {
		t.Fatalf("run %v, slowed %v, held %v", run, slowed, held)
	}
	if d.c.SpeedNow() != d.c.Speed {
		t.Fatalf("a hold that is not a duel's: %v", d.c.SpeedNow())
	}
	if got := ofOp(duelDrain(d.sa), protocol.OpcodePlayerSpeed); len(got) != 2 {
		t.Fatalf("%d speeds sent as the hold and the slow landed, want 2", len(got))
	}
	d.w.DuelCancel(d.sb, d.at(3*time.Second))
	if d.a.SpeedNow() != run {
		t.Fatalf("after the duel: %v, want %v", d.a.SpeedNow(), run)
	}
	if got := ofOp(duelDrain(d.sa), protocol.OpcodePlayerSpeed); len(got) == 0 {
		t.Fatal("no speed sent as the duel's hold came off")
	}
}
