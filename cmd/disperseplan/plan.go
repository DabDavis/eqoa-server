package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The plan (tools/disperse_patrol_20261112.py applies it, world by world):
//
//   - moves: rows of a camp spread over its ground, each on a floor its wander
//     could walk to from where it stood (entity.DisperseWalk: no wall, no fall,
//     not into the water), a body's width and more from every other creature,
//     facing anywhere. A camp is the creatures within campLink of one another;
//     one is spread when it is stacked (two or more bodies on one spot,
//     stackNear), laid out in rows (gridLanes: an import's even lines), or a GM
//     named it (-gm). In a stacked camp only the stacked rows move, one of each
//     stack keeping its place; a camp in rows, or a GM's, moves whole.
//   - wander: npcwander rows (a row's own wander): a GM's camps roam; a camp of
//     a standing kind gets one wanderer in four; a lone creature of a standing
//     kind wanders; the far roamers (farRoamers) wander their ground.
//
// Left alone: rows a GM or a camp report placed or moved (gm_audit), rows near
// a camp report, rows quests tie by their id (queststeps, questwalls), rows with
// a path, a camp spawn or a wander row already, spawn-only names, pets (they go
// with their owners), townsfolk and guards (but a GM's camp), the NPC bit,
// named creatures (a name of two rows or fewer in the world), kinds a GM's
// npcmove row stands, creatures off the ground or in the water (not moved).

const (
	campLink   = 12.0  // creatures this near one another are one camp
	stackNear  = 1.5   // two bodies this near stand on one spot
	minSpacing = 6.0   // the least a camp's creatures are spread apart
	gmReach    = 40.0  // a GM's spot names the camps within this of it
	gmKinds    = 150.0 // or, naming its kinds, theirs within this of it
	reportNear = 60.0  // a camp report's ground
	namedMax   = 2     // a name this rare in its world is a named creature
	levelReach = 12.0  // a new spot no further above or below its old floor
)

// farRoamers are the kinds that roam a canyon or a zone (the owner,
// 2026-11-12: "skeletal giant and others are not roaming correctly, they only
// move a small distance"), with the radius they roam: the giants and cyclopes
// Sandstorm's census saw walking (npcmove roam 1), and the named roamers the
// players' guides (EQOA-Data) say roam.
var farRoamers = map[string]float32{
	"skeletal giant": 300, "hill giant": 300, "cyclops": 300, "cyclops seer": 250, "mummified ogre": 250,
	"giant skeleton": 300, "gigantic skeletal": 300,
	"nubja": 300, "frostborn iii": 200, "goblin thief": 200, "tobul": 250, "dresk the lost": 300,
	"starn the aimless": 300, "plagued kobold hunter": 300, "wandering priestess": 300, "keller": 250,
	"krend catsbane": 300, "varae the chronicler": 300, "captain ealesha": 120, "creepyman": 150,
	"teledic": 150, "trapper mcgee": 250, "wandering herbalis": 300, "orc gatherer": 250,
	"surefall shambler": 200, "final thought": 150,
}

type pos struct {
	X, Y, Z float32
	Facing  int
}

type move struct {
	ID     int64  `json:"id"`
	World  uint8  `json:"world"`
	Name   string `json:"name"`
	Camp   int    `json:"camp"`
	Why    string `json:"why"`
	Before pos    `json:"before"`
	After  pos    `json:"after"`
}

type wander struct {
	ID     int64   `json:"id"`
	World  uint8   `json:"world"`
	Name   string  `json:"name"`
	Radius float32 `json:"radius"`
	Note   string  `json:"note"`
	X      float32 `json:"x"`
	Z      float32 `json:"z"`
}

type camp struct {
	ID     int     `json:"id"`
	World  uint8   `json:"world"`
	X      float32 `json:"x"`
	Z      float32 `json:"z"`
	N      int     `json:"n"`
	Radius float32 `json:"radius"`
	Why    string  `json:"why"`
	GM     string  `json:"gm,omitempty"`
	Kinds  string  `json:"kinds"`
	Moved  int     `json:"moved"`
	Failed int     `json:"failed"`
}

