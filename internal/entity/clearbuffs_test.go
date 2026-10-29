package entity

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// buffRig is a level 50 player wearing every kind of effect a player can
// carry here, each put on as a landing puts it (addEffectLocked): a stat
// buff, a speed buff, invisibility, an illusion, the Eye line's power
// ceiling, a pact's price on its caster (a harmful one), a meal, and one
// slot #effect put on with no effect behind it.
type buffRig struct {
	w    *World
	sess *session.Session
	p    *Player
	t0   time.Time
}

func newBuffRig(t *testing.T, port int) *buffRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 3, 1), Port: port}, uint32(port), 1, 1, 2, true)
	p := &Player{ServerID: EntityID(port), Name: "Jibrish", Class: classEnchanter, Level: 50, HP: 900, MaxHP: 900,
		Power: 800, MaxPower: 800, Speed: RunSpeed, ModelID: 0x6AB9C990, Size: 1, X: 100, Y: 50, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x10000000 + uint32(port))
	r := &buffRig{w: w, sess: sess, p: p, t0: time.Unix(1_800_000_000, 0)}
	until := r.t0.Add(30 * time.Minute)
	w.mu.Lock()
	for _, ae := range []*ActiveEffect{
		{SpellID: 101, Name: "Rockskin", Icon: 0x1111, Until: until, Mods: StatMods{AC: 150, MaxHP: 30, MaxPower: 60}},
		{SpellID: 102, Name: "Spirit of the Wolf", Icon: 0x2222, Until: until, Speed: true},
		{SpellID: 103, Name: "Camouflage", Icon: 0x3333, Until: until, Invis: true},
		{SpellID: 104, Name: "Mouse Form", Icon: 0x4444, Until: until, Form: 0x0E030048, FormSize: 0.3},
		{SpellID: 105, Name: "Raven Eye", Icon: 0x5555, Until: until, Mods: StatMods{Wis: 140}, PowerCap: 0, PowerCapped: true},
		{SpellID: -106, Name: "Infernal Pact", Icon: 0x6666, Until: until, Mods: StatMods{Str: -10}},
		{SpellID: FoodEffectID, Name: "Field Rations", Icon: 0x7777, Until: until, Mods: StatMods{HoT: 5}},
	} {
		w.addEffectLocked(p, ae, r.t0)
	}
	p.HP = p.MaxHPNow() // at the top of what Rockskin gives
	w.mu.Unlock()
	if !w.AddEffect(sess, BuffSlot{Icon: 0x8888, Name: "a GM's slot"}) {
		t.Fatal("no room for #effect's slot")
	}
	sess.Queue().GetPending()
	p.StatsDirty = false
	return r
}

func (r *buffRig) sent() [][]byte {
	var out [][]byte
	for _, m := range r.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// F3 20261029-011146 (jib: "a GM Test command to clear all buffs on the
// character issuing it"): ClearBuffs ends every effect at once as its time
// running out ends it: the same player and the same messages as effectSystem
// past every effect's end (the run speed's 0x00F8, nothing else; the
// channels on the next tick), the harmful price with the rest, the meal too,
// and #effect's slot besides; the names said in their order.
func TestClearBuffsEndsEveryEffectAsItsTimeDoes(t *testing.T) {
	natural := newBuffRig(t, 7601)
	natural.w.mu.Lock()
	natural.w.effectSystem(natural.t0.Add(31 * time.Minute))
	natural.w.mu.Unlock()
	wantSent := natural.sent()

	r := newBuffRig(t, 7601)
	if !r.p.Invisible() || r.p.MaxPowerNow() != 0 || r.p.Mods.Str != -10 || r.p.SpeedNow() == RunSpeed {
		t.Fatalf("the rig wears nothing: invisible %v, max power %d, STR %+d, speed %v", r.p.Invisible(), r.p.MaxPowerNow(), r.p.Mods.Str, r.p.SpeedNow())
	}
	names, slots, ok := r.w.ClearBuffs(r.sess)
	want := []string{"Rockskin", "Spirit of the Wolf", "Camouflage", "Mouse Form", "Raven Eye", "Infernal Pact", "Field Rations"}
	if !ok || slots != 1 || len(names) != len(want) {
		t.Fatalf("ClearBuffs: %q, %d slots, %v", names, slots, ok)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("name %d: %q, want %q", i, names[i], want[i])
		}
	}
	got := r.sent()
	if len(got) != len(wantSent) || len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodePlayerSpeed {
		t.Fatalf("sent %x, the natural end %x (want its one 0x00F8)", got, wantSent)
	}
	for i := range got {
		if !bytes.Equal(got[i], wantSent[i]) {
			t.Errorf("message %d: %x, the natural end's %x", i, got[i], wantSent[i])
		}
	}
	a, b := natural.p, r.p
	if len(b.Effects) != 0 || len(b.Buffs) != 0 || b.Mods != (StatMods{}) || !b.StatsDirty {
		t.Errorf("left: effects %d, slots %v, mods %+v, dirty %v", len(b.Effects), b.Buffs, b.Mods, b.StatsDirty)
	}
	if b.HP != a.HP || b.Power != a.Power || b.MaxHPNow() != a.MaxHPNow() || b.MaxPowerNow() != a.MaxPowerNow() ||
		b.SpeedNow() != a.SpeedNow() || b.Invisible() || a.Invisible() {
		t.Errorf("cleared HP %d/%d power %d/%d speed %v; the natural end's HP %d/%d power %d/%d speed %v",
			b.HP, b.MaxHPNow(), b.Power, b.MaxPowerNow(), b.SpeedNow(), a.HP, a.MaxHPNow(), a.Power, a.MaxPowerNow(), a.SpeedNow())
	}
	if model, size := b.Body(); model != uint32(b.ModelID) || size != b.Size {
		t.Errorf("the illusion's body stayed: %#x at %v", model, size)
	}
	if b.HP != 900 || b.Power != 0 || b.SpeedNow() != RunSpeed {
		t.Errorf("HP %d (want the 900 Rockskin no longer raises), power %d (where the ceiling left it), speed %v", b.HP, b.Power, b.SpeedNow())
	}
	// a logout now keeps none of them
	if rows, ok := r.w.SavedEffects(r.p.ServerID, r.t0.Add(time.Minute)); !ok || len(rows) != 0 {
		t.Errorf("kept over a logout: %d rows, %v", len(rows), ok)
	}
	// again: nothing on, nothing said
	if names, slots, ok := r.w.ClearBuffs(r.sess); !ok || len(names) != 0 || slots != 0 || len(r.sent()) != 0 {
		t.Errorf("a second clear: %q, %d slots, %v", names, slots, ok)
	}
}

