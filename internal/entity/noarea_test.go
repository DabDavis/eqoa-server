package entity

import "testing"

// Heirloom petition 2740: a creature table npcnoarea names (Zaoran Plateau's
// Unmarked Headstones) is passed by a storm landing beside it, and still takes
// a storm cast at it (its own target) and a spell of one target; the control,
// with no row, the storm takes it.
func TestNoAreaTouchesANoAreaCreature(t *testing.T) {
	layout := func(spared bool) (*areaRig, *NPC, *NPC) {
		r := newAreaRig(t)
		if spared {
			r.w.SetNoAreaNames(map[string]bool{"unmarked headstone": true})
		}
		return r, r.creature(2001, "Deathfist Pawn", 1020, 1000), r.creature(2002, "Unmarked Headstone", 1020, 1010)
	}
	r, pawn, stone := layout(true)
	r.cast(t, fireStorm(), pawn.ServerID)
	if got, want := hits(pawn, stone), []int{82, 0}; !equalInts(got, want) {
		t.Errorf("a storm at the pawn: lost %v, want %v (the pawn, the headstone beside it)", got, want)
	}

	r, pawn, stone = layout(true)
	r.cast(t, fireStorm(), stone.ServerID)
	if got := hits(stone, pawn); got[0] != 82 || got[1] != 36 {
		t.Errorf("a storm at the headstone: lost %v, want [82 36] (its own target, the pawn by it)", got)
	}
	r, _, stone = layout(true)
	one := fireStorm()
	one.Area = SpellArea{}
	r.cast(t, one, stone.ServerID)
	if got := hits(stone); got[0] != 82 {
		t.Errorf("a spell of one target at the headstone: lost %d, want 82", got[0])
	}

	r, pawn, stone = layout(false)
	r.cast(t, fireStorm(), pawn.ServerID)
	if got := hits(stone); got[0] != 36 {
		t.Errorf("with no row: the storm took %d from the headstone, want 36", got[0])
	}
}
