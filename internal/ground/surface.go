package ground

import "math"

// Where a body can stand: every surface of the game's collision at a place,
// the terrain's and what is placed on it, for the server's view of a
// player's own moves (entity/movecheck.go). CLIENT's walk finds its floor by
// sweeping a ball of 0.001 down from 1.2 over the feet to 0.6 under them
// (VIWalkControl::CalcFit, m_parm3C, m_parm38, m_parm30) through
// VIScene::Collide, which takes a face only from its front: so a body on a
// floor stands on a face that looks up, at its height to a thousandth, on
// the terrain (any slope: the sweep does not ask) or on anything placed
// (a house's floor, a bridge, a stair, a keep's wall walk). Floor (ground.go)
// is the terrain's floors alone, for creatures; this is all of it.

// Surface is a surface at a place: its height there, whether its front
// looks up (a floor; one looking down is a ceiling, met from below), and
// whether it is placed (a walls file's) or the terrain's.
type Surface struct {
	Y      float32
	Up     bool
	Placed bool
}

// Surfaces is every surface of world's collision over (x, z) between the
// heights lo and hi (lo below hi), in no order; nil where the world has
// neither file or nothing is there.
func (g *Ground) Surfaces(worldN uint8, x, z, lo, hi float32) []Surface {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []Surface
	in := func(h float32) bool { return h >= lo && h <= hi }
	if w := g.worlds[worldN]; w != nil {
		for i, e := range w.entries {
			if e.verts == 0 || x < e.minX || x > e.maxX || z < e.minZ || z > e.maxZ {
				continue
			}
			zg, err := g.zoneLocked(w, i)
			if err != nil {
				continue
			}
			f := &zg.floors
			c := zg.cellX(f, x)*f.nz + zg.cellZ(f, z)
			for _, ti := range f.cells[f.offsets[c]:f.offsets[c+1]] {
				t := zg.tri(ti)
				if h, ok := heightAt(t, x, z); ok && in(h) {
					out = append(out, Surface{Y: h, Up: facesUp(t)})
				}
			}
			wg := &zg.wallGrid
			c = zg.cellX(wg, x)*wg.nz + zg.cellZ(wg, z)
			for _, ti := range wg.cells[wg.offsets[c]:wg.offsets[c+1]] {
				t := zg.wall(ti)
				if h, ok := heightAt(t, x, z); ok && in(h) {
					out = append(out, Surface{Y: h, Up: facesUp(t)})
				}
			}
		}
	}
	if w := g.walls[worldN]; w != nil {
		a, b := Vertex{x, hi, z}, Vertex{x, lo, z}
		for i, e := range w.zones {
			if e.count == 0 || !overlaps(b, a, e.lo, e.hi) {
				continue
			}
			zz, err := g.wallZoneLocked(w, i)
			if err != nil {
				continue
			}
			cx, _, cz, _ := zz.span(b, a) // one cell: the segment is upright
			c := cx*zz.nz + cz
			for _, k := range zz.cells[zz.offsets[c]:zz.offsets[c+1]] {
				p := &zz.places[k]
				if !overlaps(b, a, p.lo, p.hi) {
					continue
				}
				buf, err := w.buffer(p.buf)
				if err != nil {
					continue
				}
				out = p.surfaces(buf, a, b, out)
			}
		}
	}
	return out
}

// facesUp reports a triangle's front looking up, the front as the game's
// collision takes it (crosses: the plane facing (c-b) x (b-a)); the
// terrain's triangles come turned as the game turns them (zoneGrid.tri and
// zoneGrid.wall).
func facesUp(t tri) bool {
	ux, uz := t[1].X-t[0].X, t[1].Z-t[0].Z
	vx, vz := t[2].X-t[0].X, t[2].Z-t[0].Z
	return uz*vx-ux*vz < 0 // (b-a) x (c-a) looks down: its negation up
}

// surfaces appends where the world's upright segment from a (top) to b
// (bottom) meets a triangle of p's buffer, from either side, its height and
// whether its front looks up.
func (p *placed) surfaces(buf *wallBuffer, a, b Vertex, out []Surface) []Surface {
	la, lb := apply(&p.inv, a), apply(&p.inv, b)
	lo, hi := lowest(la, lb), highest(la, lb)
	for _, s := range buf.strips {
		if !overlaps(lo, hi, s.lo, s.hi) {
			continue
		}
		v := buf.verts[s.first : s.first+s.n]
		for k := 0; k+2 < len(v); k++ {
			t0, t1, t2 := stripTri(v, k)
			if t, front, ok := segTri(la, lb, t0, t1, t2); ok {
				out = append(out, Surface{Y: a.Y + float32(t)*(b.Y-a.Y), Up: front, Placed: true})
			}
		}
	}
	return out
}

