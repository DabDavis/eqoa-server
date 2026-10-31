package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

func TestSwingTableAgainstTheCaptures(t *testing.T) {
	var dual, shield, bare [21]bool
	dual[14], dual[15] = true, true
	shield[14], shield[17] = true, true
	// pcaps/ps2_main_capture.pcap's level 51 bard dual-wields two rapiers:
	// GetSwing kind 5, Attack4, 6 s — the swing whose replay puts 0x000F at
	// the PS2's three times (the Go client's egress_combat_test).
	if k := GetSwing(5, 51, dual); k != 5 || SwingDelay(k) != 6*time.Second || SwingEffectName(k) != "Attack4" {
		t.Errorf("bard 51 dual: kind %d %v %s", k, SwingDelay(k), SwingEffectName(k))
	}
	if k := GetSwing(5, 51, shield); k != 2 || SwingDelay(k) != 4*time.Second {
		t.Errorf("bard 51 with a shield: kind %d %v", k, SwingDelay(k))
	}
	if k := GetSwing(0, 10, bare); k != 0 || SwingDelay(k) != 3*time.Second {
		t.Errorf("warrior 10 bare-handed: kind %d %v", k, SwingDelay(k))
	}
	// The effect ids Sandstorm sends in an NPC's 0x00B1: Attack1 live on
	// 2026-10-16, Attack3 in ps2_main_capture.
	if got := HashResourceID("Attack1"); got != 0xDA9BEA11 {
		t.Errorf("Attack1 = %08X", got)
	}
	if got := HashResourceID("Attack3"); got != 0xDA9BEA13 {
		t.Errorf("Attack3 = %08X", got)
	}
}

func TestSwingReachIsTargetInRange(t *testing.T) {
	var w [21]bool
	if s := SwingFor(0, 30, w, false, 0, 0); s.Reach != 4.5 || s.Never {
		t.Errorf("bare hands: %+v", s)
	}
	if s := SwingFor(0, 30, w, true, 3, 0); s.Reach != 4.5 || s.Never {
		t.Errorf("a sword: %+v", s)
	}
	if s := SwingFor(1, 30, w, true, 9, 30); s.Reach != 30 || s.Never {
		t.Errorf("a bow of range 30: %+v", s)
	}
	if s := SwingFor(0, 30, w, true, 0, 0); !s.Never {
		t.Errorf("attack type 0 is never in reach: %+v", s)
	}
	p := &Player{Swing: Swing{Reach: 4.5}}
	npc := &NPC{X: 7, Y: 0, Z: 0}
	if inReach(p, npc) {
		t.Error("7 units in reach of 4.5")
	}
	// a large target's +3 and a huge one's +5 are its body's (reach_test.go)
	npc.X, npc.Y = 3, 3.5 // 3D: 4.6
	if inReach(p, npc) {
		t.Error("the reach is a 3D distance")
	}
}

type fight struct {
	w     *World
	sess  *session.Session
	p     *Player
	npc   *NPC
	t0    time.Time
	npcID uint32
	me    uint32
}

func newFight(t *testing.T) *fight {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Tester", Level: 20, HP: 500, MaxHP: 500, X: 100, Y: 10, Z: 100, OffMod: 50, WeaponDamage: 10}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x00AA0001)
	npc := &NPC{ServerID: 2002, Name: "Crab", Level: 15, HP: 100000, MaxHP: 100000, X: 102, Y: 10, Z: 100}
	w.NPCs[npc.ServerID] = npc
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = 0x0131_3011
	return &fight{w: w, sess: sess, p: p, npc: npc, t0: time.Unix(1_700_000_000, 0), npcID: 0x0131_3011, me: 0x00AA0001}
}

func (f *fight) tick(at time.Duration) {
	f.w.mu.Lock()
	f.w.attackSystem(f.t0.Add(at))
	f.w.mu.Unlock()
}

// damage is every 0x00DB queued so far: (source, amount, target).
func (f *fight) damage(t *testing.T) [][3]int64 {
	t.Helper()
	var out [][3]int64
	for _, m := range f.sess.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeDamageIndication {
			continue
		}
		r := &vims{b: d[2:]}
		src := int64(r.u32())
		amt := int64(r.signed())
		dst := int64(r.u32())
		out = append(out, [3]int64{src, amt, dst})
	}
	return out
}

type vims struct {
	b []byte
	p int
}

