package main

import (
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/ground"
)

// plan is what the tool will do, world by world.
type plan struct {
	in        *input
	g         *ground.Ground
	env       *ground.Env
	groundDir string
	words     []string
	camps     []campPoint
	seed      int64
	kinds     map[string]*kind // the wildlife kinds, by key
	prima     map[string][]primaEntry
	biomes    map[string][]string
	worlds    []*worldPlan
}

// kind is a wildlife kind: every creature of a name that the server
// wanders, of a body of its own, more than one of it.
type kind struct {
	key      string
	spelling string // as most of its rows write it
	rows     []*npcRow
	spots    map[int]int // in how many spots of each world its creatures stand (chains within spotLink)
}

// spotLink chains a kind's creatures into one spot.
const spotLink = 150.0

func (k *kind) spotsIn(w int) int { return k.spots[w] }

type move struct {
	row     *npcRow
	clump   int
	x, y, z float64
}

type fill struct {
	kind       *kind
	template   *npcRow
	level      int
	x, y, z    float64
	facing     int
	source     string // "local": the wildlife round it; "prima": the bestiary's for the place
	placeLevel int    // the place's level (localLevel)
}

// worldPlan is one world's plan and what it measured.
type worldPlan struct {
	w      int
	land   *landGrid
	clumps int
	moves  []move
	fills  []fill
	kept   map[string]int // why a wandering creature stayed, by reason
	empty  int            // gap samples no wildlife was near enough to fill
	before coverage
	after  coverage
	// the creatures as they stand before and after, for the maps
	pointsBefore, pointsAfter []pt
}

// coverage is the placeable land by its distance from the nearest creature.
type coverage struct {
	Land int
	Far  [4]int // beyond 150, 250, 300, 500
}

var coverBands = [4]float64{150, 250, 300, 500}

func newPlan(in *input, g *ground.Ground, env *ground.Env, groundDir string, words []string, camps []campPoint,
	seed int64) *plan {
	p := &plan{in: in, g: g, env: env, groundDir: groundDir, words: words, camps: camps, seed: seed,
		kinds: map[string]*kind{}}
	by := map[string][]*npcRow{}
	for _, r := range in.rows {
		if r.key != "" {
			by[r.key] = append(by[r.key], r)
		}
	}
	for key, rows := range by {
		if in.pets[key] || strings.HasSuffix(key, "animation") || in.water[key] == "water" || in.water[key] == "both" {
			continue // a pet's body, or a creature of the water or its edge
		}
		var usable []*npcRow
		for _, r := range rows {
			if r.model != 0 && !in.players[r.model] && r.typ&0x80 == 0 && !r.spawnOnly {
				usable = append(usable, r)
			}
		}
		if len(usable) < 2 {
			continue // one of a kind: a named creature
		}
		wild, open, wet := 0, 0, 0
		for _, r := range usable {
			if r.wild {
				wild++
			}
			if r.open {
				open++
			}
			if r.wet {
				wet++
			}
		}
		if wild*2 < len(usable) || open*2 < len(usable) || wet*5 > len(usable) {
			continue // not wildlife, mostly in caves and cellars, or of the water
		}
		p.kinds[key] = &kind{key: key, spelling: spelling(usable), rows: usable, spots: spotCount(usable)}
	}
	return p
}

// spotCount is in how many spots of each world rows stand: chains within
// spotLink.
func spotCount(rows []*npcRow) map[int]int {
	parent := make([]int, len(rows))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if rows[i].world == rows[j].world && math.Hypot(rows[i].x-rows[j].x, rows[i].z-rows[j].z) <= spotLink {
				if a, b := find(i), find(j); a != b {
					parent[a] = b
				}
			}
		}
	}
	out := map[int]int{}
	seen := map[int]bool{}
	for i, r := range rows {
		if root := find(i); !seen[root] {
			seen[root] = true
			out[r.world]++
		}
	}
	return out
}

// spelling is a kind's name as most of its rows write it, trimmed: the lower
// case one on a tie (tools/seed_wildlife.py's spelling).
func spelling(rows []*npcRow) string {
	count := map[string]int{}
	for _, r := range rows {
		count[strings.TrimSpace(r.name)]++
	}
	best := ""
	for s, c := range count {
		if best == "" || c > count[best] || (c == count[best] && lowerFirst(s) && !lowerFirst(best)) ||
			(c == count[best] && lowerFirst(s) == lowerFirst(best) && s < best) {
			best = s
		}
	}
	return best
}

