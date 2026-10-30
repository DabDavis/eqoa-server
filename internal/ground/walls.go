package ground

// What stands on the ground: the collision of everything a world's zones
// place on their terrain (the actors: a house, a city's wall, a fence, a
// tree's trunk, a table), which a creature's wander does not walk through
// (entity/movement.go). F3 pass 24, the user: "they are walking outside the
// walls of the tavern basement, also city cat does the same and walks through
// walls". Neriak's tavern is a house placed on the floor of the city's
// cavern: its walls are an actor's collision, and the ground (ground.go)
// holds the terrain alone, so a rat that spawned inside wandered the street
// under the house and out through its walls.
//
// The game keeps each collision buffer once and places it by a transform
// (VIScene's actors: vanilla's crates/vi/src/world.rs), and so does the file:
// Tunaria's 220,862 placements of 1,396 buffers are 16 MB, where the placed
// triangles would be 23.1 million, three times the terrain's. The bake
// (cmd/groundbake) writes <world>.walls beside <world>.ground; the server
// reads a buffer when a placement of it is first asked about, and keeps it,
// and a zone's placements a zone at a time, as it reads the ground.
//
// A file: "EQWL", a version (1), the buffer and zone counts; each buffer's
// entry (where its strips lie, how many strips and vertices, its bounds in
// its own space); each zone's entry (its placements' bounds in the world,
// where they lie, how many); then the buffers' strips (a vertex count and
// that many vertices, x, y and z as float32, in the buffer's own space); then
// the zones' placements (a buffer's index and the 3x4 transform, by rows,
// that takes the buffer's space to the world's). Little-endian.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	wallsMagic   = "EQWL"
	wallsVersion = 1

	// placeCell is the side of the grid a zone's placements are found by.
	placeCell float32 = 16

	bufEntrySize  = 8 + 4 + 4 + 6*4
	wallEntrySize = 6*4 + 8 + 4
	placementSize = 4 + 12*4
)

// Buffer is a collision buffer in its own space: its strips.
type Buffer struct {
	Strips [][]Vertex
}

// Placement is a buffer placed in the world: a vertex (x, y, z) of it is at
// M times (x, y, z, 1), M a 3x4 matrix by rows.
type Placement struct {
	Buffer uint32
	M      [12]float32
}

// WallZone is a zone's placements.
type WallZone struct {
	Placements []Placement
}

// bufEntry is a buffer's line in a walls file's table.
type bufEntry struct {
	offset        int64
	strips, verts uint32
	lo, hi        Vertex
}

// wallEntry is a zone's line in a walls file's table.
type wallEntry struct {
	lo, hi Vertex
	offset int64
	count  uint32
}

func lowest(a, b Vertex) Vertex  { return Vertex{min(a.X, b.X), min(a.Y, b.Y), min(a.Z, b.Z)} }
func highest(a, b Vertex) Vertex { return Vertex{max(a.X, b.X), max(a.Y, b.Y), max(a.Z, b.Z)} }

var (
	noLow  = Vertex{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(1))}
	noHigh = Vertex{float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(-1))}
)

// apply is m times (v, 1).
func apply(m *[12]float32, v Vertex) Vertex {
	return Vertex{
		m[0]*v.X + m[1]*v.Y + m[2]*v.Z + m[3],
		m[4]*v.X + m[5]*v.Y + m[6]*v.Z + m[7],
		m[8]*v.X + m[9]*v.Y + m[10]*v.Z + m[11],
	}
}

// placedBounds is the world box round a buffer's box (lo, hi) placed by m.
func placedBounds(m *[12]float32, lo, hi Vertex) (Vertex, Vertex) {
	plo, phi := noLow, noHigh
	for c := 0; c < 8; c++ {
		v := lo
		if c&1 != 0 {
			v.X = hi.X
		}
		if c&2 != 0 {
			v.Y = hi.Y
		}
		if c&4 != 0 {
			v.Z = hi.Z
		}
		p := apply(m, v)
		plo, phi = lowest(plo, p), highest(phi, p)
	}
	return plo, phi
}