func (r *vims) u32() uint32 {
	v := binary.LittleEndian.Uint32(r.b[r.p:])
	r.p += 4
	return v
}

func (r *vims) signed() int32 {
	var u uint32
	var shift uint
	for {
		c := r.b[r.p]
		r.p++
		u |= uint32(c&0x7F) << shift
		shift += 7
		if c&0x80 == 0 {
			break
		}
	}
	return int32(u>>1) ^ -int32(u&1)
}

var swing6s = Swing{Kind: 5, Delay: 6 * time.Second, Reach: 4.5}

// A tap: byte 36 is back at 0 before the first round begins, so nothing
// lands (Sandstorm, live: a Crab at 255/255 for 25 s).
func TestAutoAttackTapStartsNothing(t *testing.T) {
	f := newFight(t)
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0) {
		t.Fatal("not engaged")
	}
	f.p.AutoAttack = true
	f.tick(100 * time.Millisecond)
	f.p.AutoAttack = false // released 0.2 s later
	f.tick(250 * time.Millisecond)
	f.tick(6 * time.Second)
	if len(f.damage(t)) != 0 || f.npc.HP != f.npc.MaxHP {
		t.Fatalf("a tap hit: %v", f.damage(t))
	}
	// the fight is kept, waiting for the button: no round while it is up
	if f.p.AttackTarget == 0 || f.p.Attacking() {
		t.Errorf("after a tap: engaged %v, attacking %v; want engaged and not attacking", f.p.AttackTarget != 0, f.p.Attacking())
	}
}

// Held: rounds of the swing (Attack4: four hits, 0.5, 1, 1.7 and 2.4 s into a
// round every 6 s), the first round the press's (its hits from the 0x000F,
// as the client's swing, its button read 0.25 s in), each hit a 0x00DB from
// me to the NPC; let go, the round in hand lands whole and no other begins,
// though the NPC fights on (Sandstorm: a bard's hits went on 1.15 s past the
// release, and no further); pressed again, with no 0x000F, the next round
// begins (F3 20261026-020605).
func TestAutoAttackRounds(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.p.AutoAttack = true
	at := func(d time.Duration) int { f.tick(d); return len(f.damage(t)) }
	if at(249*time.Millisecond) != 0 || at(499*time.Millisecond) != 0 {
		t.Fatal("hit before the first round's first hit was due")
	}
	if at(500*time.Millisecond) != 1 {
		t.Fatal("no first hit 0.5 s after the press")
	}
	d := f.damage(t)
	if d[0][0] != int64(f.me) || d[0][1] >= 0 || d[0][2] != int64(f.npcID) {
		t.Fatalf("first hit %v, want (me, -n, crab)", d[0])
	}
	if at(1000*time.Millisecond) != 2 || at(1699*time.Millisecond) != 2 || at(1700*time.Millisecond) != 3 {
		t.Fatal("the second and third hits off their times")
	}
	f.p.AutoAttack = false // released before the round's last hit
	if at(2400*time.Millisecond) != 4 {
		t.Fatal("the round in hand did not land whole after the release")
	}
	if f.npc.CurrentEnemy != f.p.ServerID {
		t.Fatal("the NPC was hit and does not fight back")
	}
	if at(6000*time.Millisecond) != 4 || at(7000*time.Millisecond) != 4 {
		t.Error("a new round with the button up")
	}
	if f.p.AttackTarget == 0 || f.p.Attacking() {
		t.Fatal("the fight was let go with the button up, or still counts as attacking")
	}
	// pressed again between the client's swings: no 0x000F, the round begins
	f.p.AutoAttack = true
	if at(8000*time.Millisecond) != 4 || at(8499*time.Millisecond) != 4 || at(8500*time.Millisecond) != 5 {
		t.Error("pressed again, the next round did not begin with the press's tick (first hit 0.5 s in)")
	}
}

