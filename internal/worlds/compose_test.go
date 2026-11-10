package worlds

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

// rec is a record: its header (type, version, body size, children) and body.
func rec(ty uint16, version int16, count int, body []byte) []byte {
	b := binary.LittleEndian.AppendUint16(nil, ty)
	b = binary.LittleEndian.AppendUint16(b, uint16(version))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(body)))
	b = binary.LittleEndian.AppendUint32(b, uint32(count))
	return append(b, body...)
}

func le64(v int64) []byte  { return binary.LittleEndian.AppendUint64(nil, uint64(v)) }
func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func lef(v float32) []byte { return le32(math.Float32bits(v)) }

// fakeWorld is a world file of zones in the given cells, each a zone record
// whose ResourceTable names one record of the zone by its file offset and
// whose ZoneStaticTable names it too (and holds a -1), as retail's do; its
// regions are byte k for the k-th sub-cell. It returns the file and each
// zone's record offset.
func fakeWorld(cells [][2]int, rows, cols int) ([]byte, []int64) {
	head := append([]byte("FJBO"), le32(1)...)
	head = append(head, le32(43855)...)
	head = append(head, le32(0)...)
	head = append(head, le64(32)...)
	head = append(head, le64(-1)...)
	// zone records, each placed at its final offset: root (12) + world (12)
	var zones [][]byte
	var offs []int64
	at := int64(32 + 12 + 12)
	for i := range cells {
		marker := rec(0x6000, 0, 0, []byte{byte(i), 1, 2, 3})
		// ZoneResources first: the marker at its body
		res := rec(0x3100, 0, 1, marker)
		markerAt := at + 12 + 12 // zone header, resources header
		rt := append(le32(1), le64(markerAt)...)
		rt = append(rt, le32(uint32(len(marker)))...)
		rt = append(rt, le32(0x7F000000+uint32(i))...)
		st := append(le32(2), le64(markerAt)...)
		st = append(st, le32(uint32(len(marker)))...)
		st = append(st, le64(-1)...)
		st = append(st, le32(0)...)
		st = append(st, le64(-1)...)
		st = append(st, le32(0)...)
		st = append(st, le64(-1)...)
		st = append(st, le32(0)...)
		base := rec(0x3200, 0, 2, append(rec(0x9000, 0, 0, rt), rec(0x32C0, 0, 0, st)...))
		z := rec(0x3000, 1, 2, append(res, base...))
		zones = append(zones, z)
		offs = append(offs, at)
		at += int64(len(z))
	}
	var world []byte
	for _, z := range zones {
		world = append(world, z...)
	}
	hdrBody := le32(uint32(len(cells)))
	for _, v := range []float32{0, -1000, 0, float32(cols * 2000), 1000, float32(rows * 2000)} {
		hdrBody = append(hdrBody, lef(v)...)
	}
	hdrBody = append(hdrBody, le32(uint32(rows))...)
	hdrBody = append(hdrBody, le32(uint32(cols))...)
	hdrBody = append(hdrBody, le32(4)...)
	for _, v := range []float32{2000, 1000, 0, 1000} {
		hdrBody = append(hdrBody, lef(v)...)
	}
	px := le32(uint32(len(cells)))
	for i, c := range cells {
		p := append(le64(offs[i]), le64(offs[i]+int64(len(zones[i])-12))...)
		p = append(p, le32(99)...)
		x0, z0 := float32(c[0]*2000), float32(c[1]*2000)
		for _, v := range []float32{x0 + 1000, 0, z0 + 1000, x0 + 10, -5, z0 + 10, x0 + 1990, 50, z0 + 1990} {
			p = append(p, lef(v)...)
		}
		px = append(px, p...)
	}
	reg := make([]byte, rows*cols*16)
	for i := range reg {
		reg[i] = byte(i)
	}
	baseRec := rec(0x8200, 2, 4, bytes.Join([][]byte{rec(0x8220, 1, 0, hdrBody), rec(0x8210, 2, 0, px),
		rec(0x8230, 0, 0, make([]byte, 40)), rec(0x8240, 0, 0, reg)}, nil))
	root := rec(0x8000, 0, 2, append(rec(0x8100, 0, len(zones), world), baseRec...))
	return append(head, root...), offs
}

