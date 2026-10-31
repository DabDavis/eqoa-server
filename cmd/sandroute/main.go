// Command sandroute plans a flight over a world for the harvester's census
// of another server's creatures (eqoa harvest's goto steps, track on): rows
// across the land every -rows units, a stop every -stop units along each.
// Sandstorm shows a client the creatures within about 125 of it, height
// counted, so the flight keeps close to the ground: -clear flies that high
// over the highest floor near each leg; -below (the user's, 2026-10-31:
// "maybe we should do below the terrain, may be closer and show more mobs
// at once") that deep under the lowest, so every straight leg stays under
// the ground, about level with what walks on it, and out of its reach.
//
// It prints the steps, split into runs of at most -run units of flight, one
// run a line; with -zones (the user's: "if we do by each zone, may be
// better") one run per 2,000-unit zone of CLIENT's grid in the area, named
// (-words gives WORDS.TXT), snaking through the grid, a line each:
// "<zone>\t<steps>". Each run starts with a straight transit, at the same
// rule, from where the one before ended (-from: where the flight begins).
//
//	sandroute -ground worlds/ground -world 1 [-x0 4000 -x1 10000 -z0 2000 -z1 10000]
//	    [-rows 200] [-stop 50] [-look 20] [-clear 60 | -below 12] [-run 40000]
//	    [-from x,z] [-zones -words WORDS.TXT]
//	sandroute ... -from x,z -to x,z   (one flight, to move a character)
//	sandroute ... -check plan.txt     (where a plan's flight is in water)
//
// No stop is ever under the top of the water (or slime, lava, mud) in its
// legs' columns (dryUp): Sandstorm drowns a player under a water's surface
// even where CLIENT's leaf is dry, under the sea floor.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/ground"
)

type pt struct{ x, z float64 }

type planner struct {
	g          *ground.Ground
	env        *ground.Env // the water, slime, lava and mud (<world>.env), nil: none known
	w          uint8
	stop, look float64
	clear      float64
	below      float64
	lifted     int // stops dryUp lifted out of water, slime, lava or mud
	tops       map[[2]int32]column
}

// dry reports land at (x, z): a floor, not under water, slime, lava or mud.
func (p *planner) dry(x, z float64) bool {
	h, ok := p.top(x, z)
	if !ok {
		return false
	}
	if p.env != nil {
		if fl, ok := p.env.Flags(p.w, float32(x), float32(h)+1, float32(z)); ok && fl&(ground.EnvWater|ground.EnvSlime|ground.EnvLava|ground.EnvMud) != 0 {
			return false
		}
	}
	return true
}

func (p *planner) top(x, z float64) (float64, bool) {
	h, ok := p.g.Floor(p.w, float32(x), 1e6, float32(z))
	return float64(h), ok
}

// bad reports a point in water, slime, lava or mud (its zone leaf's flags,
// as CLIENT's walk finds them).
func (p *planner) bad(x, y, z float64) bool {
	if p.env == nil {
		return false
	}
	fl, ok := p.env.Flags(p.w, float32(x), float32(y), float32(z))
	return ok && fl&(ground.EnvWater|ground.EnvSlime|ground.EnvLava|ground.EnvMud) != 0
}

// waterTop is the top of the water (or slime, lava, mud) in the column at
// (x, z): its highest point, looked for 2 at a time from 600 under the
// column's floor to 400 over it; false, none. Kept by the whole unit.
func (p *planner) waterTop(x, z float64) (float64, bool) {
	k := [2]int32{int32(math.Round(x)), int32(math.Round(z))}
	if c, ok := p.tops[k]; ok {
		return c.y, c.ok
	}
	top, _ := p.top(x, z)
	var c column
	for h := top - 600; h <= top+400; h += 2 {
		if p.bad(x, h, z) {
			c = column{h, true}
		}
	}
	p.tops[k] = c
	return c.y, c.ok
}

// column is a column's water top (waterTop).
type column struct {
	y  float64
	ok bool
}

