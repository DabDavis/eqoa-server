package ground

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// A quad as a strip of four corners, (x0, z0) to (x1, z1), its height y(x, z).
func quad(x0, z0, x1, z1 float32, y func(x, z float32) float32) []Vertex {
	return []Vertex{{x0, y(x0, z0), z0}, {x0, y(x0, z1), z1}, {x1, y(x1, z0), z0}, {x1, y(x1, z1), z1}}
}

// writeWorld writes zones as world n's ground file in dir.
func writeWorld(t *testing.T, dir string, n int, zones []Zone) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, map[int]string{0: "0.ground", 2: "2.ground"}[n]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Write(f, zones); err != nil {
		t.Fatal(err)
	}
}

func near(a, b, eps float32) bool { return math.Abs(float64(a-b)) <= float64(eps) }

// A hill's slope read back within the quantisation; a ledge 5 over the
// ground beneath it is not stepped onto from below but stood on from above;
// a wall is no floor; nothing where there is no terrain; a world with no
// file says so. The control: the highest floor whatever the height, which
// would lift a creature under the ledge onto it.
func TestTheFloorOfAHillALedgeAndAWall(t *testing.T) {
	dir := t.TempDir()
	slope := func(x, z float32) float32 { return 100 + 0.5*(x-1000) } // rises 1 in 2 toward +x
	flat := func(float32, float32) float32 { return 50 }
	ledge := func(float32, float32) float32 { return 55 }
	wall := []Vertex{{2010, 50, 2000}, {2011, 60, 2000}, {2010, 50, 2010}, {2011, 60, 2010}} // 84 degrees
	writeWorld(t, dir, 0, []Zone{
		{Strips: [][]Vertex{quad(1000, 1000, 1100, 1100, slope)}},
		{Strips: [][]Vertex{quad(2000, 2000, 2050, 2050, flat), quad(2020, 2020, 2030, 2030, ledge), wall}},
	})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	if y, ok := g.Floor(0, 1050, 200, 1050); !ok || !near(y, 125, 0.13) {
		t.Errorf("the slope at x 1050: %.3f %v, want 125", y, ok)
	}
	if y, ok := g.Floor(0, 1050, 124, 1050); !ok || !near(y, 125, 0.13) {
		t.Errorf("a step up the slope: %.3f %v, want 125", y, ok)
	}
	if y, ok := g.Floor(0, 2025, 50, 2025); !ok || !near(y, 50, 0.13) {
		t.Errorf("under the ledge, from the ground: %.3f %v, want 50 (the ledge is 5 up)", y, ok)
	}
	if y, ok := g.Floor(0, 2025, 55.2, 2025); !ok || !near(y, 55, 0.13) {
		t.Errorf("on the ledge: %.3f %v, want 55", y, ok)
	}
	if y, ok := g.Floor(0, 2010.5, 58, 2005); !ok || !near(y, 50, 0.13) {
		t.Errorf("at the wall: %.3f %v, want the ground's 50 (a wall is no floor)", y, ok)
	}
	if y, ok := g.Floor(0, 5000, 77, 5000); ok || y != 77 {
		t.Errorf("no terrain: %.3f %v, want 77 kept", y, ok)
	}
	if y, ok := g.Floor(3, 1050, 77, 1050); ok || y != 77 {
		t.Errorf("a world with no file: %.3f %v", y, ok)
	}
	if w, z := g.Worlds(); w != 1 || z != 2 {
		t.Errorf("%d worlds, %d zones", w, z)
	}
}

// Zones are loaded as they are asked for and kept up to keep, the least
// lately used let go first, and read again when asked again.
func TestZonesLoadAsAskedAndTheOldestGo(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 10 }
	var zones []Zone
	for i := 0; i < keep+5; i++ {
		x := float32(i * 100)
		zones = append(zones, Zone{Strips: [][]Vertex{quad(x, 0, x+50, 50, flat)}})
	}
	writeWorld(t, dir, 2, zones)
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for i := range zones {
		if _, ok := g.Floor(2, float32(i*100)+25, 10, 25); !ok {
			t.Fatalf("zone %d: no floor", i)
		}
	}
	if g.Loads != keep+5 || len(g.worlds[2].loaded) != keep {
		t.Errorf("%d loads, %d kept, want %d and %d", g.Loads, len(g.worlds[2].loaded), keep+5, keep)
	}
	g.Floor(2, float32((keep+4)*100)+25, 10, 25) // the newest: still loaded
	if g.Loads != keep+5 {
		t.Errorf("the newest read again")
	}
	g.Floor(2, 25, 10, 25) // the first: let go, read again
	if g.Loads != keep+6 {
		t.Errorf("the oldest was kept")
	}
}

// A zone too wide for the quantisation is refused at the bake, not
// silently folded.
func TestAZoneTooWideIsRefused(t *testing.T) {
	flat := func(float32, float32) float32 { return 0 }
	f, err := os.Create(filepath.Join(t.TempDir(), "0.ground"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Write(f, []Zone{{Strips: [][]Vertex{quad(0, 0, 17000, 10, flat)}}}); err == nil {
		t.Error("a zone 17000 wide was written")
	}
}
