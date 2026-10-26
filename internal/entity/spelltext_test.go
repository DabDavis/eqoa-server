package entity

import (
	"reflect"
	"testing"
	"time"
)

// Lines in EQOA-Data's form, read into what the cast does. The numbers are
// made up; the shapes are the ones the lists use most.
func TestParseSpellText(t *testing.T) {
	cases := []struct {
		line string
		want SpellDef
	}{
		{"Target, Decrease Hitpoints by 20 ; Increase 8% based on DEX",
			SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -20, AmountMax: -20, Scale: StatDex, ScalePct: 8}}}},
		{"Target, Increase HP per tic by 15 ; Increase 25% based on CHA, Decrease Power by 1, 24sec",
			SpellDef{Target: TargetSingle, Duration: 24 * time.Second, Effects: []SpellEffect{
				{Kind: EffectHPTick, Amount: 15, AmountMax: 15, Scale: StatCha, ScalePct: 25},
				{Kind: EffectPower, Amount: -1, AmountMax: -1}}}},
		// a verb carried over, MaxHP and Max Power, an instant HP part
		{"Target, Increase AC by 15, Max Power by 6, MaxHP by 3, HP by 3, 30min",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatAC, Amount: 15, AmountMax: 15},
				{Kind: EffectStat, Stat: StatMaxPower, Amount: 6, AmountMax: 6},
				{Kind: EffectStat, Stat: StatMaxHP, Amount: 3, AmountMax: 3},
				{Kind: EffectHP, Amount: 3, AmountMax: 3}}}},
		{"Self, Increase WIS by 2, 30min",
			SpellDef{Target: TargetSelf, Duration: 30 * time.Minute, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatWis, Amount: 2, AmountMax: 2}}}},
		{"Group, Increase STR & DEX by 9, 5.5min",
			SpellDef{Target: TargetGroup, Duration: 330 * time.Second, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatStr, Amount: 9, AmountMax: 9},
				{Kind: EffectStat, Stat: StatDex, Amount: 9, AmountMax: 9}}}},
		{"Target, Increase FR & CR by 20 ; Increase 15% based on WIS, 30min",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatFR, Amount: 20, AmountMax: 20},
				{Kind: EffectStat, Stat: StatCR, Amount: 20, AmountMax: 20, Scale: StatWis, ScalePct: 15}}}},
		// "Resist Ice" is cold resist, named once or for a list (the wizard's
		// Element Guard; jib's review, F3 pass 19): without it both were unread
		{"Self, Increase Resist Ice by 30, 10min",
			SpellDef{Target: TargetSelf, Duration: 10 * time.Minute, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatCR, Amount: 30, AmountMax: 30}}}},
		{"Target, Increase Resist Fire & Ice by 45",
			SpellDef{Target: TargetSingle, Duration: buffDuration, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatFR, Amount: 45, AmountMax: 45},
				{Kind: EffectStat, Stat: StatCR, Amount: 45, AmountMax: 45}}}},
		{"Target, Decrease Hitpoints by 12 and roots, 30sec",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Second, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectRoot},
				{Kind: EffectHP, Amount: -12, AmountMax: -12}}}},
		{"Target, Debuff Hp by 30 Modified 15% based on DEX",
			SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -30, AmountMax: -30, Scale: StatDex, ScalePct: 15}}}},
		{"Target, Buff MaxHP by 100 and AC by 100, 30min",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatMaxHP, Amount: 100, AmountMax: 100},
				{Kind: EffectStat, Stat: StatAC, Amount: 100, AmountMax: 100}}}},
		{"Target, Decrease AC by 12 for 30 seconds",
			SpellDef{Target: TargetSingle, Duration: 30 * time.Second, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectStat, Stat: StatAC, Amount: -12, AmountMax: -12}}}},
		{"Target, Debuff Hitpoints by 40 to 50",
			SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -50, AmountMax: -40}}}},
		{"Target, Sow, 30min", SpellDef{Target: TargetSingle, Duration: 30 * time.Minute, Effects: []SpellEffect{{Kind: EffectSpeed}}}},
		{"Target, Invis, 5.5min", SpellDef{Target: TargetSingle, Duration: 330 * time.Second, Effects: []SpellEffect{{Kind: EffectInvis}}}},
		{"Target, root, 36sec", SpellDef{Target: TargetSingle, Duration: 36 * time.Second, Harmful: true, Effects: []SpellEffect{{Kind: EffectRoot}}}},
		{"Returns you to last bind point", SpellDef{Target: TargetSingle, Effects: []SpellEffect{{Kind: EffectGate}}}},
		{"Target undead, Decrease Hitpoints by 60 ; Increase 30% based on WIS",
			SpellDef{Target: TargetUndead, Harmful: true, Effects: []SpellEffect{
				{Kind: EffectHP, Amount: -60, AmountMax: -60, Scale: StatWis, ScalePct: 30}}}},
		{"Target, Increase Hate by 40", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHate, Amount: 40, AmountMax: 40}}}},
		{"Target, Decrease Drain HP by 40", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHP, Amount: -40, AmountMax: -40, Drain: DrainCaster}}}},
		{"Target, Drain STR by 10 and give to self, 60sec", SpellDef{Target: TargetSingle, Duration: time.Minute, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectStat, Stat: StatStr, Amount: -10, AmountMax: -10, Drain: DrainCaster}}}},
		{"Target, Decrease HP via Backstab by 50 ; Increase 20% based on DEX", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHP, Amount: -50, AmountMax: -50, Scale: StatDex, ScalePct: 20}}}},
		{"Target, Decrease Hitpoints by 200 aoe Decrease Hitpoints by 90 - Lightning Based AoE", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHP, Amount: -200, AmountMax: -200}}}},
		// a buff with no time lasts buffDuration (spellnotes.go)
		{"Self Buff MaxHP by 15 & AC by 28", SpellDef{Target: TargetSelf, Duration: buffDuration, Effects: []SpellEffect{
			{Kind: EffectStat, Stat: StatMaxHP, Amount: 15, AmountMax: 15},
			{Kind: EffectStat, Stat: StatAC, Amount: 28, AmountMax: 28}}}},
		{"Target Invis 5min", SpellDef{Target: TargetSingle, Duration: 5 * time.Minute, Effects: []SpellEffect{{Kind: EffectInvis}}}},
		{"Target, Pickpocket, +87Hate", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHate, Amount: 87, AmountMax: 87}}, Unread: []string{"pickpocket"}}},
		{"Damage by 130, Modified 30% based on DEX", SpellDef{Target: TargetSingle, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectHP, Amount: -130, AmountMax: -130, Scale: StatDex, ScalePct: 30}}}},
		// a charm makes its target a pet (charm.go), cast at an enemy
		{"Target, Charm, 2min", SpellDef{Target: TargetSingle, Duration: 2 * time.Minute, Harmful: true, Effects: []SpellEffect{
			{Kind: EffectCharm}}}},
		// what is not acted on is kept for the log and makes nothing
		{"Target, Removes Minor Poison", SpellDef{Target: TargetSingle, Unread: []string{"removes minor poison"}}},
		{"", SpellDef{}},
	}
	for _, c := range cases {
		got := ParseSpellText(c.line)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q\n got %+v\nwant %+v", c.line, got, c.want)
		}
	}
}
