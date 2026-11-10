package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1155, Larrybird: Shroud of Hate "was procing for 1 dmg", every one of its 390
// answers in the log. Its record says 1 and the level's whole share; the shield read the 1
// alone. It goes by its bearer's level, as measured on Sandstorm (52-53 at 50). The workbook's
// other level-scaled shields and procs take their share too; one with none is as it was.
func TestShieldsAndProcsTakeTheirLevelShare(t *testing.T) {
	cast := func(name string) *castState { return &castState{spell: CastSpell{Name: name, Level: 6}} }
	p := &Player{Level: 60}
	var none *Shield
	if s := none.with(SpellEffect{Kind: EffectShield, Amount: 1, AmountMax: 1, LevelPct: 100}, cast("Shroud of Hate"), p); s.Min != 62 || s.Max != 63 {
		t.Errorf("Shroud of Hate at 60: %d to %d, want 62 to 63", s.Min, s.Max)
	}
	if s := none.with(SpellEffect{Kind: EffectShield, Amount: 2, AmountMax: 2, LevelPct: 100}, cast("Divine Protection"), p); s.Min != 62 || s.Max != 62 {
		t.Errorf("Divine Protection at 60: %d to %d, want 62", s.Min, s.Max)
	}
	if s := none.with(SpellEffect{Kind: EffectShield, Amount: 10, AmountMax: 10, LevelPct: 80}, cast("O'Keils Embers"), p); s.Min != 58 {
		t.Errorf("O'Keils Embers at 60: %d, want 58", s.Min)
	}
	if s := none.with(SpellEffect{Kind: EffectShield, Amount: 4, AmountMax: 4}, cast("Thistlecoat"), p); s.Min != 4 || s.Max != 4 {
		t.Errorf("a shield with no level share: %d to %d, want 4", s.Min, s.Max)
	}
	if n := procBonus(SpellEffect{Kind: EffectProc, Amount: 10, LevelPct: 200}, p); n != 120 {
		t.Errorf("Flametongue's share at 60: %d, want 120", n)
	}
	if n := procBonus(SpellEffect{Kind: EffectProc, Amount: 10, LevelPct: 200}, nil); n != 0 {
		t.Errorf("no caster: %d, want none", n)
	}
}

// Petition 1156, Jibrish: Shroud of Death + "does not proc": the log has it procing, and all
// it showed was one more figure among the swings'. A proc plays the shield's flash at the
// creature. The control: a swing with no proc on plays none.
func TestAProcIsSeen(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.HP, guard.MaxHP = 1_000_000, 1_000_000
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = func(n int) int { return 0 }
	hit := func() int {
		drain(sess)
		r.w.mu.Lock()
		r.w.playerProcsLocked(sess, p, guard, time.Now())
		r.w.mu.Unlock()
		n := 0
		for _, c := range castsIn(drain(sess)) {
			if c.fx == fxShieldHit {
				n++
			}
		}
		return n
	}
	if n := hit(); n != 0 {
		t.Errorf("no proc on: %d flashes", n)
	}
	p.Effects = append(p.Effects, &ActiveEffect{Name: "Shroud of Death +", Proc: 112, ProcMax: 112, Until: time.Now().Add(time.Hour)})
	guard.Level = 1 // a creature far below resists none of it, so the dice's 0 lands it
	if n := hit(); n != 1 {
		t.Errorf("a proc: %d flashes, want 1", n)
	}
}

// Veil of Death's and Shroud of Death's proc is a life steal: one roll, its damage and its heal on the same hit,
// not two rolls that met by chance (petition 1156; Arcana's log: heal, then a hit 15 s on). The controls: a roll
// that misses does neither, a resisted one does neither, and a proc with no heal only hits.
func TestALifeStealProcsAsOne(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.HP, guard.MaxHP = 1_000_000, 1_000_000
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	until := time.Now().Add(time.Hour)
	steal := &ActiveEffect{Name: "Shroud of Death +", Proc: 100, ProcMax: 100, Until: until, Rules: EffectRules{ProcHeal: 100}}
	p.Effects = []*ActiveEffect{steal}
	hit := func(dice ...int) (dealt, healed int) {
		i := 0
		procIntn = func(n int) int {
			d := dice[i%len(dice)]
			i++
			return d
		}
		p.HP, guard.HP = 1000, 1_000_000
		top := p.MaxHPNow()
		if top < 3000 {
			p.MaxHP = 5000
		}
		r.w.mu.Lock()
		r.w.playerProcsLocked(sess, p, guard, time.Now())
		r.w.mu.Unlock()
		return 1_000_000 - guard.HP, p.HP - 1000
	}
	guard.Level = p.Level
	if d, h := hit(0, 99); d != 100 || h != 100 {
		t.Errorf("a proc: dealt %d, healed %d, want 100 and 100 on the same hit", d, h)
	}
	if d, h := hit(99); d != 0 || h != 0 {
		t.Errorf("a miss: dealt %d, healed %d, want neither", d, h)
	}
	if d, h := hit(0, 0); d != 100 || h != 100 {
		t.Errorf("a resisted proc: dealt %d, healed %d, want its least (100) and its heal", d, h)
	}
	steal.Rules = EffectRules{}
	if d, h := hit(0, 99); d != 100 || h != 0 {
		t.Errorf("a proc with no heal: dealt %d, healed %d", d, h)
	}
}

