package ground

import (
	"os"
	"testing"
)

// The owner's report of 2026-11-18 on the real ground (EQOA_GROUND_DIR):
// a player on the terrain under the edge of the rock east of Noroxus in
// Secrets (world 5; the rock is the LOD sprite 0x6630AE67 at 4240.1, -20.4,
// 2167.7, its level 0's collision in 5.walls), and put out beside it.
// Controls: the open ground round it and the rock's own top are free, and the
// middle of the rock is in it. Skipped without a baked ground.
func TestRealLonewulfInTheRockByNoroxus(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	const w, mid, r = 5, 1.2, 0.45
	x, y, z := float32(4238.5), float32(-21.2), float32(2164)

	// the terrain alone reads a plain floor there (the report's probe)
	if f, ok := g.Floor(w, x, 200, z); !ok || f < -21.3 || f > -21.0 {
		t.Fatalf("terrain floor at the spot %.2f %v, want -21.17", f, ok)
	}
	// but the rock is in the walls file: its underside 1.5 over the
	// terrain, its top 3.5
	var under, over bool
	for _, s := range g.Surfaces(w, x, z, -25, -10) {
		if s.Placed && !s.Up && s.Y > -20 && s.Y < -19.5 {
			under = true
		}
		if s.Placed && s.Up && s.Y > -18 && s.Y < -17.5 {
			over = true
		}
	}
	if !under || !over {
		t.Fatalf("no rock over the spot: %v", g.Surfaces(w, x, z, -25, -10))
	}
	if !g.Embedded(w, x, y, z, mid, r) {
		t.Fatal("Loneowls's place is not in the rock")
	}
	// the rock's middle (on the terrain under it) is in it too
	if !g.Embedded(w, 4240, -21.5, 2166, mid, r) {
		t.Fatal("the rock's middle is not in the rock")
	}
	// out: the nearest free place, beside it (its top is steep there)
	fx, fy, fz, top, ok := g.Free(w, x, y, z, mid, 0.75)
	if !ok {
		t.Fatal("no free place found")
	}
	t.Logf("put out at (%.2f, %.2f, %.2f), on top %v", fx, fy, fz, top)
	if g.Embedded(w, fx, fy, fz, mid, 0.75) {
		t.Fatal("the free place is in the rock")
	}
	if d := (fx-x)*(fx-x) + (fz-z)*(fz-z); d > 3*3 {
		t.Fatalf("put out %.1f away, want within 3", d)
	}
	if fy < y-1 || fy > y+1 {
		t.Fatalf("put out at height %.2f, want the ground's (%.2f)", fy, y)
	}

	// controls: open ground round the rock is free
	for _, p := range [][3]float32{{4238.5, -21.74, 2154}, {4228.5, -19.70, 2164}, {4248.5, -24.26, 2154}} {
		f, ok := g.Floor(w, p[0], p[1]+5, p[2])
		if !ok {
			t.Fatalf("no floor at %v", p)
		}
		if g.Embedded(w, p[0], f, p[2], mid, r) {
			t.Fatalf("open ground at (%.1f, %.2f, %.1f) read as in something", p[0], f, p[2])
		}
	}
	// and a body on the rock's top is on it, not in it
	if h, placed, _, ok := g.Stand(w, 4240, -14, 2167); !ok || !placed {
		t.Fatalf("no placed floor on the rock's top: %.2f %v %v", h, placed, ok)
	} else if g.Embedded(w, 4240, h, 2167, mid, r) {
		t.Fatalf("on the rock's top at %.2f read as in it", h)
	}
}
