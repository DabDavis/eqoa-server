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