// segTri is where the segment from p to q meets the triangle (a, b, c), as
// a fraction of the way, and whether it comes from the triangle's front
// (crosses' facing); false when it misses or runs in the plane.
func segTri(p, q, a, b, c Vertex) (t float64, front, ok bool) {
	dx, dy, dz := float64(q.X-p.X), float64(q.Y-p.Y), float64(q.Z-p.Z)
	e1x, e1y, e1z := float64(b.X-a.X), float64(b.Y-a.Y), float64(b.Z-a.Z)
	e2x, e2y, e2z := float64(c.X-a.X), float64(c.Y-a.Y), float64(c.Z-a.Z)
	hx, hy, hz := dy*e2z-dz*e2y, dz*e2x-dx*e2z, dx*e2y-dy*e2x
	det := e1x*hx + e1y*hy + e1z*hz
	if math.Abs(det) < 1e-12 {
		return 0, false, false
	}
	inv := 1 / det
	sx, sy, sz := float64(p.X-a.X), float64(p.Y-a.Y), float64(p.Z-a.Z)
	u := (sx*hx + sy*hy + sz*hz) * inv
	if u < 0 || u > 1 {
		return 0, false, false
	}
	qx, qy, qz := sy*e1z-sz*e1y, sz*e1x-sx*e1z, sx*e1y-sy*e1x
	v := (dx*qx + dy*qy + dz*qz) * inv
	if v < 0 || u+v > 1 {
		return 0, false, false
	}
	t = (e2x*qx + e2y*qy + e2z*qz) * inv
	if t < 0 || t > 1 {
		return 0, false, false
	}
	return t, det < 0, true
}

// Crossing is what a way crossed from its front (Crosses).
type Crossing int

const (
	CrossNone         Crossing = iota
	CrossTerrainWall           // the terrain's steep faces: a cliff, a cave's side
	CrossTerrainFloor          // the terrain's floors (or a cave's ceiling): down through the ground
	CrossPlaced                // anything placed: a house, a wall, a fence
)

func (c Crossing) String() string {
	switch c {
	case CrossTerrainWall:
		return "terrain wall"
	case CrossTerrainFloor:
		return "terrain floor"
	case CrossPlaced:
		return "placed"
	}
	return "none"
}

// Crosses is what the way from (x0, y0, z0) to (x1, y1, z1) in world passes
// through from its front, as the game's collision stops a body: the
// terrain's walls, its floors (down through the ground, or up through a
// cave's ceiling) and anything placed on it; CrossNone where it passes
// through nothing, or the world has neither file.
func (g *Ground) Crosses(worldN uint8, x0, y0, z0, x1, y1, z1 float32) Crossing {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, b := Vertex{x0, y0, z0}, Vertex{x1, y1, z1}
	if g.terrainWallLocked(worldN, a, b) {
		return CrossTerrainWall
	}
	if g.terrainFloorLocked(worldN, a, b) {
		return CrossTerrainFloor
	}
	if g.placedLocked(worldN, a, b) {
		return CrossPlaced
	}
	return CrossNone
}

// terrainFloorLocked is whether the way from a to b crosses one of the
// terrain's floors (or ceilings) from its front. Must be called with g.mu
// held.
func (g *Ground) terrainFloorLocked(worldN uint8, a, b Vertex) bool {
	lo, hi := lowest(a, b), highest(a, b)
	w := g.worlds[worldN]
	if w == nil {
		return false
	}
	for i, e := range w.entries {
		if e.verts == 0 || hi.X < e.minX || lo.X > e.maxX || hi.Z < e.minZ || lo.Z > e.maxZ {
			continue
		}
		zg, err := g.zoneLocked(w, i)
		if err != nil {
			continue
		}
		f := &zg.floors
		for cx := zg.cellX(f, lo.X); cx <= zg.cellX(f, hi.X); cx++ {
			for cz := zg.cellZ(f, lo.Z); cz <= zg.cellZ(f, hi.Z); cz++ {
				c := cx*f.nz + cz
				for _, ti := range f.cells[f.offsets[c]:f.offsets[c+1]] {
					t := zg.tri(ti)
					if crosses(a, b, t[0], t[1], t[2]) {
						return true
					}
				}
			}
		}
	}
	return false
}

// Support is the highest floor (a surface whose front looks up: the terrain
// at any slope, or anything placed) over (x, z) from down below the height y
// to up above it; false where there is none, or the world has neither file.
func (g *Ground) Support(worldN uint8, x, y, z, up, down float32) (float32, bool) {
	best, found := float32(0), false
	for _, s := range g.Surfaces(worldN, x, z, y-down, y+up) {
		if s.Up && (!found || s.Y > best) {
			best, found = s.Y, true
		}
	}
	return best, found
}

// PlacedSupport is Support of what is placed on the terrain alone (a
// bridge's deck, a walkway, a house's floor): the highest such floor over
// (x, z) from down below y to up above it; false where there is none.
func (g *Ground) PlacedSupport(worldN uint8, x, y, z, up, down float32) (float32, bool) {
	best, found := float32(0), false
	for _, s := range g.Surfaces(worldN, x, z, y-down, y+up) {
		if s.Up && s.Placed && (!found || s.Y > best) {
			best, found = s.Y, true
		}
	}
	return best, found
}

// Blocked reports what the way from (x0, y0, z0) to (x1, y1, z1) in world
// passes through from its front (Crosses), named; false for nothing.
func (g *Ground) Blocked(worldN uint8, x0, y0, z0, x1, y1, z1 float32) (string, bool) {
	c := g.Crosses(worldN, x0, y0, z0, x1, y1, z1)
	return c.String(), c != CrossNone
}

// Knows reports whether world has a ground file: where it has none, a place
// with no floor under it says nothing.
func (g *Ground) Knows(worldN uint8) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.worlds[worldN] != nil
}