type memSource map[string][]byte

func (m memSource) Open(name string) (io.ReaderAt, int64, error) {
	b, ok := m[strings.ToUpper(name)]
	if !ok {
		return nil, 0, errors.New("not on the disc")
	}
	return bytes.NewReader(b), int64(len(b)), nil
}

// A world of two worlds' zones: each zone's record is the source's but for
// its absolute offsets, which name the same record in the new file (and -1
// stays -1); its proxy names its new place and keeps the rest; the BSP walks
// every zone's middle to that zone; each zone's cells take their own world's
// profiles; the header is retail's. The controls: a zone in a taken cell, and
// a zone the source lacks, are refused.
func TestComposeTwoWorldsZones(t *testing.T) {
	a, aOffs := fakeWorld([][2]int{{1, 1}, {2, 1}, {3, 4}}, 6, 5)
	b, bOffs := fakeWorld([][2]int{{4, 0}, {0, 3}}, 5, 6)
	src := memSource{"A.ESF": a, "B.ESF": b}
	r := Recipe{World: 10, Name: "Test", Zones: []RecipeZone{
		{Name: "one", From: "a.esf", Zone: 2}, {Name: "two", From: "B.ESF", Zone: 0}, {Name: "three", From: "A.ESF", Zone: 0},
	}}
	out, built, err := Compose(src, r)
	if err != nil {
		t.Fatal(err)
	}
	if built.Size != int64(len(out)) || built.CRC != crc32.ChecksumIEEE(out) || built.Cols != 5 || built.Rows != 5 {
		t.Fatalf("built: %+v", built)
	}
	if !bytes.Equal(out[:32], append(append(append(append(append([]byte("FJBO"), le32(1)...), le32(43855)...), le32(0)...), le64(32)...), le64(-1)...)) {
		t.Fatalf("header %x", out[:32])
	}
	w, err := openWorld(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.zones) != 3 || len(w.proxies) != 3 || w.rows != 5 || w.cols != 5 {
		t.Fatalf("%d zones, %d proxies, %d x %d", len(w.zones), len(w.proxies), w.cols, w.rows)
	}
	srcs := []struct {
		file []byte
		at   int64
	}{{a, aOffs[2]}, {b, bOffs[0]}, {a, aOffs[0]}}
	for i, s := range srcs {
		got := out[w.zones[i][0] : w.zones[i][0]+w.zones[i][1]]
		orig := s.file[s.at : s.at+int64(len(got))]
		delta := w.zones[i][0] - s.at
		// the marker the tables name is at the same place in the record
		rtAt := bytes.Index(got, rec(0x9000, 0, 0, nil)[:4]) + 12
		if v := int64(binary.LittleEndian.Uint64(got[rtAt+4:])); v-w.zones[i][0] != int64(binary.LittleEndian.Uint64(orig[rtAt+4:]))-s.at {
			t.Errorf("zone %d: resource offset %d, moved by %d", i, v, delta)
		}
		if v := int64(binary.LittleEndian.Uint64(got[rtAt+4:])); out[v] != 0x00 || out[v+1] != 0x60 || out[v+12] != byte(map[int]int{0: 2, 1: 0, 2: 0}[i]) {
			t.Errorf("zone %d: the resource table names %x", i, out[v:v+16])
		}
		stAt := bytes.Index(got, rec(0x32C0, 0, 0, nil)[:4]) + 12
		if v := int64(binary.LittleEndian.Uint64(got[stAt+4:])); v != int64(binary.LittleEndian.Uint64(orig[stAt+4:]))+delta {
			t.Errorf("zone %d: static offset %d", i, v)
		}
		if v := int64(binary.LittleEndian.Uint64(got[stAt+4+12:])); v != -1 {
			t.Errorf("zone %d: -1 moved to %d", i, v)
		}
		// everything else verbatim
		diff := 0
		for k := range got {
			if got[k] != orig[k] {
				diff++
			}
		}
		if diff > 16 {
			t.Errorf("zone %d: %d bytes differ from its source", i, diff)
		}
		p := w.proxies[i]
		if int64(binary.LittleEndian.Uint64(p)) != w.zones[i][0] || binary.LittleEndian.Uint32(p[16:]) != 99 {
			t.Errorf("zone %d: proxy %x", i, p[:20])
		}
	}
	// the BSP: each zone's middle routes to it
	tree := treeOf(t, out)
	for i, p := range built.Parts {
		x, z := float32(p.Col*2000+1000), float32(p.Row*2000+1000)
		if got := tree(x, z); got != i {
			t.Errorf("(%v, %v) routes to zone %d, not %d", x, z, got, i)
		}
	}
	// the profiles: zone 1 (B's (4, 0)) takes B's over its cell (sub-cell
	// (1, 17), index 1*20+17 here, 1*24+17 in B), a cell of no zone none
	if got, want := w.regions[1*20+17], byteAt(b, 1, 17); got != want || got != 41 {
		t.Errorf("B's profile at its cell: %d (want %d)", got, want)
	}
	if w.regions[(4*4+3)*20+0] != 0 {
		t.Error("a cell of no zone has a profile")
	}

	if _, _, err := Compose(src, Recipe{World: 10, Zones: []RecipeZone{{From: "A.ESF", Zone: 0}, {From: "B.ESF", Zone: 0}, {From: "A.ESF", Zone: 0}}}); err == nil || !strings.Contains(err.Error(), "both stand in cell") {
		t.Errorf("a taken cell: %v", err)
	}
	if _, _, err := Compose(src, Recipe{World: 10, Zones: []RecipeZone{{From: "A.ESF", Zone: 3}}}); err == nil {
		t.Error("a zone the source lacks was taken")
	}
	if _, _, err := Compose(src, Recipe{World: 3, Zones: []RecipeZone{{From: "A.ESF", Zone: 0}}}); err == nil {
		t.Error("a disc world's slot was taken")
	}
}

