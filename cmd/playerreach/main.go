// Command playerreach surveys where a PLAYER can walk to in one stretch of a
// world, for tools/boxcanyon_reach_20261115.py (petition 2139, Zerkx,
// 2026-11-15: "box canyon instance there are a bunch of mobs on the hills
// that cant be get too"). cmd/canyonspread asks where a creature walks
// (ground.Floor, the highest terrain, a rise of -rise a step either way);
// this asks where a player's walk takes it, as entity/movecheck.go judges
// one:
//
//   - a player stands on any surface of the collision whose front looks up
//     (the terrain at any slope, or anything placed: ground.Surfaces), with
//     room over it for its body (no surface within -head over it);
//   - it climbs from a place to a neighbouring one no steeper than 1:1 plus a
//     step (movecheck's mcRiseSlope 1.0 and mcRiseStep 1.3: no climb of the
//     live players' 58,422 reports was steeper), landing on the highest floor
//     there under that;
//   - it walks off anything (a fall: no limit down) onto the highest floor
//     under it;
//   - its middle (1.2 over the feet) never passes through the front of a face
//     (ground.Crosses: a cliff, a placed rock, the ground, a cave's roof).
//
// The places are a grid of -step over -box, every floor of each column a
// place; the reach is counted from -seed (x,y,z: a dungeon's entrance).
// -dump DIR writes a picture's data (each column's top floor and its reach).
//
// -npcs FILE reads a JSON list of {"id","x","y","z","gone"} (the creatures to
// judge) and prints, a JSON line each: the floor it stands on; whether a
// player reaches it (a reached place within -near across and 2 up or down);
// whether it stands OVER the reached places (a player walks or falls from it
// to one: a hill's or a mesa's top, never climbed); how far the nearest
// reached place is (within -within); and, unreached and not "gone", a place
// to move it to. The unreached are taken in camps (within -camp of one
// another), nearest first, each placed whole as it stood at the nearest
// reached place that is open ground (every place within -clear reached and
// within 3 of its height; each member's within 3) where the creature's own
// floor (ground.Floor) is the same, -apart from every creature that stays,
// -campgap from every other camp placed, and off -keepoff.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/ground"
)

const (
	riseSlope = 1.0 // entity/movecheck.go mcRiseSlope
	riseStep  = 1.3 // entity/movecheck.go mcRiseStep
	middle    = 1.2 // the walk's ball over the feet (CalcFit)
)

