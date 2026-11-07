package ground

import "testing"

// Sight meets the game's collision from either side: a wall of the terrain, a
// box placed on it (a house, a rock), the terrain's floor from above and from
// beneath and a ceiling. The walk's test (Wall) lets the same lines through a
// wall's back and a floor's underside, the control that sight is not the walk.
func TestSightMeetsCollisionFromEitherSide(t *testing.T) {
	dir := t.TempDir()
	flat := func(float32, float32) float32 { return 50 }
	over := func(float32, float32) float32 { return 70 }
	wall := []Vertex{{2025, 49, 2010}, {2025, 60, 2010}, {2025, 49, 2040}, {2025, 60, 2040}}
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(2000, 2000, 2100, 2100, flat), wall, quad(2060, 2060, 2080, 2080, over)}}})
	writeWorld(t, dir, 2, []Zone{{Strips: [][]Vertex{quad(0, 0, 100, 100, flat)}}})
	writeWalls(t, dir, 0, []Buffer{box()}, []WallZone{{Placements: []Placement{placedAt(0, 2, 0, 2010, 52, 2070)}}})
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, c := range []struct {
		what                   string
		world                  uint8
		x0, y0, z0, x1, y1, z1 float32
		sees                   bool
	}{
		{"in the open", 0, 2004, 51, 2005, 2090, 51, 2005, true},
		{"through the wall's face", 0, 2020, 51, 2020, 2030, 51, 2020, false},
		{"through the wall from behind", 0, 2030, 51, 2020, 2020, 51, 2020, false},
		{"past the wall's end", 0, 2020, 51, 2005, 2030, 51, 2005, true},
		{"over the wall's top", 0, 2020, 61, 2020, 2030, 61, 2020, true},
		{"short of the wall", 0, 2020, 51, 2020, 2024, 51, 2020, true},
		{"through the box", 0, 2005, 52, 2070, 2015, 52, 2070, false},
		{"through the box the other way", 0, 2015, 52, 2070, 2005, 52, 2070, false},
		{"past the box", 0, 2005, 52, 2075, 2015, 52, 2075, true},
		{"over the box", 0, 2005, 56, 2070, 2015, 56, 2070, true},
		{"down through the floor", 0, 2010, 55, 2015, 2010, 45, 2015, false},
		{"up through the floor", 0, 2010, 45, 2015, 2010, 55, 2015, false},
		{"up through a ceiling", 0, 2070, 60, 2070, 2070, 80, 2070, false},
		{"down through a ceiling", 0, 2070, 80, 2070, 2070, 60, 2070, false},
		{"beneath the ceiling", 0, 2065, 60, 2065, 2075, 60, 2075, true},
		{"a world with no walls file", 2, 10, 51, 10, 90, 51, 90, true},
		{"a world with no ground file", 9, 10, 51, 10, 90, 51, 90, true},
	} {
		if got := g.Sees(c.world, c.x0, c.y0, c.z0, c.x1, c.y1, c.z1); got != c.sees {
			t.Errorf("%s: sees %v, want %v", c.what, got, c.sees)
		}
	}
	if g.Wall(0, 2030, 51, 2020, 2020, 51, 2020) {
		t.Error("the control: the walk's test lets a way through a wall's back pass")
	}
}
