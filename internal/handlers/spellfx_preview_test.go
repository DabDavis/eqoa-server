package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// fakeSpells is a spell book of lines by id, for SpellVisualOf.
type fakeSpells map[int]string

func (f fakeSpells) SpellPattern(id int) (*database.SpellPatternRecord, error) {
	if _, ok := f[id]; !ok {
		return nil, fmt.Errorf("no spell %d", id)
	}
	return &database.SpellPatternRecord{SpellID: id, SpellName: fmt.Sprintf("Spell %d", id), CastTime: 2}, nil
}

func (f fakeSpells) SpellEffect(id int) (string, uint32, bool, error) {
	return f[id], 0, f[id] != "", nil
}

// What the GM panel's preview reads off a spell: whether its line is cast at
// an enemy (a training dummy stands before the caster then) and whom it lands
// on (anything else is cast on the caster).
func TestSpellVisualSaysHarmfulAndTarget(t *testing.T) {
	book := fakeSpells{
		1: "Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX",
		2: "Target, Increase HP per tic by 20 ; Increase 25% based on CHA, 24sec",
		3: "Self, Increase WIS by 1, 30min",
		4: "Group, Increase Power per tic by 76, 24sec",
		5: "Target undead, Decrease Hitpoints by 194",
	}
	want := map[int]struct {
		harmful bool
		target  string
	}{1: {true, "target"}, 2: {false, "target"}, 3: {false, "self"}, 4: {false, "group"}, 5: {true, "target undead"}}
	for id, w := range want {
		v, err := SpellVisualOf(book, id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Harmful != w.harmful || v.Target != w.target {
			t.Errorf("spell %d (%q): harmful %v target %q, want %v %q", id, book[id], v.Harmful, v.Target, w.harmful, w.target)
		}
	}
	// the control: a nuke read as a heal would fail the first case
	if v, _ := SpellVisualOf(fakeSpells{1: "Target, Increase Hitpoints by 26"}, 1); v.Harmful {
		t.Error("the control must fail: a heal is not harmful")
	}
}
