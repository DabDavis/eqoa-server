// Package ground is where creatures stand: the terrain of each world, baked
// from the operator's own game disc (cmd/groundbake) into files beside the
// server (worlds/ground/<world>.ground; worlds/ is never committed: the
// server ships and stores no game data), read a zone at a time as creatures
// move.
//
// CLIENT sets a creature that does not run its own walk on the floor it
// finds within 1.5 above or below the height the server sent
// (0x00CBCE40, ExtrapolateGround), and leaves it in the air when it finds
// none: a creature moved on its spawn's height floated off every hill it
// walked down (F3 20261025-165427: "when mobs path off a hill they float into
// the air in a straight line"). With the ground the server sends the floor
// itself.
//
// Only the terrain counts (the zone rooms' own collision, not the trees,
// rocks and houses placed on it): Tunaria's is 8.15 million triangles of the
// 31.3 million its collision holds, and a creature walks on it. The disc's
// world files are not read by the server itself: the esf library reads a
// world file whole to parse its index (Tunaria 997 MB, a peak of 2 GB),
// which a small server cannot spare; the bake is done once, where there is
// the memory.
//
// A file: "EQGR", a version (1), the zone count, then each zone's entry
// (its terrain's bounds, its quantisation base, and where its strips lie
// and how many), then the zones' strips: a vertex count and that many
// vertices, x and z in quarters of a unit and y in eighths, as int16 from
// the zone's base. Little-endian.
package ground

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const (
	magic   = "EQGR"
	version = 1

	// the quantisation: x and z in quarters (a zone may span 8192), y in
	// eighths (a height within 4096 of the zone's base)
	qXZ = 4
	qY  = 8

	// StepUp is how far above a creature's height a floor may be and still
	// be stood on: a walk up a slope rises no more than this a tick, and a
	// roof or a ledge higher than it is not climbed.
	StepUp float32 = 2

	// cell is the side of the grid a zone's triangles are found by.
	cell float32 = 8

	// keep is how many zones are kept loaded at once: Tunaria's take 2.25 MB
	// each on average and load in 6 ms (12 at most); a creature moves only
	// near a player, so this is a few zones a player.
	keep = 32
)

// Vertex is a point of a strip, in world units.
type Vertex struct{ X, Y, Z float32 }

// Zone is a zone's terrain as the bake writes it: its strips.
type Zone struct {
	Strips [][]Vertex
}

// entry is a zone's line in a file's table.
type entry struct {
	minX, minZ, maxX, maxZ float32
	baseX, baseY, baseZ    float32
	offset                 int64
	strips, verts          uint32
}

const entrySize = 4*7 + 8 + 4 + 4

// Write writes a world's ground file from its zones' terrain.
func Write(w io.Writer, zones []Zone) error {
	bw := bufio.NewWriter(w)
	entries := make([]entry, len(zones))
	offset := int64(4 + 4 + 4 + entrySize*len(zones))
	for i, z := range zones {
		e := entry{minX: float32(math.Inf(1)), minZ: float32(math.Inf(1)), maxX: float32(math.Inf(-1)), maxZ: float32(math.Inf(-1))}
		minY := float32(math.Inf(1))
		for _, s := range z.Strips {
			for _, v := range s {
				e.minX, e.maxX = min(e.minX, v.X), max(e.maxX, v.X)
				e.minZ, e.maxZ = min(e.minZ, v.Z), max(e.maxZ, v.Z)
				minY = min(minY, v.Y)
			}
			e.verts += uint32(len(s))
		}
		e.strips = uint32(len(z.Strips))
		if e.verts == 0 {
			e.minX, e.minZ, e.maxX, e.maxZ = 0, 0, 0, 0
		} else {
			e.baseX, e.baseY, e.baseZ = e.minX, minY, e.minZ
			if (e.maxX-e.minX)*qXZ > math.MaxInt16*2 || (e.maxZ-e.minZ)*qXZ > math.MaxInt16*2 {
				return fmt.Errorf("zone %d spans %.0f x %.0f: too wide to quantise", i, e.maxX-e.minX, e.maxZ-e.minZ)
			}
		}
		e.offset = offset
		offset += int64(len(z.Strips))*2 + int64(e.verts)*6
		entries[i] = e
	}
	put := func(v any) error { return binary.Write(bw, binary.LittleEndian, v) }
	if _, err := bw.WriteString(magic); err != nil {
		return err
	}
	if err := put([]uint32{version, uint32(len(zones))}); err != nil {
		return err
	}
	for _, e := range entries {
		if err := put([]float32{e.minX, e.minZ, e.maxX, e.maxZ, e.baseX, e.baseY, e.baseZ}); err != nil {
			return err
		}
		if err := put(e.offset); err != nil {
			return err
		}
		if err := put([]uint32{e.strips, e.verts}); err != nil {
			return err
		}
	}
	for i, z := range zones {
		e := entries[i]
		for _, s := range z.Strips {
			if len(s) > math.MaxUint16 {
				return fmt.Errorf("zone %d: a strip of %d vertices", i, len(s))
			}
			if err := put(uint16(len(s))); err != nil {
				return err
			}
			for _, v := range s {
				q := [3]int16{quant(v.X-e.baseX, qXZ, true), quant(v.Y-e.baseY, qY, false), quant(v.Z-e.baseZ, qXZ, true)}
				if err := put(q); err != nil {
					return err
				}
			}
		}
	}
	return bw.Flush()
}