// byteAt is the regions byte of sub-cell (r, c) of a fake world.
func byteAt(file []byte, r, c int) byte {
	w, _ := openWorld(bytes.NewReader(file), int64(len(file)))
	return w.region(r, c)
}

// treeOf walks the built world's BSP as vi::world::World::find_zone does.
func treeOf(t *testing.T, out []byte) func(x, z float32) int {
	t.Helper()
	r := bytes.NewReader(out)
	head, _ := readAt(r, 0, 32)
	root, _ := readHdr(r, int64(binary.LittleEndian.Uint64(head[16:])), int64(len(out)))
	top, _ := children(r, root)
	var body []byte
	for _, c := range top {
		if c.ty == typeWorldBase {
			kids, _ := children(r, c)
			for _, k := range kids {
				if k.ty == typeWorldTree {
					body, _ = readAt(r, k.at+12, int(k.size))
				}
			}
		}
	}
	nodes := int(binary.LittleEndian.Uint32(body))
	return func(x, z float32) int {
		ref := uint32(0)
		for {
			o := 8 + 24*int(ref&0x0FFFFFFF)
			n := [3]float32{f32(body, o), f32(body, o+4), f32(body, o+8)}
			d := f32(body, o+12)
			if n[0]*x+n[2]*z+d >= 0 {
				ref = binary.LittleEndian.Uint32(body[o+16:])
			} else {
				ref = binary.LittleEndian.Uint32(body[o+20:])
			}
			if ref&leafBit != 0 {
				break
			}
		}
		return int(binary.LittleEndian.Uint32(body[8+24*nodes+4*int(ref&0x0FFFFFFF):]))
	}
}

// The prototype's world off the real disc: Aviak Village alone is the bytes
// eqoa-rust-client-vanilla builds of the same recipe (its serverworlds test
// holds the same size and CRC). Skipped without EQOA_ISO.
func TestComposeOffTheDisc(t *testing.T) {
	iso := os.Getenv("EQOA_ISO")
	if iso == "" {
		t.Skip("EQOA_ISO not set")
	}
	d, err := OpenDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	out, built, err := Compose(d, Recipe{World: 10, Name: "the Homelands", Zones: []RecipeZone{{Name: "Aviak Village", From: "TUNARIA.ESF", Zone: 115}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5880303 || built.CRC != 0xB6D94C34 || built.Parts[0].CRC != 0x7257AE33 || built.Parts[0].Col != 4 || built.Parts[0].Row != 10 {
		t.Fatalf("%d bytes, CRC %08x, %+v", len(out), built.CRC, built.Parts[0])
	}
}
