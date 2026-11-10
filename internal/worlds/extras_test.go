package worlds

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fakes' DictIDs.
const (
	idTree    = 0x7A000001 // zone A's own sprite (a SimpleSprite holding its palette and surface)
	idTreeSrf = 0x7A000002
	idTreePal = 0x7A000003
	idRock    = 0x7B000001 // zone B's: a SimpleSprite whose material names a surface B holds apart
	idRockPal = 0x7B000002
	idRockSrf = 0x7B000003 // a surface of B at the top of its resources
	idGroup   = 0x7B000004 // a group of B whose member is the rock
	idPatch   = 0x7B000005 // a sub-sprite of B (its terrain)
	idTerrain = 0x7B000006 // a sprite of B whose prim buffer stands on pre-translations
)

func surfaceRec(id uint32, fill byte) []byte {
	return rec(0x1000, 0, 0, append(le32(id), bytes.Repeat([]byte{fill}, 20)...))
}

// materialRec is a version 1 material of one layer naming texture.
func materialRec(texture uint32) []byte {
	layer := append(le32(0), le32(texture)...)
	layer = append(layer, make([]byte, 60)...)
	return rec(0x1100, 1, 0, append(le32(1), layer...))
}

// spriteRec is a SimpleSprite: its header, a palette (its own surface when
// own is set, and a material naming texture), a prim buffer of kind and a
// collision buffer of packing.
func spriteRec(id, pal, texture uint32, own []byte, kind, packing uint32) []byte {
	head := rec(0x2001, 1, 0, append(le32(id), make([]byte, 28)...))
	var srf []byte
	n := 2
	if own != nil {
		srf = rec(0x1001, 0, 1, own)
		n = 3
	}
	palette := rec(0x1110, 1, n, bytes.Join([][]byte{rec(0x1111, 1, 0, le32(pal)), srf, rec(0x1101, 0, 1, materialRec(texture))}, nil))
	pb := rec(0x1200, 2, 0, append(append(le32(id^0x00FF0000), le32(kind)...), make([]byte, 24)...))
	cb := rec(0x4200, 2, 0, append(le32(packing), make([]byte, 16)...))
	return rec(0x2000, 2, 4, bytes.Join([][]byte{head, palette, pb, cb}, nil))
}

func actorRec(sprite uint32, x, y, z float32) []byte {
	b := le32(sprite)
	for _, v := range []float32{x, y, z, 0, 0, 0, 1} {
		b = append(b, lef(v)...)
	}
	return rec(0x6000, 2, 0, append(b, 0, 0, 0, 255))
}

