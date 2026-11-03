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
	// zones.go's: the kinds its rules name, the undead's bodies and kinds
	ruled      map[string]*kind
	deadBodies map[uint32]bool
	deadKinds  map[int]map[string]*kind
	zoneLevels map[[2]int]int
	// a run round a few places (-near): a creature outside every circle
	// stays and nothing is placed outside them; fill only (-no-spread); the
	// fill's distances (-gap, -spacing); and a kind a quest near the place
	// kills or loots counted questWeight times (-quest-weight)
	near        []circle
	noSpread    bool
	gapFill     float64
	fillSpacing float64
	questWeight float64
	questGivers map[string][]place // a kind's key -> where the givers of the quests that kill or loot it stand
	// a place's level by its distance from a circle's town (-layers), in
	// place of its creatures' (localLevel): newbie ground's layers
	layers []layer
	// a fill's group (-group): its size by weight and how far its others
	// stand from the first (-group-radius); and the newbie quests' kinds kept
	// to questMin within questMinReach of their givers (-quest-min)
	groupSizes    []int
	groupWeights  []float64
	groupRadius   float64
	questMin      int
	questMinReach float64
	questSteps    []questStep
}

// questStep is a quest's kill or loot step: the names it counts (its target
// and queststeptargets' others), its quest's lowest level and where its
// quest's givers stand.
type questStep struct {
	names    []string
	minLevel int
	givers   []place
}

// questMinLevel is the highest quest -quest-min keeps creatures for: a
// starting town's first quests.
const questMinLevel = 12

// groupGap is how near two of a group may stand.
const groupGap = 8.0

// groupSize draws a group's size (1 without -group, drawing nothing).
func (p *plan) groupSize(rng *rand.Rand) int {
	if len(p.groupSizes) == 0 {
		return 1
	}
	total := 0.0
	for _, w := range p.groupWeights {
		total += w
	}
	pick := rng.Float64() * total
	for i, w := range p.groupWeights {
		if pick -= w; pick <= 0 {
			return p.groupSizes[i]
		}
	}
	return p.groupSizes[len(p.groupSizes)-1]
}

// member is a place for another of a group round its first at (x, y, z):
// within groupRadius and groupGap from the group's others, on open land the
// first's floor reaches (within 5 of its height, no wall between, out of
// the water, in its zone).
func (p *plan) member(land *landGrid, open func(int) bool, x, y, z float64, group [][2]float64, rng *rand.Rand) (float64, float64, float64, bool) {
	for try := 0; try < 12; try++ {
		a := rng.Float64() * 2 * math.Pi
		r := groupGap + rng.Float64()*(p.groupRadius-groupGap)
		mx, mz := x+r*math.Cos(a), z+r*math.Sin(a)
		if k := land.cell(mx, mz); k < 0 || !open(k) || zoneWord(land.world, mx, mz) != zoneWord(land.world, x, z) {
			continue
		}
		h, ok := p.g.Floor(uint8(land.world), float32(mx), 1e6, float32(mz))
		if !ok || math.Abs(float64(h)-y) > 5 || badAt(p.env, land.world, mx, float64(h), mz) {
			continue
		}
		if p.g.Wall(uint8(land.world), float32(x), float32(y)+1, float32(z), float32(mx), h+1, float32(mz)) {
			continue
		}
		near := false
		for _, o := range group {
			if math.Hypot(o[0]-mx, o[1]-mz) < groupGap {
				near = true
				break
			}
		}
		if !near {
			return round3(mx), round3(float64(h)), round3(mz), true
		}
	}
	return 0, 0, 0, false
}

// sectionsFor is the bestiary's sections for (x, z): sectionsAt's, or the
// world's Global table and the -near circle's own when it names them.
func (p *plan) sectionsFor(w int, x, z float64) []string {
	sections := p.sectionsAt(w, x, z)
	if s := p.nearSections(x, z); s != nil {
		var own []string
		for _, sec := range sections {
			if strings.HasSuffix(sec, "/Global") {
				own = append(own, sec)
			}
		}
		if len(own) == 0 && w == 0 {
			own = []string{"Tunaria/Global"}
		}
		sections = append(own, s...)
	}
	return sections
}