// quant is v in 1/q units as an int16; a span (x, z: from the zone's base,
// never negative) is kept past 32767 by the offset of -32768.
func quant(v float32, q float32, span bool) int16 {
	n := math.Round(float64(v * q))
	if span {
		n -= 32768
	}
	return int16(max(math.MinInt16, min(math.MaxInt16, n)))
}

func unquant(n int16, q float32, span bool) float32 {
	v := float32(n)
	if span {
		v += 32768
	}
	return v / q
}

// tri is a triangle, its three corners.
type tri [3]Vertex

// zoneGrid is a loaded zone, compact: its strips' vertices once each, each
// floor triangle by its first vertex (the strip's next two finish it), and a
// grid over the zone's bounds of the triangles each cell meets, as one array
// of offsets and one of triangle numbers.
type zoneGrid struct {
	verts      []Vertex
	tris       []int32
	minX, minZ float32
	nx, nz     int32
	offsets    []int32
	cells      []int32
	used       uint64
}

func (z *zoneGrid) tri(i int32) tri {
	v := z.tris[i]
	return tri{z.verts[v], z.verts[v+1], z.verts[v+2]}
}

// world is a ground file open: its zones' table and the zones loaded.
type world struct {
	f       *os.File
	entries []entry
	loaded  map[int]*zoneGrid
}

// Ground is the worlds' ground, a file each, their zones loaded as asked.
type Ground struct {
	mu     sync.Mutex
	worlds map[uint8]*world
	clock  uint64
	// Loads counts the zones read from the files (for a test and the log).
	Loads int
}

// Open opens the ground files in dir (<world>.ground, world 0 to 255): the
// worlds whose file is there. None there is an error.
func Open(dir string) (*Ground, error) {
	g := &Ground{worlds: make(map[uint8]*world)}
	for n := 0; n < 256; n++ {
		f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%d.ground", n)))
		if err != nil {
			continue
		}
		w, err := readTable(f)
		if err != nil {
			f.Close()
			g.Close()
			return nil, fmt.Errorf("%d.ground: %w", n, err)
		}
		g.worlds[uint8(n)] = w
	}
	if len(g.worlds) == 0 {
		return nil, fmt.Errorf("no ground files in %s", dir)
	}
	return g, nil
}

// Worlds is how many worlds have their ground, and their zones.
func (g *Ground) Worlds() (worlds, zones int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, w := range g.worlds {
		worlds++
		zones += len(w.entries)
	}
	return
}

// Close closes the files.
func (g *Ground) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, w := range g.worlds {
		w.f.Close()
	}
	g.worlds = nil
}

func readTable(f *os.File) (*world, error) {
	r := bufio.NewReader(f)
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil || string(head) != magic {
		return nil, errors.New("not a ground file")
	}
	var vz [2]uint32
	if err := binary.Read(r, binary.LittleEndian, &vz); err != nil {
		return nil, err
	}
	if vz[0] != version {
		return nil, fmt.Errorf("version %d, this server reads %d", vz[0], version)
	}
	w := &world{f: f, entries: make([]entry, vz[1]), loaded: make(map[int]*zoneGrid)}
	for i := range w.entries {
		var fl [7]float32
		var off int64
		var counts [2]uint32
		if err := binary.Read(r, binary.LittleEndian, &fl); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &off); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &counts); err != nil {
			return nil, err
		}
		w.entries[i] = entry{fl[0], fl[1], fl[2], fl[3], fl[4], fl[5], fl[6], off, counts[0], counts[1]}
	}
	return w, nil
}

