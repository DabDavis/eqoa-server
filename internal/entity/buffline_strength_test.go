package entity

import (
	"strings"
	"testing"
	"time"
)

// The stronger of a line wins, the rank only between two as strong
// (buffline.go's lineOutranks, 2026-11-14): a higher rank that gives less
// (the bard's Harmony, 55, over Power Vibe, 49) does not take and its caster
// is told; the stronger lower rank takes the weaker higher one off; of one
// rank the weaker cast after the stronger does not take (Warcry under
// Ascended Warcry).
func TestTheStrongerOfALineWins(t *testing.T) {
	weakHigh := buffOf(9001, 55, 0, "Weak High", "Self, Increase MaxHP by 30, 10min")
	strongLow := buffOf(9002, 49, 0, "Strong Low", "Self, Increase MaxHP by 134, 10min")
	if !sameLine(EffectLine(&weakHigh), EffectLine(&strongLow)) {
		t.Fatalf("not one line: %q %q", EffectLine(&weakHigh), EffectLine(&strongLow))
	}

	c := newCaster(t)
	c.castOn(t, strongLow, 0)
	c.castOn(t, weakHigh, 10*time.Second) // the higher rank, weaker
	if got := c.effectNames(); len(got) != 1 || got[0] != "Strong Low" || c.p.Mods.MaxHP != 134 {
		t.Fatalf("a weaker higher rank over a stronger: effects %v, max HP +%d", got, c.p.Mods.MaxHP)
	}
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], "Strong Low, which is stronger") {
		t.Errorf("the caster was not told: %q", red)
	}

	c = newCaster(t)
	c.castOn(t, weakHigh, 0)
	c.castOn(t, strongLow, 10*time.Second) // the stronger lower rank takes it off
	if got := c.effectNames(); len(got) != 1 || got[0] != "Strong Low" || c.p.Mods.MaxHP != 134 {
		t.Fatalf("a stronger lower rank over a weaker higher: effects %v, max HP +%d", got, c.p.Mods.MaxHP)
	}

	plain := buffOf(9003, 49, 0, "Warcry", "Self, Increase MaxHP by 650, 10min")
	ascended := buffOf(9004, 49, 0, "Ascended Warcry", "Self, Increase MaxHP by 715, 10min")
	c = newCaster(t)
	c.castOn(t, ascended, 0)
	c.castOn(t, plain, 10*time.Second) // of one rank, weaker: refused
	if got := c.effectNames(); len(got) != 1 || got[0] != "Ascended Warcry" || c.p.Mods.MaxHP != 715 {
		t.Fatalf("a weaker of one rank after a stronger: effects %v, max HP +%d", got, c.p.Mods.MaxHP)
	}
	c.castOn(t, ascended, 20*time.Second) // a recast of itself still replaces itself
	if got := c.effectNames(); len(got) != 1 || got[0] != "Ascended Warcry" {
		t.Fatalf("a recast: effects %v", got)
	}
}

// Two of a line as strong: the rank decides, as before (a shield's, a proc's
// or a speed's line gives no stat to weigh).
func TestALineOfEqualStrengthGoesByRank(t *testing.T) {
	a := &ActiveEffect{SpellID: 1, Line: "stack2 player +AC", Rank: 30, Mods: StatMods{AC: 100}}
	b := &ActiveEffect{SpellID: 2, Line: "stack2 player +AC", Rank: 20, Mods: StatMods{AC: 100}}
	if !lineOutranks(a, b) || lineOutranks(b, a) {
		t.Errorf("equal strength: want the higher rank to outrank")
	}
	if e := lineVerdict([]*ActiveEffect{a}, b); e != a {
		t.Errorf("the lower rank of one strength was not refused: %v", e)
	}
	s1 := &ActiveEffect{SpellID: 3, Line: "stack1 player shield", Rank: 40}
	s2 := &ActiveEffect{SpellID: 4, Line: "stack1 player shield", Rank: 50}
	if lineOutranks(s1, s2) || !lineOutranks(s2, s1) {
		t.Errorf("a shield's line: want the rank alone")
	}
	// the stats weighed are the shared ones: Concentration (+DEX +WIS) and
	// Group Focus (+DEX +INT) share DEX alone
	conc := &ActiveEffect{SpellID: 5, Line: "stack3 player +DEX +WIS", Rank: 60, Mods: StatMods{Dex: 55, Wis: 55}}
	focus := &ActiveEffect{SpellID: 6, Line: "stack3 player +DEX +INT", Rank: 29, Mods: StatMods{Dex: 60, Int: 60}}
	if lineStrength(conc, focus) != 55 || lineStrength(focus, conc) != 60 || !lineOutranks(focus, conc) {
		t.Errorf("shared DEX: %d vs %d", lineStrength(conc, focus), lineStrength(focus, conc))
	}
}