// The round is the class's swing (GetSwing): an enchanter's Attack1 is one
// hit every 3 s, an SK's with a shield Attack3, two hits at 0.5 and 1 s
// every 4 s (Sandstorm's, measured on both).
func TestAutoAttackRoundsByClass(t *testing.T) {
	var primaryShield [21]bool
	primaryShield[14], primaryShield[17] = true, true
	var primary [21]bool
	primary[14] = true
	cases := []struct {
		name  string
		swing Swing
		times []time.Duration // the hits of the first two rounds
	}{
		{"enchanter 49", SwingFor(12, 49, primary, true, 3, 0), []time.Duration{500 * time.Millisecond, 3500 * time.Millisecond}},
		{"shadow knight 50, sword and shield", SwingFor(3, 50, primaryShield, true, 1, 0), []time.Duration{500 * time.Millisecond, 1000 * time.Millisecond, 4500 * time.Millisecond, 5000 * time.Millisecond}},
	}
	for _, c := range cases {
		f := newFight(t)
		f.w.EngageAttack(f.sess, f.npcID, c.swing, f.t0)
		f.p.AutoAttack = true
		for i, want := range c.times {
			f.tick(want - time.Millisecond)
			if n := len(f.damage(t)); n != i {
				t.Errorf("%s: %d hits just before %v, want %d", c.name, n, want, i)
			}
			f.tick(want)
			if n := len(f.damage(t)); n != i+1 {
				t.Errorf("%s: %d hits at %v, want %d", c.name, n, want, i+1)
			}
		}
	}
}

// Out of the server's reach a round's hits land nothing and the rounds keep
// the client's beat (a swing it played: no 0x00D8): the creature 60 off from
// the press, nothing lands and the fight is kept; back in reach at 9.1 s, the
// round due at 12 s lands, not one begun at 9.1 s (which put the damage off
// the client's swings for the rest of the fight). A 0x00D8 (the client's due
// swing out of its reach, none played) calls off the round just begun, and
// the next comes 2 s on.
func TestAutoAttackOutOfReachKeepsTheBeat(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.p.AutoAttack = true
	f.npc.X = 160 // 60 units away
	f.tick(250 * time.Millisecond)
	f.tick(9 * time.Second)
	if len(f.damage(t)) != 0 || f.p.AttackTarget == 0 {
		t.Fatalf("out of reach: %v, engaged %v", f.damage(t), f.p.AttackTarget != 0)
	}
	f.npc.X = 103
	f.tick(9100 * time.Millisecond)
	f.tick(9600 * time.Millisecond)
	f.tick(12400 * time.Millisecond)
	if len(f.damage(t)) != 0 {
		t.Fatal("a round began when the reach came back, off the client's beat")
	}
	f.tick(12500 * time.Millisecond)
	if len(f.damage(t)) != 1 {
		t.Fatal("the round due at 12 s did not land its first hit at 12.5 s")
	}
	// a 0x00D8 just as the round due at 18 s begins: it is called off, and
	// the next comes 2 s after the 0x00D8
	f.tick(18 * time.Second)
	f.w.SwingOutOfReach(f.sess, f.t0.Add(18*time.Second+20*time.Millisecond))
	n := len(f.damage(t))
	for at := 18100 * time.Millisecond; at < 20*time.Second; at += 100 * time.Millisecond {
		f.tick(at)
	}
	if len(f.damage(t)) != n {
		t.Fatal("a round the client did not swing landed a hit")
	}
	f.tick(20520 * time.Millisecond)
	if len(f.damage(t)) != n+1 {
		t.Fatal("no round 2 s after the 0x00D8")
	}
}

// Another 0x000F on the same target starts the rounds from it (CLIENT sends
// one only with its swing timer run out, and starts the timer from the
// press: VIClient::ToggleAutoAttack); a 0x0010 on any other target ends the
// fight.
func TestAutoAttackRetarget(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	press := f.t0.Add(7 * time.Second)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, press)
	if !f.p.NextSwing.Equal(press) || !f.p.ButtonReadAt.Equal(press.Add(firstRound)) || !f.p.RoundStart.IsZero() {
		t.Error("a repeat 0x000F did not start the rounds from its press")
	}
	f.w.StopAttackOnRetarget(f.sess, f.npcID)
	if f.p.AttackTarget == 0 {
		t.Error("re-targeting the same NPC ended the fight")
	}
	f.w.StopAttackOnRetarget(f.sess, 0xFFFFFFFF)
	if f.p.AttackTarget != 0 {
		t.Error("a target change left the fight on")
	}
}

func TestAutoAttackRefusals(t *testing.T) {
	f := newFight(t)
	f.npc.NPCType = 0x81 // a merchant
	if f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0) {
		t.Error("engaged a not-attackable NPC")
	}
	f.npc.NPCType = 0
	f.npc.Dead = true
	if f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0) {
		t.Error("engaged a corpse")
	}
	if f.w.EngageAttack(f.sess, 0x7777, swing6s, f.t0) {
		t.Error("engaged an id nobody sees")
	}
}