type planOut struct {
	Moves  []move         `json:"moves"`
	Wander []wander       `json:"wander"`
	Camps  []camp         `json:"camps"`
	Left   map[string]int `json:"left"` // why rows were left alone, counted
}

// gmSpot is a GM's petition: spread the camp at (x, z) in world and make it
// roam (-gm file: world x z petition note).
type gmSpot struct {
	World    uint8
	X, Z     float32
	Petition string
	Kinds    map[string]bool // empty: the camps at the spot, whatever their kinds
}

var gmFile = ""

func readGM(path string) []gmSpot {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var out []gmSpot
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		fs := strings.Split(l, "\t")
		if len(fs) < 4 {
			log.Fatalf("gm file: %q", l)
		}
		w, _ := strconv.Atoi(fs[0])
		x, _ := strconv.ParseFloat(fs[1], 32)
		z, _ := strconv.ParseFloat(fs[2], 32)
		g := gmSpot{World: uint8(w), X: float32(x), Z: float32(z), Petition: fs[len(fs)-1], Kinds: map[string]bool{}}
		if len(fs) >= 5 {
			for _, k := range strings.Split(fs[3], ",") {
				if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
					g.Kinds[k] = true
				}
			}
		}
		out = append(out, g)
	}
	return out
}

func dist(ax, az, bx, bz float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(az-bz)))
}