type npcIn struct {
	ID   int64   `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
	Gone bool    `json:"gone,omitempty"` // judged, but given no place and in no one's way (it is to be removed)
}

type npcOut struct {
	ID      int64       `json:"id"`
	Stand   float64     `json:"stand"`             // the floor under it (-99999: none)
	Reached bool        `json:"reached"`           // a player stands within -near of it
	Nearest float64     `json:"nearest"`           // across to the nearest reached place (-1: none within -within)
	NearY   float64     `json:"near_y,omitempty"`  // that place's height
	To      *[3]float64 `json:"to,omitempty"`      // a place to move it to
	Over    bool        `json:"over"`              // it stands where a player comes down from to a reached place
	Camp    int         `json:"camp"`              // the unreached camp it is moved with (-1: reached)
	ToDist  float64     `json:"to_dist,omitempty"` // across, from it
}

func main() {
	groundDir := flag.String("ground", "", "the ground files")
	world := flag.Int("world", 14, "the world")
	boxFlag := flag.String("box", "", "x0,z0,x1,z1")
	seedFlag := flag.String("seed", "", "x,y,z a player stands at (the entrance)")
	stepF := flag.Float64("step", 2, "the grid's step")
	head := flag.Float64("head", 1.8, "room a body needs over a floor")
	npcsPath := flag.String("npcs", "", "the creatures to judge (JSON)")
	near := flag.Float64("near", 3, "a creature within this across of a reached place is reached")
	within := flag.Float64("within", 400, "how far across a place to move one to is looked for")
	clear := flag.Float64("clear", 6, "open ground round a place to move one to")
	apart := flag.Float64("apart", 5, "a place to move one to stays this far from the others")
	campGap := flag.Float64("campgap", 35, "a camp's place stays this far from another camp's (a social creature's 30)")
	campR := flag.Float64("camp", 20, "unreached creatures this near one another are a camp, moved together as they stood")
	keepFlag := flag.String("keepoff", "", "x,z,r: no place to move one to within r of (x, z) (default: -seed, 60)")
	slope := flag.Float64("slope", riseSlope, "the steepest climb, up a unit across")
	stepUp := flag.Float64("stepup", riseStep, "and a step more")
	dump := flag.String("dump", "", "write the survey's picture data here (top.bin float32, reach.bin byte, nx*nz)")
	flag.Parse()
	box := nums(*boxFlag, 4)
	seed := nums(*seedFlag, 3)
	g, err := ground.Open(*groundDir)
	must(err)
	keep := []float64{seed[0], seed[2], 60}
	if *keepFlag != "" {
		keep = nums(*keepFlag, 3)
	}
	w := uint8(*world)
	step := *stepF
	nx := int((box[2]-box[0])/step) + 1
	nz := int((box[3]-box[1])/step) + 1
	X := func(i int) float64 { return box[0] + float64(i)*step }
	Z := func(j int) float64 { return box[1] + float64(j)*step }

	// every column's floors, lowest first, each with room over it
	off := make([]int32, nx*nz+1)
	var hs []float32
	for j := 0; j < nz; j++ {
		for i := 0; i < nx; i++ {
			c := j*nx + i
			off[c] = int32(len(hs))
			ss := g.Surfaces(w, float32(X(i)), float32(Z(j)), -1000, 2000)
			sort.Slice(ss, func(a, b int) bool { return ss[a].Y < ss[b].Y })
			for k, s := range ss {
				if !s.Up {
					continue
				}
				if n := len(hs); n > int(off[c]) && s.Y-hs[n-1] < 0.05 {
					continue // the same floor twice (a shared edge)
				}
				room := true
				for _, o := range ss[k+1:] {
					if o.Y-s.Y > 0.05 && float64(o.Y-s.Y) < *head {
						room = false
						break
					}
				}
				if room {
					hs = append(hs, s.Y)
				}
			}
		}
	}
	off[nx*nz] = int32(len(hs))
	fmt.Fprintf(os.Stderr, "%dx%d columns, %d floors\n", nx, nz, len(hs))

	// the highest floor of column c no higher than top, as a place (-1: none)
	land := func(c int, top float32) int {
		best := -1
		for k := off[c]; k < off[c+1]; k++ {
			if hs[k] <= top {
				best = int(k)
			}
		}
		return best
	}
	colOf := make([]int32, len(hs))
	for c := 0; c < nx*nz; c++ {
		for k := off[c]; k < off[c+1]; k++ {
			colOf[k] = int32(c)
		}
	}
	cell := func(x, z float64) int {
		i, j := int(math.Round((x-box[0])/step)), int(math.Round((z-box[1])/step))
		if i < 0 || j < 0 || i >= nx || j >= nz {
			return -1
		}
		return j*nx + i
	}
	reach := make([]bool, len(hs))
	sc := cell(seed[0], seed[2])
	if sc < 0 {
		log.Fatal("-seed is off -box")
	}
	s := land(sc, float32(seed[1])+ground.StepUp)
	if s < 0 {
		log.Fatal("no floor at -seed")
	}
	lift := float32(step**slope + *stepUp)
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	// a walk from place p into the neighbouring column lands on place q
	walks := func(p, q int) bool {
		if land(int(colOf[q]), hs[p]+lift) != q {
			return false
		}
		cp, cq := int(colOf[p]), int(colOf[q])
		ha, hb := hs[p], hs[q]
		ax, az, bx, bz := float32(X(cp%nx)), float32(Z(cp/nx)), float32(X(cq%nx)), float32(Z(cq/nx))
		top := max(ha, hb) + middle
		// across at the higher middle, then up or down to the floor
		if g.Crosses(w, ax, ha+middle, az, bx, top, bz) != ground.CrossNone {
			return false
		}
		return hb >= ha || g.Crosses(w, bx, top, bz, bx, hb+middle, bz) == ground.CrossNone
	}
	reach[s] = true
	stack := []int{s}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c := int(colOf[p])
		i, j := c%nx, c/nx
		for _, d := range dirs {
			ii, jj := i+d[0], j+d[1]
			if ii < 0 || jj < 0 || ii >= nx || jj >= nz {
				continue
			}
			q := land(jj*nx+ii, hs[p]+lift)
			if q < 0 || reach[q] || !walks(p, q) {
				continue
			}
			reach[q] = true
			stack = append(stack, q)
		}
	}
	// the places a player walks (or falls) from to a reached one: a hill's top over the
	// reached canyon is one (a player comes down from it, never up to it); a canyon
	// walled off from the seed's is not
	down := append([]bool(nil), reach...)
	for k, r := range reach {
		if r {
			stack = append(stack, k)
		}
	}
	for len(stack) > 0 {
		q := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c := int(colOf[q])
		i, j := c%nx, c/nx
		for _, d := range dirs {
			ii, jj := i+d[0], j+d[1]
			if ii < 0 || jj < 0 || ii >= nx || jj >= nz {
				continue
			}
			n := jj*nx + ii
			for p := int(off[n]); p < int(off[n+1]); p++ {
				if !down[p] && hs[p]+lift >= hs[q] && walks(p, q) {
					down[p] = true
					stack = append(stack, p)
				}
			}
		}
	}
	nr := 0
	for _, r := range reach {
		if r {
			nr++
		}
	}
	fmt.Fprintf(os.Stderr, "%d floors reached from the seed\n", nr)
	if *dump != "" {
		// each column's top floor, and 1 where it is reached, 2 where only a floor under it is
		top := make([]float32, nx*nz)
		rb := make([]byte, nx*nz)
		for c := 0; c < nx*nz; c++ {
			top[c] = float32(math.NaN())
			for k := off[c]; k < off[c+1]; k++ {
				top[c] = hs[k]
				if reach[k] {
					rb[c] = 2
				}
			}
			if n := off[c+1]; n > off[c] && reach[n-1] {
				rb[c] = 1
			}
		}
		must(os.MkdirAll(*dump, 0o755))
		f, err := os.Create(*dump + "/top.bin")
		must(err)
		must(binary.Write(f, binary.LittleEndian, top))
		must(f.Close())
		must(os.WriteFile(*dump+"/reach.bin", rb, 0o644))
		must(os.WriteFile(*dump+"/grid.json", []byte(fmt.Sprintf(`{"x0":%g,"z0":%g,"step":%g,"nx":%d,"nz":%d}`, box[0], box[1], step, nx, nz)), 0o644))
	}
	if *npcsPath == "" {
		return
	}
	b, err := os.ReadFile(*npcsPath)
	must(err)
	var npcs []npcIn
	must(json.Unmarshal(b, &npcs))

	// a reached place's open ground: every place within -clear reached, its
	// floor within 3 of the place's
	open := func(p int, clear float64) bool {
		rc := int(math.Ceil(clear / step))
		c := int(colOf[p])
		i, j := c%nx, c/nx
		for dj := -rc; dj <= rc; dj++ {
			for di := -rc; di <= rc; di++ {
				if float64(di*di+dj*dj)*step*step > clear*clear {
					continue
				}
				ii, jj := i+di, j+dj
				if ii < 0 || jj < 0 || ii >= nx || jj >= nz {
					return false
				}
				n := jj*nx + ii
				ok := false
				for k := off[n]; k < off[n+1]; k++ {
					if reach[k] && math.Abs(float64(hs[k]-hs[p])) <= 3 {
						ok = true
						break
					}
				}
				if !ok {
					return false
				}
			}
		}
		return true
	}
	type res struct {
		out npcOut
		in  npcIn
	}
	rs := make([]res, len(npcs))
	rw := int(math.Ceil(*within / step))
	// reached places within -within of (x, z), nearest first
	around := func(x, z float64) []int {
		ci, cj := int(math.Round((x-box[0])/step)), int(math.Round((z-box[1])/step))
		type cand struct {
			p int
			d float64
		}
		var cands []cand
		for dj := -rw; dj <= rw; dj++ {
			for di := -rw; di <= rw; di++ {
				ii, jj := ci+di, cj+dj
				if ii < 0 || jj < 0 || ii >= nx || jj >= nz {
					continue
				}
				d := math.Hypot(X(ii)-x, Z(jj)-z)
				if d > *within {
					continue
				}
				cc := jj*nx + ii
				for k := off[cc]; k < off[cc+1]; k++ {
					if reach[k] {
						cands = append(cands, cand{int(k), d})
					}
				}
			}
		}
		sort.SliceStable(cands, func(a, b int) bool { return cands[a].d < cands[b].d })
		out := make([]int, len(cands))
		for i, c := range cands {
			out[i] = c.p
		}
		return out
	}
	for idx, n := range npcs {
		o := npcOut{ID: n.ID, Stand: -99999, Nearest: -1, Camp: -1}
		c := cell(n.X, n.Z)
		if c >= 0 {
			if p := land(c, float32(n.Y)+ground.StepUp); p >= 0 {
				o.Stand = float64(hs[p])
				o.Over = down[p]
			}
		}
		ys := n.Y
		if o.Stand > -99999 {
			ys = o.Stand
		}
		for _, p := range around(n.X, n.Z) {
			c := int(colOf[p])
			d := math.Hypot(X(c%nx)-n.X, Z(c/nx)-n.Z)
			if o.Nearest < 0 {
				o.Nearest, o.NearY = round1(d), round1(float64(hs[p]))
			}
			if d > *near {
				break
			}
			if math.Abs(float64(hs[p])-ys) <= 2 {
				o.Reached = true
				break
			}
		}
		rs[idx] = res{out: o, in: n}
	}
	// the unreached in camps: within -camp across (and 15 up or down) of another of the camp
	parent := make([]int, len(rs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for i := range rs {
		for j := i + 1; j < len(rs); j++ {
			a, b := rs[i].in, rs[j].in
			if rs[i].out.Reached || rs[j].out.Reached || rs[i].in.Gone || rs[j].in.Gone || math.Hypot(a.X-b.X, a.Z-b.Z) > *campR || math.Abs(a.Y-b.Y) > 15 {
				continue
			}
			parent[find(i)] = find(j)
		}
	}
	camps := map[int][]int{}
	var roots []int
	for i := range rs {
		if rs[i].out.Reached || rs[i].in.Gone {
			continue
		}
		r := find(i)
		if camps[r] == nil {
			roots = append(roots, r)
		}
		camps[r] = append(camps[r], i)
	}
	// nearest camps first: each placed whole, as it stood, at the nearest open reached place to its middle
	nearestOf := func(r int) float64 {
		m := math.Inf(1)
		for _, i := range camps[r] {
			if d := rs[i].out.Nearest; d >= 0 && d < m {
				m = d
			}
		}
		return m
	}
	sort.SliceStable(roots, func(a, b int) bool { return nearestOf(roots[a]) < nearestOf(roots[b]) })
	var taken, moved [][2]float64 // those that stay; those given a place
	for _, r := range rs {
		if r.out.Reached {
			taken = append(taken, [2]float64{r.in.X, r.in.Z})
		}
	}
	free := func(x, z float64) bool {
		if math.Hypot(x-keep[0], z-keep[1]) < keep[2] {
			return false
		}
		for _, t := range taken {
			if math.Hypot(t[0]-x, t[1]-z) < *apart {
				return false
			}
		}
		for _, t := range moved {
			if math.Hypot(t[0]-x, t[1]-z) < *campGap {
				return false
			}
		}
		return true
	}
	// a creature's own floor (ground.Floor, the terrain's) is the place's
	creatureFloor := func(x, z float64, h float32) bool {
		f, ok := g.Floor(w, float32(x), h+0.5, float32(z))
		return ok && math.Abs(float64(f-h)) < 0.3
	}
	for ci, root := range roots {
		mem := camps[root]
		var mx, mz float64
		for _, i := range mem {
			mx += rs[i].in.X
			mz += rs[i].in.Z
			rs[i].out.Camp = ci
		}
		mx /= float64(len(mem))
		mz /= float64(len(mem))
	anchor:
		for _, p := range around(mx, mz) {
			c := int(colOf[p])
			ax, az, ah := X(c%nx), Z(c/nx), hs[p]
			if !open(p, *clear) || !creatureFloor(ax, az, ah) {
				continue
			}
			places := make([][3]float64, len(mem))
			for k, i := range mem {
				x, z := ax+rs[i].in.X-mx, az+rs[i].in.Z-mz
				cc := cell(x, z)
				if cc < 0 {
					continue anchor
				}
				x, z = X(cc%nx), Z(cc/nx)
				best := -1
				for q := off[cc]; q < off[cc+1]; q++ {
					if reach[q] && math.Abs(float64(hs[q]-ah)) <= 8 &&
						(best < 0 || math.Abs(float64(hs[q]-ah)) < math.Abs(float64(hs[best]-ah))) {
						best = int(q)
					}
				}
				if best < 0 || !open(best, 3) || !creatureFloor(x, z, hs[best]) || !free(x, z) {
					continue anchor
				}
				places[k] = [3]float64{x, round2(float64(hs[best])), z}
			}
			for k, i := range mem {
				to := places[k]
				rs[i].out.To = &to
				rs[i].out.ToDist = round1(math.Hypot(to[0]-rs[i].in.X, to[2]-rs[i].in.Z))
			}
			for k := range mem {
				moved = append(moved, [2]float64{places[k][0], places[k][2]})
			}
			break
		}
	}
	enc := json.NewEncoder(os.Stdout)
	for _, r := range rs {
		r.out.Stand = round2(r.out.Stand)
		must(enc.Encode(r.out))
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

func nums(s string, n int) []float64 {
	parts := strings.Split(s, ",")
	if len(parts) != n {
		log.Fatalf("%q: %d numbers", s, n)
	}
	out := make([]float64, n)
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			log.Fatalf("%q: %v", s, err)
		}
		out[i] = v
	}
	return out
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
