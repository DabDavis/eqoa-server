package main

import (
	"testing"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

// world.held_rare_one_in, held_level_window and held_level_above: unset the defaults
// (1 in 500, levelreq level-5..level+2), 0 a rare record never (the owner, 2026-11-21).
func TestHeldDropRulesFromConfig(t *testing.T) {
	d := database.BuildHeldDrops(nil, nil)
	heldDropRules(config.WorldConfig{}, d)
	if d.RareOneIn != 500 || d.LevelBelow != 5 || d.LevelAbove != 2 {
		t.Errorf("unset: 1 in %d, -%d..+%d, want 500, 5, 2", d.RareOneIn, d.LevelBelow, d.LevelAbove)
	}
	zero, below, above := 0, 8, 1
	heldDropRules(config.WorldConfig{HeldRareOneIn: &zero, HeldLevelWindow: &below, HeldLevelAbove: &above}, d)
	if d.RareOneIn != 0 || d.LevelBelow != 8 || d.LevelAbove != 1 {
		t.Errorf("set: 1 in %d, -%d..+%d, want 0, 8, 1", d.RareOneIn, d.LevelBelow, d.LevelAbove)
	}
	for _, roll := range []float64{0, 0.0001, 0.5} {
		if d.RollsRare(roll) {
			t.Errorf("held_rare_one_in 0 drew rare at %v", roll)
		}
	}
}
