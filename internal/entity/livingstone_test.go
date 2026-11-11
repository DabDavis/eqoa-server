package entity

import (
	"strings"
	"testing"
	"time"
)

// Petition 1579 (Tom, a gargoyle shadow knight: "Living stone MCA for gargoyle
// doesnt stack on raid buffs"): Living Stone was of the self stack type (2),
// +AC, one line with a knight's own Ebonweave (stack 2, +AC 320): the higher
// rank (Living Stone's 60) took Ebonweave off, and Hand of Innoruuk (+AC +INT
// +STR, nested) counted only the bigger AC. With a stack type of its own
// (tools/living_stone_stack_20261111.py: its spell id, as GM Armor's) it
// stacks with both, its AC added to theirs (the knight's two, nested, count
// their bigger AC once, as before). The control: the old record clashes.
func TestLivingStoneStacksWithSelfACBuffs(t *testing.T) {
	const (
		ebonweave = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1200,"stack":2,"level":54,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"04A0","type":"Statistic Change","kind":"stat","stat":"ac","amount":320}]}`
		innoruuk  = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":2,"level":49,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"0780","type":"Statistic Change","kind":"stat","stat":"str","amount":50},{"id":"0780","type":"Statistic Change","kind":"stat","stat":"int","amount":50},{"id":"0780","type":"Statistic Change","kind":"stat","stat":"ac","amount":150}]}`
		stoneOld  = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":60,"stack":2,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"F066","type":"Dmg/Heal","kind":"hp","amount":2000},{"id":"F067","type":"Statistic Change","kind":"stat","stat":"ac","amount":400}]}`
	)
	stoneNew := strings.Replace(stoneOld, `"stack":2,`, `"stack":760156,`, 1)
	spell := func(id, level int, name, js string) CastSpell {
		return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: level, Name: name, Icon: uint32(id), CastTime: time.Second,
			PowerCost: 10, Range: 30, Def: SpellDefFrom(name, "", "", js)}
	}
	ebon, hand := spell(700692, 54, "Ebonweave", ebonweave), spell(1484, 49, "Hand of Innoruuk", innoruuk)
	for _, tc := range []struct {
		stone string
		want  []string
		ac    int
	}{
		{stoneNew, []string{"Ebonweave", "Hand of Innoruuk", "Living Stone"}, 320 + 400}, // the knight's two nested: their bigger AC once
		{stoneOld, []string{"Hand of Innoruuk", "Living Stone"}, 400},                    // the control: Ebonweave taken off, the AC its biggest once
	} {
		c := newCaster(t)
		c.p.Level, c.p.Class = 60, 3
		c.p.MaxPower = 100000
		c.castOn(t, ebon, 0)
		c.castOn(t, hand, 10*time.Second)
		c.castOn(t, spell(760156, 60, "Living Stone", tc.stone), 20*time.Second)
		got := c.effectNames()
		if strings.Join(got, ",") != strings.Join(tc.want, ",") || c.p.Mods.AC != tc.ac {
			t.Errorf("stack in %q: effects %v, AC +%d; want %v, +%d", tc.stone[60:80], got, c.p.Mods.AC, tc.want, tc.ac)
		}
	}
}
