package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Death Sacrament as tools/fix_death_sacrament.py leaves its rows, bought
// through the mastery window: rank 1 takes Decaying Flesh 914 out of the
// book and puts its version with weapons, 918, in; removing the rank gives
// 914 back and takes 918; rank 7 (no Bone Swarm in the game's records) takes
// nothing. The purchase code needs no change: it gives and takes by spell id
// (BookChange, giveAbilitiesTo, takeAbilitiesFrom). The controls: a
// necromancer who buys nothing keeps 914; the rows as tools/seed_masteries.py
// wrote them (914 added and removed) swap nothing, and their rank 7 took
// Swarming Bones and gave nothing.

// deathSacramentRows is ranks 1 and 7 of Death Sacrament, fixed or as seeded.
func deathSacramentRows(fixed bool) []database.CMRankRow {
	r1 := database.CMRankRow{ID: 6361, Category: 3, Classes: 1 << 11, Races: 2047, Mastery: "Death Sacrament", Rank: 1,
		Name: "Death Sacrament 1", Cost: 15, Level: 25, Requires: "Decaying Flesh@24", RequireSpells: "914",
		Adds: "Decaying Flesh@25", AddSpells: "918", Removes: "Decaying Flesh@24", RemoveSpells: "914",
		Description: "Pet weapons (Increases melee damage)", Enabled: true}
	r7 := database.CMRankRow{ID: 6367, Category: 3, Classes: 1 << 11, Races: 2047, Mastery: "Death Sacrament", Rank: 7,
		Name: "Death Sacrament 7", Cost: 15, Level: 55, Requires: "Swarming Bones@55", RequireSpells: "700736",
		Adds: "Bone Swarm@55", Description: "Pet weapons (Increases melee damage)", Enabled: true}
	if !fixed {
		r1.Adds, r1.AddSpells = "Decaying Flesh@24", "914"
		r7.Removes, r7.RemoveSpells = "Swarming Bones@55", "700736"
	}
	return append(testCatalog(), r1, r7)
}

// newNecromancerCM is a necromancer of 60 with Decaying Flesh 914 and
// Swarming Bones in its book and points to spend.
func newNecromancerCM(t *testing.T, fixed bool) *cmTest {
	t.Helper()
	c := newCMTest(t, 11, 0, 60)
	c.w.gw.SetCMCatalog(deathSacramentRows(fixed))
	c.book.spells = map[int]string{914: "Decaying Flesh", 700736: "Swarming Bones"}
	c.kept.book = c.book
	c.p.Mastery.Points = 200
	return c
}

func TestDeathSacramentSwapsInTheWeaponPet(t *testing.T) {
	c := newNecromancerCM(t, true)
	if rs, red := c.send(cmBuy, 6361); len(red) != 0 || len(rs) == 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Death Sacrament 1: % x %v", rs, red)
	}
	if _, base := c.book.spells[914]; base || !slices.Equal(c.book.added, []int{918}) || !slices.Equal(c.kept.removedSpells, []int{914}) {
		t.Errorf("Death Sacrament 1: added %v, took %v, 914 still there %v; want 918 in and 914 out", c.book.added,
			c.kept.removedSpells, base)
	}
	if rs, red := c.send(cmRemove, 6361); len(red) != 0 || len(rs) == 0 || rs[0][0] != cmRespRemoved {
		t.Fatalf("removing Death Sacrament 1: % x %v", rs, red)
	}
	_, base := c.book.spells[914]
	_, weapon := c.book.spells[918]
	if !base || weapon {
		t.Errorf("removed: 914 back %v, 918 still there %v; want 914 back and 918 gone", base, weapon)
	}

	c.kept.removedSpells = nil
	if rs, red := c.send(cmBuy, 6367); len(red) != 0 || len(rs) == 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Death Sacrament 7: % x %v", rs, red)
	}
	if _, ok := c.book.spells[700736]; !ok || len(c.kept.removedSpells) != 0 {
		t.Errorf("Death Sacrament 7 took %v, Swarming Bones kept %v; want nothing taken", c.kept.removedSpells, ok)
	}

	// the controls
	idle := newNecromancerCM(t, true)
	if _, kept := idle.book.spells[914]; !kept || len(idle.book.added) != 0 || len(idle.kept.removedSpells) != 0 {
		t.Errorf("a necromancer who bought nothing: 914 kept %v, added %v, taken %v", kept, idle.book.added, idle.kept.removedSpells)
	}
	seeded := newNecromancerCM(t, false)
	seeded.send(cmBuy, 6361)
	if _, kept := seeded.book.spells[914]; !kept || len(seeded.book.added) != 0 || len(seeded.kept.removedSpells) != 0 {
		t.Errorf("the seeded rank 1 (914 added and removed): 914 kept %v, added %v, taken %v; want no swap", kept,
			seeded.book.added, seeded.kept.removedSpells)
	}
	seeded.send(cmBuy, 6367)
	if _, kept := seeded.book.spells[700736]; kept || !slices.Equal(seeded.kept.removedSpells, []int{700736}) {
		t.Errorf("the seeded rank 7: Swarming Bones kept %v, taken %v; want it taken (the bug)", kept, seeded.kept.removedSpells)
	}
}