// fakeZone is a zone of cell (col, row) as retail's are: resources, the
// rooms' actor lists, their baked lighting and a base of four rooms (the
// cell's quarters) with their tree, resource table and static table, the
// tables naming records by their offsets in a file where the record is at
// at. Room 1 has an actor list (one actor of its own sprite) and its
// lighting; the others none.
func fakeZone(at int64, col, row int, resources [][]byte, ownSprite uint32) []byte {
	x0, z0 := float32(col*2000), float32(row*2000)
	res := rec(0x3100, 0, len(resources), bytes.Join(resources, nil))
	list := rec(0x3270, 0, 2, append(rec(0x3280, 0, 0, le32(1)), rec(0x6040, 0, 1, actorRec(ownSprite, x0+1500, 10, z0+500))...))
	acts := rec(0x3290, 0, 1, list)
	cbuf := rec(0x1220, 0, 0, append(le32(0), le32(0)...))
	lit := rec(0x32B0, 0, 1, rec(0x3290, 0, 2, append(rec(0x32A0, 0, 0, le32(1)), rec(0x6030, 0, 1, rec(0x6010, 0, 2, append(rec(0x6020, 0, 0, append(le32(0), le32(0)...)), cbuf...)))...)))
	var rooms []byte
	for q := 0; q < 4; q++ {
		bx, bz := x0+float32(q%2)*1000, z0+float32(q/2)*1000
		b := append(le32(0), le32(0)...)
		b = append(b, le32(0)...)
		for _, v := range []float32{bx, 0, bz, bx + 1000, 50, bz + 1000} {
			b = append(b, lef(v)...)
		}
		rooms = append(rooms, rec(0x3240, 2, 0, append(b, make([]byte, 8)...))...)
	}
	tree := rec(0x3220, 0, 0, make([]byte, 8))
	roomsRec := rec(0x3230, 0, 4, rooms)
	// the tables, filled in once the record's offsets are known
	ids := map[int64]uint32{}
	var rtEntries int
	{
		p := int64(12 + 12) // zone header, resources header
		for _, r := range resources {
			roots, _ := parseNodes(r, 0, int64(len(r)))
			var id uint32
			registers(roots[0], func(d uint32, n *node) {
				if n == roots[0] && id == 0 {
					id = d
				}
			})
			ids[p] = id
			p += int64(len(r))
			rtEntries++
		}
	}
	rt := rec(0x9000, 0, 0, make([]byte, 4+16*rtEntries))
	st := rec(0x32C0, 0, 0, make([]byte, 4+24*4))
	base := rec(0x3200, 1, 4, bytes.Join([][]byte{tree, roomsRec, rt, st}, nil))
	z := rec(0x3000, 1, 4, bytes.Join([][]byte{res, acts, lit, base}, nil))
	roots, err := parseNodes(z, 0, int64(len(z)))
	if err != nil {
		panic(err)
	}
	zn := roots[0]
	b := zn.child(0x3200)
	rtAt := b.child(0x9000).old + 12
	type e struct {
		off int64
		n   int64
		id  uint32
	}
	var es []e
	for _, r := range zn.child(0x3100).kids {
		es = append(es, e{at + r.old, 12 + r.leafBody(), ids[r.old]})
	}
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].id < es[j-1].id; j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
	binary.LittleEndian.PutUint32(z[rtAt:], uint32(len(es)))
	for i, x := range es {
		o := rtAt + 4 + 16*int64(i)
		binary.LittleEndian.PutUint64(z[o:], uint64(x.off))
		binary.LittleEndian.PutUint32(z[o+8:], uint32(x.n))
		binary.LittleEndian.PutUint32(z[o+12:], x.id)
	}
	stAt := b.child(0x32C0).old + 12
	binary.LittleEndian.PutUint32(z[stAt:], 4)
	l := zn.child(0x3290).kids[0]
	lt := zn.child(0x32B0).kids[0]
	for q := 0; q < 4; q++ {
		o := stAt + 4 + 24*int64(q)
		if q == 1 {
			binary.LittleEndian.PutUint64(z[o:], uint64(at+l.old))
			binary.LittleEndian.PutUint32(z[o+8:], uint32(12+l.leafBody()))
			binary.LittleEndian.PutUint64(z[o+12:], uint64(at+lt.old))
			binary.LittleEndian.PutUint32(z[o+20:], uint32(12+lt.leafBody()))
		} else {
			binary.LittleEndian.PutUint64(z[o:], math.MaxUint64)
			binary.LittleEndian.PutUint64(z[o+12:], math.MaxUint64)
		}
	}
	return z
}