// inverse is the transform that undoes m, false when m flattens space (a
// placement of no size: it stands in no one's way).
func inverse(m *[12]float32) ([12]float32, bool) {
	a, b, c := float64(m[0]), float64(m[1]), float64(m[2])
	d, e, f := float64(m[4]), float64(m[5]), float64(m[6])
	g, h, i := float64(m[8]), float64(m[9]), float64(m[10])
	A, B, C := e*i-f*h, f*g-d*i, d*h-e*g
	det := a*A + b*B + c*C
	if math.Abs(det) < 1e-12 {
		return [12]float32{}, false
	}
	k := 1 / det
	r := [9]float64{
		A * k, (c*h - b*i) * k, (b*f - c*e) * k,
		B * k, (a*i - c*g) * k, (c*d - a*f) * k,
		C * k, (b*g - a*h) * k, (a*e - b*d) * k,
	}
	tx, ty, tz := float64(m[3]), float64(m[7]), float64(m[11])
	return [12]float32{
		float32(r[0]), float32(r[1]), float32(r[2]), float32(-(r[0]*tx + r[1]*ty + r[2]*tz)),
		float32(r[3]), float32(r[4]), float32(r[5]), float32(-(r[3]*tx + r[4]*ty + r[5]*tz)),
		float32(r[6]), float32(r[7]), float32(r[8]), float32(-(r[6]*tx + r[7]*ty + r[8]*tz)),
	}, true
}

// WriteWalls writes a world's walls file: its buffers, and each zone's
// placements of them.
func WriteWalls(w io.Writer, bufs []Buffer, zones []WallZone) error {
	bw := bufio.NewWriter(w)
	put := func(v any) error { return binary.Write(bw, binary.LittleEndian, v) }
	bes := make([]bufEntry, len(bufs))
	offset := int64(4 + 4 + 4 + 4 + bufEntrySize*len(bufs) + wallEntrySize*len(zones))
	for i, b := range bufs {
		e := bufEntry{lo: noLow, hi: noHigh, offset: offset, strips: uint32(len(b.Strips))}
		for _, s := range b.Strips {
			if len(s) > math.MaxUint16 {
				return fmt.Errorf("buffer %d: a strip of %d vertices", i, len(s))
			}
			for _, v := range s {
				e.lo, e.hi = lowest(e.lo, v), highest(e.hi, v)
			}
			e.verts += uint32(len(s))
		}
		if e.verts == 0 {
			e.lo, e.hi = Vertex{}, Vertex{}
		}
		offset += int64(e.strips)*2 + int64(e.verts)*12
		bes[i] = e
	}
	wes := make([]wallEntry, len(zones))
	for i, z := range zones {
		e := wallEntry{lo: noLow, hi: noHigh, offset: offset, count: uint32(len(z.Placements))}
		for _, p := range z.Placements {
			if int(p.Buffer) >= len(bufs) {
				return fmt.Errorf("zone %d: a placement of buffer %d of %d", i, p.Buffer, len(bufs))
			}
			lo, hi := placedBounds(&p.M, bes[p.Buffer].lo, bes[p.Buffer].hi)
			e.lo, e.hi = lowest(e.lo, lo), highest(e.hi, hi)
		}
		if e.count == 0 {
			e.lo, e.hi = Vertex{}, Vertex{}
		}
		offset += int64(e.count) * placementSize
		wes[i] = e
	}
	if _, err := bw.WriteString(wallsMagic); err != nil {
		return err
	}
	if err := put([]uint32{wallsVersion, uint32(len(bufs)), uint32(len(zones))}); err != nil {
		return err
	}
	for _, e := range bes {
		if err := put(e.offset); err != nil {
			return err
		}
		if err := put([]uint32{e.strips, e.verts}); err != nil {
			return err
		}
		if err := put([]float32{e.lo.X, e.lo.Y, e.lo.Z, e.hi.X, e.hi.Y, e.hi.Z}); err != nil {
			return err
		}
	}
	for _, e := range wes {
		if err := put([]float32{e.lo.X, e.lo.Y, e.lo.Z, e.hi.X, e.hi.Y, e.hi.Z}); err != nil {
			return err
		}
		if err := put(e.offset); err != nil {
			return err
		}
		if err := put(e.count); err != nil {
			return err
		}
	}
	for _, b := range bufs {
		for _, s := range b.Strips {
			if err := put(uint16(len(s))); err != nil {
				return err
			}
			if err := put(s); err != nil {
				return err
			}
		}
	}
	for _, z := range zones {
		for _, p := range z.Placements {
			if err := put(p.Buffer); err != nil {
				return err
			}
			if err := put(p.M); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

// wallWorld is a walls file open: its tables, the buffers read and the
// zones loaded.
type wallWorld struct {
	f      *os.File
	bufs   []bufEntry
	zones  []wallEntry
	read   []*wallBuffer
	loaded map[int]*wallZone
}

// wallBuffer is a buffer read: its vertices and its strips, each with its
// bounds (a placed house's walls are found by the strips they lie in).
type wallBuffer struct {
	verts  []Vertex
	strips []wallStrip
}

type wallStrip struct {
	first, n int32
	lo, hi   Vertex
}

// wallZone is a zone's placements loaded, each with the transform back to
// its buffer's space and its bounds in the world, and a grid over the zone of
// the placements each cell meets.
type wallZone struct {
	places     []placed
	minX, minZ float32
	nx, nz     int32
	offsets    []int32
	cells      []int32
	used       uint64
}

type placed struct {
	buf    int32
	inv    [12]float32
	lo, hi Vertex
}

func readWalls(f *os.File) (*wallWorld, error) {
	r := bufio.NewReader(f)
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil || string(head) != wallsMagic {
		return nil, errors.New("not a walls file")
	}
	var h [3]uint32
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil {
		return nil, err
	}
	if h[0] != wallsVersion {
		return nil, fmt.Errorf("version %d, this server reads %d", h[0], wallsVersion)
	}
	if h[1] > 1<<24 || h[2] > 1<<16 {
		return nil, fmt.Errorf("%d buffers and %d zones", h[1], h[2])
	}
	w := &wallWorld{f: f, bufs: make([]bufEntry, h[1]), zones: make([]wallEntry, h[2]), read: make([]*wallBuffer, h[1]), loaded: make(map[int]*wallZone)}
	for i := range w.bufs {
		var off int64
		var counts [2]uint32
		var b [6]float32
		if err := binary.Read(r, binary.LittleEndian, &off); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &counts); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &b); err != nil {
			return nil, err
		}
		w.bufs[i] = bufEntry{off, counts[0], counts[1], Vertex{b[0], b[1], b[2]}, Vertex{b[3], b[4], b[5]}}
	}
	for i := range w.zones {
		var b [6]float32
		var off int64
		var count uint32
		if err := binary.Read(r, binary.LittleEndian, &b); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &off); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
			return nil, err
		}
		w.zones[i] = wallEntry{Vertex{b[0], b[1], b[2]}, Vertex{b[3], b[4], b[5]}, off, count}
	}
	return w, nil
}

