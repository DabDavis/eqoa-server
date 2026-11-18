package ground

import "testing"

// A body in something placed (Embedded) and the nearest free place (Free):
// a block 6 a side whose top is 2.5 over the terrain (50), and a deck 3 over
// the terrain that faces up alone (a bridge's: nothing of it under a body
// beneath it).
func TestABodyInSomethingPlacedIsPutOut(t *testing.T) {
	dir := t.TempDir()
	// the terrain, wound to face up as the game's collision reads it
	ground := []Vertex{{1950, 50, 1950}, {2100, 50, 1950}, {1950, 50, 2100}, {2100, 50, 2100}}
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{ground}}})
	deck := Buffer{Strips: [][]Vertex{face(Vertex{-5, 0, -5}, Vertex{-5, 0, 5}, Vertex{5, 0, -5}, Vertex{5, 0, 5}, Vertex{0, 1, 0})}}
	writeWalls(t, dir, 0, []Buffer{box(), deck}, []WallZone{{Placements: []Placement{
		placedAt(0, 3, 0, 2025, 49.5, 2025), // x and z 2022-2028, y 46.5-52.5
		placedAt(1, 1, 0, 2070, 53, 2070),
	}}})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	const mid, r = 1.2, 0.45
	for _, c := range []struct {
		what    string
		x, y, z float32
		want    bool
	}{
		{"open ground", 2010, 50, 2010, false},
		{"beside the block, its ball clear", 2021.4, 50, 2025, false},
		{"against the block, its ball in the face", 2021.7, 50, 2025, true},
		{"in the block's middle", 2025, 50, 2025, true},
		{"just inside its side", 2022.2, 50, 2025, true},
		{"on its top", 2025, 52.5, 2025, false},
		{"under the deck", 2070, 50, 2070, false},
		{"on the deck", 2070, 53, 2070, false},
	} {
		if got := g.Embedded(0, c.x, c.y, c.z, mid, r); got != c.want {
			t.Errorf("%s: embedded %v, want %v", c.what, got, c.want)
		}
	}

	// from the middle: on top (2.5 up) rather than out (3.75 across)
	x, y, z, top, ok := g.Free(0, 2025, 50, 2025, mid, 0.75)
	if !ok || !top || !near(x, 2025, 0) || !near(z, 2025, 0) || !near(y, 52.5, 0.01) {
		t.Errorf("from the middle: (%.2f, %.2f, %.2f) top %v %v, want on top at 52.5", x, y, z, top, ok)
	}
	// from just inside its side: out beside it, on the ground
	x, y, z, top, ok = g.Free(0, 2022.2, 50, 2025, mid, 0.75)
	if !ok || top || x > 2022-0.75 || x < 2019 || !near(y, 50, 0.13) {
		t.Errorf("from inside its side: (%.2f, %.2f, %.2f) top %v %v, want beside it west, on the ground", x, y, z, top, ok)
	}
	if g.Embedded(0, x, y, z, mid, 0.75) {
		t.Errorf("the place beside it (%.2f, %.2f, %.2f) is in it", x, y, z)
	}
	// nothing placed in a world: never in anything
	if g.Embedded(1, 2025, 50, 2025, mid, r) {
		t.Error("a world with no walls file has a body in something")
	}
}
