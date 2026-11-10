// Command worldedges closes the open edges of a composed world's zones
// (cmd/worldbuild) with the disc's own rocks, and checks that nothing is
// left open.
//
// A zone copied out of a disc world stands alone where it stood: its terrain
// ends at its cell's edge, where the world it came from went on, and a player
// walking (or swimming) past it falls into nothing, down to the world's floor.
// An edge where the zone touches another of the world's zones (Odus's homes,
// which stand side by side as on the disc) leads into the neighbour and stays
// open. Every other edge is given a ridge of rocks (extras: the disc's art
// placed as actors, internal/worlds/extras.go): pieces of a palette chosen by
// the zone's disc file, big, turned at random, each set down into the ground
// (the lowest floor under it) and tall enough to stand well over the highest
// floor under it and over the water's surface, overlapping by half their
// width, from corner to corner.
//
//	worldedges -iso disc.iso -built worlds/world10.json -ground worlds/ground \
//	    -palette tools/worlds/edges-palette.json -out tools/worlds/homelands-edges.json
//
// writes the extras file the recipe takes in (its extraFiles); then build the
// world and bake its ground again, and
//
//	worldedges -check -built worlds/world10.json -ground worlds/ground
//
// walks the server's ground (internal/ground: the terrain, everything placed
// on it, the water) along every open edge as the movement check judges a
// player's moves (entity/movecheck.go): from every place a band's width inside
// the edge, a step of a unit at a time in four directions, onto a floor no
// more than a step's climb above (1:1 plus 0.13) or falling to the next one
// down, swimming anywhere in a water's column, never through the front of a
// face at the body's middle. A way out is a step that reaches past the edge,
// or one with nothing under it; each is reported, and the command fails.
// The test is a point's, where the game's body is a ball 1.2 across: a gap
// narrower than that the check reports, and the game does not let through.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"

	"image"
	"image/color"
	"image/png"

	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/worlds"
)

const cell = 2000

// footMargin is how far below the lowest floor under it a rock's foot goes.
const footMargin = 1.5

var (
	trace     bool
	traceNear [2]float32
	mapOut    string
)

