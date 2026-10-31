package main

import "math"

// index is points by cell, for what is near a place.
type index struct {
	cell  float64
	cells map[[2]int][]pt
}

type pt struct {
	x, z float64
	v    int // the caller's
}

func newIndex(cell float64) *index {
	return &index{cell: cell, cells: map[[2]int][]pt{}}
}

func (ix *index) key(x, z float64) [2]int {
	return [2]int{int(math.Floor(x / ix.cell)), int(math.Floor(z / ix.cell))}
}

func (ix *index) add(x, z float64, v int) {
	k := ix.key(x, z)
	ix.cells[k] = append(ix.cells[k], pt{x, z, v})
}

// near calls f with every point within r of (x, z) and its distance.
func (ix *index) near(x, z, r float64, f func(p pt, d float64)) {
	n := int(math.Ceil(r / ix.cell))
	c := ix.key(x, z)
	for dx := -n; dx <= n; dx++ {
		for dz := -n; dz <= n; dz++ {
			for _, p := range ix.cells[[2]int{c[0] + dx, c[1] + dz}] {
				if d := math.Hypot(p.x-x, p.z-z); d <= r {
					f(p, d)
				}
			}
		}
	}
}

// within reports a point within r of (x, z).
func (ix *index) within(x, z, r float64) bool {
	found := false
	n := int(math.Ceil(r / ix.cell))
	c := ix.key(x, z)
	for dx := -n; dx <= n && !found; dx++ {
		for dz := -n; dz <= n && !found; dz++ {
			for _, p := range ix.cells[[2]int{c[0] + dx, c[1] + dz}] {
				if math.Hypot(p.x-x, p.z-z) <= r {
					found = true
					break
				}
			}
		}
	}
	return found
}

// nearest is the distance to the nearest point, max when none is within it,
// and that point.
func (ix *index) nearest(x, z, max float64) (float64, pt) {
	c := ix.key(x, z)
	best, bp := max, pt{v: -1}
	for ring := 0; ; ring++ {
		if float64(ring-1)*ix.cell > best {
			break
		}
		for dx := -ring; dx <= ring; dx++ {
			for dz := -ring; dz <= ring; dz++ {
				if dx != -ring && dx != ring && dz != -ring && dz != ring {
					continue
				}
				for _, p := range ix.cells[[2]int{c[0] + dx, c[1] + dz}] {
					if d := math.Hypot(p.x-x, p.z-z); d < best {
						best, bp = d, p
					}
				}
			}
		}
		if float64(ring)*ix.cell > max {
			break
		}
	}
	return best, bp
}
