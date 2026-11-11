package entity

import (
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// BookChange leaves out a spell a rank both adds and removes (an improvement
// in place: jib's review, F3 pass 19) and keeps the rest in the list's order;
// an ability never found (spell 0) stays a name, given nothing. The control:
// a rank adding one spell and removing another swaps them.
func TestCMBookChangeKeepsASpellImprovedInPlace(t *testing.T) {
	names := func(as []CMAbility) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.Name)
		}
		return out
	}
	r := ParseCMRank(database.CMRankRow{ID: 1, Name: "Edge 1",
		Adds: "Impact@16; Concussion@20; Glyph@0", AddSpells: "11,12,0",
		Removes: "Concussion@4; Impact@16; Old Glyph@4", RemoveSpells: "12,11,13"})
	gives, takes := r.BookChange()
	if !reflect.DeepEqual(names(gives), []string{"Glyph"}) || !reflect.DeepEqual(names(takes), []string{"Old Glyph"}) {
		t.Errorf("gives %v, takes %v: want [Glyph] and [Old Glyph] (Impact and Concussion kept)", names(gives), names(takes))
	}

	swap := ParseCMRank(database.CMRankRow{ID: 2, Name: "Cry 1", Adds: "Battle Cry@20", AddSpells: "501",
		Removes: "Taunt@1", RemoveSpells: "500"})
	gives, takes = swap.BookChange()
	if len(gives) != 1 || gives[0].SpellID != 501 || len(takes) != 1 || takes[0].SpellID != 500 {
		t.Errorf("the control: Cry 1 gives %+v, takes %+v; want 501 given, 500 taken", gives, takes)
	}
}

// require_quests is read as the quests' requires are (database.ParseRequires):
// ids apart are each needed, ids joined by | one group any of them meets (a
// werebeast's rank: any were's form quest; the user, 2026-11-01). A quest not
// in the game bars no one, in a group too: the group needs one of those in
// the game, and none in the game bars no one. The control: a plain list
// still needs every quest of it.
func TestCMQuestGroupIsMetByAnyOfItsQuests(t *testing.T) {
	w := NewWorld()
	all, races := 32767, 2047
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 1, Classes: all, Races: races, Name: "Bite", Level: 1, RequireQuests: "10|11|12", Enabled: true},
		{ID: 2, Classes: all, Races: races, Name: "Both", Level: 1, RequireQuests: "10, 11", Enabled: true},
		{ID: 3, Classes: all, Races: races, Name: "Drafts", Level: 1, RequireQuests: "12|13", Enabled: true},
		{ID: 4, Classes: all, Races: races, Name: "Old", Level: 1, RequireQuests: "11", Enabled: true},
	})
	p := &Player{Level: 60}
	// 10 and 11 in the game, 12 and 13 drafts (not in it)
	k := CMKnows{Quests: map[int]bool{}, QuestNames: map[int]string{10: "The Hunt", 11: "Mirin"}}
	why := func(id int) string { return w.cmBlockLocked(p, w.cms.byID[id], k) }
	if got := w.cms.byID[1].RequireQuests; !reflect.DeepEqual(got, []int{10, 11, 12}) {
		t.Errorf("Bite's quests read %v", got)
	}
	if got := why(1); got != "You must finish one of The Hunt, Mirin to buy Bite." {
		t.Errorf("Bite with no quest done: %q", got)
	}
	if got := why(2); got != "You must finish The Hunt to buy Both." {
		t.Errorf("Both with no quest done: %q", got)
	}
	if got := why(3); got != "" {
		t.Errorf("a group of drafts barred Drafts: %q", got)
	}
	k.Quests[11] = true
	if got := why(1); got != "" {
		t.Errorf("Bite with Mirin done: %q", got)
	}
	if got := why(4); got != "" {
		t.Errorf("Old with its quest done: %q", got)
	}
	if got := why(2); got != "You must finish The Hunt to buy Both." {
		t.Errorf("the control: Both with only Mirin done: %q", got)
	}
}