func plan(w *entity.World, db *database.SQLiteDB, rows []entity.DisperseRow) planOut {
	rnd := rand.New(rand.NewSource(20261112))
	out := planOut{Left: map[string]int{}}
	byID := map[entity.EntityID]*entity.DisperseRow{}
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	protected := protectedRows(db)
	reports := campReports(db)
	gms := readGM(gmFile)

	// names counted per world: a rare one is a named creature
	nameCount := map[string]int{}
	for _, r := range rows {
		nameCount[fmt.Sprintf("%d|%s", r.World, strings.ToLower(r.Name))]++
	}
	// pets by owner
	pets := map[entity.EntityID][]*entity.DisperseRow{}
	for i := range rows {
		if rows[i].PetOf != 0 {
			pets[rows[i].PetOf] = append(pets[rows[i].PetOf], &rows[i])
		}
	}

	// the creatures: what a camp is made of
	var cre []*entity.DisperseRow
	for i := range rows {
		r := &rows[i]
		if r.NPCType&0x80 != 0 || r.Model == 0 || r.SpawnOnly || r.PetOf != 0 {
			continue
		}
		if r.WhyCode == entity.DisperseWhyTownsfolk && !gmTouch(gms, r) {
			continue // townsfolk: a town's, not a camp's
		}
		cre = append(cre, r)
	}
	camps := link(cre, campLink)
	// a GM's camps first, each one camp: the rows of its kinds within gmKinds of
	// its spot, or the camps within gmReach of it
	gmOf := map[int]string{}
	{
		taken := map[entity.EntityID]bool{}
		var gmCamps [][]*entity.DisperseRow
		for _, g := range gms {
			var gc []*entity.DisperseRow
			if len(g.Kinds) > 0 {
				for _, r := range cre {
					if !taken[r.ID] && r.World == g.World && g.Kinds[strings.ToLower(r.Name)] && dist(r.X, r.Z, g.X, g.Z) <= gmKinds {
						gc = append(gc, r)
						taken[r.ID] = true
					}
				}
			} else {
				for _, c := range camps {
					near := false
					for _, r := range c {
						if r.World == g.World && dist(r.X, r.Z, g.X, g.Z) <= gmReach {
							near = true
						}
					}
					if near {
						for _, r := range c {
							if !taken[r.ID] {
								gc = append(gc, r)
								taken[r.ID] = true
							}
						}
					}
				}
			}
			if len(gc) > 0 {
				gmOf[len(gmCamps)] = g.Petition
				gmCamps = append(gmCamps, gc)
			} else {
				log.Printf("GM spot %s: no creatures found", g.Petition)
			}
		}
		var rest [][]*entity.DisperseRow
		for _, c := range camps {
			var k []*entity.DisperseRow
			for _, r := range c {
				if !taken[r.ID] {
					k = append(k, r)
				}
			}
			if len(k) > 0 {
				rest = append(rest, k)
			}
		}
		camps = append(gmCamps, rest...)
	}

	// every creature's place, for spacing (by world and 16-unit cell)
	occ := newOccupancy()
	for i := range rows {
		r := &rows[i]
		if r.NPCType&0x80 == 0 || r.Model != 0 {
			occ.add(r.ID, r.World, r.X, r.Z, r.Body)
		}
	}

	why := func(r *entity.DisperseRow, gm bool) string {
		switch {
		case r.Pathed:
			return "a path"
		case r.Camp:
			return "a GM's /camp spawn"
		case r.Guard:
			return "a town guard"
		case protected[int64(r.ID)] != "" && !gm:
			return protected[int64(r.ID)]
		case !gm && nearReport(reports, r):
			return "a camp report's ground"
		case !r.OnGround || r.InWater:
			return "off the ground or in the water"
		case nameCount[fmt.Sprintf("%d|%s", r.World, strings.ToLower(r.Name))] <= namedMax:
			return "named (rare)"
		}
		return ""
	}

	campID := 0
	for ci, c := range camps {
		campID++
		gm := gmOf[ci]
		stacked := stackedRows(c)
		grid := len(c) >= 4 && gridLanes(c)
		if len(stacked) == 0 && !grid && gm == "" {
			continue
		}
		cw := ""
		switch {
		case gm != "":
			cw = "gm"
		case grid:
			cw = "rows"
		default:
			cw = "stacked"
		}
		var cx, cz float32
		kinds := map[string]int{}
		for _, r := range c {
			cx += r.X
			cz += r.Z
			kinds[strings.ToLower(r.Name)]++
		}
		cx /= float32(len(c))
		cz /= float32(len(c))
		var rad float32
		for _, r := range c {
			rad = max(rad, dist(r.X, r.Z, cx, cz))
		}
		var body float32 = 1
		for _, r := range c {
			body = max(body, r.Body)
		}
		spacing := max(minSpacing, 2*body+2)
		want := spacing * float32(math.Sqrt(float64(len(c)))) / 1.6
		if gm != "" {
			want = max(want*1.6, 30) // the GM's word: spread out
		}
		R := min(max(rad, want, 8), 70)
		cp := camp{ID: campID, World: c[0].World, X: cx, Z: cz, N: len(c), Radius: R, Why: cw, GM: gm, Kinds: kindList(kinds)}

		// which rows move
		var movers []*entity.DisperseRow
		for _, r := range c {
			if cw == "stacked" && !stacked[r.ID] {
				continue
			}
			if y := why(r, gm != ""); y != "" {
				out.Left[y]++
				continue
			}
			movers = append(movers, r)
		}
		sort.Slice(movers, func(i, j int) bool { return movers[i].ID < movers[j].ID })
		// the rows that stay are where they are; the movers leave their spots
		for _, r := range movers {
			occ.remove(r.ID, r.World, r.X, r.Z)
		}
		for _, r := range movers {
			np, ok := place(w, rnd, occ, r, c, cx, cz, R, spacing)
			if !ok {
				cp.Failed++
				occ.add(r.ID, r.World, r.X, r.Z, r.Body) // back where it was
				out.Left["no room found"]++
				continue
			}
			cp.Moved++
			out.Moves = append(out.Moves, move{ID: int64(r.ID), World: r.World, Name: r.Name, Camp: campID, Why: cw,
				Before: pos{r.X, r.Y, r.Z, facingByte(r.Facing)}, After: np})
			occ.add(r.ID, r.World, np.X, np.Z, r.Body)
			// its pets with it, as they stood beside it
			for _, p := range pets[r.ID] {
				if protected[int64(p.ID)] != "" || !p.OnGround || p.InWater {
					continue
				}
				px, pz := np.X+(p.X-r.X), np.Z+(p.Z-r.Z)
				if y, ok := w.DisperseWalk(r.ID, np.X, np.Y, np.Z, px, pz); ok {
					occ.remove(p.ID, p.World, p.X, p.Z)
					out.Moves = append(out.Moves, move{ID: int64(p.ID), World: p.World, Name: p.Name, Camp: campID, Why: "pet of a moved row",
						Before: pos{p.X, p.Y, p.Z, facingByte(p.Facing)}, After: pos{px, y, pz, np.Facing}})
					occ.add(p.ID, p.World, px, pz, p.Body)
				}
			}
		}
		// roaming: a GM's camp all of it; a standing kind's camp one in four
		if gm != "" {
			for _, r := range c {
				if r.Pathed || r.Camp || r.Guard || nameCount[fmt.Sprintf("%d|%s", r.World, strings.ToLower(r.Name))] <= namedMax {
					continue
				}
				if !r.OnGround || r.InWater {
					continue
				}
				out.Wander = append(out.Wander, wander{ID: int64(r.ID), World: r.World, Name: r.Name, Radius: max(R+40, 100),
					Note: "GM " + gm + ": spread out and roam", X: r.X, Z: r.Z})
			}
		}
		out.Camps = append(out.Camps, cp)
	}

	// standing kinds: a camp's wanderers, lone creatures, far roamers
	got := map[int64]bool{}
	for _, wd := range out.Wander {
		got[wd.ID] = true
	}
	for _, c := range link(cre, loneReach) { // a camp of a standing kind, spread or not
		var still []*entity.DisperseRow
		for _, r := range c {
			if got[int64(r.ID)] || r.Wanders {
				continue
			}
			if r.WhyCode != entity.DisperseWhyNotWild && r.WhyCode != entity.DisperseWhyUndead {
				continue // a GM's npcmove row stands it, or it is off the ground, or townsfolk
			}
			if why(r, false) != "" || keepsPost(r.Name) {
				continue
			}
			still = append(still, r)
		}
		if len(still) == 0 {
			continue
		}
		sort.Slice(still, func(i, j int) bool { return still[i].ID < still[j].ID })
		if len(c) == 1 {
			r := still[0]
			out.Wander = append(out.Wander, wander{ID: int64(r.ID), World: r.World, Name: r.Name, Radius: 0,
				Note: "a lone creature of a standing kind wanders", X: r.X, Z: r.Z})
			got[int64(r.ID)] = true
			continue
		}
		if len(c) < 3 {
			continue // a pair stands together
		}
		var cx, cz, rad float32
		for _, r := range c {
			cx += r.X / float32(len(c))
			cz += r.Z / float32(len(c))
		}
		for _, r := range c {
			rad = max(rad, dist(r.X, r.Z, cx, cz))
		}
		n := (len(still) + 3) / 4
		perm := rnd.Perm(len(still))
		for k := 0; k < n; k++ {
			r := still[perm[k]]
			out.Wander = append(out.Wander, wander{ID: int64(r.ID), World: r.World, Name: r.Name, Radius: min(max(rad+25, 45), 90),
				Note: "one in four of a standing camp wanders round it", X: r.X, Z: r.Z})
			got[int64(r.ID)] = true
		}
	}
	for i := range rows {
		r := &rows[i]
		rr, ok := farRoamers[strings.ToLower(r.Name)]
		if !ok || got[int64(r.ID)] || r.Pathed || r.Camp || r.SpawnOnly || r.NPCType&0x80 != 0 || r.Model == 0 || r.PetOf != 0 {
			continue
		}
		if !r.OnGround || r.InWater {
			out.Left["far roamer off the ground"]++
			continue
		}
		if protected[int64(r.ID)] != "" {
			out.Left["far roamer: "+protected[int64(r.ID)]]++
			continue
		}
		out.Wander = append(out.Wander, wander{ID: int64(r.ID), World: r.World, Name: r.Name, Radius: rr,
			Note: "a far roamer roams its ground (the owner: \"they only move a small distance\")", X: r.X, Z: r.Z})
		got[int64(r.ID)] = true
	}
	sort.Slice(out.Moves, func(i, j int) bool { return out.Moves[i].ID < out.Moves[j].ID })
	sort.Slice(out.Wander, func(i, j int) bool { return out.Wander[i].ID < out.Wander[j].ID })
	return out
}