// wet is the height a leg a-b (at ya, yb) must keep over: 5 over the top of
// the water in any column it passes under, sampled every 5 units at its own
// height. Under the water's top is in it to Sandstorm, whatever CLIENT's
// leaf says there: the census druid (2026-10-31) drowned 12 under the floor of Erud's
// Crossing's harbour, where the leaf is dry and the sea was 24 to 51 over
// her (Sandstorm's 32 of 255 breath every 6 s, then 399 of her 688 HP).
// False: the leg is dry as it is.
func (p *planner) wet(a pt, ya float64, b pt, yb float64) (float64, bool) {
	n := int(math.Ceil(dist(a, b)/5)) + 1
	ceil, any := 0.0, false
	for k := 0; k <= n; k++ {
		t := float64(k) / float64(n)
		x, y, z := a.x+(b.x-a.x)*t, ya+(yb-ya)*t, a.z+(b.z-a.z)*t
		top, ok := p.waterTop(x, z)
		if !ok || y >= top+5 {
			continue
		}
		if !any || top+5 > ceil {
			ceil, any = top+5, true
		}
	}
	return ceil, any
}

// liftClear is how far over the highest floor near its legs a stop lifted
// out of the water flies when that leaves it in the open (dryUp).
const liftClear = 20

// dryUp lifts stops till no leg is wet: a wet leg lifts both its stops over
// it, and a lifted stop's other leg is looked at again. A lifted stop no
// longer 3 under every floor near its legs is in the open, where a creature
// could reach it, and flies liftClear over the highest of them. It returns
// how many stops it lifted.
func (p *planner) dryUp(path []pt, ys []float64) int {
	lifted := map[int]bool{}
	for pass := 0; pass < 100; pass++ {
		changed := false
		for i := 1; i < len(path); i++ {
			c, ok := p.wet(path[i-1], ys[i-1], path[i], ys[i])
			if !ok {
				continue
			}
			for _, j := range []int{i - 1, i} {
				if ys[j] < c {
					ys[j], changed, lifted[j] = c, true, true
				}
			}
		}
		for j := range lifted {
			lo, hi, ok := p.stopFloors(path, j)
			if ok && ys[j] > lo-3 && ys[j] < hi+liftClear {
				ys[j], changed = hi+liftClear, true
			}
		}
		if !changed {
			break
		}
	}
	return len(lifted)
}

// stopFloors are the lowest and the highest floor near stop j's legs (its
// own, for a stop alone).
func (p *planner) stopFloors(path []pt, j int) (lo, hi float64, any bool) {
	for _, k := range []int{j - 1, j + 1} {
		if k < 0 || k >= len(path) {
			continue
		}
		l, h, ok := p.floors(path[k], path[j])
		if !ok {
			continue
		}
		if !any {
			lo, hi, any = l, h, true
			continue
		}
		lo, hi = math.Min(lo, l), math.Max(hi, h)
	}
	if !any {
		if h, ok := p.top(path[j].x, path[j].z); ok {
			return h, h, true
		}
	}
	return lo, hi, any
}

// checkPlan prints, for each run of a plan (this command's output), the legs
// whose flight is wet as written, and the height each would need.
func (p *planner) checkPlan(file string) {
	b, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		name := ""
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			name, line = line[:i], line[i+1:]
		}
		var path []pt
		var ys []float64
		for _, s := range strings.Split(line, ";") {
			var x, y, z float64
			if n, _ := fmt.Sscanf(strings.TrimSpace(s), "goto %f %f %f", &x, &y, &z); n == 3 {
				path, ys = append(path, pt{x, z}), append(ys, y)
			}
		}
		var wet []string
		for i := 1; i < len(path); i++ {
			if c, ok := p.wet(path[i-1], ys[i-1], path[i], ys[i]); ok {
				wet = append(wet, fmt.Sprintf("%d:(%.0f,%.0f)@%.0f>%.0f", i, path[i].x, path[i].z, ys[i], c))
			}
		}
		fmt.Printf("%s\t%d stops, %d legs wet\t%s\n", name, len(path), len(wet), strings.Join(wet, " "))
	}
}