// worldOf is a world file of the zones (each built for its offset) in their
// cells.
func worldOf(rows, cols int, cells [][2]int, zones ...func(at int64) []byte) []byte {
	head := append([]byte("FJBO"), le32(1)...)
	head = append(head, le32(43855)...)
	head = append(head, le32(0)...)
	head = append(head, le64(32)...)
	head = append(head, le64(-1)...)
	at := int64(32 + 12 + 12)
	var recs [][]byte
	var offs []int64
	for _, mk := range zones {
		z := mk(at)
		recs = append(recs, z)
		offs = append(offs, at)
		at += int64(len(z))
	}
	hdrBody := le32(uint32(len(recs)))
	for _, v := range []float32{0, -1000, 0, float32(cols * 2000), 1000, float32(rows * 2000)} {
		hdrBody = append(hdrBody, lef(v)...)
	}
	hdrBody = append(append(append(hdrBody, le32(uint32(rows))...), le32(uint32(cols))...), le32(4)...)
	for _, v := range []float32{2000, 1000, 0, 1000} {
		hdrBody = append(hdrBody, lef(v)...)
	}
	px := le32(uint32(len(recs)))
	for i, z := range recs {
		roots, _ := parseNodes(z, 0, int64(len(z)))
		b := roots[0].child(0x3200)
		x0, z0 := float32(cells[i][0]*2000), float32(cells[i][1]*2000)
		p := append(le64(offs[i]), le64(offs[i]+b.old)...)
		p = append(p, le32(uint32(12+b.leafBody()))...)
		for _, v := range []float32{x0 + 1000, 0, z0 + 1000, x0, -5, z0, x0 + 2000, 50, z0 + 2000} {
			p = append(p, lef(v)...)
		}
		px = append(px, p...)
	}
	reg := make([]byte, rows*cols*16)
	baseRec := rec(0x8200, 2, 4, bytes.Join([][]byte{rec(0x8220, 1, 0, hdrBody), rec(0x8210, 2, 0, px),
		rec(0x8230, 0, 0, make([]byte, 40)), rec(0x8240, 0, 0, reg)}, nil))
	root := rec(0x8000, 0, 2, append(rec(0x8100, 0, len(recs), bytes.Join(recs, nil)), baseRec...))
	return append(head, root...)
}

// The extras' fakes: world A has one zone, in cell (1, 1), placing its own
// sprite; world B one in (3, 1), holding a rock (its surface apart, at the top
// of B's resources), a group of the rock, a terrain patch and a sprite on
// pre-translations.
func extrasFakes() memSource {
	a := worldOf(3, 4, [][2]int{{1, 1}}, func(at int64) []byte {
		return fakeZone(at, 1, 1, [][]byte{spriteRec(idTree, idTreePal, idTreeSrf, surfaceRec(idTreeSrf, 0xAA), 2, 1)}, idTree)
	})
	b := worldOf(3, 4, [][2]int{{0, 1}, {3, 1}}, func(at int64) []byte {
		return fakeZone(at, 0, 1, [][]byte{spriteRec(idTree+9, idTreePal+9, idTreeSrf+9, surfaceRec(idTreeSrf+9, 0xEE), 2, 1)}, idTree+9)
	}, func(at int64) []byte {
		group := rec(0x2C00, 0, 2, append(rec(0x2C10, 0, 0, append(le32(idGroup), make([]byte, 24)...)),
			rec(0x2C30, 0, 0, append(le32(1), append(le32(idRock), make([]byte, 28)...)...))...))
		patch := rec(0x2310, 1, 1, rec(0x2311, 0, 0, append(append(le32(idPatch), le32(idRockPal)...), make([]byte, 24)...)))
		return fakeZone(at, 3, 1, [][]byte{
			surfaceRec(idRockSrf, 0xBB),
			spriteRec(idRock, idRockPal, idRockSrf, nil, 2, 1),
			group,
			patch,
			spriteRec(idTerrain, idRockPal+0x100, idRockSrf, nil, 4, 3),
		}, idRock)
	})
	return memSource{"A.ESF": a, "B.ESF": b}
}

// a zone's records read back out of a built file: the zone record, its
// registrations (first wins) and the file's records by offset.
type readBack struct {
	zone *node
	reg  map[uint32]*node
	all  map[int64]*node
}

