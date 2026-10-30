package ground

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// A wall of the terrain stands in the way of a segment through its face (it
// faces west: (c-b) x (b-a) of its first triangle); one past its end, over
// its top or short of it is not stopped, nor one through it from behind,
// which the console's walk passes too (VICollide's triangles stop a body
// moving into their face). The control: the floor the wall stands on is no
// wall, though the segment goes through it too, from beneath.
func TestAWallOfTheTerrainStandsInTheWay(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 50 }
	wall := []Vertex{{2025, 49, 2010}, {2025, 60, 2010}, {2025, 49, 2040}, {2025, 60, 2040}}
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(2000, 2000, 2050, 2050, flat), wall}}})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, c := range []struct {
		what                   string
		x0, y0, z0, x1, y1, z1 float32
		want                   bool
	}{
		{"through the wall's face", 2020, 51, 2020, 2030, 51, 2020, true},
		{"slantwise through it", 2020, 51, 2015, 2030, 52, 2035, true},
		{"through it from behind", 2030, 51, 2030, 2020, 51, 2030, false},
		{"past its end", 2020, 51, 2005, 2030, 51, 2005, false},
		{"over its top", 2020, 61, 2020, 2030, 61, 2020, false},
		{"short of it", 2020, 51, 2020, 2024, 51, 2020, false},
		{"through the floor from beneath", 2010, 49, 2010, 2010, 51, 2015, false},
	} {
		if got := g.Wall(0, c.x0, c.y0, c.z0, c.x1, c.y1, c.z1); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	if y, ok := g.Floor(0, 2024.5, 51, 2020); !ok || !near(y, 50, 0.13) {
		t.Errorf("beside the wall the floor is %.3f %v, want 50", y, ok)
	}
}

// face is a quad as a strip (a and b one edge, c and d the other) wound to
// face out, as the game's collision reads a strip: its first triangle faces
// (c-b) x (b-a), and the rest the same way (stripTri).
func face(a, b, c, d, out Vertex) []Vertex {
	u := Vertex{b.X - a.X, b.Y - a.Y, b.Z - a.Z}
	v := Vertex{c.X - b.X, c.Y - b.Y, c.Z - b.Z}
	n := Vertex{v.Y*u.Z - v.Z*u.Y, v.Z*u.X - v.X*u.Z, v.X*u.Y - v.Y*u.X}
	if n.X*out.X+n.Y*out.Y+n.Z*out.Z < 0 {
		return []Vertex{b, a, d, c}
	}
	return []Vertex{a, b, c, d}
}

// box is a cube from -1 to 1 as six strips, a face each, facing out.
func box() Buffer {
	return Buffer{Strips: [][]Vertex{
		face(Vertex{-1, -1, -1}, Vertex{-1, 1, -1}, Vertex{1, -1, -1}, Vertex{1, 1, -1}, Vertex{0, 0, -1}),
		face(Vertex{-1, -1, 1}, Vertex{-1, 1, 1}, Vertex{1, -1, 1}, Vertex{1, 1, 1}, Vertex{0, 0, 1}),
		face(Vertex{-1, -1, -1}, Vertex{-1, 1, -1}, Vertex{-1, -1, 1}, Vertex{-1, 1, 1}, Vertex{-1, 0, 0}),
		face(Vertex{1, -1, -1}, Vertex{1, 1, -1}, Vertex{1, -1, 1}, Vertex{1, 1, 1}, Vertex{1, 0, 0}),
		face(Vertex{-1, -1, -1}, Vertex{-1, -1, 1}, Vertex{1, -1, -1}, Vertex{1, -1, 1}, Vertex{0, -1, 0}),
		face(Vertex{-1, 1, -1}, Vertex{-1, 1, 1}, Vertex{1, 1, -1}, Vertex{1, 1, 1}, Vertex{0, 1, 0}),
	}}
}

// placedAt is a placement of buffer b scaled by s, turned by a about the
// up axis and moved to (x, y, z).
func placedAt(b uint32, s, a, x, y, z float32) Placement {
	c, n := float32(math.Cos(float64(a)))*s, float32(math.Sin(float64(a)))*s
	return Placement{Buffer: b, M: [12]float32{c, 0, n, x, 0, s, 0, y, -n, 0, c, z}}
}