func main() {
	iso := flag.String("iso", "", "the game's disc image (for the palette's sizes)")
	builtPath := flag.String("built", "", "the world as built (worldN.json)")
	groundDir := flag.String("ground", "", "the world's ground (cmd/groundbake's folder)")
	palettePath := flag.String("palette", "", "the rocks to use (tools/worlds/edges-palette.json)")
	out := flag.String("out", "", "the extras file to write")
	check := flag.Bool("check", false, "walk the ground along every open edge and report each way out")
	band := flag.Float64("band", 100, "how far inside an edge the check starts (and the ridge stands)")
	seed := flag.Int64("seed", 1, "the layout's random seed")
	workers := flag.Int("workers", 4, "zones checked at once")
	only := flag.Int("zone", -1, "one zone only (its index in the world)")
	zonesFlag := flag.String("zones", "", "with -check: these zones only (indices, a comma between)")
	leaks := flag.String("leaks", "", "with -check: write the ways out found to this file (for -fix)")
	fix := flag.String("fix", "", "add rocks over the ways out this file names (-check -leaks) to the extras file -out names")
	flag.BoolVar(&trace, "trace", false, "print the way to the first ways out")
	near := flag.String("near", "", "with -trace: only ways out near x,z")
	flag.StringVar(&mapOut, "map", "", "with -near: a picture of the walk round there")
	flag.Parse()
	if *builtPath == "" || *groundDir == "" || !*check && (*iso == "" || *palettePath == "" || *out == "") {
		flag.Usage()
		os.Exit(2)
	}
	b, err := worlds.ReadBuilt(*builtPath)
	if err != nil {
		log.Fatal(err)
	}
	if *near != "" {
		fmt.Sscanf(*near, "%f,%f", &traceNear[0], &traceNear[1])
	}
	if *check {
		var some map[int]bool
		if *zonesFlag != "" {
			some = map[int]bool{}
			for _, z := range strings.Split(*zonesFlag, ",") {
				var i int
				if _, err := fmt.Sscan(strings.TrimSpace(z), &i); err == nil {
					some[i] = true
				}
			}
		}
		n := checkWorld(b, *groundDir, float32(*band), *workers, *only, some, *leaks)
		if n > 0 {
			log.Fatalf("%d ways out", n)
		}
		return
	}
	pal, err := readPalette(*palettePath)
	if err != nil {
		log.Fatal(err)
	}
	d, err := worlds.OpenDisc(*iso)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()
	for _, set := range pal.Sets {
		for i := range set {
			box, err := worlds.ArtBox(d, set[i].extra())
			if err != nil {
				log.Fatalf("palette %08x: %v", uint32(set[i].Sprite), err)
			}
			set[i].box = box
		}
	}
	g, err := ground.Open(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	env, err := ground.OpenEnv(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	if *fix != "" {
		fixLeaks(g, env, b, pal, *fix, *out, *seed)
		return
	}
	f := worlds.ExtrasFile{Note: "Rocks along the open edges of each zone, so no player walks or swims off a zone into nothing; written by cmd/worldedges (its palette: edges-palette.json) and checked by its -check on the server's ground. Edges where two zones touch stay open."}
	total := 0
	for i, p := range b.Parts {
		if *only >= 0 && i != *only {
			continue
		}
		set := pal.setFor(p.From, p.Zone)
		if len(set) == 0 {
			log.Fatalf("zone %d (%s %d): no palette", i, p.From, p.Zone)
		}
		var extras []worlds.Extra
		for k, e := range openEdges(b, i, g) {
			rng := rand.New(rand.NewSource(*seed + int64(i)*100 + int64(k)))
			extras = append(extras, ridge(g, env, uint8(b.World), e, set, rng, pal)...)
		}
		f.Zones = append(f.Zones, worlds.ExtrasOfOne{From: p.From, Zone: p.Zone, Name: b.Zones[i].Name, Extras: extras})
		total += len(extras)
		log.Printf("zone %2d %-20s %d open edges, %d rocks", i, b.Zones[i].Name, len(openEdges(b, i, g)), len(extras))
	}
	j, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(compact(j), '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: %d rocks in %d zones", *out, total, len(f.Zones))
}

// fixLeaks adds rocks over the ways out in leaksPath to the extras file at
// path, zone by zone.
func fixLeaks(g *ground.Ground, env *ground.Env, b worlds.Built, pal *palette, leaksPath, path string, seed int64) {
	raw, err := os.ReadFile(leaksPath)
	if err != nil {
		log.Fatal(err)
	}
	var lf leaksFile
	if err := json.Unmarshal(raw, &lf); err != nil {
		log.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var f worlds.ExtrasFile
	if err := json.Unmarshal(raw, &f); err != nil {
		log.Fatal(err)
	}
	added := 0
	for _, zl := range lf.Zones {
		p := b.Parts[zl.Zone]
		set := pal.setFor(p.From, p.Zone)
		k := -1
		for i, z := range f.Zones {
			if strings.EqualFold(z.From, p.From) && z.Zone == p.Zone {
				k = i
			}
		}
		if k < 0 {
			f.Zones = append(f.Zones, worlds.ExtrasOfOne{From: p.From, Zone: p.Zone, Name: b.Zones[zl.Zone].Name})
			k = len(f.Zones) - 1
		}
		for si, st := range zl.Stretches {
			rng := rand.New(rand.NewSource(seed*7919 + int64(zl.Zone)*1000 + int64(si) + int64(len(f.Zones[k].Extras))))
			more := fill(g, env, b, zl.Zone, st, set, rng, pal)
			f.Zones[k].Extras = append(f.Zones[k].Extras, more...)
			added += len(more)
		}
	}
	j, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, append(compact(j), '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: %d rocks added over %d zones' ways out", path, added, len(lf.Zones))
}

// compact puts each extra on a line of its own (a long file a person still
// reads by eye).
func compact(j []byte) []byte {
	var out strings.Builder
	lines := strings.Split(string(j), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "{" && i+1 < len(lines) && strings.Contains(lines[i+1], `"sprite"`) {
			var parts []string
			k := i + 1
			for ; k < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[k]), "}"); k++ {
				parts = append(parts, strings.TrimSpace(lines[k]))
			}
			inner := strings.Join(parts, " ")
			inner = strings.ReplaceAll(strings.ReplaceAll(inner, "[ ", "["), " ]", "]")
			out.WriteString(strings.Repeat(" ", len(l)-len(strings.TrimLeft(l, " "))) + "{" + inner + strings.TrimSpace(lines[k]) + "\n")
			i = k
			continue
		}
		out.WriteString(l + "\n")
	}
	return []byte(strings.TrimRight(out.String(), "\n"))
}

// piece is a rock of a palette: the art, how often it is chosen, and its box
// in its own space (off the disc).
type piece struct {
	Sprite worlds.DictID `json:"sprite"`
	From   string        `json:"from"`
	Zone   int           `json:"zone"`
	Weight float64       `json:"weight"`
	// Foot is the share of its height, from its bottom, at which it is
	// widest: where it is set into the ground. Steep is the share of its
	// height, from its bottom, too steep to walk up (none given: 0.6 and 0).
	Foot  float32 `json:"foot,omitempty"`
	Steep float32 `json:"steep,omitempty"`
	Note  string  `json:"note,omitempty"`
	box   [6]float32
}

func (p piece) extra() worlds.Extra {
	return worlds.Extra{Sprite: p.Sprite, From: p.From, Zone: p.Zone}
}

// palette is the rocks for each zone: a set by name, chosen by the zone's
// disc file and index ("TUNARIA.ESF#160"), else its disc file.
type palette struct {
	Note  string             `json:"note,omitempty"`
	Sets  map[string][]piece `json:"sets"`
	Zones map[string]string  `json:"zones"`
	// Width is the span of a rock along the edge (min, max), Rise how far
	// its top stands over the highest floor (or water) under it, MaxScale
	// the most it is grown.
	Width    [2]float32 `json:"width"`
	Rise     float32    `json:"rise"`
	MaxScale float32    `json:"maxScale"`
	// SteepGround is the rise and fall of the ground under a rock past
	// which the set's steepest is used.
	SteepGround float32 `json:"steepGround"`
}

func readPalette(path string) (*palette, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p palette
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if p.Width[0] <= 0 || p.Width[1] < p.Width[0] || p.Rise <= 0 || p.MaxScale <= 0 {
		return nil, fmt.Errorf("%s: width, rise and maxScale", path)
	}
	return &p, nil
}

func (p *palette) setFor(from string, zone int) []piece {
	if name, ok := p.Zones[fmt.Sprintf("%s#%d", strings.ToUpper(from), zone)]; ok {
		return p.Sets[name]
	}
	return p.Sets[p.Zones[strings.ToUpper(from)]]
}

// edge is an open edge of a zone's cell: from a to a+u*cell, out its
// outward normal (unit, along x or z), in the zone's cell.
type edge struct {
	ax, az float32
	ux, uz float32
	nx, nz float32
	x0, z0 float32 // the cell's corner
}

// openEdges are zone i's edges no other zone of the world touches.
func openEdges(b worlds.Built, i int, g *ground.Ground) []edge {
	cells := map[[2]int]bool{}
	for _, p := range b.Parts {
		cells[[2]int{p.Col, p.Row}] = true
	}
	p := b.Parts[i]
	x0, z0 := float32(p.Col*cell), float32(p.Row*cell)
	all := []struct {
		nb [2]int
		e  edge
	}{
		{[2]int{p.Col, p.Row - 1}, edge{ax: x0, az: z0, ux: 1, nz: -1}},
		{[2]int{p.Col + 1, p.Row}, edge{ax: x0 + cell, az: z0, uz: 1, nx: 1}},
		{[2]int{p.Col, p.Row + 1}, edge{ax: x0 + cell, az: z0 + cell, ux: -1, nz: 1}},
		{[2]int{p.Col - 1, p.Row}, edge{ax: x0, az: z0 + cell, uz: -1, nx: -1}},
	}
	var out []edge
	for _, a := range all {
		a.e.x0, a.e.z0 = x0, z0
		if !cells[a.nb] || g != nil && !linesUp(g, uint8(b.World), a.e) {
			out = append(out, a.e)
		}
	}
	return out
}

// linesUp reports whether the ground meets across edge e, where another
// zone of the world stands: the floors a quarter of a unit either side of it
// within 3 of each other all along it, but for a place in fifty (a sample on
// a vertex of the terrain can miss its triangles). Zones side by side on the
// disc share their edge's vertices (the Homelands' Odus homes: 0 of 2,000
// places differ); two of different worlds meet in steps, which a ridge closes
// as it closes an edge onto nothing.
func linesUp(g *ground.Ground, w uint8, e edge) bool {
	off, n := 0, 0
	for t := float32(2.5); t < cell; t += 8 {
		x, z := e.ax+e.ux*t, e.az+e.uz*t
		a, oka := floorAt(g, w, x-e.nx*0.25, z-e.nz*0.25)
		b, okb := floorAt(g, w, x+e.nx*0.25, z+e.nz*0.25)
		n++
		if oka != okb || oka && abs32(a-b) > 3 {
			off++
		}
	}
	return off*50 <= n
}

// floorAt is the terrain's highest floor at (x, z), false where it has none.
func floorAt(g *ground.Ground, w uint8, x, z float32) (float32, bool) {
	return g.Floor(w, x, 4000, z)
}

// waterTop is the top of the water at (x, z) over the floor y, false when
// there is none there.
func waterTop(env *ground.Env, w uint8, x, y, z float32) (float32, bool) {
	fl, ok := env.Flags(w, x, y+1.2, z)
	if !ok || fl&ground.EnvWater == 0 {
		return 0, false
	}
	top := y + 1.2
	for top < y+600 {
		if fl, ok := env.Flags(w, x, top+1, z); !ok || fl&ground.EnvWater == 0 {
			break
		}
		top++
	}
	return top, true
}

// ridge is the rocks along edge e: from its corner to past its end, each
// chosen, sized, turned and set down.
func ridge(g *ground.Ground, env *ground.Env, w uint8, e edge, set []piece, rng *rand.Rand, pal *palette) []worlds.Extra {
	total := 0.0
	for _, p := range set {
		total += p.Weight
	}
	var out []worlds.Extra
	for t := float32(0); t <= cell+1; {
		r := rng.Float64() * total
		p := set[len(set)-1]
		for _, q := range set {
			if r -= q.Weight; r < 0 {
				p = q
				break
			}
		}
		width := pal.Width[0] + float32(rng.Float64())*(pal.Width[1]-pal.Width[0])
		// on steep ground the steepest rock (a rounded one set to its
		// widest grows into a hill there; a cone, widest at its foot, less)
		if relief(g, w, e, t, width) > pal.SteepGround {
			p = steepest(set)
		}
		ex, span := place(g, env, w, e, t, p, width, 0.3, 0, rng, pal)
		out = append(out, ex)
		t += span * 0.5
	}
	return out
}

// relief is how far the ground rises and falls within width of the edge's
// point t, inside the cell.
func relief(g *ground.Ground, w uint8, e edge, t, width float32) float32 {
	lo, hi := float32(math.MaxFloat32), float32(-math.MaxFloat32)
	for a := -2; a <= 2; a++ {
		for c := 0; c <= 4; c++ {
			x := e.ax + e.ux*(t+float32(a)*width/4) - e.nx*float32(c)*width/4
			z := e.az + e.uz*(t+float32(a)*width/4) - e.nz*float32(c)*width/4
			x = min(max(x, e.x0+0.5), e.x0+cell-0.5)
			z = min(max(z, e.z0+0.5), e.z0+cell-0.5)
			if y, ok := floorAt(g, w, x, z); ok {
				lo, hi = min(lo, y), max(hi, y)
			}
		}
	}
	if lo > hi {
		return 0
	}
	return hi - lo
}

// steepest is the rock of a set whose steep share stands most over its foot.
func steepest(set []piece) piece {
	p := set[0]
	for _, q := range set {
		if q.Steep-q.Foot > p.Steep-p.Foot {
			p = q
		}
	}
	return p
}

// place is a rock p set at t along edge e: width its span along the edge
// (scaled up as it must be), its middle in of its span inside the edge, its
// top lift more over everything than the palette's rise; it says the span.
func place(g *ground.Ground, env *ground.Env, w uint8, e edge, t float32, p piece, width, in, lift float32, rng *rand.Rand, pal *palette) (worlds.Extra, float32) {
	bw, bd, bh := p.box[3]-p.box[0], p.box[5]-p.box[2], p.box[4]-p.box[1]
	narrow := min(bw, bd)
	scale := width / narrow
	heading := float32(rng.Float64() * 2 * math.Pi)
	pitch := float32((rng.Float64() - 0.5) * 0.12)
	roll := float32((rng.Float64() - 0.5) * 0.12)
	// where it stands: its middle a third of its span inside the edge;
	// how big: set into the ground to its widest (Foot, a share of its
	// height: below it a rounded rock leaves a gap over the ground its
	// neighbour does not close), its steep sides (to Steep of its height,
	// what a walk does not climb) standing over every floor it touches
	// and the water, its top over them by Rise
	steep, foot := p.Steep, p.Foot
	if steep <= foot {
		steep, foot = 0.6, 0
	}
	var x, z, base float32
	for pass := 0; pass < 8; pass++ {
		span := narrow * scale
		reach := max(bw, bd) * scale / 2
		x, z = e.ax+e.ux*t-e.nx*in*span, e.az+e.uz*t-e.nz*in*span
		lo, hi := float32(math.MaxFloat32), float32(-math.MaxFloat32)
		top := float32(-math.MaxFloat32)
		for a := -4; a <= 4; a++ {
			for c := -4; c <= 4; c++ {
				ox, oz := float32(a)/4*reach, float32(c)/4*reach
				if ox*ox+oz*oz > reach*reach*1.0001 {
					continue
				}
				// past the edge there is no ground: the edge's stands for it
				sx := min(max(x+ox, e.x0+0.5), e.x0+cell-0.5)
				sz := min(max(z+oz, e.z0+0.5), e.z0+cell-0.5)
				y, ok := floorAt(g, w, sx, sz)
				if !ok {
					continue
				}
				lo, hi = min(lo, y), max(hi, y)
				if wt, ok := waterTop(env, w, sx, y, sz); ok {
					top = max(top, wt)
				}
			}
		}
		if lo == math.MaxFloat32 {
			lo, hi = 0, 0
		}
		over := max(hi, top)
		height := bh * scale
		base = lo - foot*height - footMargin
		need := max((over+3+lift+footMargin-lo)/(steep-foot), (over+pal.Rise+lift+footMargin-lo)/(1-foot))
		if height >= need || scale >= pal.MaxScale {
			break
		}
		scale = min(need/bh*1.02, pal.MaxScale)
	}
	// the actor's place: its box's middle at (x, z), its bottom at base
	cx, cz := (p.box[0]+p.box[3])/2*scale, (p.box[2]+p.box[5])/2*scale
	sn, cs := math.Sincos(float64(heading))
	ox := float32(float64(cx)*cs + float64(cz)*sn)
	oz := float32(-float64(cx)*sn + float64(cz)*cs)
	ex := p.extra()
	ex.At = [3]float32{round(x - ox), round(base - p.box[1]*scale), round(z - oz)}
	ex.HPR = [3]float32{round3(heading), round3(pitch), round3(roll)}
	ex.Scale = round3(scale)
	return ex, narrow * scale
}

func round(v float32) float32  { return float32(math.Round(float64(v)*10) / 10) }
func round3(v float32) float32 { return float32(math.Round(float64(v)*1000) / 1000) }

// --- the check -------------------------------------------------------------

// stretch is a stretch of an open edge a way out crosses: which edge ("z-"
// the cell's low z side, "x+" its high x side, ...), from t0 to t1 along it
// (from the cell's low corner), the feet's heights there, how many places.
type stretch struct {
	Edge string  `json:"edge"`
	T0   float32 `json:"t0"`
	T1   float32 `json:"t1"`
	Ylo  float32 `json:"ylo"`
	Yhi  float32 `json:"yhi"`
	N    int     `json:"n"`
}

// column is a place's surfaces and water, read once.
type column struct {
	surfs []ground.Surface
	water float32 // the top of its water, NaN for none
}

// checkWorld walks every zone's open edges; it returns the ways out found.
func checkWorld(b worlds.Built, dir string, band float32, workers, only int, some map[int]bool, leaksOut string) int {
	type result struct {
		zone   int
		lines  []string
		leaks  int
		nodes  int
		ranges []stretch
		edges  int
	}
	jobs := make(chan int)
	results := make(chan result)
	var wg sync.WaitGroup
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := ground.Open(dir)
			if err != nil {
				log.Fatal(err)
			}
			env, err := ground.OpenEnv(dir)
			if err != nil {
				log.Fatal(err)
			}
			for i := range jobs {
				ranges, nodes, lines := walkZone(g, env, b, i, band)
				results <- result{i, lines, len(ranges), nodes, ranges, len(openEdges(b, i, g))}
			}
		}()
	}
	go func() {
		for i := range b.Parts {
			if (only < 0 || i == only) && (some == nil || some[i]) {
				jobs <- i
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	var all []result
	for r := range results {
		all = append(all, r)
		log.Printf("zone %d walked: %d places, %d stretches open", r.zone, r.nodes, r.leaks)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].zone < all[j].zone })
	leaks := 0
	for _, r := range all {
		verdict := "closed"
		if r.leaks > 0 {
			verdict = fmt.Sprintf("%d ways out", r.leaks)
		}
		fmt.Printf("zone %2d %-22s %d open edges, %d places walked: %s\n", r.zone, b.Zones[r.zone].Name, r.edges, r.nodes, verdict)
		for _, l := range r.lines {
			fmt.Println("   ", l)
		}
		leaks += r.leaks
	}
	if leaksOut != "" {
		var f leaksFile
		for _, r := range all {
			if len(r.ranges) > 0 {
				f.Zones = append(f.Zones, zoneLeaks{Zone: r.zone, Name: b.Zones[r.zone].Name, Stretches: r.ranges})
			}
		}
		j, _ := json.MarshalIndent(f, "", " ")
		if err := os.WriteFile(leaksOut, append(j, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	return leaks
}

// leaksFile is what -check -leaks writes and -fix reads: each zone's ways out.
type leaksFile struct {
	Zones []zoneLeaks `json:"zones"`
}

type zoneLeaks struct {
	Zone      int       `json:"zone"`
	Name      string    `json:"name"`
	Stretches []stretch `json:"stretches"`
}

// edgeOf is the cell's edge a stretch names: its line from the cell's low
// corner along x or z, and out of it.
func edgeOf(p worlds.BuiltZone, name string) (edge, bool) {
	x0, z0 := float32(p.Col*cell), float32(p.Row*cell)
	e := edge{x0: x0, z0: z0}
	switch name {
	case "z-":
		e.ax, e.az, e.ux, e.nz = x0, z0, 1, -1
	case "z+":
		e.ax, e.az, e.ux, e.nz = x0, z0+cell, 1, 1
	case "x-":
		e.ax, e.az, e.uz, e.nx = x0, z0, 1, -1
	case "x+":
		e.ax, e.az, e.uz, e.nx = x0+cell, z0, 1, 1
	default:
		return e, false
	}
	return e, true
}

// fill is rocks over each stretch a way out crossed: the palette's steepest,
// at its widest, closer to the edge and taller than the ridge's, every so
// often from before the stretch to past it.
func fill(g *ground.Ground, env *ground.Env, b worlds.Built, zi int, st stretch, set []piece, rng *rand.Rand, pal *palette) []worlds.Extra {
	e, ok := edgeOf(b.Parts[zi], st.Edge)
	if !ok {
		return nil
	}
	p := steepest(set)
	var out []worlds.Extra
	for t := st.T0 - 24; t <= st.T1+24; t += 24 {
		if t < -10 || t > cell+10 {
			continue
		}
		ex, _ := place(g, env, uint8(b.World), e, t, p, pal.Width[1], 0.15, 12, rng, pal)
		out = append(out, ex)
	}
	return out
}

// A step's rules (entity/movecheck.go; CLIENT's VIWalkControl): the body's
// middle 1.2 over its feet; a floor no more than a step's climb above the
// feet is stood on (1:1 and 0.13, a unit's step), one no more than 0.6 below
// is stepped down to, and with none of those the body falls to the next
// floor down; in water it swims anywhere in the column.
const (
	middle   = 1.2
	climb    = 1.13
	maxSlope = 1.15 // the steepest floor stood on (49 degrees)
	stepOut  = 20   // how far past an edge the check looks
)

// meets reports whether a step from feet (x, y, z) to feet (nx, ny, nz)
// passes through the front of a face anywhere the game's body is: its ball
// is 0.6 round at 1.2 over the feet, so the way is tried at its middle and a
// little under and over it (a point at the middle alone grazes past a face
// the ball would meet).
func meets(g *ground.Ground, w uint8, x, y, z, nx, ny, nz float32) bool {
	for _, h := range [3]float32{middle, 0.9, 1.8} {
		if g.Crosses(w, x, y+h, z, nx, ny+h, nz) != ground.CrossNone {
			return true
		}
	}
	return false
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// state is a place walked: a node of the grid and the height of the feet.
type state struct {
	i, j int32
	y    int32 // feet, in tenths
}

// walkZone floods zone i's band along its open edges from its inner side
// and reports every way out: a step past an open edge, or onto nothing.
func walkZone(g *ground.Ground, env *ground.Env, b worlds.Built, i int, band float32) ([]stretch, int, []string) {
	w := uint8(b.World)
	p := b.Parts[i]
	x0, z0 := float32(p.Col*cell), float32(p.Row*cell)
	edges := openEdges(b, i, g)
	if len(edges) == 0 {
		return nil, 0, nil
	}
	open := map[[2]float32]bool{} // an edge by its outward normal
	for _, e := range edges {
		open[[2]float32{e.nx, e.nz}] = true
	}
	// the grid: the cell and stepOut past it on the open sides; a node is in
	// the band when it is within band of an open edge (inside) or past one
	lo, hi := float32(-stepOut), float32(cell+stepOut)
	n := int32(hi - lo + 1)
	at := func(i, j int32) (float32, float32) { return x0 + lo + float32(i), z0 + lo + float32(j) }
	past := func(x, z float32) bool {
		return x < x0 && open[[2]float32{-1, 0}] || x > x0+cell && open[[2]float32{1, 0}] ||
			z < z0 && open[[2]float32{0, -1}] || z > z0+cell && open[[2]float32{0, 1}]
	}
	beyondShared := func(x, z float32) bool {
		return x < x0 && !open[[2]float32{-1, 0}] || x > x0+cell && !open[[2]float32{1, 0}] ||
			z < z0 && !open[[2]float32{0, -1}] || z > z0+cell && !open[[2]float32{0, 1}]
	}
	depth := func(x, z float32) float32 { // how far inside the nearest open edge
		d := float32(math.MaxFloat32)
		if open[[2]float32{-1, 0}] {
			d = min(d, x-x0)
		}
		if open[[2]float32{1, 0}] {
			d = min(d, x0+cell-x)
		}
		if open[[2]float32{0, -1}] {
			d = min(d, z-z0)
		}
		if open[[2]float32{0, 1}] {
			d = min(d, z0+cell-z)
		}
		return d
	}
	inBand := func(x, z float32) bool { return !beyondShared(x, z) && depth(x, z) <= band }

	// each place's surfaces and water, read once
	cols := map[[2]int32]*column{}
	colAt := func(i, j int32) *column {
		if c := cols[[2]int32{i, j}]; c != nil {
			return c
		}
		x, z := at(i, j)
		c := &column{surfs: g.Surfaces(w, x, z, -2000, 4000), water: float32(math.NaN())}
		low, any := float32(0), false
		for _, s := range c.surfs {
			if s.Up && (!any || s.Y < low) {
				low, any = s.Y, true
			}
		}
		if !any {
			low = -100
		}
		if wt, ok := waterTop(env, w, x, low, z); ok {
			c.water = wt
		}
		cols[[2]int32{i, j}] = c
		return c
	}

	// steepAt: whether the floor at (i, j) at height y slopes more than a
	// walk stands on (the game's ball, 0.6 round at 1.2 over the feet, meets
	// anything steeper and slides off it): the floors nearest y a step
	// either way along x and z
	steepAt := func(i, j int32, y float32) bool {
		near := func(i, j int32) (float32, bool) {
			if i < 0 || j < 0 || i >= n || j >= n {
				return 0, false
			}
			b, ok := float32(0), false
			for _, sf := range colAt(i, j).surfs {
				if sf.Up && abs32(sf.Y-y) <= 3 && (!ok || abs32(sf.Y-y) < abs32(b-y)) {
					b, ok = sf.Y, true
				}
			}
			return b, ok
		}
		grad := func(a, b int32, c, d int32) float32 {
			ya, oka := near(a, b)
			yb, okb := near(c, d)
			switch {
			case oka && okb:
				return (yb - ya) / 2
			case oka:
				return y - ya
			case okb:
				return yb - y
			}
			return 0
		}
		gx, gz := grad(i-1, j, i+1, j), grad(i, j-1, i, j+1)
		return gx*gx+gz*gz > maxSlope*maxSlope
	}

	seen := map[state]bool{}
	from := map[state]state{} // with -trace: the step each state was reached by
	var queue []state
	var cur state
	push := func(s state) {
		if !seen[s] {
			seen[s] = true
			queue = append(queue, s)
			if trace {
				from[s] = cur
			}
		}
	}
	tenth := func(y float32) int32 { return int32(math.Round(float64(y) * 10)) }
	// the seeds: the band's inner side, every floor of the terrain there
	for ii := int32(0); ii < n; ii++ {
		for jj := int32(0); jj < n; jj++ {
			x, z := at(ii, jj)
			if !inBand(x, z) || past(x, z) || depth(x, z) < band-1 {
				continue
			}
			// a player stands on the terrain there (on anything placed only
			// once it has walked onto it: a rock's top a seed would stand on
			// is no place a player gets to)
			// and not under anything placed there (a seed on the ground
			// inside a rock would walk out of it, through its back faces)
			c := colAt(ii, jj)
			under := false
			for _, s := range c.surfs {
				if s.Placed {
					under = true
				}
			}
			for _, s := range c.surfs {
				if s.Up && !s.Placed && !under {
					push(state{ii, jj, tenth(s.Y)})
				}
			}
			if !math.IsNaN(float64(c.water)) && !under {
				push(state{ii, jj, tenth(c.water - middle - 0.3)})
			}
		}
	}
	// the ways out, by place: (x, z) once each
	outs := map[[2]int32]float32{}
	traced := 0
	report := func(s state, why string) {
		k := [2]int32{s.i, s.j}
		if _, ok := outs[k]; !ok {
			outs[k] = float32(s.y) / 10
		}
		lx, lz := at(s.i, s.j)
		if trace && traced < 3 && (traceNear[0] == 0 || abs32(lx-traceNear[0]) < 30 && abs32(lz-traceNear[1]) < 30) {
			traced++
			fmt.Printf("way out (%s), backwards:\n", why)
			p := cur
			for k := 0; k < 400; k++ {
				x, z := at(p.i, p.j)
				if k < 400 {
					on := "air"
					for _, sf := range colAt(p.i, p.j).surfs {
						if sf.Up && math.Abs(float64(sf.Y-float32(p.y)/10)) < 0.06 {
							on = "terrain"
							if sf.Placed {
								on = "placed"
							}
						}
					}
					fmt.Printf("   (%.0f, %.1f, %.0f) %s\n", x, float32(p.y)/10, z, on)
				}
				q, ok := from[p]
				if !ok || q == p {
					break
				}
				p = q
			}
		}
	}
	steps := [4][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		s := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		cur = s
		x, z := at(s.i, s.j)
		y := float32(s.y) / 10
		swimming := false
		if fl, ok := env.Flags(w, x, y+middle, z); ok && fl&ground.EnvWater != 0 {
			swimming = true
		}
		y0 := y
		for _, d := range steps {
			y := y0
			ni, nj := s.i+d[0], s.j+d[1]
			if ni < 0 || nj < 0 || ni >= n || nj >= n {
				continue
			}
			nx, nz := at(ni, nj)
			if beyondShared(nx, nz) {
				continue
			}
			// where the feet go: the floors at the step's end (one too steep
			// to stand on is slid off: the next floor down)
			var to []float32
			nc := colAt(ni, nj)
			surfs := nc.surfs
			best, found := float32(0), false
			for _, sf := range surfs {
				if sf.Up && sf.Y <= y+climb && sf.Y >= y-0.6 && (!found || sf.Y > best) {
					best, found = sf.Y, true
				}
			}
			if found && steepAt(ni, nj, best) {
				found = false
				y = min(y, best) // what is under it, from where it was met
			}
			if found {
				to = append(to, best)
			} else {
				// falling: the next floor down, or water
				down, any := float32(0), false
				for _, sf := range surfs {
					if sf.Up && sf.Y < y && (!any || sf.Y > down) {
						down, any = sf.Y, true
					}
				}
				if any {
					to = append(to, down)
				}
			}
			if swimming && !math.IsNaN(float64(nc.water)) && y+middle <= nc.water {
				// a swimmer goes on at its height, wherever the water does
				to = append(to, y)
			}
			if len(to) == 0 {
				if inBand(nx, nz) || past(nx, nz) {
					// nothing under it: past the terrain
					if !meets(g, w, x, y, z, nx, y, nz) {
						report(state{ni, nj, s.y}, "a step onto nothing")
					}
				}
				continue
			}
			for _, ny := range to {
				if meets(g, w, x, y, z, nx, ny, nz) {
					continue
				}
				ns := state{ni, nj, tenth(ny)}
				if past(nx, nz) {
					if !seen[ns] {
						seen[ns] = true
						report(ns, "past the edge")
					}
					continue
				}
				if !inBand(nx, nz) {
					continue
				}
				push(ns)
				// in water, the whole column
				if wt := nc.water; !math.IsNaN(float64(wt)) && ny < wt {
					push(state{ni, nj, tenth(wt - middle - 0.3)})
				}
			}
		}
	}
	// the ways out as stretches of each edge (where along it, how high)
	byEdge := map[string][][2]float32{}
	for k, y := range outs {
		x, z := at(k[0], k[1])
		var name string
		var t float32
		switch {
		case z > z0+cell:
			name, t = "z+", x-x0
		case z < z0:
			name, t = "z-", x-x0
		case x > x0+cell:
			name, t = "x+", z-z0
		default:
			name, t = "x-", z-z0
		}
		byEdge[name] = append(byEdge[name], [2]float32{t, y})
	}
	var all []stretch
	for name, list := range byEdge {
		sort.Slice(list, func(i, j int) bool { return list[i][0] < list[j][0] })
		cur := stretch{Edge: name, T0: list[0][0], T1: list[0][0], Ylo: list[0][1], Yhi: list[0][1]}
		for _, p := range list {
			if p[0]-cur.T1 > 6 {
				all = append(all, cur)
				cur = stretch{Edge: name, T0: p[0], T1: p[0], Ylo: p[1], Yhi: p[1]}
			}
			cur.T1, cur.Ylo, cur.Yhi = p[0], min(cur.Ylo, p[1]), max(cur.Yhi, p[1])
			cur.N++
		}
		all = append(all, cur)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Edge < all[j].Edge || all[i].Edge == all[j].Edge && all[i].T0 < all[j].T0
	})
	var lines []string
	for _, st := range all {
		lines = append(lines, fmt.Sprintf("edge %s %.0f..%.0f along it (%d places, feet %.1f..%.1f): open", st.Edge, st.T0, st.T1, st.N, st.Ylo, st.Yhi))
	}
	if mapOut != "" && traceNear[0] != 0 {
		drawMap(mapOut, traceNear, at, n, colAt, seen, outs)
	}
	return all, len(seen), lines
}

// drawMap writes a picture of the walk round a place, a pixel a unit: what
// was reached (green, brighter higher), what is placed there (grey: its
// top), the ways out (red); with -map and -near, for finding a gap.
func drawMap(path string, near [2]float32, at func(i, j int32) (float32, float32), n int32, colAt func(i, j int32) *column, seen map[state]bool, outs map[[2]int32]float32) {
	const half = 120
	img := image.NewRGBA(image.Rect(0, 0, 2*half, 2*half))
	reached := map[[2]int32]bool{}
	for s := range seen {
		reached[[2]int32{s.i, s.j}] = true
	}
	for i := int32(0); i < n; i++ {
		x, _ := at(i, 0)
		px := int(x - near[0] + half)
		if px < 0 || px >= 2*half {
			continue
		}
		for j := int32(0); j < n; j++ {
			_, z := at(i, j)
			pz := int(z - near[1] + half)
			if pz < 0 || pz >= 2*half {
				continue
			}
			c := color.RGBA{20, 20, 30, 255}
			placed := false
			lowPlaced, terrain := float32(math.MaxFloat32), float32(-math.MaxFloat32)
			for _, sf := range colAt(i, j).surfs {
				if sf.Placed {
					placed = true
					lowPlaced = min(lowPlaced, sf.Y)
				} else if sf.Up {
					terrain = max(terrain, sf.Y)
				}
			}
			if placed {
				c = color.RGBA{110, 110, 110, 255}
				// a rock whose lowest face is over the ground: blue (a gap under it)
				if terrain > -math.MaxFloat32 && lowPlaced > terrain+1.8 {
					c = color.RGBA{60, 90, 200, 255}
				}
			}
			if reached[[2]int32{i, j}] {
				c = color.RGBA{40, 170, 60, 255}
				if placed {
					c = color.RGBA{160, 220, 80, 255}
				}
			}
			if _, ok := outs[[2]int32{i, j}]; ok {
				c = color.RGBA{230, 30, 30, 255}
			}
			img.Set(px, 2*half-1-pz, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
