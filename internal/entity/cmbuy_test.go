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
