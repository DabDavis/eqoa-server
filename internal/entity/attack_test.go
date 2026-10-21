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
	npc.NPCType = 0x400
	if !inReach(p, npc) {
		t.Error("a 0x400 target adds 3")
	}
	npc.NPCType, npc.X = 0x800, 9.4
	if !inReach(p, npc) {
		t.Error("a 0x800 target adds 5")
	}
	npc.NPCType, npc.X, npc.Y = 0, 3, 3.5 // 3D: 4.6
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

// A tap: byte 36 is back at 0 long before the first swing, and the NPC was
// never hit, so nothing lands (Sandstorm, live: a Crab at 255/255 for 25 s).
func TestAutoAttackTapStartsNothing(t *testing.T) {
	f := newFight(t)
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0) {
		t.Fatal("not engaged")
	}
	f.p.AutoAttack = false // released 0.2 s later
	f.tick(6 * time.Second)
	if len(f.damage(t)) != 0 || f.npc.HP != f.npc.MaxHP {
		t.Fatalf("a tap hit: %v", f.damage(t))
	}
	if f.p.AttackTarget != 0 {
		t.Error("the engagement outlived its first swing")
	}
}

// Held: the first hit one swing delay after the 0x000F, as 0x00DB from me to
// the NPC, the next one delay later; the NPC fights back; and the fight goes
// on after the release while it does (Sandstorm, live: 13 hits, two of them
// after the release).
func TestAutoAttackHeldThenReleased(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.p.AutoAttack = true
	f.tick(5999 * time.Millisecond)
	if len(f.damage(t)) != 0 {
		t.Fatal("hit before the first swing was due")
	}
	f.tick(6 * time.Second)
	d := f.damage(t)
	if len(d) != 1 || d[0][0] != int64(f.me) || d[0][1] >= 0 || d[0][2] != int64(f.npcID) {
		t.Fatalf("first swing: %v, want one (me, -n, crab)", d)
	}
	if f.npc.CurrentEnemy != f.p.ServerID {
		t.Fatal("the NPC was hit and does not fight back")
	}
	f.p.AutoAttack = false // released; the Crab fights on
	f.tick(11 * time.Second)
	if len(f.damage(t)) != 1 {
		t.Fatal("the second swing came early")
	}
	f.tick(12 * time.Second)
	if len(f.damage(t)) != 2 {
		t.Fatal("no swing after the release while the NPC fights back")
	}
	f.npc.CurrentEnemy = 0 // it leashed
	f.tick(18 * time.Second)
	if len(f.damage(t)) != 2 || f.p.AttackTarget != 0 {
		t.Error("swung at an NPC that stopped fighting, with the button up")
	}
}

// Out of reach the due swing waits; in reach it lands at once.
func TestAutoAttackWaitsForReach(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	f.p.AutoAttack = true
	f.npc.X = 160 // 60 units away
	f.tick(6 * time.Second)
	f.tick(9 * time.Second)
	if len(f.damage(t)) != 0 || f.p.AttackTarget == 0 {
		t.Fatalf("out of reach: %v, engaged %v", f.damage(t), f.p.AttackTarget != 0)
	}
	f.npc.X = 103
	f.tick(9100 * time.Millisecond)
	if len(f.damage(t)) != 1 {
		t.Fatal("the waiting swing did not land in reach")
	}
}

// Another 0x000F on the same target keeps the due swing; a 0x0010 on any
// other target ends the fight.
func TestAutoAttackRetarget(t *testing.T) {
	f := newFight(t)
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0)
	due := f.p.NextSwing
	f.w.EngageAttack(f.sess, f.npcID, swing6s, f.t0.Add(3*time.Second))
	if f.p.NextSwing != due {
		t.Error("a repeat 0x000F pushed the swing back")
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
	f.tick(6 * time.Second)
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