// loneReach links a standing kind's camps, spread ones and a dungeon's rooms
// too: one alone within it wanders, two stand together, one in four of three
// or more wanders round them.
const loneReach = 40

// keepsPost reports a kind that keeps its post: a guard's word in its name
// (entity/wildlife.go's wildlifeGuards), or the fire giants (F3
// 20261026-042712, jib: "Fire Giants are roaming currently when they should
// not be").
func keepsPost(name string) bool {
	n := strings.ToLower(name)
	if strings.Contains(n, "fire giant") {
		return true
	}
	for _, w := range strings.FieldsFunc(n, func(c rune) bool { return c < 'a' || c > 'z' }) {
		switch w {
		case "guard", "guardian", "sentry", "sentinel", "warden", "warder", "watcher", "watchman", "watchmen", "keeper",
			"protector", "defender", "gatekeeper", "bodyguard", "lookout", "captain", "king", "queen", "lord", "overlord":
			return true
		}
	}
	return false
}

func gmTouch(gms []gmSpot, r *entity.DisperseRow) bool {
	for _, g := range gms {
		if r.World == g.World && dist(r.X, r.Z, g.X, g.Z) <= gmKinds {
			return true
		}
	}
	return false
}

func kindList(k map[string]int) string {
	type kv struct {
		k string
		n int
	}
	var l []kv
	for a, b := range k {
		l = append(l, kv{a, b})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].n > l[j].n || l[i].n == l[j].n && l[i].k < l[j].k })
	var s []string
	for i, e := range l {
		if i == 4 {
			s = append(s, "...")
			break
		}
		s = append(s, fmt.Sprintf("%s x%d", e.k, e.n))
	}
	return strings.Join(s, ", ")
}

