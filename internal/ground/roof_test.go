package ground

import (
	"os"
	"path/filepath"
	"testing"
)

// What is over a place: a house (a box placed on the ground, 10 tall) over
// a body inside it, whose roof faces up, out of the house, and is met from
// its back; nothing beside the house, nor over one standing on its roof; a
// terrain overhang 20 over the ground; and nothing past the look's reach. The
// control: the walk's one-sided test (Wall, VICollide's) lets the same look
// up through the house's roof pass, as a body walks out of a room through a
// wall's back, so a roof must be met from either side.
func TestARoofIsWhatIsOverAPlace(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 0 }
	over := func(float32, float32) float32 { return 20 }
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(0, 0, 600, 600, flat), quad(300, 300, 320, 320, over)}}})
	writeWorld(t, dir, 2, []Zone{{Strips: [][]Vertex{quad(0, 0, 600, 600, flat)}}})
	writeWalls(t, dir, 0, []Buffer{box()}, []WallZone{{Placements: []Placement{placedAt(0, 5, 0, 100, 5, 100)}}})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, c := range []struct {
		what         string
		world        uint8
		x, y0, y1, z float32
		want         bool
	}{
		{"in the house, its roof 10 up", 0, 100, 1, 1000, 100, true},
		{"in the house, by a wall", 0, 104, 1, 1000, 96, true},
		{"beside the house", 0, 108, 1, 1000, 100, false},
		{"on the house's roof", 0, 100, 11, 1000, 100, false},
		{"under the overhang", 0, 310, 1, 1000, 310, true},
		{"beside the overhang", 0, 330, 1, 1000, 310, false},
		{"under the overhang, the look short of it", 0, 310, 1, 15, 310, false},
		{"in the house, the look short of its roof", 0, 100, 1, 9, 100, false},
		{"in the open", 0, 200, 1, 1000, 200, false},
		{"in a world with no walls file, where the house stands in world 0", 2, 100, 1, 1000, 100, false},
		{"in a world with neither file", 3, 100, 1, 1000, 100, false},
	} {
		if got := g.Roof(c.world, c.x, c.y0, c.y1, c.z); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	if g.Wall(0, 100, 1, 100, 100, 1000, 100) {
		t.Error("the control: the walk's test stopped the look up at the roof's back; a roof met only from its face proves nothing")
	}
}

// A real bake (EQOA_GROUND_DIR, with its walls files): Freeport's inn is
// roofed where Grindal stands (his spawn, on the inn's floor 3 over the
// street) and the street 12 north of him is open; Neriak's street is under the
// cavern's roof (terrain, 27 up) and its tavern under its upper floor (6.5
// up); Afkqq stood in the open when Sandstorm burned it; Sandstorm's
// Sanctuary landing (S2's world 7 at (4322, 274, 6180), the Frontiers
// world 5 file) is under 48 of rock, where this server's Sanctuary NPCs
// stand on open ground. Skipped without a bake: none is in this repository.
func TestRealRoofs(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	if _, err := os.Stat(filepath.Join(dir, "0.walls")); err != nil {
		t.Skip("the bake has no walls files")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, c := range []struct {
		what    string
		world   uint8
		x, y, z float32
		want    bool
	}{
		{"Freeport's inn, where Grindal stands", 0, 25284.0, 57.1, 15657.5, true},
		{"Freeport's street north of the inn", 0, 25284.0, 54.09, 15645.5, false},
		{"Neriak's street, under the cavern", 0, 25285.2, 0.6, 8905.7, true},
		{"Neriak's tavern", 0, 25297.8, 1.4, 8885.6, true},
		{"where Afkqq burned on Sandstorm", 0, 15290.389, 54.125732, 9248.619, false},
		{"Sandstorm's Sanctuary landing", 5, 4322.19, 274.134, 6179.78, true},
		{"this server's Hinder in the Sanctuary", 5, 4300, 253.5, 6118, false},
	} {
		if got := g.Roof(c.world, c.x, c.y+1, c.y+1000, c.z); got != c.want {
			t.Errorf("%s: %v, want %v", c.what, got, c.want)
		}
	}
	// the control: the cavern's roof is the terrain's, and the inn's the
	// walls file's; without that file the inn is open and Neriak still not
	bare := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "0.ground"), filepath.Join(bare, "0.ground")); err != nil {
		t.Fatal(err)
	}
	tg, err := Open(bare)
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	if tg.Roof(0, 25284.0, 58.1, 1057.1, 15657.5) {
		t.Error("the terrain alone roofs Freeport's inn: the walls file proves nothing")
	}
	if !tg.Roof(0, 25285.2, 1.6, 1000.6, 8905.7) {
		t.Error("the terrain alone does not roof Neriak's street")
	}
}
