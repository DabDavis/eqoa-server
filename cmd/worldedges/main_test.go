package main

import (
	"encoding/json"
	"strings"
	"testing"

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
	if n := normals(openEdges(b, 0)); len(n) != 3 || n[[2]float32{1, 0}] {
		t.Errorf("zone 0: %v (its +x side is zone 1's)", n)
	}
	if n := normals(openEdges(b, 1)); len(n) != 3 || n[[2]float32{-1, 0}] {
		t.Errorf("zone 1: %v (its -x side is zone 0's)", n)
	}
	es := openEdges(b, 2)
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
