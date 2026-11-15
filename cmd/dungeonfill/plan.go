package main

import (
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strings"
)

// The fill. A dungeon's levels hold different creatures (Runnyeye: the gate's
// novices and lookouts at 15-18, the deep halls' veterans, champions and
// witchdoctors at 24-28), so the floor is cut into height bands and each
// band is filled with the kinds standing in it now, by how many of each
// stand there, at the levels each stands at there, each a copy of the
// kind's nearest row. Camps go farthest first on open floor (every
// neighbouring column joined: a body's width off any wall) until none is
// campGap from every creature, each of one to three creatures.
type band struct{ lo, hi float32 }

type kindIn struct {
	name   string
	count  int
	lo, hi int // its levels in the band
}

type placed struct {
	name     string
	template int64
	level    int
	x, y, z  float64
	facing   int
	band     int
	camp     int
	terrain  bool
}

type planner struct {
	g         *graph
	rows      []row
	bands     []band
	keep      map[int32]bool
	in        []bool // the dungeon's floor (graph.insideOf)
	campGap   float64
	memberGap float64
	rowGap    float64
	groupW    []float64 // weight of a camp of 1, 2, 3 ...
	exclude   *regexp.Regexp
	minDeg    int // open floor: joined to this many of its 8 neighbours (8 a room's, 5 a tunnel's middle)
	wanders   *regexp.Regexp
	rnd       *rand.Rand
}

func (p *planner) bandOf(y float32) int {
	for i, b := range p.bands {
		if y >= b.lo && y < b.hi {
			return i
		}
	}
	return -1
}

// filler is whether a row's kind is a fill's: no named creature (a
// capitalised name), nothing friendly, nothing excluded.
func (p *planner) filler(r row) bool {
	if r.name == "" || r.typ&0x80 != 0 {
		return false
	}
	if c := r.name[:1]; strings.ToUpper(c) == c && strings.ToLower(c) != c {
		return false
	}
	return !p.exclude.MatchString(r.name)
}

// dist is the way two spots are apart for spacing: across, with a level's
// rise counting three times (one hall over another is far apart).
func dist(ax, ay, az, bx, by, bz float64) float64 {
	dy := 3 * (ay - by)
	return math.Sqrt((ax-bx)*(ax-bx) + (az-bz)*(az-bz) + dy*dy)
}

