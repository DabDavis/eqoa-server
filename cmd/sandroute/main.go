// Command sandroute plans a flight over a world for the harvester's census
// of another server's creatures (eqoa harvest's goto steps, track on): rows
// across the world's land every -rows units, a stop every -stop units along
// each, each stop's height the highest floor within -look of the leg into it
// and out of it plus -clear, so a straight leg never meets the ground. It
// prints the steps, split into runs of at most -run units of flight, one run
// a line, each starting with a goto from the run before's last stop.
//
//	sandroute -ground worlds/ground -world 1 [-x0 4000 -x1 10000 -z0 2000 -z1 10000]
//	    [-rows 350] [-stop 200] [-clear 45] [-run 40000]
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/ground"
)

func main() {
	dir := flag.String("ground", "worlds/ground", "the ground files")
	world := flag.Int("world", 1, "the world")
	x0 := flag.Float64("x0", 4000, "the area's west")
	x1 := flag.Float64("x1", 10000, "the area's east")
	z0 := flag.Float64("z0", 2000, "the area's north")
	z1 := flag.Float64("z1", 10000, "the area's south")
	rows := flag.Float64("rows", 350, "between rows")
	stop := flag.Float64("stop", 200, "between stops along a row")
	look := flag.Float64("look", 60, "how far either side of a leg the ground is looked at")
	clear := flag.Float64("clear", 45, "the height over the highest floor near a leg")
	run := flag.Float64("run", 40000, "a run's most flight, units")
	from := flag.String("from", "", "x,z: where the flight begins (a straight transit to the first row is put first)")
	flag.Parse()
	g, err := ground.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	w := uint8(*world)
	top := func(x, z float64) (float64, bool) {
		h, ok := g.Floor(w, float32(x), 1e6, float32(z))
		return float64(h), ok
	}
	// the highest floor within look of the segment a-b, sampled
	highest := func(ax, az, bx, bz float64) (float64, bool) {
		best, any := math.Inf(-1), false
		n := int(math.Ceil(math.Hypot(bx-ax, bz-az)/25)) + 1
		for i := 0; i <= n; i++ {
			t := float64(i) / float64(n)
			x, z := ax+(bx-ax)*t, az+(bz-az)*t
			for _, d := range [][2]float64{{0, 0}, {*look, 0}, {-*look, 0}, {0, *look}, {0, -*look}} {
				if h, ok := top(x+d[0], z+d[1]); ok {
					best, any = math.Max(best, h), true
				}
			}
		}
		return best, any
	}
	type pt struct{ x, z float64 }
	var path []pt
	east := true
	for z := *z0 + *rows/2; z < *z1; z += *rows {
		// the row's land: from its first floor to its last
		lo, hi := math.Inf(1), math.Inf(-1)
		for x := *x0; x <= *x1; x += 25 {
			if _, ok := top(x, z); ok {
				lo, hi = math.Min(lo, x), math.Max(hi, x)
			}
		}
		if lo > hi {
			continue
		}
		var row []pt
		for x := lo; x <= hi+1e-9; x += *stop {
			row = append(row, pt{x, z})
		}
		if row[len(row)-1].x < hi {
			row = append(row, pt{hi, z})
		}
		if !east {
			for i, j := 0, len(row)-1; i < j; i, j = i+1, j-1 {
				row[i], row[j] = row[j], row[i]
			}
		}
		path = append(path, row...)
		east = !east
	}
	// the transit from where the flight begins, a stop every -stop units
	if *from != "" {
		var fx, fz float64
		if _, err := fmt.Sscanf(*from, "%f,%f", &fx, &fz); err != nil {
			log.Fatalf("-from %q: want x,z", *from)
		}
		if len(path) > 0 {
			d := math.Hypot(path[0].x-fx, path[0].z-fz)
			n := int(math.Ceil(d / *stop))
			var transit []pt
			for i := 0; i < n; i++ {
				t := float64(i) / float64(n)
				transit = append(transit, pt{fx + (path[0].x-fx)*t, fz + (path[0].z-fz)*t})
			}
			path = append(transit, path...)
		}
	}
	// each stop's height: over both its legs, so every straight leg, between
	// two stops each at least its own height, stays over its ground
	ys := make([]float64, len(path))
	for i, p := range path {
		h, ok := math.Inf(-1), false
		for _, j := range []int{i - 1, i + 1} {
			if j < 0 || j >= len(path) {
				continue
			}
			if v, k := highest(path[j].x, path[j].z, p.x, p.z); k {
				h, ok = math.Max(h, v), true
			}
		}
		if !ok {
			h = 60
		}
		ys[i] = h + *clear
	}
	var runs [][]string
	var cur []string
	flown, total := 0.0, 0.0
	for i, p := range path {
		if i > 0 {
			d := math.Hypot(p.x-path[i-1].x, p.z-path[i-1].z)
			flown += d
			total += d
		}
		cur = append(cur, fmt.Sprintf("goto %.0f %.0f %.0f", p.x, ys[i], p.z))
		if flown >= *run && i+1 < len(path) {
			runs = append(runs, cur)
			cur = []string{fmt.Sprintf("goto %.0f %.0f %.0f", p.x, ys[i], p.z)}
			flown = 0
		}
	}
	runs = append(runs, cur)
	for _, r := range runs {
		fmt.Println(strings.Join(r, ";"))
	}
	log.Printf("%d stops, %.0f units of flight in %d runs; heights %.0f to %.0f", len(path), total, len(runs),
		minOf(ys), maxOf(ys))
	// how high each stop is over the ground beneath it
	var over []float64
	for i, p := range path {
		if h, ok := top(p.x, p.z); ok {
			over = append(over, ys[i]-h)
		}
	}
	sort.Float64s(over)
	if len(over) > 0 {
		q := func(f float64) float64 { return over[int(f*float64(len(over)-1))] }
		log.Printf("over the ground beneath: median %.0f, 75%% %.0f, 90%% %.0f, 99%% %.0f, most %.0f", q(.5), q(.75), q(.9), q(.99), q(1))
	}
}

func minOf(v []float64) float64 {
	m := math.Inf(1)
	for _, x := range v {
		m = math.Min(m, x)
	}
	return m
}

func maxOf(v []float64) float64 {
	m := math.Inf(-1)
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}
