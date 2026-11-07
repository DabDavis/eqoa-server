package ground

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"
)

// mapWorld writes a small world with a hill, a wall and a placed box, and its
// environment; none of it mapped yet.
func mapWorld(t *testing.T, dir string) {
	t.Helper()
	slope := func(x, z float32) float32 { return 40 + 0.25*(x-2000) }
	wall := []Vertex{{2025, 49, 2010}, {2025, 60, 2010}, {2025, 49, 2040}, {2025, 60, 2040}}
	writeWorld(t, dir, 0, []Zone{
		{Strips: [][]Vertex{quad(2000, 2000, 2050, 2050, slope), wall}},
		{Strips: [][]Vertex{quad(2050, 2000, 2100, 2050, slope)}},
		{},
	})
	writeWalls(t, dir, 0, []Buffer{box(), {}}, []WallZone{
		{Placements: []Placement{placedAt(0, 2, math.Pi/4, 2020, 45, 2020), placedAt(0, 1, 0, 2040, 45, 2020)}},
		{},
	})
}

// answers is what every query says at many places, for two grounds to be held
// to each other: the same floors, walls, surfaces, roofs and placed things.
func answers(g *Ground, seed int64, lo, hi Vertex, n int) []any {
	r := rand.New(rand.NewSource(seed))
	at := func() (float32, float32, float32) {
		return lo.X + r.Float32()*(hi.X-lo.X), lo.Y + r.Float32()*(hi.Y-lo.Y), lo.Z + r.Float32()*(hi.Z-lo.Z)
	}
	var out []any
	for i := 0; i < n; i++ {
		x, y, z := at()
		fy, ok := g.Floor(0, x, y, z)
		out = append(out, fy, ok)
		x1, y1, z1 := x+(r.Float32()-0.5)*8, y+(r.Float32()-0.5)*4, z+(r.Float32()-0.5)*8
		out = append(out, g.Wall(0, x, y, z, x1, y1, z1), g.Placed(0, x, y, z, x1, y1, z1), g.Roof(0, x, y, y+200, z))
		out = append(out, len(g.Surfaces(0, x, z, y-20, y+20)))
	}
	return out
}

