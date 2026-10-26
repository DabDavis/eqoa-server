package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The enchanter's proc line (F3 20261025-165245): Chaotic Flux, Mark and
// Glyph, as this server keeps them (spellpattern 522, 541, 571; their lines
// EQOA-Data's in spelleffect).

const (
	fluxLine = "Target, Decrease Hitpoints by 64, 5min"
	fluxDesc = "Imbues your attacks with chaotic arcane energy, giving them a chance to deal additional arcane damage."
)

func chaoticFlux() CastSpell {
	return CastSpell{SpellID: 522, DisplayIndex: 9, Level: 16, Scope: 1, Name: "Chaotic Flux", Icon: 0x1A2B3C4D,
		CastTime: time.Second, Recast: time.Second, PowerCost: 40, Range: 30, Def: ParseSpell(fluxLine, fluxDesc)}
}

// dice is a procIntn that answers its rolls in turn: a chance roll under the
// chance procs, a resist roll at or over the resist does not resist.
func dice(rolls ...int) func(int) int {
	i := 0
	return func(n int) int {
		r := rolls[i%len(rolls)]
		i++
		return min(r, n-1)
	}
}

// flatStat is a bearer whose every stat is n.
func flatStat(n int) func(Stat) int {
	return func(Stat) int { return n }
}

// A hit whose description names a proc is the proc's damage, on a friend,
// for its duration; one whose description does not, or a line with no hit,
// is as it was. The control: the line alone is a five-minute hit, harmful,
// which a friend refuses ("Target is friendly.", the report's).
func TestTheChaoticLineIsAProc(t *testing.T) {
	for _, c := range []struct {
		line, desc string
		dmg        int
	}{
		{fluxLine, fluxDesc, 64},
		{"Target, Decrease Hitpoints by 96, 5min", "Arcane Offensive Proc", 96},
		{"Target, Decrease Hitpoints by 156, 5min", "Arcane Offensive Proc", 156},
	} {
		d := ParseSpell(c.line, c.desc)
		if d.Harmful || d.Target != TargetSingle || d.Duration != 5*time.Minute || len(d.Effects) != 1 ||
			d.Effects[0] != (SpellEffect{Kind: EffectProc, Amount: c.dmg, AmountMax: c.dmg}) {
			t.Errorf("%q / %q: %+v", c.line, c.desc, d)
		}
	}
	if d := ParseSpellText(fluxLine); !d.Harmful {
		t.Errorf("the control: the line alone reads %+v, want a harmful hit", d)
	}
	if d := ParseSpell("Target, Decrease Hitpoints by 74 ; Increase 10% based on DEX",
		"Delivers arcane based damage to the target. Modified by DEX."); !d.Harmful || d.Effects[0].Kind != EffectHP {
		t.Errorf("Chaotic Feedback, a nuke: %+v", d)
	}
	if d := ParseSpell("Pet, Increase INT by 35, Add Offensive Proc:, Add DS, 10min", "Arcane Offensive Proc"); d.Harmful ||
		len(d.Effects) != 1 || d.Effects[0].Kind != EffectStat {
		t.Errorf("Burning Mark, no damage named: %+v", d)
	}
}

// The chance grows with INT to a cap; the resist with the creature's level
// over the bearer's, from none to a cap.
func TestProcChanceAndResist(t *testing.T) {
	for intel, want := range map[int]int{0: 3, 100: 8, 300: 18, 440: 25, 2000: 25} {
		if got := procChancePct(intel); got != want {
			t.Errorf("INT %d: %d%%, want %d%%", intel, got, want)
		}
	}
	for _, c := range []struct{ bearer, target, want int }{{20, 20, 5}, {20, 25, 20}, {20, 18, 0}, {10, 60, 75}} {
		if got := procResistPct(c.bearer, c.target); got != c.want {
			t.Errorf("a %d on a %d: %d%%, want %d%%", c.bearer, c.target, got, c.want)
		}
	}
}

