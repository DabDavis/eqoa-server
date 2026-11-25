package entity

import (
	"slices"
	"testing"
)

// Heirloom petition 3095 (Yevkassim): "AR buff missing from Enchanter
// Mercenary spellbook". The enchanter's one arcane resist buff, Endure Arcane
// (level 7, AR 25 + 5% CHA, 30 minutes), is kept in a level 60 enchanter
// merc's book though the floor (mercBuffFloor) leaves out a plain buff of
// under a third of its level; the control, a level 1 +1 INT, is still left
// out, and a stronger buff of the resist's line still takes it out.
func TestMercKeepsALowResistBuff(t *testing.T) {
	endure := buffOf(506, 7, 1, "Endure Arcane", "Target, Increase AR by 25 ; Increase 5% based on CHA, 30min")
	wisdom := buffOf(234, 1, 0, "Divine Wisdom", "Self, Increase INT by 1, 30min")
	if !mercResistBuff(&endure) || mercResistBuff(&wisdom) {
		t.Fatalf("resist buffs: Endure Arcane %v, Divine Wisdom %v", mercResistBuff(&endure), mercResistBuff(&wisdom))
	}
	names := func(spells ...CastSpell) []string {
		k := MercKit{Class: classEnchanter, Level: 60, Spells: spells}
		ms := &mercState{kit: k, body: newMercBody(1, &k)}
		var out []string
		for _, sp := range ms.mercBook() {
			out = append(out, sp.s.Name)
		}
		return out
	}
	got := names(endure, wisdom)
	if !slices.Contains(got, "Endure Arcane") || slices.Contains(got, "Divine Wisdom") {
		t.Errorf("a level 60 enchanter's book: %v, want Endure Arcane and not Divine Wisdom", got)
	}
	ward := buffOf(9001, 40, 1, "Arcane Ward", "Target, Increase AR by 80, 30min")
	if got := names(endure, ward); slices.Contains(got, "Endure Arcane") || !slices.Contains(got, "Arcane Ward") {
		t.Errorf("with a stronger arcane buff: %v, want Arcane Ward alone", got)
	}
}