func TestAutoAttackKillEndsTheFight(t *testing.T) {
	f := newFight(t)
	f.npc.HP = 1
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.p.AutoAttack = true
	f.tick(250 * time.Millisecond)
	f.tick(750 * time.Millisecond)
	if !f.npc.Dead || f.p.AttackTarget != 0 {
		t.Errorf("dead %v, still engaged %v", f.npc.Dead, f.p.AttackTarget != 0)
	}
	if f.p.TotalXP == 0 {
		t.Error("no XP for the kill")
	}
}

// The NPC's swing as Sandstorm sends it at a round's start: 0x00B1, its
// delay and Attack1 for a creature below Lv20 (rounds.go).
func TestNPCSwingCarriesSandstormsEffect(t *testing.T) {
	f := newFight(t)
	f.npc.CurrentEnemy = f.p.ServerID
	f.w.mu.Lock()
	f.w.aiSystem(f.t0)
	f.w.mu.Unlock()
	for _, m := range f.sess.Queue().PeekPending() {
		d := m.Data()
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeCastingCombat {
			continue
		}
		// caster, target, n, time, effect
		if len(d) != 2+4+4+1+4+4 {
			t.Fatalf("0x00B1 is %d bytes", len(d))
		}
		if c, tg := binary.LittleEndian.Uint32(d[2:]), binary.LittleEndian.Uint32(d[6:]); c != f.npcID || tg != f.me {
			t.Errorf("caster %08X target %08X", c, tg)
		}
		if ms, fx := binary.LittleEndian.Uint32(d[11:]), binary.LittleEndian.Uint32(d[15:]); ms != 3000 || fx != 0xDA9BEA11 {
			t.Errorf("time %d effect %08X, want 3000 and Attack1 (DA9BEA11)", ms, fx)
		}
		return
	}
	t.Fatal("no 0x00B1 for the NPC's swing")
}

// A fight waiting for the button (a release, a tap) is taken up by the next
// press with no 0x000F, as CLIENT sends none for a press between its swings;
// a new 0x000F on another NPC starts that fight on its own press's clock.
// The control: with the old rule (the fight let go when the button was up at
// a round's start), the press after the release lands nothing.
func TestAutoAttackPressAgainWithoutAnother0x000F(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, SwingFor(11, 60, [21]bool{}, false, 0, 0), f.t0) // a necro's Attack1
	f.p.AutoAttack = true
	f.tick(250 * time.Millisecond)
	f.tick(500 * time.Millisecond)
	if len(f.damage(t)) != 1 {
		t.Fatal("no first hit")
	}
	f.p.AutoAttack = false // the button let go (a toggle pressed off, a release)
	f.tick(3000 * time.Millisecond)
	f.tick(3100 * time.Millisecond)
	f.p.AutoAttack = true // and on again: the client swings, no 0x000F
	for at := 3200 * time.Millisecond; at <= 12*time.Second; at += 100 * time.Millisecond {
		f.tick(at)
	}
	// rounds every 3 s from the press at 3.2 s: hits at 3.7, 6.7, 9.7
	if n := len(f.damage(t)); n != 4 {
		t.Fatalf("%d hits after pressing again, want 4 (the first and three more)", n)
	}

	// another NPC: its own first round, from its 0x000F, read 0.25 s in
	other := &NPC{ServerID: 2003, Name: "Crab", Level: 15, HP: 100000, MaxHP: 100000, X: 101, Y: 10, Z: 101}
	f.w.NPCs[other.ServerID] = other
	vs := GetOrCreateVisibility(f.sess)
	vs.ChannelEntity[1] = other.ServerID
	vs.ChannelObjectID[1] = 0x0131_3012
	f.p.AutoAttack = false
	f.tick(12100 * time.Millisecond) // waiting again
	f.w.EngageAttack(f.sess, 0x0131_3012, f.p.Swing, f.t0.Add(13*time.Second))
	if !f.p.Attacking() {
		t.Error("a fresh 0x000F does not count as attacking before its button is read")
	}
	f.p.AutoAttack = true
	f.tick(13250 * time.Millisecond)
	f.tick(13499 * time.Millisecond)
	before := len(f.damage(t))
	f.tick(13500 * time.Millisecond)
	if len(f.damage(t)) != before+1 || other.HP == other.MaxHP {
		t.Error("the new fight's first hit did not come 0.5 s after its 0x000F")
	}
}