// floors are the lowest and the highest floor within look of the segment
// a-b, sampled every 25.
func (p *planner) floors(a, b pt) (lo, hi float64, any bool) {
	n := int(math.Ceil(math.Hypot(b.x-a.x, b.z-a.z)/25)) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, z := a.x+(b.x-a.x)*t, a.z+(b.z-a.z)*t
		for _, d := range [][2]float64{{0, 0}, {p.look, 0}, {-p.look, 0}, {0, p.look}, {0, -p.look}} {
			h, ok := p.top(x+d[0], z+d[1])
			if !ok {
				continue
			}
			if !any {
				lo, hi, any = h, h, true
				continue
			}
			lo, hi = math.Min(lo, h), math.Max(hi, h)
		}
	}
	return lo, hi, any
}

// edge is the highest (over) or the lowest (under) floor within look of the
// segment a-b.
func (p *planner) edge(a, b pt) (float64, bool) {
	lo, hi, ok := p.floors(a, b)
	if p.below > 0 {
		return lo, ok
	}
	return hi, ok
}

// heights are the stops' heights: each over (or under) both its legs, so
// every straight leg between two stops, each at least (at most) its own
// height, stays over (under) its ground.
func (p *planner) heights(path []pt) []float64 {
	ys := make([]float64, len(path))
	for i := range path {
		h, ok := 0.0, false
		for _, j := range []int{i - 1, i + 1} {
			if j < 0 || j >= len(path) {
				continue
			}
			v, k := p.edge(path[j], path[i])
			if !k {
				continue
			}
			if !ok || (p.below > 0 && v < h) || (p.below <= 0 && v > h) {
				h, ok = v, true
			}
		}
		if !ok {
			h, ok = p.top(path[i].x, path[i].z)
			if !ok {
				h = 60
			}
		}
		if p.below > 0 {
			ys[i] = h - p.below
		} else {
			ys[i] = h + p.clear
		}
	}
	p.lifted += p.dryUp(path, ys)
	return ys
}

// rows is a lawnmower over the box: a row every rows units, its land from
// its first dry floor to its last (the sea floor is no land), a stop every
// p.stop; starting at the corner nearest from.
func (p *planner) rows(x0, x1, z0, z1, rows float64, from pt) []pt {
	var lines [][]pt
	for z := z0 + rows/2; z < z1; z += rows {
		lo, hi := math.Inf(1), math.Inf(-1)
		for x := x0; x <= x1; x += 25 {
			if p.dry(x, z) {
				lo, hi = math.Min(lo, x), math.Max(hi, x)
			}
		}
		if lo > hi {
			continue
		}
		var row []pt
		for x := lo; x <= hi+1e-9; x += p.stop {
			row = append(row, pt{x, z})
		}
		if row[len(row)-1].x < hi {
			row = append(row, pt{hi, z})
		}
		lines = append(lines, row)
	}
	if len(lines) == 0 {
		return nil
	}
	// start at the corner nearest from: the rows from the near end, the first
	// row from its near side
	first, last := lines[0], lines[len(lines)-1]
	dTop := math.Min(dist(from, first[0]), dist(from, first[len(first)-1]))
	dBot := math.Min(dist(from, last[0]), dist(from, last[len(last)-1]))
	if dBot < dTop {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	east := dist(from, lines[0][0]) <= dist(from, lines[0][len(lines[0])-1])
	var path []pt
	for _, row := range lines {
		if !east {
			r := make([]pt, len(row))
			for i := range row {
				r[i] = row[len(row)-1-i]
			}
			row = r
		}
		path = append(path, row...)
		east = !east
	}
	return path
}

// transit is a straight line from a to b, a stop every p.stop, b left out.
func (p *planner) transit(a, b pt) []pt {
	n := int(math.Ceil(dist(a, b) / p.stop))
	var out []pt
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n)
		out = append(out, pt{a.x + (b.x-a.x)*t, a.z + (b.z-a.z)*t})
	}
	return out
}

func dist(a, b pt) float64 { return math.Hypot(a.x-b.x, a.z-b.z) }

