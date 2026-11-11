package entity

import (
	"encoding/binary"
	"testing"
)

// A level 23 druid in its gear, as Sandstorm's stat channel showed one
// (2026-10-21): the worn items' stats on the base ones, the maximum hit
// points of all the stamina plus the items' 113, the maximum power of all
// the stats plus the items' 145, and the base maxima. The hit points are
// the live game's formula's (stats.go): 566 of the stamina where Sandstorm,
// rounding STA/11 first, sent 575 (and 688 with the items' where 679).
func TestRecalcStatsWithGear(t *testing.T) {
	p := &Player{Class: 7, Level: 23,
		BaseStr: 60, BaseSta: 100, BaseAgi: 100, BaseDex: 105, BaseWis: 75, BaseInt: 55, BaseCha: 60}
	var g Gear
	for _, s := range [][2]int{{1, 28}, {2, 3}, {3, 38}, {4, 27}, {6, 18}, {8, 113}, {10, 145}, {14, 404},
		{23, 26}, {24, 38}, {25, 14}, {26, 20}, {27, 12}} {
		g.Add(s[0], s[1])
	}
	p.Gear = g
	p.RecalcStats(true)

	b := SerializeStatUpdate(p)
	at := func(o int) int { return int(int32(binary.LittleEndian.Uint32(b[o:]))) }
	for i, want := range []int{60, 128, 103, 143, 102, 55, 78} {
		if got := at(4 * i); got != want {
			t.Errorf("stat %d = %d, Sandstorm's %d", i, got, want)
		}
	}
	for i, want := range []int{60, 100, 100, 105, 75, 55, 60} {
		if got := at(116 + 4*i); got != want {
			t.Errorf("base stat %d = %d, want %d", i, got, want)
		}
	}
	// 28 HP, 32 its maximum, 36 power, 40 its maximum, 148 and 156 the base maxima
	for o, want := range map[int]int{28: 679, 32: 679, 36: 1188, 40: 1188, 148: 566, 156: 937} {
		if got := at(o); got != want {
			t.Errorf("byte %d = %d, want %d", o, got, want)
		}
	}
	// the worn AC and resists (Sandstorm showed this druid AC 529: something
	// adds more than the items' 404, not yet known), each resist with its WIS
	// 102's 14 (the beta sheet's 1 each 7 WIS, ours since 2026-11-11: the
	// items' 26 and 12)
	if at(56) != 404 || at(92) != 26+14 || at(108) != 12+14 || at(172) != 0 {
		t.Errorf("AC %d, resists %d %d, base AC %d", at(56), at(92), at(108), at(172))
	}

	// Off with it all: the base stats again, the hit points within the new maximum.
	p.Gear = Gear{}
	p.RecalcStats(false)
	if p.Sta != 100 || p.MaxHP != CalculateMaxHP(23, 7, 100) || p.HP != p.MaxHP || p.AC != 0 {
		t.Errorf("unequipped: STA %d, max HP %d, HP %d, AC %d", p.Sta, p.MaxHP, p.HP, p.AC)
	}
}

// Combat is not touched by the gear until its formulas are measured: the
// weapon damage stays level/3 and the DEF the old AC-50 one, whatever is worn.
func TestRecalcStatsKeepsCombat(t *testing.T) {
	p := &Player{Class: 3, Level: 50, BaseStr: 75, BaseSta: 204, BaseAgi: 60, BaseDex: 351, BaseWis: 55, BaseInt: 232, BaseCha: 50}
	p.Gear = Gear{AC: 1703, WeaponDamage: 560, Str: 80}
	p.RecalcStats(true)
	if p.WeaponDamage != 16 || p.DefMod != CalculateDefMod(50, 60, 50) || p.OffMod != CalculateOffMod(75, 351, 50) {
		t.Errorf("weapon damage %d, DEF %d, OFF %d", p.WeaponDamage, p.DefMod, p.OffMod)
	}
}

// The beta sheet's WIS rule (the owner, 2026-11-11: "Every 7 points of Wisdom
// adds 1 to all of your resistance"): a point to each of the six resists for
// each whole 7 of WIS, the worn and the masteries' WIS counted in the resists
// themselves (RecalcStats: the stat channel's), a spell's WIS on top of them
// (StatNow), and the 315 that counts against a spell (resist.go) as for any
// resist. The controls: WIS 6 adds nothing, and a WIS debuff takes its share.
func TestEverySevenWISIsAPointOfEachResist(t *testing.T) {
	for wis, want := range map[int]int{0: 0, 6: 0, 7: 1, 13: 1, 14: 2, 100: 14, 260: 37, -20: 0} {
		if got := wisResist(wis); got != want {
			t.Errorf("WIS %d: %d, want %d", wis, got, want)
		}
	}
	p := &Player{Class: 7, Level: 60, BaseWis: 90}
	p.Gear = Gear{Wis: 15, FR: 30}
	p.RecalcStats(true) // WIS 105: 15 each
	stats := []Stat{StatFR, StatCR, StatPR, StatDR, StatLR, StatAR}
	for _, st := range stats {
		want := 15
		if st == StatFR {
			want = 45
		}
		if got := p.StatNow(st); got != want {
			t.Errorf("WIS 105, resist %v: %d, want %d", st, got, want)
		}
	}
	if p.FireR != 45 || p.ArcaneR != 15 {
		t.Errorf("the resists themselves: fire %d, arcane %d, want 45 and 15", p.FireR, p.ArcaneR)
	}
	p.Mods.Wis = 40 // a buff: WIS 145, 20 each
	if got := p.StatNow(StatCR); got != 20 {
		t.Errorf("WIS 145 with a buff: cold %d, want 20", got)
	}
	p.Mods.Wis = -50 // a debuff: WIS 55, 7 each
	if got := p.StatNow(StatAR); got != 7 {
		t.Errorf("WIS 55 under a debuff: arcane %d, want 7", got)
	}
	p.Mods.Wis = 0
	p.BaseWis, p.Gear = 6, Gear{}
	p.RecalcStats(false)
	if got := p.StatNow(StatPR); got != 0 {
		t.Errorf("WIS 6: poison %d, want 0", got)
	}
}
