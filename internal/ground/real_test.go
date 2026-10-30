package ground

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Each of a real baked world's zones loaded once (EQOA_GROUND_DIR, world 0):
// how long a load takes and what the cache's zones hold. Skipped without a
// baked ground: none is in this repository.
func TestLoadingARealWorldsZones(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := g.worlds[0]
	var worst, total time.Duration
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for i := range w.entries {
		start := time.Now()
		g.mu.Lock()
		_, err := g.zoneLocked(w, i)
		g.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		d := time.Since(start)
		total += d
		worst = max(worst, d)
	}
	runtime.GC()
	runtime.ReadMemStats(&m1)
	t.Logf("%d zones: %v each on average, the slowest %v; %d kept hold %d MB", len(w.entries), total/time.Duration(len(w.entries)), worst, len(w.loaded), (int64(m1.HeapAlloc)-int64(m0.HeapAlloc))>>20)

	// and what stands on the ground, where the bake wrote it: each zone's
	// placements, and every buffer they place
	ww := g.walls[0]
	if ww == nil {
		return
	}
	worst, total = 0, 0
	runtime.GC()
	runtime.ReadMemStats(&m0)
	for i := range ww.zones {
		start := time.Now()
		g.mu.Lock()
		z, err := g.wallZoneLocked(ww, i)
		if err == nil {
			for _, p := range z.places {
				if _, err = ww.buffer(p.buf); err != nil {
					break
				}
			}
		}
		g.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		d := time.Since(start)
		total += d
		worst = max(worst, d)
	}
	runtime.GC()
	runtime.ReadMemStats(&m1)
	t.Logf("walls: %d zones: %v each on average, the slowest %v; %d kept and all %d buffers hold %d MB", len(ww.zones), total/time.Duration(len(ww.zones)), worst, len(ww.loaded), len(ww.bufs), (int64(m1.HeapAlloc)-int64(m0.HeapAlloc))>>20)
}

// Neriak's tavern, its rats' cellar (F3 pass 24: "they are walking outside
// the walls of the tavern basement"), in a real bake (EQOA_GROUND_DIR, with
// its walls files): the house stands on the cavern's floor (0.53), its own
// floor 0.77 above that, so a rat's knee is at 2.3. From where the breeder
// spawns the cellar's walls stop it west, east and north, and the cellar's
// hall is open; the house its rats first spawned in, 135 north, holds them
// the same. A city cat by the cellar's south wing, its knee 1.6 over the
// street, is stopped by the wing's wall and walks the street. Skipped
// without a bake: none is in this repository.
func TestNeriaksTavernHoldsItsRats(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	if _, err := os.Stat(dir + "/0.walls"); err != nil {
		t.Skip("the bake has no walls files")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if y, ok := g.Floor(0, 25296.6, 1.3, 8894); !ok || !near(y, 0.53, 0.05) {
		t.Fatalf("under the tavern the floor is %.2f %v, want the cavern's 0.53", y, ok)
	}
	for _, c := range []struct {
		what                   string
		x0, y0, z0, x1, y1, z1 float32
		want                   bool
	}{
		{"the breeder west through the cellar's wall", 25296.6, 2.3, 8894, 25270, 2.3, 8894, true},
		{"the breeder east through it", 25296.6, 2.3, 8894, 25325, 2.3, 8894, true},
		{"the breeder north through a cellar's room", 25296.6, 2.3, 8894, 25296.6, 2.3, 8875, true},
		{"across the cellar's hall", 25286, 2.3, 8895, 25310, 2.3, 8895, false},
		{"the first house's rat north through its wall", 25315.1, 2.4, 8743.2, 25315.1, 2.4, 8730, true},
		{"its breeder south through it", 25313.9, 2.4, 8767.4, 25313.9, 2.4, 8785, true},
		{"its breeder west through it", 25313.9, 2.4, 8767.4, 25300, 2.4, 8767.4, true},
		{"the cat east into the cellar's wing", 25285.2, 1.6, 8905.7, 25295, 1.6, 8905, true},
		{"the cat west along the street", 25285.2, 1.6, 8905.7, 25270, 1.6, 8910, false},
	} {
		if got := g.Wall(0, c.x0, c.y0, c.z0, c.x1, c.y1, c.z1); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	t.Logf("%d terrain zones and %d walls zones read", g.Loads, g.WallLoads)

	// the control: the terrain alone (the ground as the server had it) holds
	// no wall there, and the breeder walks out
	bare := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "0.ground"), filepath.Join(bare, "0.ground")); err != nil {
		t.Fatal(err)
	}
	tg, err := Open(bare)
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	if tg.Wall(0, 25296.6, 2.3, 8894, 25270, 2.3, 8894) {
		t.Error("the terrain alone has a wall in the tavern: the walls file proves nothing")
	}
}