// require_spells "a|b": a rank needs a, or b standing in for it (petition
// 1583, Wisp: the lightning nova line, Static Destruction, was locked to a
// wizard who owned Icy Destruction, which had traded away the Explode both
// need; each gives Fiery Explode, which the other then accepts). The
// control: the same rank needing Explode alone stays locked. And a removal
// leaves what a rank kept gives or replaces: removing Static with Icy kept
// takes out Static Explode only and hands back no Explode; removing Icy
// alone takes both its spells and hands Explode back.
func TestCMRequireSpellAltAndRemovalKeepsOtherRanksBook(t *testing.T) {
	w := NewWorld()
	all, races := 32767, 2047
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 1, Classes: all, Races: races, Name: "Icy Destruction 1", Level: 20, Requires: "Explode@12", RequireSpells: "11|22",
			Adds: "Icy Explode@12; Fiery Explode@12", AddSpells: "21,22", Removes: "Explode@12", RemoveSpells: "11", Enabled: true},
		{ID: 2, Classes: all, Races: races, Name: "Static Destruction 1", Level: 20, Requires: "Explode@12", RequireSpells: "11|22",
			Adds: "Static Explode@12; Fiery Explode@12", AddSpells: "23,22", Removes: "Explode@12", RemoveSpells: "11", Enabled: true},
		{ID: 3, Classes: all, Races: races, Name: "Old Static 1", Level: 20, Requires: "Explode@12", RequireSpells: "11",
			Adds: "Static Explode@12", AddSpells: "23", Removes: "Explode@12", RemoveSpells: "11", Enabled: true},
	})
	if got := w.cms.byID[2].Requires; len(got) != 1 || got[0].SpellID != 11 || !reflect.DeepEqual(got[0].Alts, []int{22}) {
		t.Fatalf("Static's requirement read %+v", got)
	}
	p := &Player{Level: 60, CMs: map[int]int{1: 25}}
	k := CMKnows{Book: map[int]bool{21: true, 22: true}} // Icy bought: Explode traded for Icy and Fiery Explode
	if got := w.cmBlockLocked(p, w.cms.byID[2], k); got != "" {
		t.Errorf("Static Destruction 1 after Icy: %q, want buyable", got)
	}
	if got := w.cmBlockLocked(p, w.cms.byID[3], k); got != "You must know Explode to buy Old Static 1." {
		t.Errorf("the control, Explode alone needed: %q", got)
	}
	if got := w.cmBlockLocked(&Player{Level: 60}, w.cms.byID[2], CMKnows{Book: map[int]bool{11: true}}); got != "" {
		t.Errorf("Static with Explode itself: %q", got)
	}
	if got := w.cmBlockLocked(&Player{Level: 60}, w.cms.byID[2], CMKnows{Book: map[int]bool{}}); got != "You must know Explode to buy Static Destruction 1." {
		t.Errorf("Static with neither: %q", got)
	}

	ids := func(as []CMAbility) []int {
		out := []int{}
		for _, a := range as {
			out = append(out, a.SpellID)
		}
		return out
	}
	// both owned, Static removed (the player's ranks without it)
	back, out := w.cmRemovalBookLocked(&Player{CMs: map[int]int{1: 25}}, w.cms.byID[2])
	if !reflect.DeepEqual(ids(back), []int{}) || !reflect.DeepEqual(ids(out), []int{23}) {
		t.Errorf("removing Static with Icy kept: back %v, out %v; want none back, [23] out", ids(back), ids(out))
	}
	back, out = w.cmRemovalBookLocked(&Player{CMs: map[int]int{}}, w.cms.byID[1])
	if !reflect.DeepEqual(ids(back), []int{11}) || !reflect.DeepEqual(ids(out), []int{21, 22}) {
		t.Errorf("the control, Icy alone removed: back %v, out %v; want [11] back, [21 22] out", ids(back), ids(out))
	}
}
