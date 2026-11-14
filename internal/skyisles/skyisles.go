// Package skyisles names the Plane of Sky's isles (world 4), for every place a
// location is shown (the owner, 2026-11-14: "for the location of plane of sky
// islands, for example dust is on night 1 lets make that the Location like
// Plane of Sky (Night Chain 1)"): "Plane of Sky (Night Chain 1)", "Plane of
// Sky (Main Isle)", "Plane of Sky (Isle of Wonder)".
//
// The table (isles_gen.go) is tools/sky_isles.py's, out of world 4's ground
// file: each isle's footprint seen from above, traced as an outline, and its
// name from the essences of handlers/skypass.go standing on it (an isle's
// essence takes its own key: Rain Key 1's is on the first isle of rain) and
// the bosses and landmarks the forum's 2010 walkthroughs name for it (Artiokanis,
// Trixie Longwings, Granavik, the Ancient Jester, the iron knight, King
// Ruthra, Ryini, Tamis, the Erych Arbiter ...). Isles float apart, so a spot
// is the isle whose outline holds it; where two overlap from above (snow 1's
// mountain over snow 2's igloo village, which one jumps down to) the height
// at that spot divides them; off every outline, the nearest within Near (a
// fall, a jump, a bridge), else none.
package skyisles

import "math"

// World is the Plane of Sky's world index (CLIENT's gWorldInfo: PLANESKY).
const World = 4

// Near is how far off an isle's outline a spot still names it.
const Near = 100

type isle struct {
	name   string
	lo, hi float32     // its standing ground's height range
	polys  [][]float32 // its outlines, x, z pairs
}

// split is an overlap cell: its corner, the height dividing the isles, the
// isle above and the one below.
type split struct {
	x, z, y  float32
	up, down string
}

// Isle is the name of the isle at (x, y, z) in the Plane of Sky ("Night Chain
// 1", "Main Isle"), "" off every isle.
func Isle(x, y, z float32) string {
	for _, s := range splits {
		if x >= s.x && x < s.x+splitSide && z >= s.z && z < s.z+splitSide {
			if y >= s.y {
				return s.up
			}
			return s.down
		}
	}
	best, bestOff := -1, float32(math.MaxFloat32)
	for i, is := range isles {
		for _, p := range is.polys {
			if inside(p, x, z) {
				if off := max(is.lo-y, y-is.hi, 0); best < 0 || off < bestOff {
					best, bestOff = i, off
				}
			}
		}
	}
	if best >= 0 {
		return isles[best].name
	}
	name, d := "", float32(Near)
	for _, is := range isles {
		for _, p := range is.polys {
			if e := edgeDist(p, x, z); e < d {
				name, d = is.name, e
			}
		}
	}
	return name
}

// In is the isle at (x, y, z) in world: Isle's in the Plane of Sky, "" in
// any other world.
func In(world int, x, y, z float32) string {
	if world != World {
		return ""
	}
	return Isle(x, y, z)
}

// Name is the Plane of Sky's name as a place is shown (the server's own world
// names are the disc's files', "Planesky").
const Name = "Plane of Sky"

// Place is a world's name, in the Plane of Sky with its isle: "Plane of Sky
// (Night Chain 1)", or "Plane of Sky" off every isle; worldName anywhere else.
func Place(world int, worldName string, x, y, z float32) string {
	if world != World {
		return worldName
	}
	if is := Isle(x, y, z); is != "" {
		return Name + " (" + is + ")"
	}
	return Name
}

// inside is the even-odd test of (x, z) against outline p.
func inside(p []float32, x, z float32) bool {
	in := false
	n := len(p) / 2
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		ax, az, bx, bz := p[2*i], p[2*i+1], p[2*j], p[2*j+1]
		if (az > z) != (bz > z) && x < (bx-ax)*(z-az)/(bz-az)+ax {
			in = !in
		}
	}
	return in
}

// edgeDist is how far (x, z) is from outline p's nearest edge.
func edgeDist(p []float32, x, z float32) float32 {
	best := float32(math.MaxFloat32)
	n := len(p) / 2
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		ax, az, bx, bz := p[2*i], p[2*i+1], p[2*j], p[2*j+1]
		dx, dz := bx-ax, bz-az
		t := float32(0)
		if l := dx*dx + dz*dz; l > 0 {
			t = min(1, max(0, ((x-ax)*dx+(z-az)*dz)/l))
		}
		ex, ez := ax+t*dx-x, az+t*dz-z
		best = min(best, float32(math.Sqrt(float64(ex*ex+ez*ez))))
	}
	return best
}
