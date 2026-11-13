package ground

import (
	"math/rand"
	"os"
	"testing"
)

// standPoints are n places on world's terrain (its floor from high over each),
// drawn over its zones, for the benchmarks.
func standPoints(b *testing.B, g *Ground, world uint8, n int) [][3]float32 {
	w := g.worlds[world]
	if w == nil {
		b.Skipf("no world %d", world)
	}
	r := rand.New(rand.NewSource(1))
	var pts [][3]float32
	for tries := 0; len(pts) < n && tries < 50*n; tries++ {
		e := w.entries[r.Intn(len(w.entries))]
		if e.verts == 0 {
			continue
		}
		x := e.minX + r.Float32()*(e.maxX-e.minX)
		z := e.minZ + r.Float32()*(e.maxZ-e.minZ)
		if y, ok := g.Floor(world, x, 1e5, z); ok {
			pts = append(pts, [3]float32{x, y, z})
		}
	}
	return pts
}

// Floor against Stand at places on a real world's terrain (EQOA_GROUND_DIR;
// EQOA_BENCH_WORLD, 0 by default): Stand's cost over Floor's is the placed
// column. Skipped without a baked ground.
func BenchmarkFloorAndStand(b *testing.B) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		b.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer g.Close()
	world := uint8(0)
	if os.Getenv("EQOA_BENCH_WORLD") == "14" {
		world = 14
	}
	pts := standPoints(b, g, world, 4096)
	for _, p := range pts { // every zone they meet loaded
		g.Stand(world, p[0], p[1], p[2])
	}
	b.Run("Floor", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := pts[i%len(pts)]
			g.Floor(world, p[0], p[1], p[2])
		}
	})
	b.Run("Stand", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := pts[i%len(pts)]
			g.Stand(world, p[0], p[1], p[2])
		}
	})
}
