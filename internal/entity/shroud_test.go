package entity

import (
	"testing"
	"time"
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