// questMinimum keeps each newbie quest's kill or loot step's creatures to
// questMin within questMinReach of each of its givers in the run's circles:
// groups of the step's names that are wildlife kinds, at an open spot of
// that reach far from any creature, at the place's layer within the kind's
// levels (the bestiary's for the place, else its creatures').
func (p *plan) questMinimum(land *landGrid, wp *worldPlan, open func(int) bool, rng *rand.Rand) []fill {
	if p.questMin <= 0 {
		return nil
	}
	w := land.world
	have := map[string][][2]float64{}
	for _, r := range p.in.rows {
		if r.world == w && creature(r) {
			have[r.key] = append(have[r.key], [2]float64{r.x, r.z})
		}
	}
	occupied := newIndex(100)
	for _, q := range wp.pointsAfter {
		occupied.add(q.x, q.z, q.v)
	}
	for _, f := range wp.fills {
		have[f.kind.key] = append(have[f.kind.key], [2]float64{f.x, f.z})
		occupied.add(f.x, f.z, -1)
	}
	var out []fill
	add := func(f fill) {
		out = append(out, f)
		occupied.add(f.x, f.z, -1)
		have[f.kind.key] = append(have[f.kind.key], [2]float64{f.x, f.z})
	}
	for _, st := range p.questSteps {
		if st.minLevel > questMinLevel {
			continue
		}
		var ks []*kind
		for _, n := range st.names {
			if k := p.kinds[n]; k != nil && k.made == nil {
				ks = append(ks, k)
			}
		}
		if len(ks) == 0 {
			continue
		}
		for _, g := range st.givers {
			if g.world != w || !p.inNear(g.x, g.z) {
				continue
			}
			count := func() int {
				n := 0
				for _, nm := range st.names {
					for _, s := range have[nm] {
						if math.Hypot(s[0]-g.x, s[1]-g.z) <= p.questMinReach {
							n++
						}
					}
				}
				return n
			}
			for i := 0; i < 20 && count() < p.questMin; i++ {
				k, ok := p.questSpot(land, open, occupied, g, rng)
				if !ok {
					break
				}
				kk := ks[i%len(ks)]
				x, z := land.xz(k)
				lo, hi := kk.levelRange(w)
				if plo, phi, listed := p.primaRange(kk.key, p.sectionsFor(w, x, z), lo); listed {
					lo, hi = plo, phi
				}
				place := p.layerLevel(x, z)
				if place == 0 {
					place = (lo + hi) / 2
				}
				place = max(lo, min(hi, place))
				levelOf := func() int { return max(1, max(lo, min(hi, place+[]int{-1, 0, 0, 1}[rng.Intn(4)]))) }
				fx, fy, fz := p.settle(land, k, rng)
				add(fill{kind: kk, template: template(kk, w, fx, fz), level: levelOf(), x: fx, y: fy, z: fz,
					facing: rng.Intn(256), source: "quest", placeLevel: place})
				group := [][2]float64{{fx, fz}}
				for n := p.groupSize(rng) - 1; n > 0; n-- {
					mx, my, mz, ok := p.member(land, open, fx, fy, fz, group, rng)
					if !ok {
						break
					}
					add(fill{kind: kk, template: template(kk, w, mx, mz), level: levelOf(), x: mx, y: my, z: mz,
						facing: rng.Intn(256), source: "quest", placeLevel: place})
					group = append(group, [2]float64{mx, mz})
				}
			}
		}
	}
	return out
}

// questSpot is an open sample within questMinReach of a giver (past 150 of
// it), the farthest from any creature of a few drawn, none nearer than 30.
func (p *plan) questSpot(land *landGrid, open func(int) bool, occupied *index, g place, rng *rand.Rand) (int, bool) {
	best, bestD := -1, 30.0
	for try := 0; try < 24; try++ {
		a := rng.Float64() * 2 * math.Pi
		r := 150 + rng.Float64()*(p.questMinReach-150)
		k := land.cell(g.x+r*math.Cos(a), g.z+r*math.Sin(a))
		if k < 0 || !open(k) {
			continue
		}
		x, z := land.xz(k)
		if d, _ := occupied.nearest(x, z, 500); d > bestD {
			best, bestD = k, d
		}
	}
	return best, best >= 0
}

// circle is a place a run is kept to (-near): within R of (X, Z); R0 is its
// town's own reach, from which -layers' distances count; Sections, when
// given, the bestiary's sections its wildlife comes from (with its world's
// Global table), in place of EQOA-Data's biome for the zone.
type circle struct {
	X, Z, R, R0 float64
	Sections    []string `json:",omitempty"`
}

