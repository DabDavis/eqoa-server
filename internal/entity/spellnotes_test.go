package entity

import (
	"reflect"
	"testing"
	"time"
)

// What EQOA-Data's lines leave out, and what they say beside a spell's
// effects (spellnotes.go). The lines are the lists' own.
func TestSpellNotesAndDefaults(t *testing.T) {
	hp := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: n, AmountMax: n} }
	stat := func(s Stat, n int) SpellEffect {
		return SpellEffect{Kind: EffectStat, Stat: s, Amount: n, AmountMax: n}
	}
	cases := []struct {
		name, line string
		want       SpellDef
	}{
		// jib's Courage: no time in its line, so it lasted nothing; now 30
		// minutes, and it heals what it raises the maximum by
		{"Courage", "Target, Increase Maximum HP by 40, Increase AC by 40",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatMaxHP, 40), stat(StatAC, 40), hp(40)}}},
		{"Group Valor", "Group, Increase Maximum HP by 390, Increase AC by 390, 30min, 30rad, 70hate",
			SpellDef{Target: TargetGroup, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatMaxHP, 390), stat(StatAC, 390), hp(390)}, Ignored: []string{"30rad", "70hate"}}},
		{"Resolution", "Target, buff MaxHP by 500 and AC by 500",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatMaxHP, 500), stat(StatAC, 500), hp(500)}}},
		// another class's line of the same shape keeps its words: no heal
		{"Minor Shielding", "Self Buff MaxHP by 15 & AC by 28",
			SpellDef{Target: TargetSelf, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatMaxHP, 15), stat(StatAC, 28)}}},
		// a line that says its heal is not given a second one
		{"Rockskin", "Target, Increase AC by 150, Max Power by 60, MaxHP by 30, HP by 30, 30min",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatAC, 150), stat(StatMaxPower, 60), stat(StatMaxHP, 30), hp(30)}}},
		// four stats before one "by"
		{"Ice Ward", "Target, buff DR, PR, FR, CR by 100",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				stat(StatDR, 100), stat(StatPR, 100), stat(StatFR, 100), stat(StatCR, 100)}}},
		// a group heal's radius and hate are noted, not acted on
		{"Wave of Life", "Group, Increase Hitpoints by 250 ; Increase 25% based on CHA, 15rad no hate",
			SpellDef{Target: TargetGroup, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: 250, AmountMax: 250, Scale: StatCha, ScalePct: 25}}, Ignored: []string{"15rad no hate"}}},
		// the lists' typos and a monk's wording
		{"Icy Detonation", "Target, Decrease Hitoints by 534 ; Increase 30% based on DEX",
			SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -534, AmountMax: -534, Scale: StatDex, ScalePct: 30}}}},
		{"Kick", "Target Damage Hp by 16 Modified 20% based on STR",
			SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -16, AmountMax: -16, Scale: StatStr, ScalePct: 20}}}},
		{"Form of the Great Bear", "Self, Form, Increase STR by 120, Increase MHP by 425, Increase Hp by 425, 30sec",
			SpellDef{Target: TargetSelf, Duration: 30 * time.Second, Effects: []SpellEffect{
				stat(StatStr, 120), stat(StatMaxHP, 425), hp(425)}, Appearance: true}}, // "Form": a look True Form ends (illusion.go)
		// an epic's numbers are the record's, not effects
		{"Mega's Clutch Ward", "Level 60 Epic Ward path. Power cost: 500, Cast: 3, Recast: 4. 60 epic: 85+10%wis to all resists (target)",
			SpellDef{Target: TargetSingle, Ignored: []string{"level 60 epic ward path. power cost: 500", "cast: 3",
				"recast: 4. 60 epic: 85+10%wis to all resists (target)"}}},
		// a debuff with no time is left as it is (its WIS share both stats':
		// spelltext.go statList)
		{"Weaken Death", "Target undead, Decrease STR/AGI by 27 ; Increase 15% based on WIS",
			SpellDef{Target: TargetUndead, Harmful: true, UndeadOnly: true, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatStr, Amount: -27, AmountMax: -27, Scale: StatWis, ScalePct: 15},
				{Kind: EffectStat, Stat: StatAgi, Amount: -27, AmountMax: -27, Scale: StatWis, ScalePct: 15}}}},
	}
	for _, c := range cases {
		if got := ParseSpellNamed(c.name, c.line, ""); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q\n got %+v\nwant %+v", c.name, c.line, got, c.want)
		}
	}
}
