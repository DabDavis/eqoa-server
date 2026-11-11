package entity

import "testing"

// /scale (the owner): a factor of the body's own size, on the GM's target or the GM, and 1 gives
// the size back; out-of-range factors change nothing.
func TestGMScale(t *testing.T) {
	w := &World{Players: map[EntityID]*Player{}, NPCs: map[EntityID]*NPC{}}
	gm := &Player{ServerID: 1, Name: "Gm", Size: 1}
	w.Players[1] = gm
	if _, ok := w.GMScale(gm, 2.5); !ok || gm.Size != 2.5 {
		t.Fatalf("no target: the GM scaled to %v", gm.Size)
	}
	if _, ok := w.GMScale(gm, 2); !ok || gm.Size != 2 {
		t.Errorf("a second scale is of the own size, not the scaled: %v", gm.Size)
	}
	if _, ok := w.GMScale(gm, 1); !ok || gm.Size != 1 || gm.gmScaleBase != 0 {
		t.Errorf("1 gives the own size back: %v (base %v)", gm.Size, gm.gmScaleBase)
	}
	if _, ok := w.GMScale(gm, 0); ok || gm.Size != 1 {
		t.Errorf("0 is refused: %v", gm.Size)
	}
	if _, ok := w.GMScale(gm, 50); ok || gm.Size != 1 {
		t.Errorf("50 is refused: %v", gm.Size)
	}
}