func facingByte(f float32) int {
	b := int(math.Round(float64(f) / (2 * math.Pi) * 256))
	return ((b % 256) + 256) % 256
}

// place finds r a spot in its camp (the file's comment), false for none.
func place(w *entity.World, rnd *rand.Rand, occ *occupancy, r *entity.DisperseRow, c []*entity.DisperseRow, cx, cz, R, spacing float32) (pos, bool) {
	inTown := w.DisperseInTown(r.World, r.X, r.Z)
	for relax := 0; relax < 4; relax++ {
		sp := spacing * float32(math.Pow(0.85, float64(relax)))
		for try := 0; try < 60; try++ {
			a := rnd.Float64() * 2 * math.Pi
			d := R * float32(math.Sqrt(rnd.Float64()))
			x, z := cx+d*float32(math.Sin(a)), cz+d*float32(math.Cos(a))
			if !occ.free(r.World, x, z, min(max(sp, 2*r.Body+1), occCell-0.1)) {
				continue
			}
			if !inTown && w.DisperseInTown(r.World, x, z) {
				continue
			}
			y, ok := w.DisperseWalk(r.ID, r.X, r.Y, r.Z, x, z)
			if !ok || y-r.Y > levelReach || r.Y-y > levelReach {
				continue
			}
			return pos{x, y, z, rnd.Intn(256)}, true
		}
	}
	return pos{}, false
}

