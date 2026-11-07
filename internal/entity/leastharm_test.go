package entity

import "testing"

// The user, 2026-11-07: a creature's resist never takes all of a damage effect; it lands at
// the figure of its range nearest nothing, no caster's stat or level added. What is not damage
// (a debuff, a heal) is left to the whole resist.
func TestLeastHarm(t *testing.T) {
	for _, c := range []struct {
		e    SpellEffect
		want int
		ok   bool
	}{
		{SpellEffect{Kind: EffectHP, Amount: -150, AmountMax: -100, Scale: StatInt, ScalePct: 10, LevelPct: 50}, -100, true},
		{SpellEffect{Kind: EffectHP, Amount: -12}, -12, true},
		{SpellEffect{Kind: EffectHPTick, Amount: -40, AmountMax: -30}, -30, true},
		{SpellEffect{Kind: EffectStat, Amount: -20}, 0, false},
		{SpellEffect{Kind: EffectHP, Amount: 50}, 0, false},
	} {
		l, ok := leastHarm(c.e)
		if ok != c.ok || (ok && (l.Amount != c.want || l.AmountMax != c.want || l.Scale != StatNone || l.ScalePct != 0 || l.LevelPct != 0)) {
			t.Errorf("%+v: %+v %v, want %d %v", c.e, l, ok, c.want, c.ok)
		}
		if ok {
			if n := rollAmount(l); n != c.want {
				t.Errorf("%+v: rolls %d, want %d", c.e, n, c.want)
			}
		}
	}
}
