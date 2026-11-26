package entity

import "testing"

// The cleric's Ward path (Disease, Poison, Flame, Ice and Lightning Ward):
// every rank gives its resists plus AR (arcane), and 10% of the caster's WIS
// on each (petition 3289, 2026-11-26: "my cleric has 690 WIS and still only
// 100 resists and it misses AR"). Their lines had neither; the cast code
// already scales a stat effect, so the fix is the lines' words (the scale
// clause after a list of stats is every stat of the list).
func TestWardPathScalesWithWisAndGivesArcane(t *testing.T) {
	const tail = " by 100 ; Increase 10% based on WIS, 30min"
	ranks := []struct {
		name string
		line string
		want func(m *StatMods) []int
	}{
		{"Disease Ward", "Target, buff DR, AR" + tail, func(m *StatMods) []int { return []int{m.DR, m.AR} }},
		{"Poison Ward", "Target, buff DR, PR, AR" + tail, func(m *StatMods) []int { return []int{m.DR, m.PR, m.AR} }},
		{"Flame Ward", "Target, buff DR, PR, FR, AR" + tail + ", 10hate", func(m *StatMods) []int { return []int{m.DR, m.PR, m.FR, m.AR} }},
		{"Ice Ward", "Target, buff DR, PR, FR, CR, AR" + tail, func(m *StatMods) []int { return []int{m.DR, m.PR, m.FR, m.CR, m.AR} }},
		{"Lightning Ward", "Target, buff DR, PR, FR, CR, LR, AR" + tail, func(m *StatMods) []int { return []int{m.DR, m.PR, m.FR, m.CR, m.LR, m.AR} }},
	}
	for i, r := range ranks {
		c := newCaster(t)
		c.p.Class, c.p.Level, c.p.Wis, c.p.BaseWis = 9, 60, 690, 690
		c.p.CMEffects[cmSlotCap+int(StatWis-StatStr)] = 690 - StatCapBase(60) // the masteries that lift the WIS cap
		c.castOn(t, buffOf(700600+i, 20+i, 1, r.name, r.line), 0)
		got := r.want(&c.p.Mods)
		for _, v := range got {
			if v != 169 { // 100 + 10% of 690
				t.Errorf("%s at WIS 690: resists %v, want every one 169", r.name, got)
				break
			}
		}
	}
	// Lightning Ward is all six
	c := newCaster(t)
	c.p.Class, c.p.Level, c.p.Wis, c.p.BaseWis = 9, 60, 690, 690
	c.p.CMEffects[cmSlotCap+int(StatWis-StatStr)] = 690 - StatCapBase(60) // the masteries that lift the WIS cap
	c.castOn(t, buffOf(700620, 49, 1, "Lightning Ward", ranks[4].line), 0)
	m := c.p.Mods
	if m.FR != 169 || m.CR != 169 || m.LR != 169 || m.PR != 169 || m.DR != 169 || m.AR != 169 {
		t.Fatalf("Lightning Ward: %+v", m)
	}
}
