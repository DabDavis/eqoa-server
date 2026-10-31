package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// squareLand is an n x n piece of open land, every sample joined to its
// neighbours.
func squareLand(n int) (*landGrid, []int) {
	l := &landGrid{nx: n, nz: n}
	l.h = make([]float32, n*n)
	l.kind = make([]uint8, n*n)
	l.edges = make([]uint8, n*n)
	l.piece = make([]int32, n*n)
	l.pieceSize = []int32{int32(n * n)}
	var all []int
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			k := l.at(i, j)
			l.kind[k] = sLand
			if i+1 < n {
				l.edges[k] |= 1
			}
			if j+1 < n {
				l.edges[k] |= 2
			}
			all = append(all, k)
		}
	}
	return l, all
}

// A clump of nine spread over a square of ground stands evenly: no two
// nearer than the square's side over the clump's side, less a sample's slack,
// and no sample farther than 200 from one of them.
func TestLloydSpreadsAClumpEvenly(t *testing.T) {
	l, all := squareLand(30) // 750 x 750
	spots := lloyd(l, all, l.at(15, 15), 9)
	if len(spots) != 9 {
		t.Fatalf("%d spots", len(spots))
	}
	seen := map[int]bool{}
	nearest := math.Inf(1)
	for a := range spots {
		if seen[spots[a]] {
			t.Fatalf("two on one sample: %v", spots)
		}
		seen[spots[a]] = true
		xa, za := l.xz(spots[a])
		for b := a + 1; b < len(spots); b++ {
			xb, zb := l.xz(spots[b])
			nearest = math.Min(nearest, math.Hypot(xa-xb, za-zb))
		}
	}
	if want := 750.0/3 - 2*step; nearest < want {
		t.Errorf("the nearest two %.0f apart, want at least %.0f: %v", nearest, want, spots)
	}
	// every sample within reach of one of them: the ground covered
	worst := 0.0
	for _, k := range all {
		x, z := l.xz(k)
		best := math.Inf(1)
		for _, s := range spots {
			sx, sz := l.xz(s)
			best = math.Min(best, math.Hypot(x-sx, z-sz))
		}
		worst = math.Max(worst, best)
	}
	if worst > 200 {
		t.Errorf("a sample %.0f from the nearest, want at most 200", worst)
	}
}

// A kind the bestiary lists twice takes the entry for the place's level:
// Tunaria's warthog is 26-30, the Frontiers' savannah's 36-55. A section the
// place is not in is no listing at all.
func TestPrimaRangeTakesThePlacesEntry(t *testing.T) {
	p := &plan{prima: map[string][]primaEntry{
		"warthog": {{"Tunaria/Global", 26, 30}, {"Frontiers/Savannah", 36, 55}},
	}}
	both := []string{"Tunaria/Global", "Frontiers/Savannah"}
	for _, c := range []struct {
		place, lo, hi int
	}{{28, 26, 30}, {52, 36, 55}, {33, 26, 30}, {34, 36, 55}, {60, 36, 55}} {
		if lo, hi, ok := p.primaRange("warthog", both, c.place); !ok || lo != c.lo || hi != c.hi {
			t.Errorf("at %d: %d-%d (%v), want %d-%d", c.place, lo, hi, ok, c.lo, c.hi)
		}
	}
	if _, _, ok := p.primaRange("warthog", []string{"Frontiers/Wetlands"}, 40); ok {
		t.Error("listed in a section it is not in")
	}
}

// The bestiary's rows as pdftotext -layout writes them: a section's
// heading, a name, its levels (one or a range); the contents before the
// chapter are not read.
func TestReadPrimaReadsTheBestiary(t *testing.T) {
	text := `Grizzly Bear              31-35         Med/High/Low    contents line before the chapter
The following tables represent almost all of the creature NPC's found in Tunaria and those
                                                                                  Tunaria
                                                                                  Arctic Beasts
Name                          Level   Melee Offense/Melee Defense/Magic Offense    Special Abilities
Ancient Mammoth               46-50             Med/High/Low                       Critical Hits
Freezeblood Raider              6               Med/High/Low                       Critical Hits
                                                                     Frontiers
                                                                     Savannah
Warthog                       36-55             Med/High/Low                       None
`
	path := filepath.Join(t.TempDir(), "prima.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readPrima(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]primaEntry{
		"ancient mammoth":    {"Tunaria/Arctic Beasts", 46, 50},
		"freezeblood raider": {"Tunaria/Arctic Beasts", 6, 6},
		"warthog":            {"Frontiers/Savannah", 36, 55},
	}
	if len(got) != len(want) {
		t.Fatalf("read %v", got)
	}
	for k, w := range want {
		if e := got[k]; len(e) != 1 || e[0] != w {
			t.Errorf("%s: %v, want %v", k, e, w)
		}
	}
}

// A kind's spots: chains within spotLink, by world.
func TestSpotCount(t *testing.T) {
	rows := []*npcRow{
		{world: 0, x: 0, z: 0}, {world: 0, x: 100, z: 0}, {world: 0, x: 200, z: 0}, // one chain
		{world: 0, x: 1000, z: 0}, // a second spot
		{world: 2, x: 0, z: 0},    // another world's
	}
	got := spotCount(rows)
	if got[0] != 2 || got[2] != 1 {
		t.Errorf("spots %v, want world 0: 2, world 2: 1", got)
	}
}
