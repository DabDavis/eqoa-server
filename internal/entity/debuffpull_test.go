package entity

import (
	"testing"
	"time"
)

// Live petition 5045 (Innoruuk): a debuff pulls whatever the caster wears. A tap of STR (Tap
// Strength: the creature's STR taken, given to the caster) cast at a creature that is not angry,
// by a caster who wears a stronger effect of the tap's own line, so that the caster's share is
// refused (lifetap.go shareTakesLocked), still lands on the creature and angers it: the caster its
// enemy, with threat. The line check is the caster's share's alone, never the debuff's.
func TestADebuffPullsThoughItsCastersShareDoesNotStack(t *testing.T) {
	for _, wearing := range []bool{false, true} {
		r := newAreaRig(t)
		npc := r.creature(2101, "crocodile", 1010, 1000)
		tap := spellOf(1424, 1, "Tap Strength", "Target, Drain STR by 10 and give to self, 60sec")
		tap.Def.Duration = 60 * time.Second
		if wearing {
			r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 5001, Name: "Stronger Tap", Until: r.t0.Add(time.Hour),
				Line: BuffLine(&tap), Rank: 99, Mods: StatMods{Str: 50}})
		}
		r.cast(t, tap, npc.ServerID) // begun and landed
		shared := false
		for _, e := range r.p.Effects {
			shared = shared || e.SpellID == -tap.SpellID
		}
		if shared == wearing {
			t.Errorf("wearing a stronger tap %v: the caster's share taken %v", wearing, shared)
		}
		if npc.CurrentEnemy != r.p.ServerID || npc.Threats[r.p.ServerID] < 1 {
			t.Errorf("wearing a stronger tap %v: the creature's enemy %d, the caster's threat %d: want the caster, angered",
				wearing, npc.CurrentEnemy, npc.Threats[r.p.ServerID])
		}
	}
}