// writeWalls writes world n's walls file in dir.
func writeWalls(t *testing.T, dir string, n int, bufs []Buffer, zones []WallZone) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, map[int]string{0: "0.walls", 2: "2.walls"}[n]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := WriteWalls(f, bufs, zones); err != nil {
		t.Fatal(err)
	}
}

// What is placed on the ground stands in the way where it is placed: a box
// scaled by 2 and turned 45 degrees (a diamond seen from above, its corners
// 2.83 out along the axes), a box as it is, and one in the next zone. Each
// pair's control is a segment the box would stop if its turn or its scale
// were lost; and over a box, short of one, out of one from inside (its
// faces face out: the console's walk passes them from behind) and in a world
// with no walls file nothing stops a segment.
func TestWhatStandsOnTheGroundStandsInTheWay(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 0 }
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(0, 0, 600, 600, flat)}}})
	writeWorld(t, dir, 2, []Zone{{Strips: [][]Vertex{quad(0, 0, 600, 600, flat)}}})
	writeWalls(t, dir, 0, []Buffer{box()}, []WallZone{
		{Placements: []Placement{placedAt(0, 2, math.Pi/4, 100, 0, 100), placedAt(0, 1, 0, 130, 0, 100)}},
		{Placements: []Placement{placedAt(0, 1, 0, 500, 0, 500)}},
	})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, c := range []struct {
		what                   string
		x0, y0, z0, x1, y1, z1 float32
		want                   bool
	}{
		{"through the turned box", 95, 0.5, 100, 105, 0.5, 100, true},
		{"through its corner, out past where an unturned box would end", 102.4, 0.5, 97, 102.4, 0.5, 103, true},
		{"beside its edge, inside where an unturned box would be", 101.9, 0.5, 101.5, 101.9, 0.5, 103, false},
		{"over it", 95, 2.5, 100, 105, 2.5, 100, false},
		{"out of it from inside", 100, 0.5, 100, 106, 0.5, 100, false},
		{"into the box as it is", 128.5, 0.5, 100, 129.5, 0.5, 100, true},
		{"short of it, where a box scaled by 2 would be", 127, 0.5, 100, 128.5, 0.5, 100, false},
		{"through the next zone's box", 495, 0.5, 500, 505, 0.5, 500, true},
		{"in the open", 200, 0.5, 200, 220, 0.5, 220, false},
	} {
		if got := g.Wall(0, c.x0, c.y0, c.z0, c.x1, c.y1, c.z1); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	if g.Wall(2, 95, 0.5, 100, 105, 0.5, 100) {
		t.Error("a world with no walls file has a wall")
	}
	if w, p := g.Walls(); w != 1 || p != 3 {
		t.Errorf("%d worlds with walls, %d placements; want 1 and 3", w, p)
	}
}

// The walls files' zones are loaded as they are asked for and kept up to
// keep, the least lately used let go first; a buffer is read once.
func TestWallZonesLoadAsAskedAndTheOldestGo(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 0 }
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(0, 0, 50, 50, flat)}}})
	var zones []WallZone
	for i := 0; i < keep+5; i++ {
		zones = append(zones, WallZone{Placements: []Placement{placedAt(0, 1, 0, float32(i*100), 0, 0)}})
	}
	writeWalls(t, dir, 0, []Buffer{box()}, zones)
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for i := range zones {
		x := float32(i * 100)
		if !g.Wall(0, x-3, 0, 0, x+3, 0, 0) {
			t.Fatalf("zone %d's box: no wall", i)
		}
	}
	if g.WallLoads != keep+5 || len(g.walls[0].loaded) != keep {
		t.Errorf("%d loads, %d kept, want %d and %d", g.WallLoads, len(g.walls[0].loaded), keep+5, keep)
	}
	g.Wall(0, -3, 0, 0, 3, 0, 0) // the first: let go, read again
	if g.WallLoads != keep+6 {
		t.Errorf("the oldest was kept")
	}
	read := 0
	for _, b := range g.walls[0].read {
		if b != nil {
			read++
		}
	}
	if read != 1 {
		t.Errorf("%d buffers read, want the one", read)
	}
}

// A placement of a buffer the file does not have is refused at the bake.
func TestAPlacementOfNoBufferIsRefused(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "0.walls"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := WriteWalls(f, []Buffer{box()}, []WallZone{{Placements: []Placement{placedAt(1, 1, 0, 0, 0, 0)}}}); err == nil {
		t.Error("a placement of buffer 1 of 1 was written")
	}
}
