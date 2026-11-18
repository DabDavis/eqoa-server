package ground

// A body in something placed (the owner, 2026-11-18: "lonewolf is in the
// rock by noroxus but the server should have him on top of the rock or
// outside the rock"). The rock is a LOD sprite in Secrets' zone 0
// (0x6630AE67 at 4240.1, -20.4, 2167.7), its level 0's collision in
// 5.walls like every placed rock's; its edge overhangs the ground there, its
// underside 1.3-1.5 over the terrain, so a body standing on the terrain
// under it has its middle (1.2 over its feet) under the underside and its
// head in the rock. Nothing the movement check asked saw it: the middle
// crossed no face on its way in and stood over the terrain's floor.
//
// CLIENT's walk keeps a 0.6 ball round the middle off every face it meets
// from the front (VIWalkControl's move sweep, m_parm34), so a body whose ball
// is in a face, or whose middle is inside something closed, is where the walk
// does not put one: put there by a teleport, a login, a push, or a client's
// own walk that a floor's snap carried up under an overhang.

import "math"

// Embedded reports a body with its feet at (x, y, z) in world in something
// placed on the terrain (the walls file): a ball of radius r round its
// middle (mid over its feet) meets a placed face from the face's front, or
// the middle is inside a placed thing (straight up and straight down from it
// the nearest of its faces are met from behind: a rock's top over it, its
// underside under it; a bridge's deck over a body under it is not, nothing
// of it lies under the body). The terrain is left out (its own walls and
// floors are the movement check's crossings). False where the world has no
// walls file.
func (g *Ground) Embedded(worldN uint8, x, y, z, mid, r float32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.walls[worldN]
	if w == nil {
		return false
	}
	c := Vertex{x, y + mid, z}
	lo, hi := Vertex{x - r, c.Y - r, z - r}, Vertex{x + r, c.Y + r, z + r}
	for _, zi := range w.wallsIndexLocked().near(lo.X, lo.Z, hi.X, hi.Z) {
		i, e := int(zi), w.zones[zi]
		if e.count == 0 || !overlaps(lo, hi, e.lo, e.hi) {
			continue
		}
		zz, err := g.wallZoneLocked(w, i)
		if err != nil {
			continue
		}
		x0, x1, z0, z1 := zz.span(lo, hi)
		for cx := x0; cx <= x1; cx++ {
			for cz := z0; cz <= z1; cz++ {
				cell := cx*zz.nz + cz
				for _, k := range zz.cells[zz.offsets[cell]:zz.offsets[cell+1]] {
					p := &zz.places[k]
					if !overlaps(lo, hi, p.lo, p.hi) {
						continue
					}
					buf, err := w.buffer(p.buf)
					if err != nil {
						continue
					}
					if p.ballIn(buf, c, r) || p.inside(buf, c) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ballIn reports the world ball (c, r) meeting a triangle of p's buffer from
// its front, in the buffer's own space (the radius by the placement's
// scale, the least of its axes').
func (p *placed) ballIn(buf *wallBuffer, c Vertex, r float32) bool {
	m := &p.inv
	s := min(
		float32(math.Sqrt(float64(m[0]*m[0]+m[4]*m[4]+m[8]*m[8]))),
		float32(math.Sqrt(float64(m[1]*m[1]+m[5]*m[5]+m[9]*m[9]))),
		float32(math.Sqrt(float64(m[2]*m[2]+m[6]*m[6]+m[10]*m[10]))))
	lc, lr := apply(m, c), r*s
	lo, hi := Vertex{lc.X - lr, lc.Y - lr, lc.Z - lr}, Vertex{lc.X + lr, lc.Y + lr, lc.Z + lr}
	r2 := float64(lr) * float64(lr)
	for _, st := range buf.strips {
		if !overlaps(lo, hi, st.lo, st.hi) {
			continue
		}
		v := buf.verts[st.first : st.first+st.n]
		for k := 0; k+2 < len(v); k++ {
			a, b, cc := stripTri(v, k)
			if min(a.X, b.X, cc.X) > hi.X || max(a.X, b.X, cc.X) < lo.X ||
				min(a.Y, b.Y, cc.Y) > hi.Y || max(a.Y, b.Y, cc.Y) < lo.Y ||
				min(a.Z, b.Z, cc.Z) > hi.Z || max(a.Z, b.Z, cc.Z) < lo.Z {
				continue
			}
			if ballMeetsFront(lc, r2, a, b, cc) {
				return true
			}
		}
	}
	return false
}

// ballMeetsFront reports the ball round c (its radius squared r2) meeting
// the triangle (a, b, cc) with c in front of its plane (the plane facing
// (cc-b) x (b-a), crosses').
func ballMeetsFront(c Vertex, r2 float64, a, b, cc Vertex) bool {
	ax, ay, az := float64(a.X), float64(a.Y), float64(a.Z)
	bx, by, bz := float64(b.X), float64(b.Y), float64(b.Z)
	cx, cy, cz := float64(cc.X), float64(cc.Y), float64(cc.Z)
	px, py, pz := float64(c.X), float64(c.Y), float64(c.Z)
	// the front: (cc-b) x (b-a)
	ux, uy, uz := cx-bx, cy-by, cz-bz
	wx, wy, wz := bx-ax, by-ay, bz-az
	nx, ny, nz := uy*wz-uz*wy, uz*wx-ux*wz, ux*wy-uy*wx
	if (px-ax)*nx+(py-ay)*ny+(pz-az)*nz <= 0 {
		return false // behind the plane, or in it
	}
	qx, qy, qz := closestOnTri(px, py, pz, ax, ay, az, bx, by, bz, cx, cy, cz)
	dx, dy, dz := px-qx, py-qy, pz-qz
	return dx*dx+dy*dy+dz*dz < r2
}

// closestOnTri is the point of the triangle (a, b, c) nearest p (Ericson's
// Real-Time Collision Detection, 5.1.5).
func closestOnTri(px, py, pz, ax, ay, az, bx, by, bz, cx, cy, cz float64) (float64, float64, float64) {
	abx, aby, abz := bx-ax, by-ay, bz-az
	acx, acy, acz := cx-ax, cy-ay, cz-az
	apx, apy, apz := px-ax, py-ay, pz-az
	d1 := abx*apx + aby*apy + abz*apz
	d2 := acx*apx + acy*apy + acz*apz
	if d1 <= 0 && d2 <= 0 {
		return ax, ay, az
	}
	bpx, bpy, bpz := px-bx, py-by, pz-bz
	d3 := abx*bpx + aby*bpy + abz*bpz
	d4 := acx*bpx + acy*bpy + acz*bpz
	if d3 >= 0 && d4 <= d3 {
		return bx, by, bz
	}
	vc := d1*d4 - d3*d2
	if vc <= 0 && d1 >= 0 && d3 <= 0 {
		v := d1 / (d1 - d3)
		return ax + v*abx, ay + v*aby, az + v*abz
	}
	cpx, cpy, cpz := px-cx, py-cy, pz-cz
	d5 := abx*cpx + aby*cpy + abz*cpz
	d6 := acx*cpx + acy*cpy + acz*cpz
	if d6 >= 0 && d5 <= d6 {
		return cx, cy, cz
	}
	vb := d5*d2 - d1*d6
	if vb <= 0 && d2 >= 0 && d6 <= 0 {
		w := d2 / (d2 - d6)
		return ax + w*acx, ay + w*acy, az + w*acz
	}
	va := d3*d6 - d5*d4
	if va <= 0 && d4-d3 >= 0 && d5-d6 >= 0 {
		w := (d4 - d3) / ((d4 - d3) + (d5 - d6))
		return bx + w*(cx-bx), by + w*(cy-by), bz + w*(cz-bz)
	}
	den := 1 / (va + vb + vc)
	v, w := vb*den, vc*den
	return ax + abx*v + acx*w, ay + aby*v + acy*w, az + abz*v + acz*w
}

// inside reports the world point c inside p: straight up from it and
// straight down, the nearest of p's faces are each met from behind.
func (p *placed) inside(buf *wallBuffer, c Vertex) bool {
	if c.X < p.lo.X || c.X > p.hi.X || c.Z < p.lo.Z || c.Z > p.hi.Z || c.Y < p.lo.Y || c.Y > p.hi.Y {
		return false
	}
	upFront, upOK := p.nearest(buf, c, Vertex{c.X, p.hi.Y + 1, c.Z})
	if !upOK || upFront {
		return false
	}
	downFront, downOK := p.nearest(buf, c, Vertex{c.X, p.lo.Y - 1, c.Z})
	return downOK && !downFront
}

// nearest is whether the nearest of p's faces the world segment from a to
// b meets is met from its front, and whether it meets one.
func (p *placed) nearest(buf *wallBuffer, a, b Vertex) (front, ok bool) {
	la, lb := apply(&p.inv, a), apply(&p.inv, b)
	lo, hi := lowest(la, lb), highest(la, lb)
	best := 2.0
	for _, s := range buf.strips {
		if !overlaps(lo, hi, s.lo, s.hi) {
			continue
		}
		v := buf.verts[s.first : s.first+s.n]
		for k := 0; k+2 < len(v); k++ {
			t0, t1, t2 := stripTri(v, k)
			if t, f, hit := segTri(la, lb, t0, t1, t2); hit && t < best {
				best, front, ok = t, f, true
			}
		}
	}
	return front, ok
}

// How far Free looks for a spot: up onto the top of what the body is in, and
// out round it.
const (
	FreeTop   float32 = 4   // a top this far over the feet at most
	FreeReach float32 = 8   // a spot beside this far away at most
	freeStep  float32 = 0.5 // the rings round the place this far apart
	freeDrop  float32 = 3   // a spot beside no lower than this under the feet
	freeDirs          = 16  // spots a ring
)

// Free is the nearest place a body with its feet at (x, y, z) in world,
// in something placed, stands free (not Embedded, with mid and r: the
// clearance asked, the client's own ball and a margin): on top of
// it, the floor (Stand's: facing up and gentle enough, the terrain's or
// anything placed) over the feet within FreeTop, or out beside it, a floor
// at most StepUp over and freeDrop under the feet within FreeReach, on the
// near side of every wall of the terrain's, whichever is less far (the climb
// to the top against the way across). top says which; ok is false where
// neither is (the caller keeps its own).
func (g *Ground) Free(worldN uint8, x, y, z, mid, r float32) (fx, fy, fz float32, top, ok bool) {
	free := func(px, py, pz float32) bool { return !g.Embedded(worldN, px, py, pz, mid, r) }
	best := float32(math.Inf(1))
	if h, _, _, found := g.Stand(worldN, x, y+FreeTop-StepUp, z); found && h > y+0.05 && free(x, h, z) {
		fx, fy, fz, top, ok, best = x, h, z, true, true, h-y
	}
	for d := freeStep; d <= FreeReach && d < best; d += freeStep {
		bestH := float32(math.Inf(1))
		for i := 0; i < freeDirs; i++ {
			a := 2 * math.Pi * float64(i) / freeDirs
			px, pz := x+d*float32(math.Sin(a)), z+d*float32(math.Cos(a))
			h, _, _, found := g.Stand(worldN, px, y, pz)
			if !found || h < y-freeDrop || abs32(h-y) >= bestH || !free(px, h, pz) {
				continue
			}
			g.mu.Lock()
			through := g.terrainWallLocked(worldN, Vertex{x, y + mid, z}, Vertex{px, h + mid, pz})
			g.mu.Unlock()
			if through {
				continue
			}
			fx, fy, fz, top, ok, bestH = px, h, pz, false, true, abs32(h-y)
		}
		if bestH < float32(math.Inf(1)) {
			return fx, fy, fz, top, ok
		}
	}
	return fx, fy, fz, top, ok
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
