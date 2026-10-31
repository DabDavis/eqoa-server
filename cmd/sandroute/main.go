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

// edge is the highest (over) or the lowest (under) floor within look of the
// segment a-b, sampled every 25.
func (p *planner) edge(a, b pt) (float64, bool) {
	best, any := 0.0, false
	n := int(math.Ceil(math.Hypot(b.x-a.x, b.z-a.z)/25)) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, z := a.x+(b.x-a.x)*t, a.z+(b.z-a.z)*t
		for _, d := range [][2]float64{{0, 0}, {p.look, 0}, {-p.look, 0}, {0, p.look}, {0, -p.look}} {
			h, ok := p.top(x+d[0], z+d[1])
			if !ok {
				continue
			}
			if !any || (p.below > 0 && h < best) || (p.below <= 0 && h > best) {
				best, any = h, true
			}
		}
	}
	return best, any
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

func steps(path []pt, ys []float64) string {
	s := make([]string, len(path))
	for i, q := range path {
		s[i] = fmt.Sprintf("goto %.0f %.0f %.0f", q.x, ys[i], q.z)
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
	flag.Parse()
	g, err := ground.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	p := &planner{g: g, w: uint8(*world), stop: *stop, look: *look, clear: *clear, below: *below}
	if env, err := ground.OpenEnv(*dir); err == nil && env.Worlds() > 0 {
		p.env = env
	}
	at := pt{(*x0 + *x1) / 2, (*z0 + *z1) / 2}
	if *from != "" {
		if _, err := fmt.Sscanf(*from, "%f,%f", &at.x, &at.z); err != nil {
			log.Fatalf("-from %q: want x,z", *from)
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
			cur = append(cur, fmt.Sprintf("goto %.0f %.0f %.0f", q.x, ys[i], q.z))
			if flown >= *run && i+1 < len(full) {
				fmt.Println(strings.Join(cur, ";"))
				cur = []string{cur[len(cur)-1]}
				flown = 0
			}
		}
		fmt.Println(strings.Join(cur, ";"))
		all = full
	}
	ys := p.heights(all)
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
