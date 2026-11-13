package ground

// Where a creature stands, with what is placed on the terrain (the owner,
// 2026-11-13: "mobs are going through the floor in the collinridge cemetary
// instance"). Floor is the terrain's floors alone, and a creature's moves took
// it: Collonridge Cemetery's mausoleum is a placed slab over the terrain (its
// floor at 56.4 over the terrain's 54.1, a step round it at 55.0 and 55.5), and
// its tower's floors (56.4, 62.7, 68.7, 74.8, 84.8) are placed too. A lesser
// ghoul and a Wilting Flesh that chased their players into the mausoleum were
// set on the terrain under its floor (06:55:59 to 06:56:39 UTC, the warrior on
// the floor at 56.4 and both "out of sight" at 54.1 under him, his shouts
// stopped by the floor between), and CLIENT's ground probe, a ball swept down
// from 1.5 over the height sent, starts under the slab and showed them in it.
// And Floor takes a ceiling as a floor (a cave's, the crypt's at 47.1 over its
// floor at 42.1): a creature on a stair under it stood on it, in the rock.
//
// Stand is the floor as the game's collision has it: the highest surface over
// (x, z) that faces up and is gentle enough to stand on (a floor's slope,
// floor()), the terrain's or anything placed, no more than StepUp over the
// height asked from. A ceiling (facing down) is never one, and a placed face
// steeper than a floor's (a rock's side, a wall) is not climbed. Its cost over
// Floor's is the placed column under the step, looked at only where something
// placed is (most of a world has nothing).

// standDepth is how far under the height asked from a placed floor is looked
// for where the terrain has none; StandHead how far over the floor a body
// stands on another floor is in it (Stand's roofed: Collonridge's mausoleum's
// floor is 2.3 over the terrain round it).
const (
	standDepth float32 = 400
	StandHead  float32 = 3
)

// Stand is the floor a creature at (x, y, z) in world stands on, as the game's
// collision has it: the highest surface facing up, gentle enough to stand on,
// of the terrain or anything placed on it, at (x, z) no more than StepUp over
// y; whether it is something placed's; and whether another such floor is over
// it within StandHead (roofed: a slab's top a little higher than a step, the
// body standing under it in it, where CLIENT shows it); false where the world
// has neither file or there is none (the caller keeps its own height).
func (g *Ground) Stand(worldN uint8, x, y, z float32) (h float32, placed, roofed, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	top := y + StepUp
	best, found := float32(0), false
	next, over := float32(0), false // the lowest floor over the step, within StandHead of it
	ceil := top + StandHead
	if w := g.worlds[worldN]; w != nil {
		for _, zi := range w.terrainIndexLocked().near(x, z, x, z) {
			i, e := int(zi), w.entries[zi]
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
				h, ok := heightAt(t, x, z)
				if !ok || h > ceil {
					continue
				}
				if h <= top {
					if (!found || h > best) && facesUp(t) {
						best, found = h, true
					}
				} else if (!over || h < next) && facesUp(t) {
					next, over = h, true
				}
			}
		}
	}
	if ww := g.walls[worldN]; ww != nil {
		// the placed floors between the terrain's (or standDepth under y) and
		// StandHead over the step: an upright segment down through them
		low := y - standDepth
		if found {
			low = best
		}
		a, b := Vertex{x, ceil, z}, Vertex{x, low, z}
		for _, zi := range ww.wallsIndexLocked().near(x, z, x, z) {
			i, e := int(zi), ww.zones[zi]
			if e.count == 0 || !overlaps(b, a, e.lo, e.hi) {
				continue
			}
			zz, err := g.wallZoneLocked(ww, i)
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
				buf, err := ww.buffer(p.buf)
				if err != nil {
					continue
				}
				p.stand(buf, a, b, top, &best, &found, &placed, &next, &over)
			}
		}
	}
	return best, placed && found, over && found && next-best <= StandHead, found
}

// stand takes the floors of p's buffer the world's upright segment from a
// (top) down to b meets (a triangle met from its front, facing up, its normal
// in the world no steeper than a floor's: floor()) into Stand's: the highest no
// higher than cut (best, found, placed), the lowest over it (next, over).
func (p *placed) stand(buf *wallBuffer, a, b Vertex, cut float32, best *float32, found, placed *bool, next *float32, over *bool) {
	la, lb := apply(&p.inv, a), apply(&p.inv, b)
	lo, hi := lowest(la, lb), highest(la, lb)
	for _, s := range buf.strips {
		if !overlaps(lo, hi, s.lo, s.hi) {
			continue
		}
		v := buf.verts[s.first : s.first+s.n]
		for k := 0; k+2 < len(v); k++ {
			t0, t1, t2 := stripTri(v, k)
			if min(t0.X, t1.X, t2.X) > hi.X || max(t0.X, t1.X, t2.X) < lo.X ||
				min(t0.Z, t1.Z, t2.Z) > hi.Z || max(t0.Z, t1.Z, t2.Z) < lo.Z ||
				min(t0.Y, t1.Y, t2.Y) > hi.Y || max(t0.Y, t1.Y, t2.Y) < lo.Y {
				continue // the segment's box misses the triangle's
			}
			t, front, ok := segTri(la, lb, t0, t1, t2)
			if !ok || !front {
				continue
			}
			h := a.Y + float32(t)*(b.Y-a.Y)
			if h <= cut {
				if (!*found || h > *best) && p.gentle(t0, t1, t2) {
					*best, *found, *placed = h, true, true
				}
			} else if (!*over || h < *next) && p.gentle(t0, t1, t2) {
				*next, *over = h, true
			}
		}
	}
}

// gentle reports a triangle of p's buffer that faces the world's up more than
// sideways once placed (floor()'s 60 degrees): its normal taken to the world by
// the transpose of the placement's inverse.
func (p *placed) gentle(a, b, c Vertex) bool {
	nx, ny, nz := normal(tri{a, b, c})
	m := &p.inv
	wx := m[0]*nx + m[4]*ny + m[8]*nz
	wy := m[1]*nx + m[5]*ny + m[9]*nz
	wz := m[2]*nx + m[6]*ny + m[10]*nz
	l2 := wx*wx + wy*wy + wz*wz
	return l2 > 1e-12 && wy*wy > 0.25*l2
}
