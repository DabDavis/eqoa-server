package handlers

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/worlds"
)

// The Lightwolf Vale (tools/worlds/lightwolf.json, slot 17): Ember's stand and camp at Castle Lightwolf, Dusk's in
// the werewolves' village south of it, the camps behind their stands (each farther from the other team's stand than
// its own), the shrines on either flank of the line between the stands and the spring near its middle, the field a
// box round both bases with room past each camp, and the decoration (its extras and the edges' rocks, one recipe)
// holding both guild banners, so the banners stand in Highbourne's bodies.
func TestCTFLightwolfLayout(t *testing.T) {
	r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "lightwolf.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "the Lightwolf Vale" || len(r.Zones) != 1 || r.Zones[0].Zone != 24 {
		t.Fatalf("world %d %q, zones %+v", r.World, r.Name, len(r.Zones))
	}
	var banners, rocks int
	for _, e := range r.Zones[0].Extras {
		switch {
		case e.Sprite == 0xB714B430 || e.Sprite == 0xF0CFEF05:
			banners++
		case e.From == "ODUS.ESF":
			rocks++
		}
	}
	if banners != 4 || rocks == 0 {
		t.Fatalf("%d guild banners, %d edge rocks: want 4 and the edges' file taken in", banners, rocks)
	}
	raw := []byte(`{"zones": [{"from": "TUNARIA.ESF", "zone": 24, "extras": [{"sprite": "0xB714B430", "at": [0, 0, 0]}, {"sprite": "0xF0CFEF05", "at": [0, 0, 0]}]}]}`)
	lay, err := ReadArena(worlds.Built{Recipe: r}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if lay.Name != "the Lightwolf Vale" || lay.Teams[0].Body != 0xB714B430 || lay.Teams[1].Body != 0xF0CFEF05 {
		t.Fatalf("%q: bodies %08X %08X", lay.Name, lay.Teams[0].Body, lay.Teams[1].Body)
	}
	dist := func(ax, az, bx, bz float32) float64 { return math.Hypot(float64(ax-bx), float64(az-bz)) }
	castle, village := lay.Teams[0], lay.Teams[1]
	// Castle Lightwolf's courtyard and forecourt; the village round (9120, 7530)
	if castle.Stand.X < 9170 || castle.Stand.X > 9340 || castle.Stand.Z < 6470 || castle.Stand.Z > 6650 ||
		castle.Camp.X < 9177 || castle.Camp.X > 9329 || castle.Camp.Z < 6479 || castle.Camp.Z > 6550 {
		t.Fatalf("Ember's stand %+v or camp %+v is not at the castle", castle.Stand, castle.Camp)
	}
	if dist(village.Stand.X, village.Stand.Z, 9120, 7530) > 60 {
		t.Fatalf("Dusk's stand %+v is not in the village", village.Stand)
	}
	for i, tm := range lay.Teams {
		foe := lay.Teams[1-i].Stand
		if dist(tm.Camp.X, tm.Camp.Z, foe.X, foe.Z) <= dist(tm.Stand.X, tm.Stand.Z, foe.X, foe.Z) {
			t.Fatalf("team %d's camp is not behind its stand", i)
		}
		// room past the camp before the field's edge
		bx, bz := tm.Camp.X+(tm.Camp.X-tm.Stand.X)*0.3, tm.Camp.Z+(tm.Camp.Z-tm.Stand.Z)*0.3
		if !lay.InField(bx, bz) {
			t.Fatalf("team %d's camp is at the field's edge", i)
		}
	}
	// the shrines on either side of the line between the stands, the spring near its middle
	a, b := castle.Stand, village.Stand
	side := func(x, z float32) float32 { return (b.X-a.X)*(z-a.Z) - (b.Z-a.Z)*(x-a.X) }
	var sides []float32
	for _, u := range lay.PowerUps {
		switch u.Kind {
		case "speed":
			sides = append(sides, side(u.At.X, u.At.Z))
		case "heal":
			if dist(u.At.X, u.At.Z, (a.X+b.X)/2, (a.Z+b.Z)/2) > 150 {
				t.Fatalf("the spring at %+v is far from the middle", u.At)
			}
		}
	}
	if len(sides) != 2 || sides[0]*sides[1] >= 0 {
		t.Fatalf("the speed shrines are not one on each flank: %v", sides)
	}
	// the field shuts out the ground well past each base and either flank
	for _, p := range [][2]float32{{a.X, 6300}, {b.X, 7850}, {8700, 7000}, {9750, 7000}} {
		if lay.InField(p[0], p[1]) {
			t.Fatalf("%v is in the field", p)
		}
	}
}

// The arena's places over its baked ground (EQOA_GROUND_DIR with world 17's files, cmd/groundbake -world
// 17=world17.ESF; skipped without): every stand, camp and power-up has its height at the game's floor there, on the
// terrain, nothing placed over it within a body's height and no body there in anything placed (the decoration's
// banners, breastworks and braziers stand clear of them).
func TestCTFLightwolfPlacesOnTheFloor(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	if !g.Knows(17) {
		t.Skip("the baked ground has no world 17")
	}
	r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "lightwolf.json"))
	if err != nil {
		t.Fatal(err)
	}
	lay, err := ReadArena(worlds.Built{Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	type place struct {
		name    string
		x, y, z float32
	}
	var places []place
	for _, tm := range lay.Teams {
		places = append(places, place{tm.Short + "'s stand", tm.Stand.X, tm.Stand.Y, tm.Stand.Z},
			place{tm.Short + "'s camp", tm.Camp.X, tm.Camp.Y, tm.Camp.Z})
	}
	for _, u := range lay.PowerUps {
		places = append(places, place{u.Name, u.At.X, u.At.Y, u.At.Z})
	}
	for _, p := range places {
		for _, d := range [][2]float32{{0, 0}, {3, 0}, {-3, 0}, {0, 3}, {0, -3}} {
			x, z := p.x+d[0], p.z+d[1]
			h, placed, roofed, ok := g.Stand(17, x, p.y+5, z)
			if !ok || placed || roofed {
				t.Fatalf("%s (%+.0f, %+.0f): floor %.2f (ok %v, placed %v, roofed %v)", p.name, d[0], d[1], h, ok, placed, roofed)
			}
			if d == [2]float32{} && (h < p.y-0.6 || h > p.y+0.6) {
				t.Fatalf("%s: the layout's height %.1f, the floor %.2f", p.name, p.y, h)
			}
			if g.Embedded(17, x, h, z, 1.5, 1.0) {
				t.Fatalf("%s (%+.0f, %+.0f): a body there is in something placed", p.name, d[0], d[1])
			}
		}
	}
}