// An instant strike is held to the arts' cooldown; one paid for with an item is not (petition 1036, Agony).
func TestAnArtPaidForWithAnItemIsNotHeld(t *testing.T) {
	strike := &CastSpell{CastTime: 0, Def: SpellDef{Harmful: true}}
	if !isArt(strike) {
		t.Error("an instant harmful spell is an art")
	}
	agony := &CastSpell{CastTime: 0, Def: SpellDef{Harmful: true, Reagent: "Rogue's Dagger", ReagentCount: 1}}
	if isArt(agony) {
		t.Error("Agony is held to the arts' cooldown")
	}
	if isArt(&CastSpell{CastTime: 3, Def: SpellDef{Harmful: true}}) {
		t.Error("a spell with a cast time is an art")
	}
}

// figuresIn is every 0x00DB among msgs: (source, amount, target).
func figuresIn(msgs [][]byte) [][3]int64 {
	var out [][3]int64
	for _, m := range msgs {
		if len(m) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(m)) != protocol.OpcodeDamageIndication {
			continue
		}
		r := &vims{b: m[2:]}
		out = append(out, [3]int64{int64(r.u32()), int64(r.signed()), int64(r.u32())})
	}
	return out
}

// A proc's heal is seen at full health too (the user, 2026-11-08: "the heal proc should
// show green numbers for player": a healer kept Larrybird full, and his Shroud of Death's
// heals showed nothing): a life steal's figure, a defensive heal proc's and a mark's, each
// green (positive) over the player with nothing healed. The controls: a spell's heal on a
// full player shows none, and a hurt player's life steal heals as before.
func TestAProcsHealIsSeenAtFullHealth(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.HP, guard.MaxHP = 1_000_000, 1_000_000
	guard.Level = 1
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = func(n int) int { return 0 }
	me := int64(sess.PlayerObjectID())
	now := time.Now()
	p.MaxHP = 5000
	full := func() { p.HP = p.MaxHPNow(); drain(sess) }
	healed := func(what string, amount int64) {
		t.Helper()
		figs := figuresIn(drain(sess))
		found := false
		for _, f := range figs {
			found = found || f == [3]int64{me, amount, me}
		}
		if !found || p.HP != p.MaxHPNow() {
			t.Errorf("%s at full health: figures %v, HP %d/%d; want (self, +%d, self) and nothing healed", what, figs, p.HP, p.MaxHPNow(), amount)
		}
	}

	p.Effects = []*ActiveEffect{{Name: "Shroud of Death +", Proc: 100, ProcMax: 100, Until: now.Add(time.Hour), Rules: EffectRules{ProcHeal: 100}}}
	full()
	r.w.mu.Lock()
	r.w.playerProcsLocked(sess, p, guard, now)
	r.w.mu.Unlock()
	healed("a life steal", 100)

	p.Effects = []*ActiveEffect{{Name: "Spirit Shield", Until: now.Add(time.Hour), Rules: EffectRules{ProcHeal: 52, ProcHealDef: true}}}
	full()
	r.w.mu.Lock()
	r.w.procHealsLocked(p, guard, true, now)
	r.w.mu.Unlock()
	healed("a defensive heal proc", 52)

	p.Effects = nil
	guard.Effects = []*ActiveEffect{{Name: "Mark of Faith", Until: now.Add(time.Minute), Rules: EffectRules{MarkHeal: 65}}}
	full()
	r.w.mu.Lock()
	r.w.markHealPlayerLocked(sess, p, guard, now)
	r.w.mu.Unlock()
	healed("a mark", 65)
	guard.Effects = nil

	// the controls: a spell's heal on a full player, none; a hurt one's life steal, healed
	full()
	r.w.mu.Lock()
	r.w.healLocked(p, sess, p, 300, numberHeal)
	r.w.mu.Unlock()
	if figs := figuresIn(drain(sess)); len(figs) != 0 {
		t.Errorf("a spell's heal on a full player: %v", figs)
	}
	p.Effects = []*ActiveEffect{{Name: "Shroud of Death +", Proc: 100, ProcMax: 100, Until: now.Add(time.Hour), Rules: EffectRules{ProcHeal: 100}}}
	p.HP = 1000
	drain(sess)
	r.w.mu.Lock()
	r.w.playerProcsLocked(sess, p, guard, now)
	r.w.mu.Unlock()
	if figs := figuresIn(drain(sess)); p.HP != 1100 || len(figs) != 2 || figs[1] != [3]int64{me, 100, me} {
		t.Errorf("a hurt player's life steal: HP %d, figures %v", p.HP, figs)
	}
}