// The GM's own alone: another player's effects stay on (the control), and a
// heal over time cleared gives no last tick (its time is not up), where its
// natural end does give one.
func TestClearBuffsIsTheIssuersAlone(t *testing.T) {
	r := newBuffRig(t, 7602)
	other := &Player{ServerID: 7603, Name: "Bystander", Level: 50, HP: 900, MaxHP: 900, Speed: RunSpeed}
	r.w.mu.Lock()
	r.w.Players[other.ServerID] = other
	r.w.addEffectLocked(other, &ActiveEffect{SpellID: 101, Name: "Rockskin", Icon: 0x1111, Caster: r.p.ServerID,
		Until: r.t0.Add(30 * time.Minute), Mods: StatMods{AC: 150}}, r.t0)
	hot := &ActiveEffect{SpellID: 107, Name: "Minor Healing", Icon: 0x9999, Caster: r.p.ServerID,
		Until: r.t0.Add(12 * time.Second), NextTick: r.t0.Add(12 * time.Second), HPTick: 35, Ticks: 1}
	r.w.addEffectLocked(r.p, hot, r.t0)
	r.p.HP = 100
	r.w.mu.Unlock()
	r.sent()
	if _, _, ok := r.w.ClearBuffs(r.sess); !ok {
		t.Fatal("no player")
	}
	if len(other.Effects) != 1 || other.Mods.AC != 150 || len(other.Buffs) != 1 {
		t.Errorf("the bystander lost its Rockskin: %d effects, AC %+d", len(other.Effects), other.Mods.AC)
	}
	if r.p.HP != 100 {
		t.Errorf("the cleared heal over time healed: HP %d", r.p.HP)
	}
	for _, m := range r.sent() {
		if opOfMsg(m) == protocol.OpcodeDamageIndication {
			t.Errorf("a 0x00DB on a clear: %x", m)
		}
	}
	// the natural end of the same heal ticks its last
	n := newBuffRig(t, 7604)
	n.w.mu.Lock()
	n.w.addEffectLocked(n.p, &ActiveEffect{SpellID: 107, Name: "Minor Healing", Icon: 0x9999, Caster: n.p.ServerID,
		Until: n.t0.Add(12 * time.Second), NextTick: n.t0.Add(12 * time.Second), HPTick: 35, Ticks: 1}, n.t0)
	n.p.HP = 100
	n.w.effectSystem(n.t0.Add(12 * time.Second))
	n.w.mu.Unlock()
	if n.p.HP != 135 {
		t.Errorf("the control: the heal's natural end left HP %d, want its last tick's 135", n.p.HP)
	}
	if _, _, ok := r.w.ClearBuffs(nil); ok {
		t.Error("no session, and a player was cleared")
	}
}