// A map holds exactly what the decode makes (it is written from it), every query
// answers the same over it, and nothing of a zone's is on the heap: the slices are
// the mapping's. The control: without the map the same world decodes.
func TestAMapIsTheDecode(t *testing.T) {
	dir := t.TempDir()
	mapWorld(t, dir)
	plain, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if m, n := plain.Mapped(); m != 0 || n != 2 {
		t.Fatalf("before the maps are written: %d of %d mapped", m, n)
	}
	lo, hi := Vertex{1990, 30, 1990}, Vertex{2110, 80, 2060}
	before := answers(plain, 1, lo, hi, 2000)

	wrote := 0
	if err := WriteMaps(dir, func(string, int64) { wrote++ }); err != nil {
		t.Fatal(err)
	}
	if wrote != 2 {
		t.Fatalf("%d maps written, want 2", wrote)
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if m, n := g.Mapped(); m != 2 || n != 2 {
		t.Fatalf("%d of %d mapped", m, n)
	}
	w, pw := g.worlds[0], plain.worlds[0]
	for i := range w.entries {
		a, b := w.mappedZone(i), mustZone(t, pw, i)
		if !reflect.DeepEqual(nonNil(a.verts), nonNil(b.verts)) || !reflect.DeepEqual(nonNil(a.tris), nonNil(b.tris)) ||
			!reflect.DeepEqual(nonNil(a.walls), nonNil(b.walls)) || !reflect.DeepEqual(nonNil(a.floors.cells), nonNil(b.floors.cells)) ||
			!reflect.DeepEqual(nonNil(a.floors.offsets), nonNil(b.floors.offsets)) || !reflect.DeepEqual(nonNil(a.wallGrid.cells), nonNil(b.wallGrid.cells)) ||
			!reflect.DeepEqual(nonNil(a.wallGrid.offsets), nonNil(b.wallGrid.offsets)) ||
			a.floors.nx != b.floors.nx || a.floors.nz != b.floors.nz || a.wallGrid.nx != b.wallGrid.nx || a.minX != b.minX || a.minZ != b.minZ {
			t.Errorf("zone %d differs from its decode", i)
		}
		if len(a.verts) > 0 && !inMap(w.m, unsafe.Pointer(&a.verts[0])) {
			t.Errorf("zone %d's vertices are not the mapping's", i)
		}
	}
	after := answers(g, 1, lo, hi, 2000)
	if !reflect.DeepEqual(before, after) {
		t.Error("a query answers otherwise over the map")
	}
	hits := 0
	for _, v := range before {
		if v == true {
			hits++
		}
	}
	if hits < 100 {
		t.Errorf("the control: only %d answers said yes of %d: the queries found nothing to compare", hits, len(before))
	}
}

func mustZone(t *testing.T, w *world, i int) *zoneGrid {
	t.Helper()
	z, err := w.load(i)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

// nonNil is a slice with an empty one as nil, for DeepEqual.
func nonNil[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

func inMap(m *mapFile, p unsafe.Pointer) bool {
	base := uintptr(unsafe.Pointer(&m.data[0]))
	return uintptr(p) >= base && uintptr(p) < base+uintptr(len(m.data))
}

// A map of another bake is not used: the source changed (a new bake), or the file
// is not one. The world decodes, its answers the new bake's. Read as a mapped map
// it would answer the old world's floors.
func TestAMapOfAnotherBakeIsNotUsed(t *testing.T) {
	dir := t.TempDir()
	flat := func(y float32) func(float32, float32) float32 {
		return func(float32, float32) float32 { return y }
	}
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(0, 0, 100, 100, flat(50))}}})
	if err := WriteMaps(dir, nil); err != nil {
		t.Fatal(err)
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := g.Mapped(); m != 1 {
		t.Fatalf("the fresh map is not used: %d", m)
	}
	if y, ok := g.Floor(0, 50, 60, 50); !ok || !near(y, 50, 0.13) {
		t.Fatalf("the map's floor: %v %v", y, ok)
	}
	g.Close()

	// a new bake, the same size: only the CRC can tell
	old, _ := os.Stat(filepath.Join(dir, "0.ground"))
	writeWorld(t, dir, 0, []Zone{{Strips: [][]Vertex{quad(0, 0, 100, 100, flat(60))}}})
	now, _ := os.Stat(filepath.Join(dir, "0.ground"))
	if old.Size() != now.Size() {
		t.Fatalf("the two bakes differ in size: %d and %d", old.Size(), now.Size())
	}
	g, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := g.Mapped(); m != 0 {
		t.Errorf("a stale map is used")
	}
	if y, ok := g.Floor(0, 50, 70, 50); !ok || !near(y, 60, 0.13) {
		t.Errorf("the new bake's floor: %v %v, want 60", y, ok)
	}
	g.Close()

	// and rubbish where the map was
	if err := os.WriteFile(filepath.Join(dir, "0.ground.map"), []byte("not a map at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if m, _ := g.Mapped(); m != 0 {
		t.Errorf("rubbish is mapped")
	}
	if y, ok := g.Floor(0, 50, 70, 50); !ok || !near(y, 60, 0.13) {
		t.Errorf("beside rubbish the floor: %v %v, want 60", y, ok)
	}
}

// Writing a map again leaves one already mapped as it was: the new file takes the
// name, the old file's pages stay until unmapped (a running server keeps working).
func TestAMapWrittenAgainLeavesTheMappedOne(t *testing.T) {
	dir := t.TempDir()
	mapWorld(t, dir)
	if err := WriteMaps(dir, nil); err != nil {
		t.Fatal(err)
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	lo, hi := Vertex{1990, 30, 1990}, Vertex{2110, 80, 2060}
	before := answers(g, 2, lo, hi, 300)
	if err := WriteMaps(dir, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, answers(g, 2, lo, hi, 300)) {
		t.Error("the mapped file changed under a running server")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.map-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// The environment mapped is ReadEnv's, tree for tree; and what is not one still
// gets ReadEnv's word for it.
func TestAnEnvironmentMappedIsTheOneRead(t *testing.T) {
	dir := t.TempDir()
	in := &WorldEnv{
		Tree: EnvTree{Nodes: []EnvNode{{NX: 1, D: -10, Front: LeafBit | 1, Back: LeafBit}}, Leaves: []uint32{0, 1}},
		Zones: []EnvTree{
			{Leaves: []uint32{EnvWater}},
			{Nodes: []EnvNode{{NY: 1, D: -5, Front: LeafBit, Back: LeafBit | 1}}, Leaves: []uint32{0, EnvLava}},
		},
	}
	path := filepath.Join(dir, "0.env")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteEnv(f, in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	r, _ := os.Open(path)
	defer r.Close()
	read, err := ReadEnv(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := os.Open(path)
	defer r2.Close()
	m := mapEnv(r2)
	if m == nil {
		t.Skip("no mapping on this platform")
	}
	if !reflect.DeepEqual(m.Tree, read.Tree) || !reflect.DeepEqual(m.Zones[1], read.Zones[1]) || !reflect.DeepEqual(nonNil(m.Zones[0].Nodes), nonNil(read.Zones[0].Nodes)) {
		t.Errorf("mapped %+v, read %+v", m, read)
	}
	for _, p := range [][3]float32{{0, 0, 0}, {20, 0, 0}, {20, 9, 0}} {
		a, aok := m.Flags(p[0], p[1], p[2])
		b, bok := read.Flags(p[0], p[1], p[2])
		if a != b || aok != bok {
			t.Errorf("at %v: mapped %v %v, read %v %v", p, a, aok, b, bok)
		}
	}
	// short by a byte, and a wrong magic: not mapped, ReadEnv says why
	data, _ := os.ReadFile(path)
	bad := filepath.Join(dir, "1.env")
	os.WriteFile(bad, data[:len(data)-1], 0o644)
	b1, _ := os.Open(bad)
	defer b1.Close()
	if mapEnv(b1) != nil {
		t.Error("a short file was mapped")
	}
	data[0] = 'X'
	os.WriteFile(bad, data, 0o644)
	b2, _ := os.Open(bad)
	defer b2.Close()
	if mapEnv(b2) != nil {
		t.Error("a wrong magic was mapped")
	}
}

// A real baked ground (EQOA_GROUND_DIR, with its maps written beside it by
// cmd/groundmap): every zone of every world and every walls buffer is its decode,
// and every query answers the same over the map at tens of thousands of places
// round the world's own zones. Skipped without one: none is in this repository.
func TestRealMapsAreTheDecode(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if m, n := g.Mapped(); m != n {
		t.Skipf("%d of %d files mapped: run cmd/groundmap first", m, n)
	}
	// the same files with no maps: a copy's names would do, but Open takes a
	// folder, so the decode goes through the same world with its map set aside
	for n, w := range g.worlds {
		m := w.m
		for i := range w.entries {
			a := w.mappedZone(i)
			w.m = nil
			b, err := w.load(i)
			w.m = m
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(nonNil(a.verts), nonNil(b.verts)) || !reflect.DeepEqual(nonNil(a.tris), nonNil(b.tris)) ||
				!reflect.DeepEqual(nonNil(a.walls), nonNil(b.walls)) || !reflect.DeepEqual(nonNil(a.floors.cells), nonNil(b.floors.cells)) ||
				!reflect.DeepEqual(nonNil(a.floors.offsets), nonNil(b.floors.offsets)) || !reflect.DeepEqual(nonNil(a.wallGrid.cells), nonNil(b.wallGrid.cells)) ||
				!reflect.DeepEqual(nonNil(a.wallGrid.offsets), nonNil(b.wallGrid.offsets)) {
				t.Fatalf("world %d zone %d differs from its decode", n, i)
			}
		}
		t.Logf("world %d: %d zones the same", n, len(w.entries))
	}
	for n, w := range g.walls {
		m := w.m
		for i := range w.bufs {
			a := w.mappedBuffer(int32(i))
			w.m = nil
			w.read[i] = nil
			b, err := w.buffer(int32(i))
			w.read[i] = nil
			w.m = m
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(nonNil(a.verts), nonNil(b.verts)) || !reflect.DeepEqual(nonNil(a.strips), nonNil(b.strips)) {
				t.Fatalf("world %d's walls buffer %d differs from its decode", n, i)
			}
		}
		for i := range w.zones {
			a := w.mappedZone(i)
			w.m = nil
			b, err := w.load(i)
			w.m = m
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(nonNil(a.places), nonNil(b.places)) || !reflect.DeepEqual(nonNil(a.offsets), nonNil(b.offsets)) ||
				!reflect.DeepEqual(nonNil(a.cells), nonNil(b.cells)) || a.nx != b.nx || a.nz != b.nz {
				t.Fatalf("world %d's walls zone %d differs from its decode", n, i)
			}
		}
		t.Logf("world %d: %d walls buffers and %d zones the same", n, len(w.bufs), len(w.zones))
	}
	// and the answers, over the map and with every zone decoded
	w0 := g.worlds[0]
	var lo, hi Vertex
	lo, hi = noLow, noHigh
	for _, e := range w0.entries {
		if e.verts == 0 {
			continue
		}
		lo, hi = lowest(lo, Vertex{e.minX, e.baseY, e.minZ}), highest(hi, Vertex{e.maxX, e.baseY + 200, e.maxZ})
	}
	over := answers(g, 3, lo, hi, 40000)
	dec, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	for _, w := range dec.worlds {
		w.m.close()
		w.m = nil
	}
	for _, w := range dec.walls {
		w.m.close()
		w.m = nil
	}
	if !reflect.DeepEqual(over, answers(dec, 3, lo, hi, 40000)) {
		t.Error("a query answers otherwise over the maps")
	}
}
