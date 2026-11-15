package main

import (
	"math"
	"math/rand"
	"regexp"
	"testing"
)

// grid is a flat hall of n x n floors, cell apart, every neighbour joined,
// one piece; plus a lone floor (a canopy's) far off.
func grid(n int, cell float64) *graph {
	g := &graph{cell: cell, nx: n + 1, nz: n}
	g.first = make([]int32, g.nx*g.nz+1)
	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.nz; j++ {
			c := g.col(i, j)
			g.first[c] = int32(len(g.nodes))
			if i < n || j == 0 {
				g.nodes = append(g.nodes, node{x: (float64(i) + 0.5) * cell, z: (float64(j) + 0.5) * cell, y: 10,
					terrain: true, roofed: true, col: int32(c)})
			}
		}
	}
	g.first[g.nx*g.nz] = int32(len(g.nodes))
	g.adj = make([][]int32, len(g.nodes))
	at := func(i, j int) int32 {
		c := g.col(i, j)
		if g.first[c] == g.first[c+1] {
			return -1
		}
		return g.first[c]
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for _, d := range [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
				ii, jj := i+d[0], j+d[1]
				if ii < 0 || jj < 0 || ii >= n || jj >= n {
					continue
				}
				a, b := at(i, j), at(ii, jj)
				g.adj[a] = append(g.adj[a], b)
				g.adj[b] = append(g.adj[b], a)
			}
		}
	}
	for k := range g.nodes {
		g.nodes[k].comp = 0
	}
	g.nodes[len(g.nodes)-1].comp = 1 // the lone floor, its own piece
	return g
}

func TestInsideOfDropsSmallPieces(t *testing.T) {
	g := grid(20, 2)
	in := g.insideOf(map[int32]bool{0: true, 1: true}, 100, 5)
	last := len(g.nodes) - 1
	if in[last] {
		t.Fatal("a one-floor piece is inside")
	}
	if !in[0] || !in[last-1] {
		t.Fatal("the hall is not inside")
	}
	g.nodes[0].roofed = false
	in = g.insideOf(map[int32]bool{0: true}, 5, 5) // over 5 high: roofed only
	if in[0] || !in[1] {
		t.Fatal("an unroofed floor over the inside height counted (or a roofed one not)")
	}
}

func TestPlanSpacingAndKinds(t *testing.T) {
	g := grid(40, 2) // an 80 x 80 hall
	rows := []row{
		{id: 1, name: "goblin miner", x: 10, z: 10, y: 10, level: 20},
		{id: 2, name: "goblin miner", x: 70, z: 70, y: 10, level: 21},
		{id: 3, name: "Skulpik", x: 40, z: 40, y: 10, level: 29},            // named: never a fill's kind
		{id: 4, name: "Feral Spirit", x: 20, z: 60, y: 10, level: 12},       // capitalised and excluded
		{id: 5, name: "goblin veteran", x: 60, z: 20, y: 30, level: 24},     // another band
		{id: 6, name: "filth bat", x: 30, z: 30, y: 10, level: 20},          // a wanderer
		{id: 7, name: "royal guard goblin", x: 50, z: 50, y: 10, level: 28}, // excluded
	}
	keep := map[int32]bool{0: true}
	p := &planner{g: g, rows: rows, bands: []band{{0, 20}, {20, 40}}, keep: keep, in: g.insideOf(keep, 100, 5),
		campGap: 15, memberGap: 4, rowGap: 9, groupW: []float64{30, 45, 25}, minDeg: 5,
		exclude: regexp.MustCompile(`(?i)(spirit|royal guard)`), wanders: regexp.MustCompile(`(?i)bat\b`),
		rnd: rand.New(rand.NewSource(1))}
	plan := p.plan()
	if len(plan) < 10 {
		t.Fatalf("only %d placed in an 80 x 80 hall", len(plan))
	}
	camps := map[int][]placed{}
	for _, a := range plan {
		if a.name != "goblin miner" && a.name != "filth bat" {
			t.Errorf("a %s placed: only the band's own unnamed kinds", a.name)
		}
		if a.name == "goblin miner" && (a.level < 20 || a.level > 21) {
			t.Errorf("a miner at %d: its band's levels are 20-21", a.level)
		}
		if a.template != 1 && a.template != 2 && a.template != 6 {
			t.Errorf("template %d", a.template)
		}
		for _, r := range rows {
			if d := math.Hypot(a.x-r.x, a.z-r.z); d < 8 && math.Abs(float64(r.y)-a.y) < 2 {
				t.Errorf("%s at (%.1f, %.1f) is %.1f from %s", a.name, a.x, a.z, d, r.name)
			}
		}
		camps[a.camp] = append(camps[a.camp], a)
	}
	var centres []placed
	for _, c := range camps {
		if len(c) > 3 {
			t.Errorf("a camp of %d", len(c))
		}
		centres = append(centres, c[0])
		for i := range c {
			for j := i + 1; j < len(c); j++ {
				if d := math.Hypot(c[i].x-c[j].x, c[i].z-c[j].z); d < 4-2 { // jitter: 0.6 a cell each way
					t.Errorf("camp mates %.1f apart", d)
				}
			}
		}
	}
	for i := range centres {
		for j := i + 1; j < len(centres); j++ {
			if d := math.Hypot(centres[i].x-centres[j].x, centres[i].z-centres[j].z); d < 15-6 {
				t.Errorf("camps %d and %d %.1f apart", centres[i].camp, centres[j].camp, d)
			}
		}
	}
	again := (&planner{g: g, rows: rows, bands: p.bands, keep: keep, in: p.in, campGap: 15, memberGap: 4, rowGap: 9,
		groupW: p.groupW, minDeg: 5, exclude: p.exclude, wanders: p.wanders, rnd: rand.New(rand.NewSource(1))}).plan()
	if len(again) != len(plan) || again[0] != plan[0] {
		t.Fatal("the same seed made another plan")
	}
}

func TestWanderersOnTerrainOnly(t *testing.T) {
	g := grid(30, 2)
	for k := range g.nodes {
		g.nodes[k].terrain = false // all placed floor: a bat has nowhere
	}
	rows := []row{{id: 6, name: "filth bat", x: 5, z: 5, y: 10, level: 20}}
	keep := map[int32]bool{0: true}
	p := &planner{g: g, rows: rows, bands: []band{{0, 20}}, keep: keep, in: g.insideOf(keep, 100, 5), campGap: 15,
		memberGap: 4, rowGap: 9, groupW: []float64{1}, minDeg: 5, exclude: regexp.MustCompile(`^$`),
		wanders: regexp.MustCompile(`(?i)bat\b`), rnd: rand.New(rand.NewSource(1))}
	if plan := p.plan(); len(plan) != 0 {
		t.Fatalf("%d bats placed on placed floor", len(plan))
	}
}

func TestReadFlags(t *testing.T) {
	b, err := readBoxes("16200,12000,16460,12350;17880,13700,17550,13450")
	if err != nil || len(b) != 2 || b[1].x0 != 17550 || b[1].z1 != 13700 {
		t.Fatalf("boxes %v %v", b, err)
	}
	if _, err := readBands("10:5"); err == nil {
		t.Fatal("a band upside down read")
	}
	if f := facingToward(0, 0, 0, 10); f != 0 {
		t.Fatalf("facing +z %d", f)
	}
	if f := facingToward(0, 0, 10, 0); f != 64 {
		t.Fatalf("facing +x %d", f)
	}
}
