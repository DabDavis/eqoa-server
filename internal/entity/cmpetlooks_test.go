package entity

import (
	"reflect"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A line whose ranks each replace the one before (Fallenangel's petition
// 1994: the necromancer's pet looks, as tools/pet_looks_20261113.py writes
// them): bought rank by rank the book holds one pet; a reset (every rank at
// once, in any order) gives back the first spell alone and takes every look
// out, where undoing rank by rank by id had put the earlier looks back; a
// template's ranks bought at once give the last look alone; the last rank
// removed gives back the look before it.
func TestCMLineOfLooks(t *testing.T) {
	const all, races = 32767, 2047
	w := &World{}
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 6368, Classes: all, Races: races, Name: "Unholy Sacrament 4", Level: 60, Requires: "Swarming Death@60", RequireSpells: "1",
			Adds: "Bone Swarming Death (60)@60", AddSpells: "2", Removes: "Swarming Death@60", RemoveSpells: "1", Enabled: true},
		{ID: 6371, Classes: all, Races: races, Name: "Bone Collection", Level: 60, Adds: "Bone Swarming Death (60)@60", AddSpells: "2",
			RequireRanks: "6368", Enabled: true},
		{ID: 6372, Classes: all, Races: races, Name: "Unholy Ghoulish Sacrament", Parent: 6371, Level: 60,
			Adds: "Ghoulish Death@60", AddSpells: "3", Removes: "Bone Swarming Death (60)@60", RemoveSpells: "2", Enabled: true},
		{ID: 6373, Classes: all, Races: races, Name: "Wraith's Unholy Sacrament", Parent: 6372, Level: 60,
			Adds: "Haunting Death@60", AddSpells: "4", Removes: "Ghoulish Death@60", RemoveSpells: "3", Enabled: true},
		{ID: 6374, Classes: all, Races: races, Name: "Crimson Unholy Sacrament", Parent: 6373, Level: 60,
			Adds: "Red Swarming Death (60)@60", AddSpells: "5", Removes: "Haunting Death@60", RemoveSpells: "4", Enabled: true},
	})
	ids := func(as []CMAbility) []int {
		out := []int{}
		for _, a := range as {
			out = append(out, a.SpellID)
		}
		sort.Ints(out)
		return out
	}
	line := []int{6368, 6371, 6372, 6373, 6374}
	// bought rank by rank: the book is Swarming Death's place held by the last look alone
	book := map[int]bool{1: true}
	for _, id := range line {
		g, tk := w.cms.byID[id].BookChange()
		for _, a := range g {
			book[a.SpellID] = true
		}
		for _, a := range tk {
			delete(book, a.SpellID)
		}
	}
	if !reflect.DeepEqual(book, map[int]bool{5: true}) {
		t.Fatalf("the line bought rank by rank: book %v, want the Red Swarming Death alone", book)
	}
	ranks := func(ids ...int) []*CMRank {
		var out []*CMRank
		for _, id := range ids {
			out = append(out, w.cms.byID[id])
		}
		return out
	}
	// a reset, in either order
	for _, order := range [][]int{line, {6374, 6373, 6372, 6371, 6368}} {
		out, back := CMRemovalBook(ranks(order...), nil)
		if !reflect.DeepEqual(ids(back), []int{1}) || !reflect.DeepEqual(ids(out), []int{2, 3, 4, 5}) {
			t.Errorf("reset of %v: back %v, out %v; want [1] back, [2 3 4 5] out", order, ids(back), ids(out))
		}
	}
	// the control: undone rank by rank in id order, as a reset did before, the looks came back
	book = map[int]bool{5: true}
	for _, id := range line {
		g, tk := w.cms.byID[id].BookChange()
		for _, a := range g {
			delete(book, a.SpellID)
		}
		for _, a := range tk {
			book[a.SpellID] = true
		}
	}
	if len(book) == 1 {
		t.Errorf("the control: undone rank by rank the book is %v, expected the earlier looks back", book)
	}
	// a template's ranks bought at once
	give, take := CMAddBook(ranks(line...))
	if !reflect.DeepEqual(ids(give), []int{5}) || !reflect.DeepEqual(ids(take), []int{1, 2, 3, 4}) {
		t.Errorf("the line bought at once: give %v, take %v; want [5], [1 2 3 4]", ids(give), ids(take))
	}
	// the last rank removed, the rest kept: the look before it back
	back, out := w.cmRemovalBookLocked(&Player{CMs: map[int]int{6368: 15, 6371: 1, 6372: 1, 6373: 1}}, w.cms.byID[6374])
	if !reflect.DeepEqual(ids(back), []int{4}) || !reflect.DeepEqual(ids(out), []int{5}) {
		t.Errorf("Crimson removed: back %v, out %v; want [4] back, [5] out", ids(back), ids(out))
	}
	// the three lasts removed at once (a template keeping 6368 and 6371): Bone Swarming Death back
	out, back = CMRemovalBook(ranks(6372, 6373, 6374), ranks(6368, 6371))
	if !reflect.DeepEqual(ids(back), []int{2}) || !reflect.DeepEqual(ids(out), []int{3, 4, 5}) {
		t.Errorf("6372-6374 removed, 6368 kept: back %v, out %v; want [2] back, [3 4 5] out", ids(back), ids(out))
	}
}