// buffer is buffer i of w, read now if it is not.
func (w *wallWorld) buffer(i int32) (*wallBuffer, error) {
	if b := w.read[i]; b != nil {
		return b, nil
	}
	e := w.bufs[i]
	buf := make([]byte, int(e.strips)*2+int(e.verts)*12)
	if _, err := w.f.ReadAt(buf, e.offset); err != nil {
		return nil, err
	}
	b := &wallBuffer{verts: make([]Vertex, 0, e.verts), strips: make([]wallStrip, 0, e.strips)}
	p := 0
	for s := uint32(0); s < e.strips; s++ {
		n := int(binary.LittleEndian.Uint16(buf[p:]))
		p += 2
		st := wallStrip{first: int32(len(b.verts)), n: int32(n), lo: noLow, hi: noHigh}
		for k := 0; k < n; k++ {
			v := Vertex{
				math.Float32frombits(binary.LittleEndian.Uint32(buf[p:])),
				math.Float32frombits(binary.LittleEndian.Uint32(buf[p+4:])),
				math.Float32frombits(binary.LittleEndian.Uint32(buf[p+8:])),
			}
			p += 12
			b.verts = append(b.verts, v)
			st.lo, st.hi = lowest(st.lo, v), highest(st.hi, v)
		}
		if n >= 3 {
			b.strips = append(b.strips, st)
		}
	}
	w.read[i] = b
	return b, nil
}

// load reads zone i's placements of w and grids them.
func (w *wallWorld) load(i int) (*wallZone, error) {
	e := w.zones[i]
	buf := make([]byte, int(e.count)*placementSize)
	if _, err := w.f.ReadAt(buf, e.offset); err != nil {
		return nil, err
	}
	z := &wallZone{minX: e.lo.X, minZ: e.lo.Z, places: make([]placed, 0, e.count)}
	z.nx = int32((e.hi.X-e.lo.X)/placeCell) + 1
	z.nz = int32((e.hi.Z-e.lo.Z)/placeCell) + 1
	for k := 0; k < int(e.count); k++ {
		q := buf[k*placementSize:]
		bi := binary.LittleEndian.Uint32(q)
		if int(bi) >= len(w.bufs) {
			return nil, fmt.Errorf("a placement of buffer %d of %d", bi, len(w.bufs))
		}
		var m [12]float32
		for j := range m {
			m[j] = math.Float32frombits(binary.LittleEndian.Uint32(q[4+4*j:]))
		}
		inv, ok := inverse(&m)
		if !ok || w.bufs[bi].verts == 0 {
			continue
		}
		lo, hi := placedBounds(&m, w.bufs[bi].lo, w.bufs[bi].hi)
		z.places = append(z.places, placed{buf: int32(bi), inv: inv, lo: lo, hi: hi})
	}
	z.offsets = make([]int32, int(z.nx)*int(z.nz)+1)
	for _, p := range z.places {
		x0, x1, z0, z1 := z.span(p.lo, p.hi)
		for cx := x0; cx <= x1; cx++ {
			for cz := z0; cz <= z1; cz++ {
				z.offsets[cx*z.nz+cz+1]++
			}
		}
	}
	for c := 1; c < len(z.offsets); c++ {
		z.offsets[c] += z.offsets[c-1]
	}
	z.cells = make([]int32, z.offsets[len(z.offsets)-1])
	next := append([]int32(nil), z.offsets[:len(z.offsets)-1]...)
	for k, p := range z.places {
		x0, x1, z0, z1 := z.span(p.lo, p.hi)
		for cx := x0; cx <= x1; cx++ {
			for cz := z0; cz <= z1; cz++ {
				c := cx*z.nz + cz
				z.cells[next[c]] = int32(k)
				next[c]++
			}
		}
	}
	return z, nil
}