func (p *planner) plan() []placed {
	g := p.g
	// the kinds of each band
	kinds := make([][]kindIn, len(p.bands))
	for _, r := range p.rows {
		b := p.bandOf(r.y)
		if b < 0 || !p.filler(r) {
			continue
		}
		found := false
		for i := range kinds[b] {
			k := &kinds[b][i]
			if k.name == r.name {
				k.count++
				k.lo, k.hi = min(k.lo, r.level), max(k.hi, r.level)
				found = true
			}
		}
		if !found {
			kinds[b] = append(kinds[b], kindIn{r.name, 1, r.level, r.level})
		}
	}
	for b := range kinds {
		sort.Slice(kinds[b], func(i, j int) bool { return kinds[b][i].name < kinds[b][j].name })
	}
	// the open floor
	open := make([]bool, len(g.nodes))
	deg := make([]int, len(g.nodes))
	for k := range g.nodes {
		deg[k] = len(g.adj[k])
	}
	var cand []int32
	for k, n := range g.nodes {
		if !p.in[k] || deg[k] < p.minDeg || p.bandOf(n.y) < 0 || len(kinds[p.bandOf(n.y)]) == 0 {
			continue
		}
		{
			open[k] = true
			cand = append(cand, int32(k))
		}
	}
	// what stands already
	type pt struct{ x, y, z float64 }
	var taken []pt
	for _, r := range p.rows {
		taken = append(taken, pt{r.x, float64(r.y), r.z})
	}
	near := make([]float64, len(cand))
	for i, k := range cand {
		n := g.nodes[k]
		near[i] = math.Inf(1)
		for _, t := range taken {
			near[i] = math.Min(near[i], dist(n.x, float64(n.y), n.z, t.x, t.y, t.z))
		}
	}
	var out []placed
	camp := 0
	for {
		best, bd := -1, p.campGap
		for i := range cand {
			if near[i] >= bd {
				best, bd = i, near[i]
			}
		}
		if best < 0 {
			break
		}
		c := g.nodes[cand[best]]
		b := p.bandOf(c.y)
		// the camp's spots: its centre, then open floor a step or two along
		size := 1 + pick(p.rnd, p.groupW)
		if deg[cand[best]] < 8 {
			size = 1 // a tunnel: one at a time
		}
		spots := []int32{cand[best]}
		ring := p.ringOf(cand[best], open)
		p.rnd.Shuffle(len(ring), func(i, j int) { ring[i], ring[j] = ring[j], ring[i] })
		for _, k := range ring {
			if len(spots) >= size {
				break
			}
			n := g.nodes[k]
			ok := true
			for _, s := range spots {
				m := g.nodes[s]
				if dist(n.x, float64(n.y), n.z, m.x, float64(m.y), m.z) < p.memberGap {
					ok = false
				}
			}
			for _, t := range taken {
				if dist(n.x, float64(n.y), n.z, t.x, t.y, t.z) < p.rowGap {
					ok = false
				}
			}
			if ok && p.bandOf(n.y) == b {
				spots = append(spots, k)
			}
		}
		camp++
		for _, s := range spots {
			n := g.nodes[s]
			k := p.kindFor(kinds[b], n.terrain)
			if k == nil {
				continue
			}
			lv := k.lo + p.rnd.Intn(k.hi-k.lo+1)
			jx, jz := (p.rnd.Float64()-0.5)*g.cell*0.6, (p.rnd.Float64()-0.5)*g.cell*0.6
			if len(g.adj[s]) < 8 {
				jx, jz = 0, 0 // a tunnel's: its middle
			}
			out = append(out, placed{name: k.name, template: p.template(k.name, n.x, float64(n.y), n.z), level: lv,
				x: round2(n.x + jx), y: round2(float64(n.y)), z: round2(n.z + jz), facing: p.rnd.Intn(256),
				band: b, camp: camp, terrain: n.terrain})
			taken = append(taken, pt{n.x, float64(n.y), n.z})
		}
		for i, k := range cand {
			n := g.nodes[k]
			for _, s := range spots {
				m := g.nodes[s]
				near[i] = math.Min(near[i], dist(n.x, float64(n.y), n.z, m.x, float64(m.y), m.z))
			}
		}
		near[best] = 0
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ringOf is the open floor two to three steps along the floor from k.
func (p *planner) ringOf(k int32, open []bool) []int32 { return p.ringTo(k, open, 3) }

// ringTo is the open floor two to far steps along the floor from k, nearest first.
func (p *planner) ringTo(k int32, open []bool, far int) []int32 {
	seen := map[int32]int{k: 0}
	q := []int32{k}
	var out []int32
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		if seen[v] >= far {
			continue
		}
		for _, w := range p.g.adj[v] {
			if _, ok := seen[w]; ok {
				continue
			}
			seen[w] = seen[v] + 1
			q = append(q, w)
			if seen[w] >= 2 && open[w] {
				out = append(out, w)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if seen[out[i]] != seen[out[j]] {
			return seen[out[i]] < seen[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// kindFor draws a kind of the band by its count; a wanderer (an animal the
// server walks on the terrain's floor) only onto the terrain's own floor.
func (p *planner) kindFor(ks []kindIn, terrain bool) *kindIn {
	var w []float64
	for _, k := range ks {
		if !terrain && p.wanders.MatchString(k.name) {
			w = append(w, 0)
			continue
		}
		w = append(w, float64(k.count))
	}
	total := 0.0
	for _, v := range w {
		total += v
	}
	if total == 0 {
		return nil
	}
	return &ks[pick(p.rnd, w)]
}

func pick(r *rand.Rand, w []float64) int {
	total := 0.0
	for _, v := range w {
		total += v
	}
	x := r.Float64() * total
	for i, v := range w {
		if x < v {
			return i
		}
		x -= v
	}
	return len(w) - 1
}

// template is the row of that name nearest the spot.
func (p *planner) template(name string, x, y, z float64) int64 {
	best, bd := int64(0), math.Inf(1)
	for _, r := range p.rows {
		if r.name != name {
			continue
		}
		if d := dist(x, y, z, r.x, float64(r.y), r.z); d < bd {
			best, bd = r.id, d
		}
	}
	return best
}
