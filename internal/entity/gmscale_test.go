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

// /speed (the owner): a factor of the own speed, 1 gives it back.
func TestGMSpeed(t *testing.T) {
	p := &Player{Speed: 8}
	if got := p.SetGMSpeed(2); got != 16 {
		t.Fatalf("2x of 8: %v", got)
	}
	if got := p.SetGMSpeed(10); got != 80 || p.GMSpeedBase() != 8 {
		t.Errorf("10x is of the own 8, not of 16: %v (base %v)", got, p.GMSpeedBase())
	}
	if got := p.SetGMSpeed(1); got != 8 || p.gmSpeedBase != 0 {
		t.Errorf("1 gives 8 back: %v", got)
	}
}
