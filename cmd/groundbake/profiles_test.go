package main

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/esf"
	"github.com/eqoa/server/internal/weather"
)

// synthWorld is a world file with a root, a world record and a base record
// whose header (0x8220, of version) and regions (0x8240) are a 2x3-cell grid
// of subdiv 4, 2000-unit cells, origin (1000, 0, 1000), each sub-cell's
// profile its (i + 8*j) % 22: no game data.
func synthWorld(version int16) ([]byte, []byte) {
	const rows, cols, sub = 2, 3, 4
	w := esf.NewWriter()
	root := w.WriteNodeBegin(esf.TypeRoot, 0, 2)
	world := w.WriteNodeBegin(esf.TypeWorld, 0, 0)
	w.WriteInt32(0)
	w.WriteNodeEnd(world)
	base := w.WriteNodeBegin(esf.TypeWorldBase, 2, 2)
	hdr := w.WriteNodeBegin(esf.TypeWorldBaseHeader, version, 0)
	if version != 0 {
		w.WriteInt32(42) // the zone count
	}
	for _, v := range []float32{0, -100, 0, cols * 2000, 100, rows * 2000} {
		w.WriteFloat32(v)
	}
	w.WriteInt32(rows)
	w.WriteInt32(cols)
	if version != 0 {
		w.WriteInt32(sub)
	}
	w.WriteFloat32(2000)
	w.WriteFloat32(1000)
	w.WriteFloat32(0)
	w.WriteFloat32(1000)
	w.WriteNodeEnd(hdr)
	reg := w.WriteNodeBegin(esf.TypeWorldRegions, 0, 0)
	data := make([]byte, rows*cols*sub*sub)
	for j := 0; j < rows*sub; j++ {
		for i := 0; i < cols*sub; i++ {
			data[j*cols*sub+i] = byte((i + 8*j) % 22)
		}
	}
	w.WriteBytes(data)
	w.WriteNodeEnd(reg)
	w.WriteNodeEnd(base)
	w.WriteNodeEnd(root)
	return w.Finalize(), data
}

func TestProfilesFromASyntheticWorld(t *testing.T) {
	for _, version := range []int16{0, 1} {
		file, data := synthWorld(version)
		f, err := esf.OpenBytes(file)
		if err != nil {
			t.Fatal(err)
		}
		root, err := f.Root()
		if err != nil {
			t.Fatal(err)
		}
		p, err := bakeProfiles(f, root)
		if err != nil || p == nil {
			t.Fatalf("version %d: %v %v", version, p, err)
		}
		if p.Rows != 2 || p.Cols != 3 || p.Subdiv != 4 || p.Cell != 2000 || p.OriginX != 1000 || p.OriginZ != 1000 || !bytes.Equal(p.Data, data) {
			t.Fatalf("version %d: %+v", version, p)
		}
		// through the file and back, to the right squares
		var b bytes.Buffer
		if err := weather.WriteProfiles(&b, p); err != nil {
			t.Fatal(err)
		}
		q, err := weather.ReadProfiles(&b)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ x, z float32 }{{10, 10}, {1499, 20}, {5999, 3999}, {2600, 2100}, {5900, 3900}} {
			sx, sz := weather.SquareAt(c.x, c.z)
			got, ok := q.At(sx, sz)
			if want := (sx + 8*sz) % 22; !ok || got != want {
				t.Errorf("version %d (%g, %g): square (%d, %d) profile %d %v, want %d", version, c.x, c.z, sx, sz, got, ok, want)
			}
		}
		if _, ok := q.At(12, 0); ok {
			t.Errorf("version %d: a square past the grid", version)
		}
	}
}

func TestNoRegionsNoProfiles(t *testing.T) {
	w := esf.NewWriter()
	root := w.WriteNodeBegin(esf.TypeRoot, 0, 1)
	base := w.WriteNodeBegin(esf.TypeWorldBase, 1, 0)
	w.WriteNodeEnd(base)
	w.WriteNodeEnd(root)
	f, err := esf.OpenBytes(w.Finalize())
	if err != nil {
		t.Fatal(err)
	}
	r, _ := f.Root()
	if p, err := bakeProfiles(f, r); p != nil || err != nil {
		t.Fatalf("%v %v", p, err)
	}
}