// The shares of a -layers fill's pick: the wildlife round the place, the
// bestiary's kinds of the place's biome, and its Global table's; within each,
// a kind by its weight (the wildlife by nearness, twice when the bestiary
// lists it for the biome; a quest's kind questWeight times).
const (
	layerLocalShare  = 0.5
	layerBiomeShare  = 0.35
	layerGlobalShare = 0.15
)

// nearSections is the sections the circle (x, z) is in gives, nil for none.
func (p *plan) nearSections(x, z float64) []string {
	for _, c := range p.near {
		if len(c.Sections) > 0 && math.Hypot(x-c.X, z-c.Z) <= c.R {
			return c.Sections
		}
	}
	return nil
}

// layer is a place's level at a distance from a town's edge (-layers).
type layer struct {
	D     float64
	Level float64
}

// layerLevel is the level -layers gives (x, z): by its distance from the
// edge of the nearest town among the circles it is in, between the layers
// in a line, the last one's beyond them.
func (p *plan) layerLevel(x, z float64) int {
	d := math.Inf(1)
	for _, c := range p.near {
		if h := math.Hypot(x-c.X, z-c.Z); h <= c.R {
			d = math.Min(d, math.Max(0, h-c.R0))
		}
	}
	if math.IsInf(d, 1) || len(p.layers) == 0 {
		return 0
	}
	ls := p.layers
	if d <= ls[0].D {
		return int(math.Round(ls[0].Level))
	}
	for i := 1; i < len(ls); i++ {
		if d <= ls[i].D {
			f := (d - ls[i-1].D) / (ls[i].D - ls[i-1].D)
			return int(math.Round(ls[i-1].Level + f*(ls[i].Level-ls[i-1].Level)))
		}
	}
	return int(math.Round(ls[len(ls)-1].Level))
}

// levelRange is the levels of a kind's creatures in a world, 0 0 with none.
func (k *kind) levelRange(w int) (lo, hi int) {
	for _, r := range k.rows {
		if r.world != w || r.level <= 0 {
			continue
		}
		if lo == 0 || r.level < lo {
			lo = r.level
		}
		hi = max(hi, r.level)
	}
	return lo, hi
}

// place is a spot in a world.
type place struct {
	world int
	x, z  float64
}

// questReach is how far from a quest's giver its kill or loot step's kind
// counts questWeight times: a town's first quests send it this far.
const questReach = 2500.0

// inNear says whether (x, z) is in the run's places (every place without -near).
func (p *plan) inNear(x, z float64) bool {
	if len(p.near) == 0 {
		return true
	}
	for _, c := range p.near {
		if math.Hypot(x-c.X, z-c.Z) <= c.R {
			return true
		}
	}
	return false
}

// questNear says whether a quest whose giver stands within questReach of
// (x, z) in world w kills or loots the kind.
func (p *plan) questNear(key string, w int, x, z float64) bool {
	for _, g := range p.questGivers[key] {
		if g.world == w && math.Hypot(x-g.x, z-g.z) <= questReach {
			return true
		}
	}
	return false
}

