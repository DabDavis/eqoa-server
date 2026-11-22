package entity

import (
	"testing"
	"time"
)

// Eldrich Rune (board 1350): each positive melee hit it meets is stopped whole,
// its pool spent by the whole hit (clamped at 0) and 1 hit point through; the
// hit that reaches the pool is covered whole and breaks it. The acceptance
// cases (pool before, hit -> through, pool after), then a hit after the break
// taken whole. The control: the same pool without the rune flag lets the
// breaking hit's overage through, as a Stoneskin does.
func TestEldrichRuneStopsEachHitWhole(t *testing.T) {
	rune := func(c *caster, left int, isRune bool) *ActiveEffect {
		ae := &ActiveEffect{SpellID: 900616, Name: "Eldrich Rune", Until: c.at(time.Hour),
			Rules: EffectRules{MitigatePct: 100, MitigateLeft: left, MitigateCapped: true, MitigateRune: isRune}}
		c.p.Effects = []*ActiveEffect{ae}
		return ae
	}
	for _, k := range []struct{ left, hit, through, after int }{
		{3500, 100, 1, 3400}, {3500, 3500, 1, 0}, {1, 10000, 1, 0}, {1, 2, 1, 0}, {2, 1, 1, 1}, {1, 1, 1, 0},
	} {
		c := newCaster(t)
		ae := rune(c, k.left, true)
		c.w.mu.Lock()
		got := c.w.mitigateLocked(c.p, k.hit, c.at(0))
		c.w.mu.Unlock()
		broken := len(c.p.Effects) == 0
		if got != k.through || ae.Rules.MitigateLeft != k.after || broken != (k.after == 0) {
			t.Errorf("pool %d, hit %d: %d through, pool %d, broken %v; want %d, %d", k.left, k.hit, got, ae.Rules.MitigateLeft, broken, k.through, k.after)
		}
		if broken {
			c.w.mu.Lock()
			if n := c.w.mitigateLocked(c.p, 300, c.at(time.Second)); n != 300 {
				t.Errorf("pool %d, hit %d: the next hit after the break took %d, want 300", k.left, k.hit, n)
			}
			if n := c.w.mitigateLocked(c.p, 0, c.at(time.Second)); n != 0 {
				t.Errorf("a hit of 0: %d", n)
			}
			c.w.mu.Unlock()
		}
	}
	c := newCaster(t)
	rune(c, 1, false)
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	if n := c.w.mitigateLocked(c.p, 10000, c.at(0)); n != 9999 {
		t.Errorf("the control, no rune: %d through, want 9999", n)
	}
}

// The flag is the workbook's mitigation.rune, read only with a pool.
func TestTheRuneFlagIsRead(t *testing.T) {
	js := `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":300,"effects":[{"id":"R1","type":"Mitigation","kind":"mitigation","mitigation":{"pct":100,"max":-3500,"rune":true}}]}`
	d := SpellDefFrom("Eldrich Rune", "", "", js)
	if len(d.Effects) != 1 || d.Effects[0].Kind != EffectMitigate || !d.Effects[0].Rune || d.Effects[0].AmountMax != 3500 {
		t.Errorf("read %+v (unsupported %v)", d.Effects, d.Unsupported)
	}
}
