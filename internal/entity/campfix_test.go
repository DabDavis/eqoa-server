package entity

import "testing"

// layeredGround is terrain at 48 with a ledge at 54 on x 100-110: Floor is
// the highest of them no more than the ground's step (2) above the height
// asked, as internal/ground's.
type layeredGround struct{}

func (layeredGround) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	best, found := float32(0), false
	for _, h := range []float32{48, 54} {
		if h == 54 && (x < 100 || x > 110) {
			continue
		}
		if h <= y+campfixStep && (!found || h > best) {
			best, found = h, true
		}
	}
	if !found {
		return y, false
	}
	return best, true
}

// A camp report's add or move stands on the ground's highest floor within 2.5
// of the mark's height, above or below it, and keeps the mark's own height
// with none that near or no ground at all.
func TestCampfixFloorNearIsTheHighestFloorWithinReach(t *testing.T) {
	w := NewWorld()
	if y, ok := w.CampfixFloorNear(0, 50, 50, 50, 2.5); ok || y != 50 {
		t.Fatalf("no ground: %v %v, want the mark's own", y, ok)
	}
	w.SetGround(layeredGround{})
	for _, tc := range []struct {
		x, y  float32
		floor float32
		ok    bool
	}{
		{50, 50, 48, true},    // 2 above the terrain
		{50, 46, 48, true},    // 2 below it: the floor above is within reach
		{50, 50.5, 48, true},  // 2.5 above: the edge of the reach
		{50, 51, 51, false},   // the control: 3 above, out of reach
		{50, 45, 45, false},   // 3 below
		{105, 55, 54, true},   // on the ledge, the terrain under it not taken
		{105, 50, 48, true},   // under the ledge, 4 below it: the terrain
		{105, 52.5, 54, true}, // 1.5 under the ledge: the ledge, the highest within reach
	} {
		y, ok := w.CampfixFloorNear(0, tc.x, tc.y, 0, 2.5)
		if ok != tc.ok || y != tc.floor {
			t.Errorf("at (%v, %v): %v %v, want %v %v", tc.x, tc.y, y, ok, tc.floor, tc.ok)
		}
	}
}