// kind is a wildlife kind: every creature of a name that the server
// wanders, of a body of its own, more than one of it.
type kind struct {
	key      string
	spelling string // as most of its rows write it
	rows     []*npcRow
	spots    map[int]int // in how many spots of each world its creatures stand (chains within spotLink)
	made     *made       // a kind this server has no row of, made from a body (made.go); rows are its body's
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
	source     string // "local": the wildlife round it; "prima": the bestiary's for the place; "made": made.go's
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
		kinds: map[string]*kind{}, gapFill: gapFill, fillSpacing: fillSpacing, questWeight: 1}
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
		if p.noSpread {
			wp.kept["-no-spread: nothing moves"]++
			continue
		}
		if !p.inNear(r.x, r.z) {
			wp.kept["outside -near"]++
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
	// the zones left as they are (zones.go): nothing placed in them
	left := make([]bool, len(land.kind))
	for k := range left {
		if land.kind[k] == sLand {
			x, z := land.xz(k)
			if r := p.ruleAt(w, x, z); (r != nil && r.leave) || !p.inNear(x, z) {
				left[k] = true
			}
		}
	}
	open := func(k int) bool {
		if !land.placeable(k) || left[k] {
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
	// the rules' camps (zones.go), then the fill round them
	occupied := newIndex(100)
	for _, q := range wp.pointsAfter {
		occupied.add(q.x, q.z, q.v)
	}
	campFills := p.placeCamps(land, w, open, occupied, rng)
	for _, f := range campFills {
		wp.pointsAfter = append(wp.pointsAfter, pt{f.x, f.z, -1})
	}
	var gapFills []fill
	gapFills, wp.empty = p.fillGaps(land, wp, open, camps, rng)
	for _, f := range gapFills {
		wp.pointsAfter = append(wp.pointsAfter, pt{f.x, f.z, -1})
	}
	wp.fills = append(campFills, gapFills...)
	questFills := p.questMinimum(land, wp, open, rng)
	for _, f := range questFills {
		wp.pointsAfter = append(wp.pointsAfter, pt{f.x, f.z, -1})
	}
	wp.fills = append(wp.fills, questFills...)
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
	if z := p.ruleAt(r.world, r.x, r.z); z != nil && z.leave {
		return "in a zone left as it is (" + z.zone + ")"
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
// creatures do not stand on a grid) within the sample's zone, on the floor
// there, else the sample's own.
func (p *plan) settle(land *landGrid, k int, rng *rand.Rand) (x, y, z float64) {
	sx, sz := land.xz(k)
	sh := float64(land.h[k])
	for try := 0; try < 4; try++ {
		x = sx + (rng.Float64()*2-1)*jitter
		z = sz + (rng.Float64()*2-1)*jitter
		if zoneWord(land.world, x, z) != zoneWord(land.world, sx, sz) {
			continue
		}
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
		if k.made != nil {
			continue // its rows are its body's, not its own (made.go)
		}
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
		if d, _ := occupied.nearest(x, z, 5000); d > p.gapFill {
			gaps = append(gaps, gap{k, d})
		}
	}
	sort.Slice(gaps, func(a, b int) bool {
		if gaps[a].d != gaps[b].d {
			return gaps[a].d > gaps[b].d
		}
		return gaps[a].k < gaps[b].k
	})
	// the undead a zone of the dead copies (zones.go), where they stand
	type dead struct {
		k     *kind
		level int
	}
	var deads []dead
	deadIx := newIndex(250)
	dk := p.undeadKinds(w)
	deadKeys := make([]string, 0, len(dk))
	for key := range dk {
		deadKeys = append(deadKeys, key)
	}
	sort.Strings(deadKeys)
	for _, key := range deadKeys {
		for _, r := range dk[key].rows {
			deadIx.add(r.x, r.z, len(deads))
			deads = append(deads, dead{dk[key], r.level})
		}
	}
	var fills []fill
	empty := 0
	for _, gp := range gaps {
		x, z := land.xz(gp.k)
		if occupied.within(x, z, p.fillSpacing) {
			continue
		}
		place := localLevel(x, z)
		if len(p.layers) > 0 {
			place = p.layerLevel(x, z) // newbie ground's layers, not its creatures'
		}
		// a zone with creatures of its own (zones.go)
		if rule := p.ruleAt(w, x, z); rule != nil && (rule.undead || len(rule.kinds) > 0) {
			var k *kind
			var level int
			if zl := p.zoneLevel(w, x, z); zl > 0 {
				place = zl // the zone's own level, not its neighbours'
			}
			if rule.undead {
				weight := map[*kind]float64{}
				lvls := map[*kind][]weighed{}
				for reach := kindReach; reach <= 3*kindReach && len(weight) == 0; reach += kindReach {
					deadIx.near(x, z, reach, func(q pt, d float64) {
						dd := deads[q.v]
						weight[dd.k] += nearness(d)
						lvls[dd.k] = append(lvls[dd.k], weighed{dd.level, nearness(d)})
					})
				}
				var ks []*kind
				total := 0.0
				for kk, wt := range weight {
					if place > 0 && abs(weightedMedian(lvls[kk])-place) > levelBand {
						continue
					}
					ks = append(ks, kk)
					total += wt
				}
				sort.Slice(ks, func(a, b int) bool { return ks[a].key < ks[b].key })
				pick := rng.Float64() * total
				for _, kk := range ks {
					pick -= weight[kk]
					if pick <= 0 {
						k = kk
						break
					}
				}
				if k == nil && len(ks) > 0 {
					k = ks[len(ks)-1]
				}
				if k != nil {
					level = max(1, weightedMedian(lvls[k])+[]int{-1, 0, 0, 1}[rng.Intn(4)])
				}
			} else {
				k, level = p.pickRuleKind(rule, nil, rng)
			}
			if k == nil {
				empty++
				continue
			}
			fx, fy, fz := p.settle(land, gp.k, rng)
			src := "zone"
			if rule.undead {
				src = "undead"
			}
			fills = append(fills, fill{kind: k, template: template(k, w, fx, fz), level: level,
				x: fx, y: fy, z: fz, facing: rng.Intn(256), source: src, placeLevel: place})
			occupied.add(fx, fz, -1)
			continue
		}
		sections := p.sectionsFor(w, x, z)
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
			lo, hi int    // the bestiary's levels, 0 0 for none
			src    string // where it came from, when the run's layers drew it
		}
		var cands []cand
		if len(p.layers) > 0 && place > 0 {
			// the layer's level: a kind whose levels (the bestiary's for the
			// place, else its creatures' in the world) take it in, from three
			// groups each with its share of the pick (layerLocalShare...)
			var biome, global []string
			for _, s := range sections {
				if strings.HasSuffix(s, "/Global") {
					global = append(global, s)
				} else {
					biome = append(biome, s)
				}
			}
			type layered struct {
				c     cand
				group int // 0 the wildlife round it, 1 the biome's, 2 the Global table's
				raw   float64
			}
			var ls []layered
			seen := map[*kind]bool{}
			consider := func(k *kind, raw float64, local bool) {
				if seen[k] {
					return
				}
				group := 1
				lo, hi, listed := p.primaRange(k.key, biome, place)
				inBiome := listed
				if !listed {
					group = 2
					if lo, hi, listed = p.primaRange(k.key, global, place); !listed {
						if !local {
							return
						}
						if lo, hi = k.levelRange(w); lo == 0 || k.spotsIn(w) < 2 {
							return
						}
					}
				}
				if place < lo-1 || place > hi+1 {
					return
				}
				if local {
					group = 0
					if inBiome {
						raw *= 2
					}
				}
				if p.questWeight != 1 && p.questNear(k.key, w, x, z) {
					raw *= p.questWeight
				}
				seen[k] = true
				ls = append(ls, layered{cand{k, 0, place, lo, hi, [...]string{"local", "biome", "global"}[group]}, group, raw})
			}
			for k, wt := range weight {
				consider(k, wt, true)
			}
			for _, key := range keys {
				consider(p.kinds[key], 1, false)
			}
			var tot [3]float64
			for _, l := range ls {
				tot[l.group] += l.raw
			}
			share := [3]float64{layerLocalShare, layerBiomeShare, layerGlobalShare}
			for _, l := range ls {
				l.c.w = share[l.group] * l.raw / tot[l.group]
				cands = append(cands, l.c)
			}
			weight = nil
		}
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
			if p.questWeight != 1 && p.questNear(k.key, w, x, z) {
				wt *= p.questWeight
			}
			cands = append(cands, cand{k, wt, lk, lo, hi, ""})
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
				wt := 1.0
				if p.questWeight != 1 && p.questNear(key, w, x, z) {
					wt = p.questWeight
				}
				cands = append(cands, cand{k, wt, max(lo, min(hi, place)), lo, hi, ""})
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
		pickOne := func() cand {
			pick := rng.Float64() * total
			for _, c := range cands {
				if pick -= c.w; pick <= 0 {
					return c
				}
			}
			return cands[len(cands)-1]
		}
		levelOf := func(c cand) int {
			level := c.level + []int{-1, 0, 0, 1}[rng.Intn(4)]
			if c.hi > 0 {
				level = max(c.lo, min(c.hi, level))
			}
			return max(1, level)
		}
		sourceOf := func(c cand) string {
			from := source
			if c.src != "" {
				from = c.src
			}
			if c.k.made != nil {
				from = "made"
			}
			return from
		}
		chosen := pickOne()
		level := levelOf(chosen)
		fx, fy, fz := p.settle(land, gp.k, rng)
		fills = append(fills, fill{kind: chosen.k, template: template(chosen.k, w, fx, fz), level: level,
			x: fx, y: fy, z: fz, facing: rng.Intn(256), source: sourceOf(chosen), placeLevel: place})
		occupied.add(fx, fz, -1)
		// the rest of its group (-group), each a kind drawn again
		group := [][2]float64{{fx, fz}}
		for n := p.groupSize(rng) - 1; n > 0; n-- {
			mx, my, mz, ok := p.member(land, open, fx, fy, fz, group, rng)
			if !ok {
				break
			}
			c := pickOne()
			fills = append(fills, fill{kind: c.k, template: template(c.k, w, mx, mz), level: levelOf(c),
				x: mx, y: my, z: mz, facing: rng.Intn(256), source: sourceOf(c), placeLevel: place})
			occupied.add(mx, mz, -1)
			group = append(group, [2]float64{mx, mz})
		}
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
