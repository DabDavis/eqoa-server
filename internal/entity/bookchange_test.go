package entity

import (
	"encoding/binary"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// bookRig is a level 50 enchanter in the world with three effects on it and
// four lockouts running or over, at t0.
type bookRig struct {
	w    *World
	sess *session.Session
	p    *Player
	t0   time.Time
}

const (
	bookExpansive = 700328 // a self buff of the path that goes
	bookPowerJet  = 700340 // the path that comes
	bookLimitless = 700462
	bookWard      = 700339 // another's ward on the player, its spell going too
	bookRockskin  = 101    // a spell that stays
)

func newBookRig(t *testing.T, port int) *bookRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 3, 2), Port: port}, uint32(port), 1, 1, 2, true)
	p := &Player{ServerID: EntityID(port), Name: "Jibrish", Class: classEnchanter, Level: 50, HP: 900, MaxHP: 900,
		Power: 800, MaxPower: 800, Speed: RunSpeed, ModelID: 0x6AB9C990, Size: 1, X: 100, Y: 50, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	r := &bookRig{w: w, sess: sess, p: p, t0: time.Unix(1_800_000_000, 0)}
	until := r.t0.Add(20 * time.Minute)
	w.mu.Lock()
	for _, ae := range []*ActiveEffect{
		{SpellID: bookExpansive, Name: "Expansive Mind", Icon: 0x1111, Caster: p.ServerID, Until: until, Mods: StatMods{MaxPower: 260}},
		{SpellID: bookWard, Name: "Poison Ward", Icon: 0x2222, Caster: 4242, Until: until, Mods: StatMods{PR: 100}},
		{SpellID: bookRockskin, Name: "Rockskin", Icon: 0x3333, Caster: p.ServerID, Until: until, Mods: StatMods{AC: 150}},
	} {
		w.addEffectLocked(p, ae, r.t0)
	}
	p.Lockouts = []castLockout{
		{spellID: bookExpansive, displayIndex: 2, until: r.t0.Add(5 * time.Minute)}, // goes, running
		{spellID: bookPowerJet, displayIndex: 6, until: r.t0.Add(time.Minute)},      // comes back to 2, running
		{spellID: bookRockskin, displayIndex: 1, until: r.t0.Add(30 * time.Second)}, // stays
		{spellID: bookLimitless, displayIndex: 9, until: r.t0.Add(-time.Second)},    // comes back to 3, over
	}
	w.mu.Unlock()
	sess.Queue().GetPending()
	return r
}

// cooldowns is the 0x00CD messages sent since the last look, as index and
// milliseconds left; the rest are dropped.
func (r *bookRig) cooldowns() [][2]int64 {
	var out [][2]int64
	for _, m := range r.sess.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeCooldownTimer {
			continue
		}
		rd := buffer.NewReader(d[2:])
		index, _ := rd.ReadByte()
		rd.ReadUint32()
		ms, _ := rd.Read7BitEncodedInt64()
		out = append(out, [2]int64{int64(index), ms})
	}
	return out
}

func (r *bookRig) effectNames() []string {
	var out []string
	for _, e := range r.p.Effects {
		out = append(out, e.Name)
	}
	return out
}

// A book change (petition 1325's path change): the effect the player put on
// itself with a spell gone ends as #clearbuffs ends one (its stat change off),
// while one another put on it with a spell gone too, and the player's own of a
// spell that stayed, run on. A lockout of a spell gone keeps running and shows
// nowhere: SendLockouts leaves it out and its end says nothing, so the spell
// is not cast before its time if it comes back; one of a spell come is shown
// at its new index at once with what is left, and its end said there; one
// over is not shown, its end said at the new index too. The controls: a
// lockout of a spell that stayed is shown and ended where it was.
func TestABookChangeKeepsTheLockoutsAndEndsTheSpellsOwnEffects(t *testing.T) {
	r := newBookRig(t, 7611)
	now := r.t0.Add(time.Second)
	ended, shown := r.w.BookChanged(r.sess, []int{bookExpansive, bookWard}, map[int]int{bookPowerJet: 2, bookLimitless: 3}, now)
	if !slices.Equal(ended, []string{"Expansive Mind"}) || shown != 1 {
		t.Errorf("ended %q, shown %d; want Expansive Mind alone and one lockout shown", ended, shown)
	}
	if got := r.effectNames(); !slices.Equal(got, []string{"Poison Ward", "Rockskin"}) {
		t.Errorf("the effects left: %q, want another's ward and the spell that stayed", got)
	}
	if r.p.Mods.MaxPower != 0 || r.p.Mods.AC != 150 || r.p.Mods.PR != 100 {
		t.Errorf("the stats summed again: %+v", r.p.Mods)
	}
	if got := r.cooldowns(); len(got) != 1 || got[0][0] != 2 || got[0][1] != (59*time.Second).Milliseconds() {
		t.Errorf("0x00CD sent %v, want Power Jet's 59 s at its new index 2", got)
	}
	if n := r.w.SendLockouts(r.sess, now.Add(time.Second)); n != 2 {
		t.Errorf("SendLockouts sent %d, want Power Jet's and Rockskin's (not the gone Expansive Mind's)", n)
	}
	if got := r.cooldowns(); len(got) != 2 || got[0][0] != 2 || got[1][0] != 1 {
		t.Errorf("SendLockouts' 0x00CD %v, want indices 2 and 1", got)
	}

	// the ends: Limitless Mind's (over) and Rockskin's at the next tick, at 3
	// and 1; Power Jet's at 2; Expansive Mind's nowhere, though it still runs
	r.w.mu.Lock()
	r.w.castSystem(now)
	r.w.mu.Unlock()
	if got := r.cooldowns(); !slices.Equal(got, [][2]int64{{3, 0}}) {
		t.Errorf("the next tick's ends %v, want the over lockout's at its new index 3", got)
	}
	r.w.mu.Lock()
	r.w.castSystem(r.t0.Add(2 * time.Minute))
	r.w.mu.Unlock()
	if got := r.cooldowns(); !slices.Equal(got, [][2]int64{{2, 0}, {1, 0}}) {
		t.Errorf("two minutes on %v, want Power Jet's end at 2 and Rockskin's at 1", got)
	}
	var running bool
	for _, l := range r.p.Lockouts {
		running = running || (l.spellID == bookExpansive && l.displayIndex == -1)
	}
	if !running {
		t.Errorf("Expansive Mind's lockout: %+v, want it running with no index", r.p.Lockouts)
	}
	waits, _ := r.p.castWaitFor(&CastSpell{SpellID: bookExpansive}, r.t0.Add(2*time.Minute))
	if waits != castLocked {
		t.Errorf("Expansive Mind given back within its lockout: %v, want it locked out", waits)
	}
	r.w.mu.Lock()
	r.w.castSystem(r.t0.Add(6 * time.Minute))
	r.w.mu.Unlock()
	if got := r.cooldowns(); len(got) != 0 || len(r.p.Lockouts) != 0 {
		t.Errorf("its end: %v sent, %d lockouts left; want nothing sent and none left", got, len(r.p.Lockouts))
	}
}

// Casting is whether a cast is on its way: begun and not landed.
func TestCastingIsACastOnItsWay(t *testing.T) {
	r := newBookRig(t, 7612)
	if r.w.Casting(r.sess) {
		t.Fatal("casting with no cast")
	}
	r.p.Casting = &castState{}
	if !r.w.Casting(r.sess) {
		t.Error("not casting with a cast on")
	}
}
