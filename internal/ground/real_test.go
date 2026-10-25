package ground

import (
	"os"
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
}
