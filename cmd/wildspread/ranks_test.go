package main

import (
	"math"
	"os"
	"testing"
)

// A kind's creatures standing in a straight row are a rank, and the creatures
// chained to it a block; a loose group, a pair and a lone row of another kind
// far off are not.
func TestFindRanks(t *testing.T) {
	var rows []*npcRow
	id := int64(0)
	add := func(name string, x, z float64) int64 {
		id++
		rows = append(rows, &npcRow{id: id, name: name, key: name, world: 0, x: x, z: z, model: 7})
		return id
	}
	// Kelinar's parade: four gnoll reavers 5 apart in a row, four shamans
	// beside them, one withered at the block's edge
	var block []int64
	for i := 0; i < 4; i++ {
		block = append(block, add("gnoll reaver", 1000, 1000+5*float64(i)))
		block = append(block, add("gnoll shaman", 1006, 1000+5*float64(i)))
	}
	block = append(block, add("withered", 1015, 1010))
	// a loose group of three (a zones1103 fill's) and a pair in a row
	loose := []int64{add("spineback", 3000, 3000), add("spineback", 3010, 3008), add("spineback", 2996, 3011)}
	pair := []int64{add("griffenne", 5000, 5000), add("griffenne", 5000, 5005)}
	// a row of three 8-14 apart (Kelinar's withered)
	far := []int64{add("withered", 7000, 7000), add("withered", 7000, 7008), add("withered", 7001, 7022)}
	// an NPC (0x80) row is no creature's
	id++
	rows = append(rows, &npcRow{id: id, name: "Guard", key: "guard", x: 9000, z: 9000, model: 7, typ: 0x80})
	rows = append(rows, &npcRow{id: id + 1, name: "Guard", key: "guard", x: 9000, z: 9005, model: 7, typ: 0x80})
	rows = append(rows, &npcRow{id: id + 2, name: "Guard", key: "guard", x: 9000, z: 9010, model: 7, typ: 0x80})
	guards := []int64{id, id + 1, id + 2}

	got := findRanks(rows)
	for _, i := range append(block, far...) {
		if !got[i] {
			t.Errorf("row %d: in a block, not found", i)
		}
	}
	for _, i := range append(append(loose, pair...), guards...) {
		if got[i] {
			t.Errorf("row %d: no rank's, found", i)
		}
	}
}

func TestLineShape(t *testing.T) {
	l, w := lineShape([][2]float64{{0, 0}, {0, 5}, {0, 10}, {0, 15}})
	if math.Abs(l-2*math.Sqrt(3*125.0/4)) > 1e-9 || w > 1e-9 {
		t.Errorf("a row: length %v width %v", l, w)
	}
	if _, w := lineShape([][2]float64{{0, 0}, {10, 0}, {0, 10}, {10, 10}}); w < 4 {
		t.Errorf("a square: width %v", w)
	}
}

// -zone keeps a run to CLIENT's zones by name: Tunaria's Kelinar is the cell
// x 12,000-14,000, z 24,000-26,000.
func TestZones(t *testing.T) {
	words := make([]string, 1200)
	words[1087], words[1086] = "Kelinar", "Gerntar Mines"
	if !zoneKnown(words, "kelinar") || zoneKnown(words, "nowhere") {
		t.Error("zoneKnown")
	}
	p := &plan{words: words}
	if !p.inNear(30000, 30000) {
		t.Error("no -zone: everywhere")
	}
	p.zones = []string{"kelinar"}
	for _, c := range []struct {
		x, z float64
		in   bool
	}{{13305, 24737, true}, {12001, 25999, true}, {13060, 26507, false}, {11999, 25000, false}} {
		if p.inNear(c.x, c.z) != c.in {
			t.Errorf("(%v, %v): in %v", c.x, c.z, !c.in)
		}
	}
	p.curWorld = 1
	if p.inNear(13305, 24737) {
		t.Error("Rathe's grid has no Kelinar")
	}
	p.curWorld, p.zones, p.notZones = 0, nil, []string{"kelinar"}
	if p.inNear(13305, 24737) || !p.inNear(13060, 26507) {
		t.Error("-not-zone Kelinar: out of Kelinar, in Gerntar Mines")
	}
}

func TestReadHold(t *testing.T) {
	path := t.TempDir() + "/hold.txt"
	if err := os.WriteFile(path, []byte("# held\n12\n 34 # two\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readHold("@" + path); err != nil || len(got) != 2 || got[0] != 12 || got[1] != 34 {
		t.Errorf("got %v %v", got, err)
	}
	for _, bad := range []string{"12", "@" + path + ".none"} {
		if _, err := readHold(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
}