func lowerFirst(s string) bool { return s != "" && s[0] >= 'a' && s[0] <= 'z' }

// creature reports an NPC a player fights: no 0x80, a body, in the world.
func creature(r *npcRow) bool {
	return r.typ&0x80 == 0 && r.model != 0 && !r.spawnOnly
}

// world plans one world: the land, the clumps spread, the gaps filled.
func (p *plan) world(w int) {
	in := p.in
	wp := &worldPlan{w: w, kept: map[string]int{}}
	var rows []*npcRow
	for _, r := range in.rows {
		if r.world == w {
			rows = append(rows, r)
		}
	}
	// the towns' people (entity/towns.go), and the seeds of the reachable land
	towns := newIndex(64)
	var seeds [][2]float64
	for _, f := range townPeople(rows, in.players) {
		towns.add(f.x, f.z, 0)
	}
	for _, r := range rows {
		if r.spawnOnly || r.model == 0 {
			continue
		}
		if h, ok := p.g.Floor(uint8(w), float32(r.x), 1e6, float32(r.z)); ok && math.Abs(float64(h)-r.y) <= onGround {
			seeds = append(seeds, [2]float64{r.x, r.z})
		}
	}
	land, err := buildLand(p.g, p.env, p.groundDir, w, towns, p.words, seeds)
	if err != nil {
		say("world %d: %v (left as it is)", w, err)
		return
	}
	wp.land = land

	// which creatures move
	camps := newIndex(256)
	for _, c := range p.camps {
		if x, z, ok := c.at(w); ok {
			camps.add(x, z, 0)
		}
	}
	var movers []*npcRow
	for _, r := range rows {
		if !creature(r) {
			continue
		}
		if why := p.stays(r, land, towns, camps); why != "" {
			if r.wanders {
				wp.kept[why]++
			}
			continue
		}
		movers = append(movers, r)
	}
	moving := map[int64]bool{}
	for _, r := range movers {
		moving[r.id] = true
	}
	// everything that does not move: the creatures that stay (what the gaps
	// are measured from) and every NPC (what a placed creature keeps clear of)
	fixed := newIndex(100)
	others := newIndex(64)
	for _, r := range rows {
		if r.spawnOnly || moving[r.id] {
			continue
		}
		others.add(r.x, r.z, int(r.id))
		if creature(r) {
			fixed.add(r.x, r.z, int(r.id))
			wp.pointsBefore = append(wp.pointsBefore, pt{r.x, r.z, int(r.id)})
			wp.pointsAfter = append(wp.pointsAfter, pt{r.x, r.z, int(r.id)})
		}
	}
	for _, r := range movers {
		wp.pointsBefore = append(wp.pointsBefore, pt{r.x, r.z, int(r.id)})
	}
	open := func(k int) bool {
		if !land.placeable(k) {
			return false
		}
		x, z := land.xz(k)
		return !others.within(x, z, npcClear)
	}
	wp.before = measure(land, open, wp.pointsBefore)

	rng := rand.New(rand.NewSource(p.seed + int64(w)*7919))
	wp.moves, wp.clumps = p.spread(land, movers, open, rng)
	for _, m := range wp.moves {
		wp.pointsAfter = append(wp.pointsAfter, pt{m.x, m.z, int(m.row.id)})
	}
	wp.fills, wp.empty = p.fillGaps(land, wp, open, camps, rng)
	for _, f := range wp.fills {
		wp.pointsAfter = append(wp.pointsAfter, pt{f.x, f.z, -1})
	}
	wp.after = measure(land, open, wp.pointsAfter)
	p.worlds = append(p.worlds, wp)
	say("world %d: %d creatures, %d spread in %d clumps, %d filled; open land beyond 250 of a creature %.1f%% -> %.1f%%",
		w, len(wp.pointsBefore), len(wp.moves), wp.clumps, len(wp.fills),
		pct(wp.before.Far[1], wp.before.Land), pct(wp.after.Far[1], wp.after.Land))
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// stays is why a creature does not move ("" when it does).
func (p *plan) stays(r *npcRow, land *landGrid, towns, camps *index) string {
	in := p.in
	switch {
	case !r.wanders:
		return "stands" // the server does not wander it: a camp's, a named one's
	case !r.wild:
		return "a roamer of its own (npcmove: a camp's or Sandstorm's wander), not wildlife"
	case p.kinds[r.key] == nil:
		return "not a wildlife kind (one of a kind, a pet's name, of the water, mostly in caves)"
	case p.kinds[r.key].spotsIn(r.world) < 2:
		return "its kind stands in one spot of the world alone: a camp"
	case in.protected[r.id] != "":
		return in.protected[r.id]
	}
	for _, pr := range in.places {
		if pr.key == r.key && pr.world == r.world && math.Hypot(pr.x-r.x, pr.z-r.z) <= pr.r+5 {
			return "a faction rule binds it to its place (npcfaction)"
		}
	}
	h, ok := p.g.Floor(uint8(r.world), float32(r.x), 1e6, float32(r.z))
	switch {
	case !ok || math.Abs(float64(h)-r.y) > onGround:
		return "off the open ground (a cave, a cellar, a floor)"
	case badAt(p.env, r.world, r.x, r.y, r.z):
		return "in the water"
	case towns.within(r.x, r.z, townClear):
		return "in or near a town"
	case camps.within(r.x, r.z, campKeep):
		return "an EQOA-Data camp's"
	}
	k := land.cell(r.x, r.z)
	if k < 0 || !land.placeable(k) {
		return "on no open land the tool reaches"
	}
	return ""
}

// measure is the placeable land by its distance from the nearest point.
func measure(land *landGrid, open func(int) bool, points []pt) coverage {
	ix := newIndex(100)
	for _, q := range points {
		ix.add(q.x, q.z, q.v)
	}
	var c coverage
	for k := range land.kind {
		if !open(k) {
			continue
		}
		c.Land++
		x, z := land.xz(k)
		d, _ := ix.nearest(x, z, coverBands[3]+1)
		for b, top := range coverBands {
			if d > top {
				c.Far[b]++
			}
		}
	}
	return c
}

// spread spaces each clump's creatures over its own ground (the file's doc).
func (p *plan) spread(land *landGrid, movers []*npcRow, open func(int) bool, rng *rand.Rand) ([]move, int) {
	// clumps: chains of movers within clumpLink
	parent := make([]int, len(movers))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	ix := newIndex(clumpLink)
	for i, r := range movers {
		ix.add(r.x, r.z, i)
	}
	for i, r := range movers {
		ix.near(r.x, r.z, clumpLink, func(q pt, _ float64) {
			if q.v > i {
				if a, b := find(i), find(q.v); a != b {
					parent[a] = b
				}
			}
		})
	}
	groups := map[int][]*npcRow{}
	for i, r := range movers {
		groups[find(i)] = append(groups[find(i)], r)
	}
	type clump struct {
		members []*npcRow
		mx, mz  float64
		centre  int
		samples []int
	}
	var clumps []*clump
	roots := make([]int, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(a, b int) bool { return groups[roots[a]][0].id < groups[roots[b]][0].id })
	for _, root := range roots {
		ms := groups[root]
		sort.Slice(ms, func(a, b int) bool { return ms[a].id < ms[b].id })
		c := &clump{members: ms}
		for _, r := range ms {
			c.mx += r.x
			c.mz += r.z
		}
		c.mx /= float64(len(ms))
		c.mz /= float64(len(ms))
		clumps = append(clumps, c)
	}
	// each open sample to the nearest clump's middle within territory
	middles := newIndex(territory)
	for i, c := range clumps {
		middles.add(c.mx, c.mz, i)
	}
	owner := make([]int32, len(land.kind))
	for k := range owner {
		owner[k] = -1
		if !open(k) {
			continue
		}
		x, z := land.xz(k)
		if d, q := middles.nearest(x, z, territory); q.v >= 0 && d <= territory {
			owner[k] = int32(q.v)
		}
	}
	// a clump's ground: its samples joined to its middle without leaving it
	for i, c := range clumps {
		start := -1
		best := math.Inf(1)
		for _, r := range c.members {
			k := land.cell(r.x, r.z)
			if k >= 0 && owner[k] == int32(i) {
				x, z := land.xz(k)
				if d := math.Hypot(x-c.mx, z-c.mz); d < best {
					start, best = k, d
				}
			}
		}
		if start < 0 {
			// its middle's own sample, or the nearest of its ground
			k := land.cell(c.mx, c.mz)
			if k >= 0 && owner[k] == int32(i) {
				start = k
			}
		}
		if start < 0 {
			continue
		}
		seen := map[int]bool{start: true}
		queue := []int{start}
		for len(queue) > 0 {
			k := queue[0]
			queue = queue[1:]
			c.samples = append(c.samples, k)
			land.neighbours(k, func(nb int) {
				if !seen[nb] && owner[nb] == int32(i) {
					seen[nb] = true
					queue = append(queue, nb)
				}
			})
		}
		c.centre = start
	}
	var moves []move
	for i, c := range clumps {
		n := len(c.members)
		if len(c.samples) < n || len(c.samples) == 0 {
			continue
		}
		spots := lloyd(land, c.samples, c.centre, n)
		// members to spots: both round the middle by angle
		order := make([]int, n)
		for k := range order {
			order[k] = k
		}
		angle := func(x, z float64) float64 { return math.Atan2(z-c.mz, x-c.mx) }
		sort.Slice(order, func(a, b int) bool {
			ra, rb := c.members[order[a]], c.members[order[b]]
			return angle(ra.x, ra.z) < angle(rb.x, rb.z)
		})
		sort.Slice(spots, func(a, b int) bool {
			xa, za := land.xz(spots[a])
			xb, zb := land.xz(spots[b])
			return angle(xa, za) < angle(xb, zb)
		})
		for k, m := range order {
			r := c.members[m]
			x, y, z := p.settle(land, spots[k], rng)
			moves = append(moves, move{row: r, clump: i, x: x, y: y, z: z})
		}
	}
	return moves, len(clumps)
}

// lloyd spreads n points evenly over samples (Lloyd's relaxation from the
// farthest-point start at centre): each the middle of the samples nearest
// it. The points are samples.
func lloyd(land *landGrid, samples []int, centre, n int) []int {
	xs := make([]float64, len(samples))
	zs := make([]float64, len(samples))
	for i, k := range samples {
		xs[i], zs[i] = land.xz(k)
	}
	pts := []int{}
	// the start: the sample nearest the centre, then each the sample
	// farthest from those chosen
	ci := 0
	cx, cz := land.xz(centre)
	for i := range samples {
		if math.Hypot(xs[i]-cx, zs[i]-cz) < math.Hypot(xs[ci]-cx, zs[ci]-cz) {
			ci = i
		}
	}
	pts = append(pts, ci)
	dmin := make([]float64, len(samples))
	for i := range samples {
		dmin[i] = math.Hypot(xs[i]-xs[ci], zs[i]-zs[ci])
	}
	for len(pts) < n {
		far := 0
		for i := range samples {
			if dmin[i] > dmin[far] {
				far = i
			}
		}
		pts = append(pts, far)
		for i := range samples {
			dmin[i] = math.Min(dmin[i], math.Hypot(xs[i]-xs[far], zs[i]-zs[far]))
		}
	}
	if n > 1 {
		assign := make([]int, len(samples))
		for round := 0; round < lloydRounds; round++ {
			sx := make([]float64, n)
			sz := make([]float64, n)
			cnt := make([]int, n)
			for i := range samples {
				best, bd := 0, math.Inf(1)
				for j, q := range pts {
					if d := (xs[i]-xs[q])*(xs[i]-xs[q]) + (zs[i]-zs[q])*(zs[i]-zs[q]); d < bd {
						best, bd = j, d
					}
				}
				assign[i] = best
				sx[best] += xs[i]
				sz[best] += zs[i]
				cnt[best]++
			}
			moved := false
			for j := range pts {
				if cnt[j] == 0 {
					continue
				}
				mx, mz := sx[j]/float64(cnt[j]), sz[j]/float64(cnt[j])
				// the point's own sample nearest its region's middle
				best, bd := pts[j], math.Inf(1)
				for i := range samples {
					if assign[i] != j {
						continue
					}
					if d := (xs[i]-mx)*(xs[i]-mx) + (zs[i]-mz)*(zs[i]-mz); d < bd {
						best, bd = i, d
					}
				}
				if best != pts[j] {
					pts[j] = best
					moved = true
				}
			}
			if !moved {
				break
			}
		}
	}
	out := make([]int, len(pts))
	for i, q := range pts {
		out[i] = samples[q]
	}
	return out
}

// settle is a creature's place at a sample: a little off it (so the
// creatures do not stand on a grid), on the floor there, else the sample's
// own.
func (p *plan) settle(land *landGrid, k int, rng *rand.Rand) (x, y, z float64) {
	sx, sz := land.xz(k)
	sh := float64(land.h[k])
	for try := 0; try < 4; try++ {
		x = sx + (rng.Float64()*2-1)*jitter
		z = sz + (rng.Float64()*2-1)*jitter
		h, ok := p.g.Floor(uint8(land.world), float32(x), 1e6, float32(z))
		if !ok || math.Abs(float64(h)-sh) > 5 || badAt(p.env, land.world, x, float64(h), z) {
			continue
		}
		if p.g.Wall(uint8(land.world), float32(sx), float32(sh)+1, float32(sz), float32(x), h+1, float32(z)) {
			continue
		}
		return round3(x), round3(float64(h)), round3(z)
	}
	return sx, sh, sz
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// fillGaps places new creatures on the land still farther than gapFill from
// any creature (the file's doc); how many gap samples had nothing to fill
// them with.
//
// A new creature's kind is one of the free wildlife round the gap (on the
// open ground, out of the water, no camp's; the moved ones where they go),
// drawn by nearness: a kind found in one spot of the world alone is a camp
// and is not drawn unless Prima's bestiary lists it for the place, and a
// kind whose creatures here are far off the place's level (levelBand) is not
// drawn at all; one the bestiary lists for the place counts twice. Where no
// free wildlife round it will do, the bestiary's own creatures for the
// place at its level are drawn from (those this server has a body for). The
// place's level is the creatures' round it (localLevel), the players'
// race bodies left out (a town's guards).
func (p *plan) fillGaps(land *landGrid, wp *worldPlan, open func(int) bool, camps *index, rng *rand.Rand) ([]fill, int) {
	w := land.world
	occupied := newIndex(100)
	for _, q := range wp.pointsAfter {
		occupied.add(q.x, q.z, q.v)
	}
	movedTo := map[int64]move{}
	for _, m := range wp.moves {
		movedTo[m.row.id] = m
	}
	type wild struct {
		k     *kind
		level int
	}
	var wilds []wild
	wildIx := newIndex(250)
	keys := make([]string, 0, len(p.kinds))
	for key := range p.kinds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		k := p.kinds[key]
		for _, r := range k.rows {
			if r.world != w || !r.wild || !r.open || r.wet || camps.within(r.x, r.z, campKeep) {
				continue
			}
			x, z := r.x, r.z
			if m, ok := movedTo[r.id]; ok {
				x, z = m.x, m.z
			}
			wildIx.add(x, z, len(wilds))
			wilds = append(wilds, wild{k, r.level})
		}
	}
	// the levels of the place: every creature but the players' race bodies
	var levels []int
	levelIx := newIndex(250)
	for _, r := range p.in.rows {
		if r.world != w || !creature(r) || p.in.players[r.model] {
			continue
		}
		x, z := r.x, r.z
		if m, ok := movedTo[r.id]; ok {
			x, z = m.x, m.z
		}
		levelIx.add(x, z, len(levels))
		levels = append(levels, r.level)
	}
	localLevel := func(x, z float64) int {
		var ls []weighed
		for reach := kindReach; reach <= 3*kindReach && len(ls) == 0; reach += kindReach {
			levelIx.near(x, z, reach, func(q pt, d float64) {
				ls = append(ls, weighed{levels[q.v], nearness(d)})
			})
		}
		return weightedMedian(ls)
	}
	type gap struct {
		k int
		d float64
	}
	var gaps []gap
	for k := range land.kind {
		if !open(k) {
			continue
		}
		x, z := land.xz(k)
		if d, _ := occupied.nearest(x, z, 5000); d > gapFill {
			gaps = append(gaps, gap{k, d})
		}
	}
	sort.Slice(gaps, func(a, b int) bool {
		if gaps[a].d != gaps[b].d {
			return gaps[a].d > gaps[b].d
		}
		return gaps[a].k < gaps[b].k
	})
	var fills []fill
	empty := 0
	for _, gp := range gaps {
		x, z := land.xz(gp.k)
		if occupied.within(x, z, fillSpacing) {
			continue
		}
		place := localLevel(x, z)
		sections := p.sectionsAt(w, x, z)
		// the free wildlife round it, by kind: its weight and its levels here
		weight := map[*kind]float64{}
		lvls := map[*kind][]weighed{}
		for reach := kindReach; reach <= 3*kindReach && len(weight) == 0; reach += kindReach {
			wildIx.near(x, z, reach, func(q pt, d float64) {
				wl := wilds[q.v]
				weight[wl.k] += nearness(d)
				lvls[wl.k] = append(lvls[wl.k], weighed{wl.level, nearness(d)})
			})
		}
		type cand struct {
			k      *kind
			w      float64
			level  int
			lo, hi int // the bestiary's levels, 0 0 for none
		}
		var cands []cand
		for k, wt := range weight {
			lk := weightedMedian(lvls[k])
			if place > 0 && abs(lk-place) > levelBand {
				continue
			}
			lo, hi, listed := p.primaRange(k.key, sections, lk)
			if listed {
				wt *= 2
			} else {
				lo, hi = 0, 0
				if k.spotsIn(w) < 2 {
					continue
				}
			}
			cands = append(cands, cand{k, wt, lk, lo, hi})
		}
		source := "local"
		if len(cands) == 0 && place > 0 {
			// the bestiary's creatures for the place, at its level
			for _, key := range keys {
				k := p.kinds[key]
				lo, hi, listed := p.primaRange(key, sections, place)
				if !listed || place+primaReach < lo || place-primaReach > hi {
					continue
				}
				cands = append(cands, cand{k, 1, max(lo, min(hi, place)), lo, hi})
			}
			source = "prima"
		}
		if len(cands) == 0 {
			empty++
			continue
		}
		sort.Slice(cands, func(a, b int) bool { return cands[a].k.key < cands[b].k.key })
		total := 0.0
		for _, c := range cands {
			total += c.w
		}
		pick := rng.Float64() * total
		chosen := cands[len(cands)-1]
		for _, c := range cands {
			pick -= c.w
			if pick <= 0 {
				chosen = c
				break
			}
		}
		level := chosen.level + []int{-1, 0, 0, 1}[rng.Intn(4)]
		if chosen.hi > 0 {
			level = max(chosen.lo, min(chosen.hi, level))
		}
		level = max(1, level)
		fx, fy, fz := p.settle(land, gp.k, rng)
		fills = append(fills, fill{kind: chosen.k, template: template(chosen.k, w, fx, fz), level: level,
			x: fx, y: fy, z: fz, facing: rng.Intn(256), source: source, placeLevel: place})
		occupied.add(fx, fz, -1)
	}
	return fills, empty
}

// How far a kind's creatures round a gap may be off the place's level and
// still be drawn, and how far the bestiary's levels for a kind.
const (
	levelBand  = 8
	primaReach = 3
)

type weighed struct {
	v int
	w float64
}

// nearness is a creature's weight at distance d.
func nearness(d float64) float64 { return 1 / ((d + 150) * (d + 150)) }

// weightedMedian of the values by their weights, 0 for none.
func weightedMedian(vs []weighed) int {
	if len(vs) == 0 {
		return 0
	}
	sort.Slice(vs, func(a, b int) bool { return vs[a].v < vs[b].v })
	sum := 0.0
	for _, v := range vs {
		sum += v.w
	}
	acc := 0.0
	for _, v := range vs {
		acc += v.w
		if acc >= sum/2 {
			return v.v
		}
	}
	return vs[len(vs)-1].v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// template is the row a new creature copies: of its kind's most common body,
// the nearest in its world (tools/seed_wildlife.py's template).
func template(k *kind, w int, x, z float64) *npcRow {
	count := map[uint32]int{}
	for _, r := range k.rows {
		count[r.model]++
	}
	var common uint32
	for m, c := range count {
		if c > count[common] || (c == count[common] && m < common) {
			common = m
		}
	}
	var best *npcRow
	bd := math.Inf(1)
	for _, r := range k.rows {
		if r.model != common {
			continue
		}
		d := math.Hypot(r.x-x, r.z-z)
		if r.world != w {
			d += 1e9
		}
		if d < bd || (d == bd && r.id < best.id) {
			best, bd = r, d
		}
	}
	return best
}

// townPeople is entity/towns.go's towns: chains of NPCs with a players' race
// body each within 60 of the next, of at least 10.
func townPeople(rows []*npcRow, players map[uint32]bool) []*npcRow {
	var folk []*npcRow
	for _, r := range rows {
		if r.model != 0 && players[r.model] {
			folk = append(folk, r)
		}
	}
	parent := make([]int, len(folk))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	ix := newIndex(60)
	for i, f := range folk {
		ix.add(f.x, f.z, i)
	}
	for i, f := range folk {
		ix.near(f.x, f.z, 60, func(q pt, _ float64) {
			if q.v > i {
				if a, b := find(i), find(q.v); a != b {
					parent[a] = b
				}
			}
		})
	}
	size := map[int]int{}
	for i := range folk {
		size[find(i)]++
	}
	var out []*npcRow
	for i, f := range folk {
		if size[find(i)] >= 10 {
			out = append(out, f)
		}
	}
	return out
}