// load reads zone i of w and grids its floors (a triangle that faces up
// more than it faces sideways: a wall is no floor).
func (w *world) load(i int) (*zoneGrid, error) {
	e := w.entries[i]
	buf := make([]byte, int(e.strips)*2+int(e.verts)*6)
	if _, err := w.f.ReadAt(buf, e.offset); err != nil {
		return nil, err
	}
	z := &zoneGrid{verts: make([]Vertex, 0, e.verts), minX: e.minX, minZ: e.minZ}
	z.nx = int32((e.maxX-e.minX)/cell) + 1
	z.nz = int32((e.maxZ-e.minZ)/cell) + 1
	p := 0
	for s := uint32(0); s < e.strips; s++ {
		n := int(binary.LittleEndian.Uint16(buf[p:]))
		p += 2
		first := int32(len(z.verts))
		for k := 0; k < n; k++ {
			x := int16(binary.LittleEndian.Uint16(buf[p:]))
			y := int16(binary.LittleEndian.Uint16(buf[p+2:]))
			zz := int16(binary.LittleEndian.Uint16(buf[p+4:]))
			p += 6
			z.verts = append(z.verts, Vertex{e.baseX + unquant(x, qXZ, true), e.baseY + unquant(y, qY, false), e.baseZ + unquant(zz, qXZ, true)})
		}
		for k := int32(0); k+2 < int32(n); k++ {
			if floor(tri{z.verts[first+k], z.verts[first+k+1], z.verts[first+k+2]}) {
				z.tris = append(z.tris, first+k)
			}
		}
	}
	// the grid: each triangle counted in every cell its bounds meet, then
	// placed (two passes, one array)
	z.offsets = make([]int32, int(z.nx)*int(z.nz)+1)
	span := func(t tri) (x0, x1, z0, z1 int32) {
		return z.cellX(min(t[0].X, t[1].X, t[2].X)), z.cellX(max(t[0].X, t[1].X, t[2].X)),
			z.cellZ(min(t[0].Z, t[1].Z, t[2].Z)), z.cellZ(max(t[0].Z, t[1].Z, t[2].Z))
	}
	for i := range z.tris {
		x0, x1, z0, z1 := span(z.tri(int32(i)))
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
	for i := range z.tris {
		x0, x1, z0, z1 := span(z.tri(int32(i)))
		for cx := x0; cx <= x1; cx++ {
			for cz := z0; cz <= z1; cz++ {
				c := cx*z.nz + cz
				z.cells[next[c]] = int32(i)
				next[c]++
			}
		}
	}
	return z, nil
}

// cellX and cellZ are a coordinate's cell in the zone's grid, kept within
// it (a quantised corner may stand a hair past the bounds).
func (z *zoneGrid) cellX(x float32) int32 { return max(0, min(z.nx-1, int32((x-z.minX)/cell))) }
func (z *zoneGrid) cellZ(v float32) int32 { return max(0, min(z.nz-1, int32((v-z.minZ)/cell))) }

// floor reports a triangle that faces up more than sideways (steeper than
// 60 degrees is a wall), and is not degenerate.
func floor(t tri) bool {
	ux, uy, uz := t[1].X-t[0].X, t[1].Y-t[0].Y, t[1].Z-t[0].Z
	vx, vy, vz := t[2].X-t[0].X, t[2].Y-t[0].Y, t[2].Z-t[0].Z
	nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
	l2 := nx*nx + ny*ny + nz*nz
	return l2 > 1e-8 && ny*ny > 0.25*l2
}

// heightAt is the height of t at (x, z) when (x, z) is within it seen from
// above.
func heightAt(t tri, x, z float32) (float32, bool) {
	ax, az, bx, bz, cx, cz := t[0].X, t[0].Z, t[1].X, t[1].Z, t[2].X, t[2].Z
	d := (bz-cz)*(ax-cx) + (cx-bx)*(az-cz)
	if d == 0 {
		return 0, false
	}
	w0 := ((bz-cz)*(x-cx) + (cx-bx)*(z-cz)) / d
	w1 := ((cz-az)*(x-cx) + (ax-cx)*(z-cz)) / d
	w2 := 1 - w0 - w1
	const eps = -1e-4
	if w0 < eps || w1 < eps || w2 < eps {
		return 0, false
	}
	return w0*t[0].Y + w1*t[1].Y + w2*t[2].Y, true
}

// Floor is the floor a creature at (x, y, z) in world stands on: the
// highest terrain at (x, z) no more than StepUp above y; false where the
// world has no ground file or no terrain there (the caller keeps its own
// height).
func (g *Ground) Floor(worldN uint8, x, y, z float32) (float32, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.worlds[worldN]
	if w == nil {
		return y, false
	}
	best, found := float32(0), false
	for i, e := range w.entries {
		if e.verts == 0 || x < e.minX || x > e.maxX || z < e.minZ || z > e.maxZ {
			continue
		}
		zg, err := g.zoneLocked(w, i)
		if err != nil {
			continue
		}
		c := zg.cellX(x)*zg.nz + zg.cellZ(z)
		for _, ti := range zg.cells[zg.offsets[c]:zg.offsets[c+1]] {
			if h, ok := heightAt(zg.tri(ti), x, z); ok && h <= y+StepUp && (!found || h > best) {
				best, found = h, true
			}
		}
	}
	if !found {
		return y, false
	}
	return best, true
}

// zoneLocked is zone i of w, loaded now if it is not, the least lately used
// let go past keep. Must be called with g.mu held.
func (g *Ground) zoneLocked(w *world, i int) (*zoneGrid, error) {
	g.clock++
	if z := w.loaded[i]; z != nil {
		z.used = g.clock
		return z, nil
	}
	z, err := w.load(i)
	if err != nil {
		return nil, err
	}
	g.Loads++
	z.used = g.clock
	w.loaded[i] = z
	total := 0
	for _, ww := range g.worlds {
		total += len(ww.loaded)
	}
	for total > keep {
		var oldW *world
		oldI, oldUsed := -1, uint64(math.MaxUint64)
		for _, ww := range g.worlds {
			for j, zz := range ww.loaded {
				if zz.used < oldUsed {
					oldW, oldI, oldUsed = ww, j, zz.used
				}
			}
		}
		delete(oldW.loaded, oldI)
		total--
	}
	return z, nil
}