// steps are a path's goto steps, each height rounded up (a stop lifted to
// 55.4 out of the water must not come out as 55).
func steps(path []pt, ys []float64) string {
	s := make([]string, len(path))
	for i, q := range path {
		s[i] = fmt.Sprintf("goto %.0f %.0f %.0f", q.x, math.Ceil(ys[i]), q.z)
	}
	return strings.Join(s, ";")
}

func length(path []pt) float64 {
	d := 0.0
	for i := 1; i < len(path); i++ {
		d += dist(path[i-1], path[i])
	}
	return d
}

func main() {
	dir := flag.String("ground", "worlds/ground", "the ground files")
	world := flag.Int("world", 1, "the world")
	x0 := flag.Float64("x0", 4000, "the area's west")
	x1 := flag.Float64("x1", 10000, "the area's east")
	z0 := flag.Float64("z0", 2000, "the area's north")
	z1 := flag.Float64("z1", 10000, "the area's south")
	rows := flag.Float64("rows", 200, "between rows")
	stop := flag.Float64("stop", 50, "between stops along a row")
	look := flag.Float64("look", 20, "how far either side of a leg the ground is looked at")
	clear := flag.Float64("clear", 60, "over: the height over the highest floor near a leg")
	below := flag.Float64("below", 0, "under: the depth under the lowest floor near a leg (0: fly over)")
	run := flag.Float64("run", 40000, "a run's most flight, units (without -zones)")
	from := flag.String("from", "", "x,z: where the flight begins")
	zones := flag.Bool("zones", false, "a run per 2,000-unit zone of the grid, snaking")
	wordsPath := flag.String("words", "", "WORDS.TXT, for -zones' names")
	check := flag.String("check", "", "a plan to check instead: where its flight is in water, slime, lava or mud")
	to := flag.String("to", "", "x,z: one flight from -from straight to here instead, by the same rules")
	skipList := flag.String("skip", "", "-zones: zones left out (done), comma-separated names, so the first transit starts at -from")
	flag.Parse()
	g, err := ground.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	p := &planner{g: g, w: uint8(*world), stop: *stop, look: *look, clear: *clear, below: *below, tops: map[[2]int32]column{}}
	if env, err := ground.OpenEnv(*dir); err == nil && env.Worlds() > 0 {
		p.env = env
	} else {
		log.Printf("no environment files in %s: water, slime, lava and mud unknown", *dir)
	}
	if *check != "" {
		p.checkPlan(*check)
		return
	}
	at := pt{(*x0 + *x1) / 2, (*z0 + *z1) / 2}
	if *from != "" {
		if _, err := fmt.Sscanf(*from, "%f,%f", &at.x, &at.z); err != nil {
			log.Fatalf("-from %q: want x,z", *from)
		}
	}
	if *to != "" {
		var b pt
		if _, err := fmt.Sscanf(*to, "%f,%f", &b.x, &b.z); err != nil {
			log.Fatalf("-to %q: want x,z", *to)
		}
		path := append(p.transit(at, b), b)
		ys := p.heights(path)
		fmt.Println(steps(path, ys))
		log.Printf("%d stops, %.0f units, %d lifted out of water, slime, lava or mud", len(path), length(path), p.lifted)
		return
	}
	skip := map[string]bool{}
	for _, n := range strings.Split(*skipList, ",") {
		if n = strings.TrimSpace(n); n != "" {
			skip[strings.ToLower(n)] = true
		}
	}
	var all []pt
	if *zones {
		var words []string
		if *wordsPath != "" {
			b, err := os.ReadFile(*wordsPath)
			if err != nil {
				log.Fatal(err)
			}
			words = append([]string{""}, strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), "\xef\xbb\xbf"), "\r\n", "\n"), "\n")...)
		}
		type cell struct{ col, row int }
		var cells [][]cell
		for zz := math.Floor(*z0/2000) * 2000; zz < *z1; zz += 2000 {
			var line []cell
			for xx := math.Floor(*x0/2000) * 2000; xx < *x1; xx += 2000 {
				line = append(line, cell{int(xx / 2000), int(zz / 2000)})
			}
			cells = append(cells, line)
		}
		// the grid's rows snaking, the first from the end nearest where it begins
		if len(cells) > 0 && len(cells[0]) > 1 {
			c0, cN := cells[0][0], cells[0][len(cells[0])-1]
			if dist(at, pt{float64(cN.col)*2000 + 1000, float64(cN.row)*2000 + 1000}) <
				dist(at, pt{float64(c0.col)*2000 + 1000, float64(c0.row)*2000 + 1000}) {
				for i := range cells {
					for a, b := 0, len(cells[i])-1; a < b; a, b = a+1, b-1 {
						cells[i][a], cells[i][b] = cells[i][b], cells[i][a]
					}
				}
			}
		}
		for i, line := range cells {
			if i%2 == 1 {
				for a, b := 0, len(line)-1; a < b; a, b = a+1, b-1 {
					line[a], line[b] = line[b], line[a]
				}
			}
			for _, c := range line {
				bx0, bz0 := math.Max(float64(c.col)*2000, *x0), math.Max(float64(c.row)*2000, *z0)
				bx1, bz1 := math.Min(float64(c.col+1)*2000, *x1), math.Min(float64(c.row+1)*2000, *z1)
				path := p.rows(bx0, bx1, bz0, bz1, *rows, at)
				if len(path) == 0 {
					continue
				}
				name := fmt.Sprintf("zone %d,%d", c.col, c.row)
				if word := zoneWord(*world, c.col-1, c.row-1); word > 0 && word < len(words) {
					name = strings.TrimSpace(words[word])
				}
				if skip[strings.ToLower(name)] {
					continue
				}
				full := append(p.transit(at, path[0]), path...)
				ys := p.heights(full)
				fmt.Printf("%s\t%s\n", name, steps(full, ys))
				log.Printf("%s: %d stops, %.0f units (%.0f of it the transit)", name, len(full), length(full), dist(at, path[0]))
				all = append(all, full...)
				at = path[len(path)-1]
			}
		}
	} else {
		path := p.rows(*x0, *x1, *z0, *z1, *rows, at)
		full := append(p.transit(at, path[0]), path...)
		ys := p.heights(full)
		var cur []string
		flown := 0.0
		for i, q := range full {
			if i > 0 {
				flown += dist(full[i-1], q)
			}
			cur = append(cur, fmt.Sprintf("goto %.0f %.0f %.0f", q.x, math.Ceil(ys[i]), q.z))
			if flown >= *run && i+1 < len(full) {
				fmt.Println(strings.Join(cur, ";"))
				cur = []string{cur[len(cur)-1]}
				flown = 0
			}
		}
		fmt.Println(strings.Join(cur, ";"))
		all = full
	}
	p.lifted = 0
	ys := p.heights(all)
	log.Printf("%d stops lifted out of water, slime, lava or mud", p.lifted)
	var over []float64
	for i, q := range all {
		if h, ok := p.top(q.x, q.z); ok {
			over = append(over, ys[i]-h)
		}
	}
	sort.Float64s(over)
	if len(over) > 0 {
		qf := func(f float64) float64 { return over[int(f*float64(len(over)-1))] }
		log.Printf("%d stops, %.0f units of flight; against the ground beneath: 1%% %.0f, median %.0f, 99%% %.0f",
			len(all), length(all), qf(.01), qf(.5), qf(.99))
	}
}

// zoneWord is CLIENT's zone grid's word for a cell (row, col as ZoneName
// takes them), 0 off a grid this command knows (the decomp's vientity.cpp:
// Rathe's sZones1, Odus's sZones2).
func zoneWord(world, col, row int) int {
	grids := map[int][][]int{1: {
		{641, 1177, 1181, 1185},
		{641, 1178, 1182, 1186},
		{641, 1179, 1183, 1187},
		{641, 1180, 1184, 1188},
	}, 2: {
		{641, 1441, 1451, 1446, 1456, 641},
		{641, 1442, 1447, 1452, 1457, 641},
		{641, 1443, 1448, 1453, 1458, 641},
		{641, 1444, 1449, 1454, 1459, 641},
		{641, 1445, 1450, 1455, 1460, 641},
	}}
	g := grids[world]
	if row < 0 || row >= len(g) || col < 0 || col >= len(g[row]) {
		return 0
	}
	return g[row][col]
}