// procRig is an enchanter of 20 (INT 140), a warrior friend of 20 in its
// view, each with a session, and a creature both see.
type procRig struct {
	*caster
	fsess  *session.Session
	friend *Player
	fid    uint32 // the friend as the enchanter knows it
	npcF   uint32 // the creature as the friend knows it
}

func newProcRig(t *testing.T) *procRig {
	c := newCaster(t)
	c.p.Class, c.p.Level, c.p.Int = classEnchanter, 20, 140
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 20, 5000, 5000
	fsess := c.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 2, 2, 2, 3, true)
	friend := &Player{ServerID: 1002, Name: "Tank", Class: 0, Level: 20, HP: 900, MaxHP: 900, Str: 100, Dex: 80, Int: 60,
		Speed: 8.833333, X: 101, Y: 10, Z: 100}
	c.w.Players[friend.ServerID] = friend
	fsess.SetECSEntity(friend)
	fsess.SetCharacterInWorld(true)
	fsess.SetPlayerObjectID(0x0140AAA0)
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = friend.ServerID, 0x0140AAA1
	fv := GetOrCreateVisibility(fsess)
	fv.ChannelEntity[0], fv.ChannelObjectID[0] = c.npc.ServerID, 0x01312EA0
	return &procRig{caster: c, fsess: fsess, friend: friend, fid: 0x0140AAA1, npcF: 0x01312EA0}
}

// figures is each 0x00DB a session was sent: its amount.
func figures(s *session.Session) []int {
	var out []int
	for _, m := range s.Queue().GetPending() {
		b := m.Data()
		if len(b) >= 7 && protocol.GameOpcode(binary.LittleEndian.Uint16(b)) == protocol.OpcodeDamageIndication {
			out = append(out, int(int32(zigzagDecode(b[6:]))))
		}
	}
	return out
}

func zigzagDecode(b []byte) uint32 {
	var v uint64
	for i, s := 0, uint(0); i < len(b) && i < 10; i, s = i+1, s+7 {
		v |= uint64(b[i]&0x7F) << s
		if b[i]&0x80 == 0 {
			break
		}
	}
	return uint32(int64(v>>1) ^ -int64(v&1))
}

// Chaotic Flux lands on a friend (it was "Target is friendly."), shown on
// its effects channel with the proc's damage kept; a creature refuses it as
// any friend's spell. The friend's melee then procs 64, told to the friend;
// a roll over the chance does not proc, nor one the creature resists.
func TestChaoticFluxLandsOnAFriendAndProcs(t *testing.T) {
	r := newProcRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	if !r.w.StartCast(r.sess, chaoticFlux(), r.fid, r.at(0)) {
		t.Fatalf("Chaotic Flux on a friend did not begin: %x", r.sent())
	}
	r.tick(time.Second + castLand)
	if len(r.friend.Effects) != 1 || r.friend.Effects[0].Proc != 64 || len(r.friend.Buffs) != 1 || r.friend.Buffs[0].Name != "Chaotic Flux" {
		t.Fatalf("on the friend: %+v, buffs %+v", r.friend.Effects, r.friend.Buffs)
	}
	if r.w.StartCast(r.sess, chaoticFlux(), r.npcO, r.at(10*time.Second)) {
		t.Error("cast at a creature")
	}

	hp := r.npc.HP
	r.fsess.Queue().GetPending()
	procIntn = dice(0, 99) // under the chance, over the resist
	r.w.mu.Lock()
	r.w.playerProcsLocked(r.fsess, r.friend, r.npc, r.at(20*time.Second))
	r.w.mu.Unlock()
	if r.npc.HP != hp-64 {
		t.Errorf("the proc took %d, want 64", hp-r.npc.HP)
	}
	if f := figures(r.fsess); len(f) != 1 || f[0] != -64 {
		t.Errorf("the friend was told %v, want [-64]", f)
	}
	for name, roll := range map[string][]int{"over the chance": {99, 99}, "resisted": {0, 0}} {
		procIntn = dice(roll...)
		hp = r.npc.HP
		r.w.mu.Lock()
		r.w.playerProcsLocked(r.fsess, r.friend, r.npc, r.at(21*time.Second))
		r.w.mu.Unlock()
		if r.npc.HP != hp {
			t.Errorf("%s: it took %d", name, hp-r.npc.HP)
		}
	}
	// over at its end
	procIntn = dice(0, 99)
	hp = r.npc.HP
	r.w.mu.Lock()
	r.w.playerProcsLocked(r.fsess, r.friend, r.npc, r.at(time.Second+castLand+5*time.Minute))
	r.w.mu.Unlock()
	if r.npc.HP != hp {
		t.Errorf("a proc after its five minutes took %d", hp-r.npc.HP)
	}
}

