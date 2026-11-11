package entity

import "testing"

// lfgLevelRange is the live client's LevelRange as vanilla's level_range
// ports it: its tests' figures (groupsearch.rs), the chopped and the rounded
// product alike away from a tie; at lo 50 (66.5, a tie) the two differ.
func TestLFGLevelRangeIsTheClients(t *testing.T) {
	for _, c := range []struct{ lo, hi, low, high int }{
		{1, 1, 1, 7}, {10, 10, 4, 16}, {30, 30, 22, 40}, {60, 60, 45, 60}, {20, 30, 22, 27}, {10, 50, 16, 16},
	} {
		r := lfgLevelRange(c.lo, c.hi)
		for _, got := range r {
			if got != [2]int{c.low, c.high} {
				t.Errorf("levels %d-%d: %v, want %d-%d", c.lo, c.hi, r, c.low, c.high)
			}
		}
	}
	if r := lfgLevelRange(50, 50); r[0][1] != 60 || r[1][1] != 60 {
		t.Errorf("50: %v", r) // 67 either way, held to 60
	}
	if r := lfgLevelRange(45, 45); r[0] != [2]int{33, 60} && r[0] != [2]int{33, 59} {
		t.Errorf("45: %v", r)
	}
}