// link is the camps: creatures within campLink of one another (and within 6
// up or down), by world.
func link(rs []*entity.DisperseRow, reach float32) [][]*entity.DisperseRow {
	type key struct {
		w      uint8
		cx, cz int32
	}
	cell := func(r *entity.DisperseRow) key {
		return key{r.World, int32(math.Floor(float64(r.X / reach))), int32(math.Floor(float64(r.Z / reach)))}
	}
	grid := map[key][]int{}
	for i, r := range rs {
		grid[cell(r)] = append(grid[cell(r)], i)
	}
	par := make([]int, len(rs))
	for i := range par {
		par[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for par[i] != i {
			par[i] = par[par[i]]
			i = par[i]
		}
		return i
	}
	for i, r := range rs {
		k := cell(r)
		for dx := int32(-1); dx <= 1; dx++ {
			for dz := int32(-1); dz <= 1; dz++ {
				for _, j := range grid[key{k.w, k.cx + dx, k.cz + dz}] {
					o := rs[j]
					if j > i && dist(r.X, r.Z, o.X, o.Z) <= reach && math.Abs(float64(r.Y-o.Y)) < 6 {
						if a, b := find(i), find(j); a != b {
							par[a] = b
						}
					}
				}
			}
		}
	}
	m := map[int][]*entity.DisperseRow{}
	for i := range rs {
		m[find(i)] = append(m[find(i)], rs[i])
	}
	var out [][]*entity.DisperseRow
	for _, v := range m {
		sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0].ID < out[j][0].ID })
	return out
}

// stackedRows is the rows of a camp standing on another's spot (stackNear),
// all but the first (by id) of each stack.
func stackedRows(c []*entity.DisperseRow) map[entity.EntityID]bool {
	out := map[entity.EntityID]bool{}
	for i, r := range c {
		if out[r.ID] {
			continue
		}
		for _, o := range c[i+1:] {
			if !out[o.ID] && dist(r.X, r.Z, o.X, o.Z) < stackNear && math.Abs(float64(r.Y-o.Y)) < 3 {
				out[o.ID] = true
			}
		}
	}
	return out
}

// gridLanes reports a camp laid out in rows: seen along its own axis, or the
// world's, or diagonally, most of it stands in lanes (a lane: three or more
// within 1.5 across) evenly stepped along them (the steps'
// spread under a fifth of their mean, the mean 2 to 12): an import's
// grid, not a camp.
func gridLanes(c []*entity.DisperseRow) bool {
	n := len(c)
	var mx, mz float64
	for _, r := range c {
		mx += float64(r.X)
		mz += float64(r.Z)
	}
	mx /= float64(n)
	mz /= float64(n)
	var sxx, szz, sxz float64
	for _, r := range c {
		dx, dz := float64(r.X)-mx, float64(r.Z)-mz
		sxx += dx * dx
		szz += dz * dz
		sxz += dx * dz
	}
	pca := 0.5 * math.Atan2(2*sxz, sxx-szz)
	for _, ang := range []float64{0, math.Pi / 4, pca, pca + math.Pi/2} {
		ca, sa := math.Cos(ang), math.Sin(ang)
		type uv struct{ u, v float64 }
		pts := make([]uv, n)
		for i, r := range c {
			dx, dz := float64(r.X)-mx, float64(r.Z)-mz
			pts[i] = uv{dx*ca + dz*sa, -dx*sa + dz*ca}
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].v < pts[j].v })
		inLanes := 0
		lane := []uv{pts[0]}
		flush := func() {
			if len(lane) >= 3 {
				sort.Slice(lane, func(i, j int) bool { return lane[i].u < lane[j].u })
				var steps []float64
				for k := 1; k < len(lane); k++ {
					steps = append(steps, lane[k].u-lane[k-1].u)
				}
				var m, s float64
				for _, x := range steps {
					m += x
				}
				m /= float64(len(steps))
				for _, x := range steps {
					s += (x - m) * (x - m)
				}
				s = math.Sqrt(s / float64(len(steps)))
				if m >= 2 && m <= 12 && s < 0.2*m {
					inLanes += len(lane)
				}
			}
		}
		for k := 1; k < n; k++ {
			if pts[k].v-lane[0].v > 1.5 { // a lane is a line, 1.5 across at most
				flush()
				lane = lane[:0]
			}
			lane = append(lane, pts[k])
		}
		flush()
		if inLanes >= 4 && float64(inLanes) >= 0.6*float64(n) {
			return true
		}
	}
	return false
}