// The friend's own melee hit procs after its damage: two figures, the
// melee's and the proc's.
func TestAMeleeHitProcs(t *testing.T) {
	r := newProcRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	r.friend.Effects = []*ActiveEffect{{SpellID: 522, Name: "Chaotic Flux", Proc: 64, ProcMax: 64, Until: time.Now().Add(time.Minute)}}
	procIntn = dice(0, 99)
	r.fsess.Queue().GetPending()
	r.w.mu.Lock()
	killed := r.w.playerHitLocked(r.fsess, r.friend, r.npc, r.npcF)
	r.w.mu.Unlock()
	f := figures(r.fsess)
	if killed || len(f) != 2 || f[1] != -64 || r.npc.HP != r.npc.MaxHP+f[0]+f[1] {
		t.Errorf("figures %v, HP %d/%d", f, r.npc.HP, r.npc.MaxHP)
	}
	r.friend.Effects = nil
	r.w.mu.Lock()
	r.w.playerHitLocked(r.fsess, r.friend, r.npc, r.npcF)
	r.w.mu.Unlock()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("the control, no proc on: figures %v", f)
	}
}

// On a pet: it lands (a scope-1 spell at the pet, as on a friend), and the
// pet's hit procs, its owner told as of the pet's hits.
func TestAPetsHitProcs(t *testing.T) {
	r := newPetRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	pet, _ := r.summon(t, fireElemental(), 0)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	flux := chaoticFlux()
	if !r.w.StartCast(r.sess, flux, petID, r.at(20*time.Second)) {
		t.Fatalf("Chaotic Flux on the pet did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(20*time.Second + flux.CastTime + castLand))
	r.w.mu.Unlock()
	if len(pet.Effects) != 1 || pet.Effects[0].Proc != 64 {
		t.Fatalf("on the pet: %+v", pet.Effects)
	}
	procIntn = dice(0, 99)
	r.sent()
	r.w.mu.Lock()
	ps := r.w.petStateLocked(r.p)
	pet.Effects[0].Until = r.at(10 * time.Minute)
	hp := r.enemy.HP
	r.w.petHitLocked(ps, r.enemy, r.at(30*time.Second))
	r.w.mu.Unlock()
	f := figures(r.sess)
	if len(f) != 2 || f[1] != -64 || hp-r.enemy.HP != -f[0]+64 {
		t.Errorf("the owner was told %v; the crab lost %d", f, hp-r.enemy.HP)
	}
}

// Kept over a logout: the proc's damage rides in the mods column; a row
// kept before procs reads as none.
func TestAProcIsKeptOverALogout(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	p := &Player{Effects: []*ActiveEffect{{SpellID: 522, Name: "Chaotic Flux", Proc: 64, ProcMax: 64, Until: now.Add(time.Minute)}}}
	rows := savedEffects(p, now)
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7000}, 1, 1, 1, 2, true)
	q := &Player{ServerID: 5}
	w.Players[5] = q
	s.SetECSEntity(q)
	if n := w.RestoreEffects(s, rows, now); n != 1 || q.Effects[0].Proc != 64 || q.Effects[0].ProcMax != 64 {
		t.Fatalf("restored %d: %+v", n, q.Effects)
	}
	old := rows[0]
	old.Mods, old.SpellID = `{"Str":5}`, 99
	if w.RestoreEffects(s, append(rows[:0:0], old), now); q.Effects[1].Proc != 0 || q.Effects[1].Mods.Str != 5 {
		t.Errorf("an old row: %+v", q.Effects[1])
	}
}