func readZone(t *testing.T, out []byte, zi int) readBack {
	t.Helper()
	w, err := openWorld(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	roots, err := parseNodes(out, w.zones[zi][0], w.zones[zi][0]+w.zones[zi][1])
	if err != nil {
		t.Fatal(err)
	}
	rb := readBack{zone: roots[0], reg: map[uint32]*node{}, all: map[int64]*node{}}
	registers(rb.zone, func(id uint32, n *node) {
		if _, ok := rb.reg[id]; !ok {
			rb.reg[id] = n
		}
	})
	var index func(n *node)
	index = func(n *node) {
		rb.all[n.old] = n
		for _, k := range n.kids {
			index(k)
		}
	}
	index(rb.zone)
	return rb
}

// tablesHold checks a zone's tables as a PS2 reads them: every resource
// table entry names a record of the zone that registers its DictID, with its
// length, in DictID order; every static table entry names its room's actor
// list or lighting, with its length.
func tablesHold(t *testing.T, rb readBack) {
	t.Helper()
	base := rb.zone.child(0x3200)
	rt := base.child(0x9000).tail
	n := int(binary.LittleEndian.Uint32(rt))
	if len(rt) != 4+16*n {
		t.Fatalf("resource table: %d entries in %d bytes", n, len(rt))
	}
	var last uint32
	for i := 0; i < n; i++ {
		off, ln, id := int64(binary.LittleEndian.Uint64(rt[4+16*i:])), binary.LittleEndian.Uint32(rt[4+16*i+8:]), binary.LittleEndian.Uint32(rt[4+16*i+12:])
		r := rb.all[off]
		if r == nil || rb.reg[id] != r || int64(ln) != 12+r.leafBody() {
			t.Errorf("resource table entry %d: %#08x at %d (%d bytes) is not its record", i, id, off, ln)
		}
		if i > 0 && id <= last {
			t.Errorf("resource table entry %d: %#08x after %#08x", i, id, last)
		}
		last = id
	}
	st := base.child(0x32C0).tail
	for q := 0; q < int(binary.LittleEndian.Uint32(st)); q++ {
		for k, want := range []uint16{0x3270, 0x3290} {
			o := 4 + 24*q + 12*k
			off, ln := int64(binary.LittleEndian.Uint64(st[o:])), binary.LittleEndian.Uint32(st[o+8:])
			if off < 0 {
				continue
			}
			r := rb.all[off]
			idRec := map[uint16]uint16{0x3270: 0x3280, 0x3290: 0x32A0}[want]
			if r == nil || r.ty != want || int64(ln) != 12+r.leafBody() || r.child(idRec) == nil || word(r.child(idRec).tail, 0) != uint32(q) {
				t.Errorf("static table room %d (%#x): %d (%d bytes) is not its record", q, want, off, ln)
			}
		}
	}
}

// actorsOf is a zone's actors by room: (sprite, x, z) each.
func actorsOf(rb readBack) map[uint32][][3]uint32 {
	out := map[uint32][][3]uint32{}
	for _, l := range rb.zone.child(0x3290).kids {
		room := word(l.child(0x3280).tail, 0)
		for _, a := range l.child(0x6040).kids {
			out[room] = append(out[room], [3]uint32{word(a.tail, 0), word(a.tail, 4), word(a.tail, 12)})
		}
	}
	return out
}

// Extras in a composed zone: one of the zone's own sprite (nothing copied)
// filed after the room's own actor, and B's group placed in A's zone: the
// group, the rock it names and the surface the rock's material names are
// copied in after A's resources, each after what it names, byte for byte
// B's, with a resource table entry each in DictID order; a room with no
// list is given one in room order, its static table entry naming it; every
// offset of both tables names its record (a PS2 reads them), the proxy names
// the base and its length. The zone with no extras is the bytes the world
// without any extras has. The controls follow.
func TestComposeExtras(t *testing.T) {
	src := extrasFakes()
	recipe := func(extras ...Extra) Recipe {
		return Recipe{World: 10, Name: "Test", Zones: []RecipeZone{
			{Name: "a", From: "A.ESF", Zone: 0, Extras: extras},
			{Name: "b", From: "B.ESF", Zone: 1},
		}}
	}
	plain, plainBuilt, err := Compose(src, recipe())
	if err != nil {
		t.Fatal(err)
	}
	own := Extra{Sprite: idTree, At: [3]float32{3600, 12, 2900}, HPR: [3]float32{1, 0, 0}, Scale: 2}
	group := Extra{Sprite: idGroup, From: "b.esf", Zone: 1, At: [3]float32{2100, 5, 3100}}
	out, built, err := Compose(src, recipe(own, group))
	if err != nil {
		t.Fatal(err)
	}
	if built.Size != int64(len(out)) || built.CRC != crc32.ChecksumIEEE(out) || built.Parts[0].Extras != 2 || built.Parts[0].Brought != 3 {
		t.Fatalf("built: %+v", built.Parts[0])
	}
	if built.Parts[0].CRC != plainBuilt.Parts[0].CRC || built.Parts[0].Size != plainBuilt.Parts[0].Size {
		t.Error("a zone's CRC is its record's on the disc, extras or none")
	}
	a := readZone(t, out, 0)
	tablesHold(t, a)
	tablesHold(t, readZone(t, plain, 0))
	// the actors: (3600, 2900) is room 1's quarter of the cell, after the
	// room's own actor; (2100, 3100) room 2's, which gets a list of its own
	got := actorsOf(a)
	f := math.Float32bits
	if len(got) != 2 || len(got[1]) != 2 || got[1][0] != [3]uint32{idTree, f(3500), f(2500)} || got[1][1] != [3]uint32{idTree, f(3600), f(2900)} ||
		len(got[2]) != 1 || got[2][0] != [3]uint32{idGroup, f(2100), f(3100)} {
		t.Errorf("actors by room: %v", got)
	}
	if lists := a.zone.child(0x3290).kids; len(lists) != 2 || word(lists[0].child(0x3280).tail, 0) != 1 || word(lists[1].child(0x3280).tail, 0) != 2 {
		t.Error("the rooms' lists are not in room order")
	}
	// the copies: after A's own resource, the surface, the rock, the group
	res := a.zone.child(0x3100).kids
	if len(res) != 4 {
		t.Fatalf("%d resources", len(res))
	}
	b := readZone(t, src["B.ESF"], 1)
	for k, id := range []uint32{idRockSrf, idRock, idGroup} {
		if a.reg[id] != res[1+k] || !bytes.Equal(recordBytes(res[1+k]), recordBytes(b.reg[id])) {
			t.Errorf("copy %d (%#08x) is not B's record, in its place", k, id)
		}
	}
	if a.reg[idPatch] != nil || a.reg[idTerrain] != nil {
		t.Error("art the group does not name was copied")
	}
	// the proxy: its zone, its base, the base's length
	w, _ := openWorld(bytes.NewReader(out), int64(len(out)))
	p := w.proxies[0]
	zb := a.zone.child(0x3200)
	if int64(binary.LittleEndian.Uint64(p)) != w.zones[0][0] || int64(binary.LittleEndian.Uint64(p[8:])) != zb.old || int64(binary.LittleEndian.Uint32(p[16:])) != 12+zb.leafBody() {
		t.Errorf("proxy %x", p[:20])
	}
	// the zone with none is as it was without any
	pw, _ := openWorld(bytes.NewReader(plain), int64(len(plain)))
	if !bytes.Equal(out[w.zones[1][0]:w.zones[1][0]+w.zones[1][1]], func() []byte {
		r := append([]byte(nil), plain[pw.zones[1][0]:pw.zones[1][0]+pw.zones[1][1]]...)
		if _, err := rebase(r, w.zones[1][0]-pw.zones[1][0]); err != nil {
			t.Fatal(err)
		}
		return r
	}()) {
		t.Error("the zone with no extras is not as it is with none anywhere")
	}
	// the same extra twice brings nothing twice
	if _, built2, err := Compose(src, recipe(group, group)); err != nil || built2.Parts[0].Brought != 3 {
		t.Errorf("twice: %v, %+v", err, built2.Parts[0])
	}

	// the controls
	for _, c := range []struct {
		name string
		e    Extra
		want string
	}{
		{"art not named", Extra{Sprite: idRock, At: [3]float32{2100, 5, 2100}}, "names no art"},
		{"art not there", Extra{Sprite: 0x12345678, From: "B.ESF", Zone: 1, At: [3]float32{2100, 5, 2100}}, "does not register"},
		{"a terrain patch", Extra{Sprite: idPatch, From: "B.ESF", Zone: 1, At: [3]float32{2100, 5, 2100}}, "sub-sprite"},
		{"pre-translated", Extra{Sprite: idTerrain, From: "B.ESF", Zone: 1, At: [3]float32{2100, 5, 2100}}, "pre-translations"},
		{"a zone not read", Extra{Sprite: idRock, From: "B.ESF", Zone: 7, At: [3]float32{2100, 5, 2100}}, "has 2 zones"},
	} {
		if _, _, err := Compose(src, recipe(c.e)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// a DictID the zone registers is the zone's: B's rock's material names
	// B's surface, which A's zone holds (as other bytes): A's is used, and
	// nothing of B's surface comes in
	mine := extrasFakes()
	mine["A.ESF"] = worldOf(3, 4, [][2]int{{1, 1}}, func(at int64) []byte {
		return fakeZone(at, 1, 1, [][]byte{spriteRec(idTree, idTreePal, idTreeSrf, surfaceRec(idRockSrf, 0xCC), 2, 1)}, idTree)
	})
	if _, built, err := Compose(mine, recipe(Extra{Sprite: idRock, From: "B.ESF", Zone: 1, At: [3]float32{2100, 5, 2100}})); err != nil || built.Parts[0].Brought != 1 {
		t.Errorf("a surface the zone holds is the zone's, not copied: %v, %+v", err, built.Parts[0])
	}
	// a world whose other zone holds what the extras bring, as other bytes,
	// is refused (the first registration wins, there and in the server's
	// bake): C's zone holds the group's id as a sprite of its own
	other := extrasFakes()
	other["C.ESF"] = worldOf(3, 4, [][2]int{{0, 1}, {3, 1}}, func(at int64) []byte {
		return fakeZone(at, 0, 1, [][]byte{spriteRec(idTree+9, idTreePal+9, idTreeSrf+9, surfaceRec(idTreeSrf+9, 0xEE), 2, 1)}, idTree+9)
	}, func(at int64) []byte {
		return fakeZone(at, 3, 1, [][]byte{spriteRec(idGroup, idTreePal+7, idTreeSrf, surfaceRec(idTreeSrf, 0xDD), 2, 1)}, idGroup)
	})
	r := recipe(group)
	r.Zones[1].From = "C.ESF"
	if _, _, err := Compose(other, r); err == nil || !strings.Contains(err.Error(), "other bytes") {
		t.Errorf("another zone's id as other bytes: %v", err)
	}
	// the same bytes are no clash: B's own zone, beside A's, holds the rock
	// and the surface as they were copied
	if _, _, err := Compose(src, recipe(group)); err != nil {
		t.Errorf("B's own zone beside the copies: %v", err)
	}
}

// A recipe's extras files: their zones' extras added after each zone's own,
// by the zone they name; then the recipe names no file, and what is built
// says each extra. A file naming a zone the recipe has not is refused.
func TestRecipeExtraFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("edges.json", map[string]any{"zones": []map[string]any{
		{"from": "b.esf", "zone": 1, "extras": []map[string]any{{"sprite": "0x7B000001", "at": []float32{1, 2, 3}, "scale": 4}}},
	}})
	write("r.json", map[string]any{"world": 10, "name": "T", "extraFiles": []string{"edges.json"}, "zones": []map[string]any{
		{"name": "a", "from": "A.ESF", "zone": 0},
		{"name": "b", "from": "B.ESF", "zone": 1, "extras": []map[string]any{{"sprite": 2064646145, "at": []float32{5, 6, 7}}}},
	}})
	r, err := ReadRecipe(filepath.Join(dir, "r.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ExtraFiles) != 0 || len(r.Zones[0].Extras) != 0 || len(r.Zones[1].Extras) != 2 ||
		r.Zones[1].Extras[0].Sprite != 2064646145 || r.Zones[1].Extras[1].Sprite != 0x7B000001 || r.Zones[1].Extras[1].Scale != 4 {
		t.Fatalf("%+v", r)
	}
	b, _ := json.Marshal(r.Zones[1].Extras[1])
	if !strings.Contains(string(b), `"sprite":"0x7B000001"`) || strings.Contains(string(b), "color") {
		t.Errorf("written as %s", b)
	}
	write("bad.json", map[string]any{"zones": []map[string]any{{"from": "B.ESF", "zone": 2, "extras": []map[string]any{}}}})
	write("r2.json", map[string]any{"world": 10, "extraFiles": []string{"bad.json"}, "zones": []map[string]any{{"from": "B.ESF", "zone": 1}}})
	if _, err := ReadRecipe(filepath.Join(dir, "r2.json")); err == nil || !strings.Contains(err.Error(), "not one of the recipe's") {
		t.Errorf("a file's zone the recipe lacks: %v", err)
	}
	for _, bad := range []Extra{{}, {Sprite: 1, Scale: -1}, {Sprite: 1, At: [3]float32{float32(math.NaN())}}, {Sprite: 1, From: "X", Zone: 0x10000}} {
		if (Recipe{World: 10, Zones: []RecipeZone{{From: "A.ESF", Extras: []Extra{bad}}}}).check() == nil {
			t.Errorf("%+v taken", bad)
		}
	}
}

// Extras off the real disc (EQOA_ISO): Aviak Village given its own tree
// (nothing copied), a rock of another Tunaria zone and a rock of Odus's,
// copied in. The bytes are what eqoa-rust-client-vanilla's serverworlds
// test builds of the same recipe (its size and CRC are held there too).
func TestComposeExtrasOffTheDisc(t *testing.T) {
	iso := os.Getenv("EQOA_ISO")
	if iso == "" {
		t.Skip("EQOA_ISO not set")
	}
	d, err := OpenDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	out, built, err := Compose(d, discExtrasRecipe())
	if err != nil {
		t.Fatal(err)
	}
	rb := readZone(t, out, 0)
	tablesHold(t, rb)
	t.Logf("%d bytes, CRC %08x, %d records brought", len(out), built.CRC, built.Parts[0].Brought)
	if len(out) != discExtrasSize || built.CRC != discExtrasCRC {
		t.Errorf("%d bytes, CRC %08x: not %d, %08x", len(out), built.CRC, discExtrasSize, discExtrasCRC)
	}
}

// discExtrasRecipe is the off-the-disc test's recipe (vanilla's
// serverworlds test composes the same).
func discExtrasRecipe() Recipe {
	return Recipe{World: 10, Name: "the Homelands", Zones: []RecipeZone{{Name: "Aviak Village", From: "TUNARIA.ESF", Zone: 115, Extras: []Extra{
		{Sprite: 0x194FA7CB, At: [3]float32{9300, 84, 21300}, HPR: [3]float32{0.5, 0, 0}},
		{Sprite: 0xF0178AAC, From: "ODUS.ESF", Zone: 22, At: [3]float32{9250, 84, 21350}, HPR: [3]float32{1.25, 0.1, 0}, Scale: 2.5},
		{Sprite: 0x5698614C, From: "ODUS.ESF", Zone: 22, At: [3]float32{8100, 90, 20100}, Color: &[4]uint8{10, 20, 30, 200}},
	}}}}
}

const (
	discExtrasSize = 6139437
	discExtrasCRC  = 0x25A4139A
)