// protectedRows is the rows left alone, and why: a GM's or a camp report's
// placing or move (gm_audit), a quest's tie (queststeps.npcid, questwalls), a
// wander row already (npcwander).
func protectedRows(db *database.SQLiteDB) map[int64]string {
	out := map[int64]string{}
	rs, err := db.Query(`SELECT rowkey, action, COALESCE(before,''), COALESCE(after,'') FROM gm_audit WHERE tbl='npcs'`)
	if err != nil {
		log.Fatal(err)
	}
	for rs.Next() {
		var key, act, b, a string
		if err := rs.Scan(&key, &act, &b, &a); err != nil {
			log.Fatal(err)
		}
		var k struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal([]byte(key), &k) != nil || k.ID == 0 {
			continue
		}
		switch act {
		case "insert", "create":
			out[k.ID] = "placed by a GM or a tool (gm_audit)"
		case "update":
			var bm, am map[string]any
			json.Unmarshal([]byte(b), &bm)
			json.Unmarshal([]byte(a), &am)
			for _, c := range []string{"x_coord", "y_coord", "z_coord"} {
				if fmt.Sprint(bm[c]) != fmt.Sprint(am[c]) {
					if out[k.ID] == "" {
						out[k.ID] = "moved by a GM or a camp report (gm_audit)"
					}
				}
			}
		}
	}
	rs.Close()
	for _, q := range []struct{ sql, why string }{
		{`SELECT DISTINCT npcid FROM queststeps WHERE npcid != 0`, "a quest step's NPC"},
		{`SELECT npcid FROM questwalls`, "a quest wall"},
		{`SELECT npcid FROM npcwander`, "a wander row already"},
		{`SELECT npcid FROM npcpaths`, "a path"},
	} {
		rs, err := db.Query(q.sql)
		if err != nil {
			log.Fatal(err)
		}
		for rs.Next() {
			var id int64
			rs.Scan(&id)
			if out[id] == "" {
				out[id] = q.why
			}
		}
		rs.Close()
	}
	return out
}

type report struct {
	world uint8
	x, z  float32
}

func campReports(db *database.SQLiteDB) []report {
	rs, err := db.Query(`SELECT world, x, z FROM campfix_reports WHERE status IN ('applied', 'partial', 'waiting')`)
	if err != nil {
		log.Fatal(err)
	}
	defer rs.Close()
	var out []report
	for rs.Next() {
		var r report
		rs.Scan(&r.world, &r.x, &r.z)
		out = append(out, r)
	}
	return out
}

func nearReport(rp []report, r *entity.DisperseRow) bool {
	for _, p := range rp {
		if p.world == r.World && dist(p.x, p.z, r.X, r.Z) <= reportNear {
			return true
		}
	}
	return false
}

// occupancy is every creature's place, for the spacing.
type occupancy struct {
	cells map[[3]int32]map[entity.EntityID][3]float32
}

const occCell = 16

func newOccupancy() *occupancy {
	return &occupancy{cells: map[[3]int32]map[entity.EntityID][3]float32{}}
}

func occKey(w uint8, x, z float32) [3]int32 {
	return [3]int32{int32(w), int32(math.Floor(float64(x / occCell))), int32(math.Floor(float64(z / occCell)))}
}

func (o *occupancy) add(id entity.EntityID, w uint8, x, z, body float32) {
	k := occKey(w, x, z)
	if o.cells[k] == nil {
		o.cells[k] = map[entity.EntityID][3]float32{}
	}
	o.cells[k][id] = [3]float32{x, z, body}
}

func (o *occupancy) remove(id entity.EntityID, w uint8, x, z float32) {
	delete(o.cells[occKey(w, x, z)], id)
}

// free reports no creature within need of (x, z) (need under 16).
func (o *occupancy) free(w uint8, x, z, need float32) bool {
	k := occKey(w, x, z)
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			for _, p := range o.cells[[3]int32{k[0], k[1] + dx, k[2] + dz}] {
				if dist(p[0], p[1], x, z) < need {
					return false
				}
			}
		}
	}
	return true
}

func writePlan(path string, p planOut) {
	b, err := json.MarshalIndent(p, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
}
