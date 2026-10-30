package ground

// What stands over a place: whether a body there is under a roof. A vampire
// takes the sun's damage only under the open sky (entity/vampire.go; the user,
// 2026-10-30: "shade protecting vampire from dot would be good"), and the open
// sky is a place with nothing over it in the collision the game keeps: no
// terrain (a cave's roof, an overhang, the rock over Neriak's cavern) and
// nothing placed on the terrain (a house's ceiling and roof, a floor above, a
// bridge). Plants have no collision on the console (the bake leaves the
// FloraSprites out) and a tree's collision is its trunk, so a tree's leaves
// are no roof.
//
// The game has a measure of its own, which this is not: the baked vertex
// colours' alpha is a sun-visibility factor (the strip programs scale the
// sun's light by it), and VIScene::CalcOutdoorsIntensity reads it off the
// static geometry 15 under the camera for the rain's sound and the lightning.
// Neither bake keeps colours, so the server asks the collision instead.

// Roof reports whether anything is over (x, z) between the heights y0 and y1
// (y0 below y1) in world: a triangle of the terrain whose height at (x, z) is
// in (y0, y1], from its floors (a ceiling faces down, which the load keeps
// with the floors: a floor is a triangle facing up or down more than
// sideways) or its walls (a steep overhang); or a triangle of anything placed
// on the terrain that the vertical segment meets, from either side (a roof's
// collision faces out of the house, up, and a body under it looks at its
// back). False where the world has neither file.
func (g *Ground) Roof(worldN uint8, x, y0, y1, z float32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	over := func(h float32, ok bool) bool { return ok && h > y0 && h <= y1 }
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
				if over(heightAt(zg.tri(ti), x, z)) {
					return true
				}
			}
			wg := &zg.wallGrid
			c = zg.cellX(wg, x)*wg.nz + zg.cellZ(wg, z)
			for _, ti := range wg.cells[wg.offsets[c]:wg.offsets[c+1]] {
				if over(heightAt(zg.wall(ti), x, z)) {
					return true
				}
			}
		}
	}
	if w := g.walls[worldN]; w != nil {
		a, b := Vertex{x, y0, z}, Vertex{x, y1, z}
		for i, e := range w.zones {
			if e.count == 0 || !overlaps(a, b, e.lo, e.hi) {
				continue
			}
			zz, err := g.wallZoneLocked(w, i)
			if err != nil {
				continue
			}
			cx, _, cz, _ := zz.span(a, b) // one cell: the segment is upright
			c := cx*zz.nz + cz
			for _, k := range zz.cells[zz.offsets[c]:zz.offsets[c+1]] {
				p := &zz.places[k]
				if !overlaps(a, b, p.lo, p.hi) {
					continue
				}
				buf, err := w.buffer(p.buf)
				if err != nil {
					continue
				}
				if p.meets(buf, a, b) {
					return true
				}
			}
		}
	}
	return false
}

// meets reports the world segment from a to c meeting a triangle of p's
// buffer b from either side, in the buffer's own space.
func (p *placed) meets(b *wallBuffer, a, c Vertex) bool {
	la, lc := apply(&p.inv, a), apply(&p.inv, c)
	lo, hi := lowest(la, lc), highest(la, lc)
	for _, s := range b.strips {
		if !overlaps(lo, hi, s.lo, s.hi) {
			continue
		}
		v := b.verts[s.first : s.first+s.n]
		for k := 0; k+2 < len(v); k++ {
			t0, t1, t2 := stripTri(v, k)
			if meets(la, lc, t0, t1, t2) {
				return true
			}
		}
	}
	return false
}

// meets reports the segment from p to q passing through the triangle (a, b,
// c) from either side, its ends included: crosses without the face's test.
// One in the triangle's own plane runs along it and does not.
func meets(p, q, a, b, c Vertex) bool {
	dx, dy, dz := float64(q.X-p.X), float64(q.Y-p.Y), float64(q.Z-p.Z)
	e1x, e1y, e1z := float64(b.X-a.X), float64(b.Y-a.Y), float64(b.Z-a.Z)
	e2x, e2y, e2z := float64(c.X-a.X), float64(c.Y-a.Y), float64(c.Z-a.Z)
	hx, hy, hz := dy*e2z-dz*e2y, dz*e2x-dx*e2z, dx*e2y-dy*e2x
	det := e1x*hx + e1y*hy + e1z*hz
	if det > -1e-12 && det < 1e-12 {
		return false
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
