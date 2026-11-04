package ground

// zoneIndex is a grid over a world's zones, each cell the zones whose bounds
// meet it, so a query asks the few zones near it rather than walking every
// zone of the world (Tunaria has 175 of each kind, and a creature's wander leg
// asks the ground some fifty times: the walk over every zone was most of a
// leg's check, claude-notes/server-tick-profile-20261103.md). It changes
// nothing a query finds: a zone it gives is tested as before, and every zone
// whose bounds meet a query's box is among those it gives (each query takes
// the highest of what it finds, or whether it found any, so their order does
// not matter).
type zoneIndex struct {
	minX, minZ float32
	nx, nz     int
	cells      [][]int32
}

// zoneIndexCell is a cell's side: a zone is about 2,000 a side, so a zone is
// in some 64 cells, and a query (a point, or a way a few units long) asks the
// one, two or four zones at its cell.
const zoneIndexCell float32 = 256

// newZoneIndex is the index of n zones, each zone's bounds in x and z by
// bounds (false: a zone with nothing in it, left out).
func newZoneIndex(n int, bounds func(i int) (minX, minZ, maxX, maxZ float32, ok bool)) *zoneIndex {
	ix := &zoneIndex{}
	any := false
	var maxX, maxZ float32
	for i := 0; i < n; i++ {
		x0, z0, x1, z1, ok := bounds(i)
		if !ok {
			continue
		}
		if !any {
			ix.minX, ix.minZ, maxX, maxZ, any = x0, z0, x1, z1, true
			continue
		}
		ix.minX, ix.minZ = min(ix.minX, x0), min(ix.minZ, z0)
		maxX, maxZ = max(maxX, x1), max(maxZ, z1)
	}
	if !any {
		return ix
	}
	ix.nx = int((maxX-ix.minX)/zoneIndexCell) + 1
	ix.nz = int((maxZ-ix.minZ)/zoneIndexCell) + 1
	ix.cells = make([][]int32, ix.nx*ix.nz)
	for i := 0; i < n; i++ {
		x0, z0, x1, z1, ok := bounds(i)
		if !ok {
			continue
		}
		for cx := ix.cellX(x0); cx <= ix.cellX(x1); cx++ {
			for cz := ix.cellZ(z0); cz <= ix.cellZ(z1); cz++ {
				c := cx*ix.nz + cz
				ix.cells[c] = append(ix.cells[c], int32(i))
			}
		}
	}
	return ix
}

// cellX and cellZ are the cell of a coordinate, held to the grid (a place
// off it is near the cells at its edge, whose zones' own bounds then say no).
func (ix *zoneIndex) cellX(x float32) int {
	return min(max(int((x-ix.minX)/zoneIndexCell), 0), ix.nx-1)
}

func (ix *zoneIndex) cellZ(z float32) int {
	return min(max(int((z-ix.minZ)/zoneIndexCell), 0), ix.nz-1)
}

// near is every zone whose bounds may meet the box (x0..x1, z0..z1), each
// once: the cell's own list for a box in one cell (not to be changed), else a
// new one.
func (ix *zoneIndex) near(x0, z0, x1, z1 float32) []int32 {
	if ix.nx == 0 {
		return nil
	}
	cx0, cx1, cz0, cz1 := ix.cellX(x0), ix.cellX(x1), ix.cellZ(z0), ix.cellZ(z1)
	if cx0 == cx1 && cz0 == cz1 {
		return ix.cells[cx0*ix.nz+cz0]
	}
	var out []int32
	for cx := cx0; cx <= cx1; cx++ {
		for cz := cz0; cz <= cz1; cz++ {
		next:
			for _, i := range ix.cells[cx*ix.nz+cz] {
				for _, o := range out {
					if o == i {
						continue next
					}
				}
				out = append(out, i)
			}
		}
	}
	return out
}

// terrainIndexLocked is the index of w's terrain zones, made at its first
// query. Must be called with g.mu held.
func (w *world) terrainIndexLocked() *zoneIndex {
	if w.index == nil {
		w.index = newZoneIndex(len(w.entries), func(i int) (float32, float32, float32, float32, bool) {
			e := w.entries[i]
			return e.minX, e.minZ, e.maxX, e.maxZ, e.verts != 0
		})
	}
	return w.index
}

// wallsIndexLocked is the index of w's walls zones, made at its first query.
// Must be called with g.mu held.
func (w *wallWorld) wallsIndexLocked() *zoneIndex {
	if w.index == nil {
		w.index = newZoneIndex(len(w.zones), func(i int) (float32, float32, float32, float32, bool) {
			e := w.zones[i]
			return e.lo.X, e.lo.Z, e.hi.X, e.hi.Z, e.count != 0
		})
	}
	return w.index
}
