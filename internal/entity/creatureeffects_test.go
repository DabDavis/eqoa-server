package entity

import (
	"testing"
	"time"
)

// Only the best of a line counts on a creature (bestOfLine), as on a player:
// two ranks of one AC debuff's line on it take off what the higher takes off,
// not both; an effect of another line counts beside it.
func TestOnlyTheBestOfADebuffsLineCountsOnACreature(t *testing.T) {
	npc := &NPC{ServerID: 5, Name: "Desert Crab", Level: 30}
	until := time.Now().Add(time.Hour)
	npc.Effects = []*ActiveEffect{
		{SpellID: 1, Until: until, Line: "player -AC", Rank: 10, Mods: StatMods{AC: -60}},
		{SpellID: 2, Until: until, Line: "player -AC", Rank: 25, Mods: StatMods{AC: -150}},
		{SpellID: 3, Until: until, Line: "player -STR", Rank: 12, Mods: StatMods{Str: -20}},
	}
	if m := npc.creatureMods(); m.AC != -150 || m.Str != -20 {
		t.Errorf("AC %d, STR %d; want the higher rank's -150 and the other line's -20", m.AC, m.Str)
	}
}

// The simulator casts a book's pure debuffs (the user, 2026-11-01: "make the
// sim cast pure debuffs too"): a harmful spell at a target with no damage
// whose lasting part lowers a stat that counts in a creature's fights is a
// debuff, only the highest of a line in the book, cast whenever its line is
// not on the foe; one lowering STA alone (nothing on a creature) is not. A
// warrior that opens with an AC debuff on its foe kills it sooner than one
// that does not, on the same dice.
func TestTheSimCastsPureDebuffs(t *testing.T) {
	debuff := func(id, level int, name, line string) CastSpell {
		return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: level, Scope: 1, Name: name, CastTime: time.Second,
			PowerCost: 10, Range: 20, Def: ParseSpellText(line)}
	}
	weak := debuff(9301, 10, "Test Rust", "Target, Decrease AC by 60, 30sec")
	strong := debuff(9302, 25, "Test Corrosion", "Target, Decrease AC by 150, 30sec")
	tired := debuff(9303, 25, "Test Weariness", "Target, Decrease STA by 50, 30sec")
	for _, c := range []struct {
		s    CastSpell
		want spellRole
	}{{weak, roleDebuff}, {strong, roleDebuff}, {tired, roleNone}} {
		if got := simRole(&c.s); got != c.want {
			t.Errorf("%q: role %d, want %d", c.s.Name, got, c.want)
		}
	}
	plain := simKit(0, 30)
	debuffing := simKit(0, 30)
	debuffing.Spells = append(debuffing.Spells, weak, strong, tired)
	f := newSimFight(debuffing, SimNPC{Level: 31}, SeedBalance())
	f.sortBook()
	if len(f.debuffs) != 1 || f.debuffs[0].SpellID != strong.SpellID {
		t.Fatalf("the book's debuffs %v; want Test Corrosion alone (the highest of its line; STA is no debuff)", f.debuffs)
	}
	a := SimulateFight(debuffing, SimNPC{Level: 31}, SeedBalance(), 5, 10*time.Minute)
	b := SimulateFight(plain, SimNPC{Level: 31}, SeedBalance(), 5, 10*time.Minute)
	if a.Debuffs == 0 || b.Debuffs != 0 {
		t.Fatalf("debuffs cast: %d with the spell, %d without", a.Debuffs, b.Debuffs)
	}
	if !a.Won || !b.Won || a.Time >= b.Time {
		t.Errorf("with the debuff: won %v in %v; without: won %v in %v; want both won, the debuffer sooner", a.Won, a.Time, b.Won, b.Time)
	}
}