// span is the cells a box meets, kept within the grid.
func (z *wallZone) span(lo, hi Vertex) (x0, x1, z0, z1 int32) {
	cell := func(v, base float32, n int32) int32 { return max(0, min(n-1, int32((v-base)/placeCell))) }
	return cell(lo.X, z.minX, z.nx), cell(hi.X, z.minX, z.nx), cell(lo.Z, z.minZ, z.nz), cell(hi.Z, z.minZ, z.nz)
}

// overlaps reports two boxes meeting.
func overlaps(alo, ahi, blo, bhi Vertex) bool {
	return alo.X <= bhi.X && ahi.X >= blo.X && alo.Y <= bhi.Y && ahi.Y >= blo.Y && alo.Z <= bhi.Z && ahi.Z >= blo.Z
}

// crosses reports the segment from p to q passing through the triangle
// (a, b, c) from its face, its ends included: from in front of the
// triangle's plane to behind it, the way the game's collision stops a body
// (VICollide's swept sphere against a triangle takes one that starts in
// front of the plane and moves toward its back, the plane facing
// (c-b) x (b-a), VIPlane's of three points: vanilla's collide_tri). One
// crossing from behind passes, as the console's walk does (a house's wall
// faces out of the wall, into the room and into the street), and one in the
// triangle's own plane runs along it.
func crosses(p, q, a, b, c Vertex) bool {
	dx, dy, dz := float64(q.X-p.X), float64(q.Y-p.Y), float64(q.Z-p.Z)
	e1x, e1y, e1z := float64(b.X-a.X), float64(b.Y-a.Y), float64(b.Z-a.Z)
	e2x, e2y, e2z := float64(c.X-a.X), float64(c.Y-a.Y), float64(c.Z-a.Z)
	hx, hy, hz := dy*e2z-dz*e2y, dz*e2x-dx*e2z, dx*e2y-dy*e2x
	det := e1x*hx + e1y*hy + e1z*hz // the way's dot the plane's facing
	if det > -1e-12 {
		return false // from behind, or along it
	}
	inv := 1 / det
	sx, sy, sz := float64(p.X-a.X), float64(p.Y-a.Y), float64(p.Z-a.Z)
	u := (sx*hx + sy*hy + sz*hz) * inv
	if u < 0 || u > 1 {
		return false
	}
	qx, qy, qz := sy*e1z-sz*e1y, sz*e1x-sx*e1z, sx*e1y-sy*e1x
	v := (dx*qx + dy*qy + dz*qz) * inv
	if v < 0 || u+v > 1 {
		return false
	}
	t := (e2x*qx + e2y*qy + e2z*qz) * inv
	return t >= 0 && t <= 1
}

// hit reports the world segment from a to b passing through a triangle of
// p's buffer b, in the buffer's own space.
func (p *placed) hit(b *wallBuffer, a, c Vertex) bool {
	la, lc := apply(&p.inv, a), apply(&p.inv, c)
	lo, hi := lowest(la, lc), highest(la, lc)
	for _, s := range b.strips {
		if !overlaps(lo, hi, s.lo, s.hi) {
			continue
		}
		v := b.verts[s.first : s.first+s.n]
		for k := 0; k+2 < len(v); k++ {
			a, b, c := stripTri(v, k)
			if crosses(la, lc, a, b, c) {
				return true
			}
		}
	}
	return false
}

// stripTri is triangle k of a strip as the game's collision walks it, every
// other one turned so they all face one way (VICollide::Collide's flip).
func stripTri(v []Vertex, k int) (Vertex, Vertex, Vertex) {
	if k%2 == 0 {
		return v[k], v[k+1], v[k+2]
	}
	return v[k+1], v[k], v[k+2]
}
