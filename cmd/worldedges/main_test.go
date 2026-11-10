package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/worlds"
)

// A zone's open edges are the ones no other zone of the world touches: of
// three zones, two side by side ((2, 1) and (3, 1)) and one apart ((5, 4)),
// the pair keep their shared edge open and the lone one has all four.
// Each edge runs the cell's side and looks out of it.
func TestOpenEdges(t *testing.T) {
	b := worlds.Built{Parts: []worlds.BuiltZone{{Col: 2, Row: 1}, {Col: 3, Row: 1}, {Col: 5, Row: 4}}}
	normals := func(es []edge) map[[2]float32]bool {
		m := map[[2]float32]bool{}
		for _, e := range es {
			m[[2]float32{e.nx, e.nz}] = true
		}
		return m
	}
	if n := normals(openEdges(b, 0, nil)); len(n) != 3 || n[[2]float32{1, 0}] {
		t.Errorf("zone 0: %v (its +x side is zone 1's)", n)
	}
	if n := normals(openEdges(b, 1, nil)); len(n) != 3 || n[[2]float32{-1, 0}] {
		t.Errorf("zone 1: %v (its -x side is zone 0's)", n)
	}
	es := openEdges(b, 2, nil)
	if len(es) != 4 {
		t.Fatalf("zone 2: %d open edges", len(es))
	}
	for _, e := range es {
		// the edge's two ends are corners of the cell, and out is away from it
		for _, k := range []float32{0, cell} {
			x, z := e.ax+e.ux*k, e.az+e.uz*k
			if (x != 10000 && x != 12000) || (z != 8000 && z != 10000) {
				t.Errorf("edge %+v: (%v, %v) is not a corner of the cell", e, x, z)
			}
		}
		mx, mz := e.ax+e.ux*cell/2+e.nx*10, e.az+e.uz*cell/2+e.nz*10
		if mx > 10000 && mx < 12000 && mz > 8000 && mz < 10000 {
			t.Errorf("edge %+v looks into its cell", e)
		}
	}
	// a stretch's edge (the check's names) is the same line
	for _, name := range []string{"z-", "z+", "x-", "x+"} {
		e, ok := edgeOf(b.Parts[2], name)
		mx, mz := e.ax+e.ux*cell/2+e.nx*10, e.az+e.uz*cell/2+e.nz*10
		if !ok || mx > 10000 && mx < 12000 && mz > 8000 && mz < 10000 {
			t.Errorf("%s: %+v", name, e)
		}
	}
	if _, ok := edgeOf(b.Parts[2], "y+"); ok {
		t.Error("no such edge")
	}
}

// The extras file puts each extra on a line of its own and reads back as
// it was written.
func TestCompact(t *testing.T) {
	f := worlds.ExtrasFile{Note: "n", Zones: []worlds.ExtrasOfOne{{From: "ODUS.ESF", Zone: 8, Extras: []worlds.Extra{
		{Sprite: 0x7FFD2E87, From: "ODUS.ESF", Zone: 10, At: [3]float32{1, 2, 3}, HPR: [3]float32{0.5, 0, 0}, Scale: 6.5},
		{Sprite: 0x4A6801F0, From: "TUNARIA.ESF", Zone: 143, At: [3]float32{4, 5, 6}, Scale: 20},
	}}}}
	j, _ := json.MarshalIndent(f, "", " ")
	c := compact(j)
	if lines := strings.Count(string(c), `"sprite"`); lines != 2 || strings.Count(string(c), "\n") > 12 {
		t.Errorf("not compact:\n%s", c)
	}
	var back worlds.ExtrasFile
	if err := json.Unmarshal(c, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Zones) != 1 || len(back.Zones[0].Extras) != 2 || back.Zones[0].Extras[1].Scale != 20 || back.Zones[0].Extras[0].Sprite != 0x7FFD2E87 {
		t.Errorf("%+v", back)
	}
}

// Where a body is put by the check: a floor under another sheet of the
// terrain, or under a roof lower than its head, is no place to stand; a swim
// at the water's top is no swim where the water is too shallow or where its
// top is inside the ground.
func TestPlacesABodyTakes(t *testing.T) {
	up := func(y float32) ground.Surface { return ground.Surface{Y: y, Up: true} }
	down := func(y float32) ground.Surface { return ground.Surface{Y: y} }
	if !buried([]ground.Surface{up(10), up(15)}, 10) || buried([]ground.Surface{up(10), up(15)}, 15) {
		t.Error("buried: the lower of two sheets is under the upper")
	}
	if buried([]ground.Surface{up(10), down(13), up(40)}, 10) {
		t.Error("buried: a cave's floor has its roof over it")
	}
	for _, c := range []struct {
		surfs []ground.Surface
		want  bool
	}{
		{[]ground.Surface{up(10)}, true},
		{[]ground.Surface{up(10), down(11), up(40)}, false}, // a cave's tip
		{[]ground.Surface{up(10), down(13), up(40)}, true},
		{[]ground.Surface{up(10), up(15)}, false},
	} {
		if roomOver(c.surfs, 10) != c.want {
			t.Errorf("roomOver(%v) is not %v", c.surfs, c.want)
		}
	}
	for _, c := range []struct {
		col  column
		want bool
	}{
		{column{surfs: []ground.Surface{up(5)}, water: 20}, true},
		{column{surfs: []ground.Surface{up(19)}, water: 20}, false},                  // shallow: stood in
		{column{surfs: []ground.Surface{up(5), down(10), up(30)}, water: 20}, false}, // inside the hill
		{column{surfs: []ground.Surface{up(5)}, water: float32(math.NaN())}, false},  // no water
		{column{surfs: []ground.Surface{up(5), down(25), up(30)}, water: 20}, true},  // a flooded cave
	} {
		if _, ok := swimAt(&c.col); ok != c.want {
			t.Errorf("swimAt(%+v) is not %v", c.col, c.want)
		}
	}
}

// A ridge's seed goes by its zone and side alone, so a world given another
// zone (another order, another neighbour) lays the rest as it did.
func TestEdgeSeed(t *testing.T) {
	south := edge{nz: -1}
	if edgeSeed(1, "odus.esf", 19, south) != edgeSeed(1, "ODUS.ESF", 19, south) {
		t.Error("a disc file by any case is one")
	}
	if edgeSeed(1, "ODUS.ESF", 19, south) == edgeSeed(1, "ODUS.ESF", 19, edge{nx: 1}) ||
		edgeSeed(1, "ODUS.ESF", 19, south) == edgeSeed(1, "ODUS.ESF", 22, south) ||
		edgeSeed(1, "ODUS.ESF", 19, south) == edgeSeed(2, "ODUS.ESF", 19, south) {
		t.Error("another side, zone or seed is another ridge")
	}
}

// -keep reads a recipe's homes (their starts and people) as places no rock
// may cover, and a zone with no home names none.
func TestKeep(t *testing.T) {
	path := t.TempDir() + "/r.json"
	recipe := `{"zones": [{"name": "a", "home": {"start": {"x": 10, "y": 5, "z": 20},
		"npcs": [{"name": "gatekeeper", "at": [30, 5, 40]}, {"name": "no place"}]}}, {"name": "b"}]}`
	if err := os.WriteFile(path, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	k, err := readKeep(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != 2 || k[0] != [2]float32{10, 20} || k[1] != [2]float32{30, 40} {
		t.Fatalf("%v", k)
	}
	keepClear = k
	defer func() { keepClear = nil }()
	if !covers(35, 40, 6) || covers(35, 40, 4) || covers(100, 100, 50) {
		t.Error("covers: within the radius only")
	}
}
