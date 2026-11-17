package entity

import (
	"math"
	"os"
	"testing"
)

// Heirloom petition 2202 on the real ground (EQOA_GROUND_DIR, EQOA_REAL_DB;
// EQOA_MODELS for every body's edge, else the Megaladon's 8.332 alone): the
// Megaladon (size 4) where it fought Zerkziee, its feet at 50.8 in the channel
// along the Isle of Dread's beach, looked at from the dry sand 20 off round
// it. Five places on the beach had no sight of it at a man's heights (the
// sand over its head) and see its body now; the one behind the rise of the
// beach to its south-east is still hidden, at every height up to its scaled
// head. Skipped without the files.
func TestRealMegaladonSeenFromTheBeach(t *testing.T) {
	const shark uint32 = 0x69842C02
	w, _, done := landWaterWorld(t)
	defer done()
	radii := map[uint32]float32{shark: 8.332}
	if m := os.Getenv("EQOA_MODELS"); m != "" {
		if r, err := LoadModelRadii(m); err == nil && len(r) > 0 {
			radii = r
		}
	}
	SetModelRadii(radii)
	t.Cleanup(func() { SetModelRadii(nil) })
	w.mu.Lock()
	defer w.mu.Unlock()
	meg := &NPC{Name: "Megaladon", ModelID: int64(shark), Size: 4, World: 0, X: 24272.2, Y: 50.8, Z: 3046.0}
	for a, want := range map[int]bool{0: true, 1: true, 3: true, 4: true, 5: true, 2: false} {
		s, c := math.Sincos(float64(a) * math.Pi / 8)
		p := &Player{Name: "Zerkziee", World: 0, X: meg.X + 20*float32(c), Z: meg.Z + 20*float32(s)}
		y, ok := w.ground.Floor(0, p.X, 200, p.Z)
		if !ok {
			t.Fatalf("no floor at (%.1f, %.1f)", p.X, p.Z)
		}
		p.Y = y
		if w.wetAtLocked(0, p.X, p.Y, p.Z) {
			t.Fatalf("(%.1f, %.1f, %.1f) is in the water", p.X, p.Y, p.Z)
		}
		was := w.inSightLocked(0, p.X, p.Y, p.Z, meg.X, meg.Y, meg.Z)
		if got := w.seesCreatureLocked(p, meg); got != want || (want && was) {
			t.Errorf("from (%.1f, %.1f, %.1f): seen %v (at a man's heights %v), want %v", p.X, p.Y, p.Z, got, was, want)
		}
	}
}

// The same five places on the beach are reached by the Megaladon's breath
// (its Pestilent Breath, radius 40): an area's line from a creature looks
// from its own heights too (sight.go's clearFromBodyLocked), where at a man's
// over its feet the sand spared all of them. The place behind the rise is
// still spared. Skipped without the files.
func TestRealMegaladonBreathReachesTheBeach(t *testing.T) {
	const shark uint32 = 0x69842C02
	w, _, done := landWaterWorld(t)
	defer done()
	SetModelRadii(map[uint32]float32{shark: 8.332})
	t.Cleanup(func() { SetModelRadii(nil) })
	w.mu.Lock()
	defer w.mu.Unlock()
	meg := &NPC{Name: "Megaladon", ModelID: int64(shark), Size: 4, World: 0, X: 24272.2, Y: 50.8, Z: 3046.0}
	place := npcAreaPlace(meg, meg.X, meg.Y, meg.Z, false)
	man := place
	man.casterBody = 1
	for a, want := range map[int]bool{0: true, 1: true, 3: true, 4: true, 5: true, 2: false} {
		s, c := math.Sincos(float64(a) * math.Pi / 8)
		x, z := meg.X+20*float32(c), meg.Z+20*float32(s)
		y, ok := w.ground.Floor(0, x, 200, z)
		if !ok {
			t.Fatalf("no floor at (%.1f, %.1f)", x, z)
		}
		if got, was := w.reachesLocked(place, x, y, z), w.reachesLocked(man, x, y, z); got != want || (want && was) {
			t.Errorf("at (%.1f, %.1f, %.1f): reached %v (from a man's heights %v), want %v", x, y, z, got, was, want)
		}
	}
}
