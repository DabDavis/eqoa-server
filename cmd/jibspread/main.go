// Command jibspread plans the GM Jibrish's petitions 5050, 5052 and 5053 (2026-11-14: "a static pile of non roaming
// NPC's ... They need to roam throughout the zone"; "a pack of unroaming mobs in a row ... All of these need to roam
// throughout the zone"; "unmoving mobs in a row, these need to roam throughout the zone") for
// tools/jib_resize_20261114.py to apply. It reads a COPY of the server's database (opening it migrates its schema, as
// the server does) and the baked ground, and writes the plan (JSON); it never writes the database's rows.
//
// The rows of unmoving creatures laid out in lines (one import's pattern: a kind per lane, evenly stepped, 3 to 12
// apart, none wandering of its own kind, or a stack of one kind in a line) are found in every world: a line is
// creatures within lineLink of one another, three or more that may be moved (not a path's, a camp spawn's, a quest's,
// a spawn-only name, a pet, a guard, townsfolk, in a town, the NPC bit, a named creature, off the ground or in the
// water, nor moved or placed by a GM or a tool), a third or more of them standing (no wander of their own kind: mostly wildlife wanders round its spawn already) and
// laid in lanes (gridLanes) or along one line. The plan spreads those in the zones the GM toured (-zones: CLIENT's
// 2,000-unit cells, x/2000,z/2000) over their zone's walkable ground (every point reached from the line by steps of
// landStep a creature's wander could walk: entity.DisperseWalk, a floor all the way, no wall, no fall, no water; not
// in a town, margin from the zone's edges, the highest floor there), farthest first from every other spread spot and
// standing creature, and gives each a wander row of its own (npcwander) of radius roamRadius, cut to its distance from
// the zone's edge so its roam stays in its zone. The lines elsewhere are listed, not planned.
//
//	jibspread -db COPY.db -ground DIR -check plan.json   (after the tool's --apply on a copy)
//	jibspread -db COPY.db -ground DIR -zones 2:2,3:2,2:3,3:3 -must 5050:7691.5:7691.4,... -out plan.json
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
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

const (
	lineLink   = 8.0   // creatures this near one another are one line or pile
	zoneSize   = 2000. // CLIENT's zone cell
	landStep   = 20.0  // the land graph's step
	edgeMargin = 100.0 // a spread spot this far inside its zone's edge at least
	roamRadius = 300.0 // a spread creature's roam, cut to its distance from the zone's edge
	roamLeast  = 100.0
	namedMax   = 2   // a name this rare in its world is a named creature
	mustNear   = 25. // a GM's spot (where he stood) names the creatures within this of it
)

type pos struct {
	X, Y, Z float32
	Facing  int
}

type move struct {
	Ticket string  `json:"ticket"`
	Line   int     `json:"line"`
	ID     int64   `json:"id"`
	World  uint8   `json:"world"`
	Name   string  `json:"name"`
	Before pos     `json:"before"`
	After  pos     `json:"after"`
	Radius float32 `json:"radius"`
	Zone   string  `json:"zone"`
}

type line struct {
	ID      int      `json:"id"`
	World   uint8    `json:"world"`
	X       float32  `json:"x"`
	Z       float32  `json:"z"`
	Cell    [2]int   `json:"cell"`
	N       int      `json:"n"`
	Movable int      `json:"movable"`
	Stand   int      `json:"stand"` // of the movable, those that stand of their kind
	Kinds   string   `json:"kinds"`
	IDs     []int64  `json:"ids"`
	Planned bool     `json:"planned"`
	Ticket  string   `json:"ticket,omitempty"`
	Left    []string `json:"left,omitempty"`
}

type planOut struct {
	Moves []move `json:"moves"`
	Lines []line `json:"lines"`
}

func dist(ax, az, bx, bz float32) float32 { return float32(math.Hypot(float64(ax-bx), float64(az-bz))) }

func main() {
	dbPath := flag.String("db", "", "a COPY of the server's database")
	groundDir := flag.String("ground", "", "the baked ground")
	zonesArg := flag.String("zones", "", "the zones to spread in, world 0: cx:cz,...")
	mustArg := flag.String("must", "", "the GM's spots: ticket:x:z,... (world 0)")
	out := flag.String("out", "", "the plan (JSON)")
	check := flag.String("check", "", "after an apply: say how each row of this plan (JSON) moves now, and stop")
	flag.Parse()
	if *dbPath == "" || *groundDir == "" || (*out == "" && *check == "") {
		log.Fatal("-db, -ground and -out (or -check) are needed")
	}
	zones := map[[2]int]bool{}
	for _, s := range strings.Split(*zonesArg, ",") {
		if p := strings.Split(s, ":"); len(p) == 2 {
			a, _ := strconv.Atoi(p[0])
			b, _ := strconv.Atoi(p[1])
			zones[[2]int{a, b}] = true
		}
	}
	type must struct {
		ticket string
		x, z   float32
	}
	var musts []must
	for _, s := range strings.Split(*mustArg, ",") {
		if p := strings.Split(s, ":"); len(p) == 3 {
			x, _ := strconv.ParseFloat(p[1], 32)
			z, _ := strconv.ParseFloat(p[2], 32)
			musts = append(musts, must{p[0], float32(x), float32(z)})
		}
	}
	w, db := loadWorld(*dbPath, *groundDir)
	rows := w.DisperseSurvey()
	if *check != "" {
		checkPlan(rows, *check)
		return
	}
	prot := protectedRows(db)
	names := map[[2]any]int{}
	for _, r := range rows {
		names[[2]any{r.World, strings.ToLower(r.Name)}]++
	}
	why := func(r *entity.DisperseRow) string {
		switch {
		case prot[int64(r.ID)] != "":
			return prot[int64(r.ID)]
		case r.Pathed:
			return "a path"
		case r.Camp:
			return "a camp spawn"
		case r.SpawnOnly:
			return "spawn only"
		case r.PetOf != 0:
			return "a pet"
		case r.Guard, r.Townsman, r.InTown:
			return "a guard, townsfolk or in a town"
		case r.NPCType&0x80 != 0:
			return "the NPC bit"
		case r.Model == 0:
			return "no body"
		case names[[2]any{r.World, strings.ToLower(r.Name)}] <= namedMax:
			return "a named creature"
		case !r.OnGround || r.InWater || r.Keeps == "water":
			return "off the ground or of the water"
		}
		return ""
	}
	var ptrs []*entity.DisperseRow
	for i := range rows {
		ptrs = append(ptrs, &rows[i])
	}
	var p planOut
	type todo struct {
		l   *line
		mv  []*entity.DisperseRow
		cel [2]int
	}
	var todos []todo
	// a GM's spot is its line: every creature within mustNear of it, whatever links it
	var groups [][]*entity.DisperseRow
	var gticket []string
	inMust := map[entity.EntityID]bool{}
	for _, m := range musts {
		var g []*entity.DisperseRow
		for _, r := range ptrs {
			if r.World == 0 && !inMust[r.ID] && dist(r.X, r.Z, m.x, m.z) <= mustNear {
				g = append(g, r)
				inMust[r.ID] = true
			}
		}
		sort.Slice(g, func(i, j int) bool { return g[i].ID < g[j].ID })
		groups = append(groups, g)
		gticket = append(gticket, m.ticket)
	}
	var rest []*entity.DisperseRow
	for _, r := range ptrs {
		if !inMust[r.ID] {
			rest = append(rest, r)
		}
	}
	for _, c := range link(rest, lineLink) {
		groups = append(groups, c)
		gticket = append(gticket, "")
	}
	for gi, c := range groups {
		if len(c) == 0 {
			continue
		}
		var mv []*entity.DisperseRow
		var left []string
		stand := 0
		for _, r := range c {
			if y := why(r); y != "" {
				left = append(left, fmt.Sprintf("%d %s: %s", r.ID, r.Name, y))
				continue
			}
			mv = append(mv, r)
			if !r.Wanders {
				stand++
			}
		}
		var mx, mz float32
		for _, r := range c {
			mx += r.X
			mz += r.Z
		}
		mx /= float32(len(c))
		mz /= float32(len(c))
		ticket := gticket[gi]
		if os.Getenv("JIBSPREAD_DEBUG") != "" {
			for _, r := range c {
				if fmt.Sprint(r.ID) == os.Getenv("JIBSPREAD_DEBUG") {
					for _, o := range c {
						log.Printf("debug %d %s (%.1f,%.1f) wanders %v %s left %q", o.ID, o.Name, o.X, o.Z, o.Wanders, o.Why, why(o))
					}
					log.Printf("debug lanes %v line %v", gridLanes(mv), oneLine(mv))
				}
			}
		}
		if ticket == "" && !(len(mv) >= 3 && (gridLanes(mv) || (stand == len(mv) && oneLine(mv)))) {
			continue
		}
		kinds := map[string]int{}
		var ids []int64
		for _, r := range c {
			kinds[r.Name]++
			ids = append(ids, int64(r.ID))
		}
		var ks []string
		for k, n := range kinds {
			ks = append(ks, fmt.Sprintf("%s x%d", k, n))
		}
		sort.Strings(ks)
		cel := [2]int{int(math.Floor(float64(mx / zoneSize))), int(math.Floor(float64(mz / zoneSize)))}
		l := line{ID: len(p.Lines) + 1, World: c[0].World, X: mx, Z: mz, Cell: cel, N: len(c), Movable: len(mv),
			Stand: stand, Kinds: strings.Join(ks, ", "), IDs: ids, Ticket: ticket, Left: left}
		l.Planned = c[0].World == 0 && (zones[cel] || ticket != "") && len(mv) > 0
		if ticket == "" && 3*stand < len(mv) {
			// mostly wildlife, which wanders round its spawn already: not the GM's "unmoving" rows
			l.Planned = false
		}
		if ticket == "" && len(left) > 0 {
			// a named creature's, a pet's, a guard's: a camp of someone's, not an import's line
			l.Planned = false
		}
		p.Lines = append(p.Lines, l)
		if l.Planned {
			todos = append(todos, todo{&p.Lines[len(p.Lines)-1], mv, cel})
		}
	}
	// spread, zone by zone: every planned line of a zone over that zone's land together
	byZone := map[[2]int][]int{}
	var zs [][2]int
	for i, t := range todos {
		if byZone[t.cel] == nil {
			zs = append(zs, t.cel)
		}
		byZone[t.cel] = append(byZone[t.cel], i)
	}
	rnd := rand.New(rand.NewSource(20261114))
	for _, zc := range zs {
		x0, z0 := float32(zc[0])*zoneSize, float32(zc[1])*zoneSize
		land := map[[2]int]float32{} // reachable grid point -> floor
		for _, i := range byZone[zc] {
			r := todos[i].mv[0]
			flood(w, r, x0, z0, land)
		}
		// candidate spots: inside the margin, not in a town, on the highest floor
		type cand struct {
			x, y, z float32
			d       float32 // to the nearest standing creature or chosen spot
		}
		var cs []cand
		for k, y := range land {
			x, z := x0+float32(k[0])*landStep, z0+float32(k[1])*landStep
			if x-x0 < edgeMargin || x0+zoneSize-x < edgeMargin || z-z0 < edgeMargin || z0+zoneSize-z < edgeMargin {
				continue
			}
			if w.DisperseInTown(0, x, z) {
				continue
			}
			if top, ok := w.DisperseFloor(0, x, 100000, z); !ok || top-y > 2 {
				continue
			}
			cs = append(cs, cand{x, y, z, math.MaxFloat32})
		}
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].x != cs[j].x {
				return cs[i].x < cs[j].x
			}
			return cs[i].z < cs[j].z
		})
		moving := map[entity.EntityID]bool{}
		for _, i := range byZone[zc] {
			for _, r := range todos[i].mv {
				moving[r.ID] = true
			}
		}
		near := func(x, z float32) float32 {
			d := float32(math.MaxFloat32)
			for _, r := range rows {
				if r.World == 0 && !moving[r.ID] && math.Abs(float64(r.X-x)) < 400 && math.Abs(float64(r.Z-z)) < 400 {
					d = min(d, dist(r.X, r.Z, x, z))
				}
			}
			return d
		}
		for i := range cs {
			cs[i].d = min(near(cs[i].x, cs[i].z), 400)
		}
		zname := fmt.Sprintf("zone %d,%d", zc[0], zc[1])
		for _, i := range byZone[zc] {
			t := todos[i]
			for _, r := range t.mv {
				if len(cs) == 0 {
					t.l.Left = append(t.l.Left, fmt.Sprintf("%d %s: no land left", r.ID, r.Name))
					continue
				}
				best := 0
				for j := range cs {
					if cs[j].d > cs[best].d {
						best = j
					}
				}
				c := cs[best]
				edge := min(c.x-x0, x0+zoneSize-c.x, c.z-z0, z0+zoneSize-c.z)
				rad := float32(math.Round(float64(max(roamLeast, min(roamRadius, edge)))))
				p.Moves = append(p.Moves, move{Ticket: t.l.Ticket, Line: t.l.ID, ID: int64(r.ID), World: 0, Name: r.Name,
					Before: pos{r.X, r.Y, r.Z, facingByte(r.Facing)}, After: pos{c.x, round2(c.y), c.z, rnd.Intn(256)},
					Radius: rad, Zone: zname})
				for j := range cs {
					cs[j].d = min(cs[j].d, dist(cs[j].x, cs[j].z, c.x, c.z))
				}
			}
		}
	}
	b, _ := json.MarshalIndent(p, "", " ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		log.Fatal(err)
	}
	planned := 0
	for _, l := range p.Lines {
		if l.Planned {
			planned++
		}
	}
	log.Printf("%d lines (%d planned), %d moves", len(p.Lines), planned, len(p.Moves))
}

func round2(v float32) float32 { return float32(math.Round(float64(v)*100) / 100) }

// flood adds to land every grid point of the zone at (x0, z0) creature r's wander could reach from where it stands by
// steps of landStep (entity.DisperseWalk).
func flood(w *entity.World, r *entity.DisperseRow, x0, z0 float32, land map[[2]int]float32) {
	n := int(zoneSize / landStep)
	type node struct {
		i, j int
		y    float32
	}
	si, sj := int(math.Round(float64((r.X-x0)/landStep))), int(math.Round(float64((r.Z-z0)/landStep)))
	sy, ok := w.DisperseWalk(r.ID, r.X, r.Y, r.Z, x0+float32(si)*landStep, z0+float32(sj)*landStep)
	if !ok {
		return
	}
	if _, seen := land[[2]int{si, sj}]; seen {
		return
	}
	land[[2]int{si, sj}] = sy
	q := []node{{si, sj, sy}}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			i, j := c.i+d[0], c.j+d[1]
			if i < 0 || j < 0 || i > n || j > n {
				continue
			}
			if _, seen := land[[2]int{i, j}]; seen {
				continue
			}
			fx, fz := x0+float32(c.i)*landStep, z0+float32(c.j)*landStep
			y, ok := w.DisperseWalk(r.ID, fx, c.y, fz, x0+float32(i)*landStep, z0+float32(j)*landStep)
			if !ok || math.Abs(float64(y-c.y)) > 12 {
				continue
			}
			land[[2]int{i, j}] = y
			q = append(q, node{i, j, y})
		}
	}
}

// oneLine reports creatures evenly stepped along one straight line: across it 1.5 at most from its axis, the steps
// along it 2 to 12 on the mean and each within 40% of the mean.
func oneLine(c []*entity.DisperseRow) bool {
	n := float64(len(c))
	var mx, mz float64
	for _, r := range c {
		mx += float64(r.X)
		mz += float64(r.Z)
	}
	mx /= n
	mz /= n
	var sxx, szz, sxz float64
	for _, r := range c {
		dx, dz := float64(r.X)-mx, float64(r.Z)-mz
		sxx += dx * dx
		szz += dz * dz
		sxz += dx * dz
	}
	ang := 0.5 * math.Atan2(2*sxz, sxx-szz)
	ca, sa := math.Cos(ang), math.Sin(ang)
	us := make([]float64, 0, len(c))
	for _, r := range c {
		dx, dz := float64(r.X)-mx, float64(r.Z)-mz
		if math.Abs(-dx*sa+dz*ca) > 1.5 {
			return false
		}
		us = append(us, dx*ca+dz*sa)
	}
	sort.Float64s(us)
	m := (us[len(us)-1] - us[0]) / float64(len(us)-1)
	if m < 2 || m > 12 {
		return false
	}
	for k := 1; k < len(us); k++ {
		if math.Abs(us[k]-us[k-1]-m) > 0.4*m {
			return false
		}
	}
	return true
}

func loadWorld(dbPath, groundDir string) (*entity.World, *database.SQLiteDB) {
	db, err := database.NewSQLite(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	store := database.NewNPCStore(db)
	w := entity.NewWorld()
	if m, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(m)
	} else {
		log.Fatal(err)
	}
	w.LoadNPCs(store)
	g, err := ground.Open(groundDir)
	if err != nil {
		log.Fatal(err)
	}
	w.SetGround(g)
	if e, err := ground.OpenEnv(groundDir); err == nil && e.Worlds() > 0 {
		w.SetEnv(e)
	} else {
		log.Fatalf("env: %v", err)
	}
	if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
		w.LoadNPCPaths(recs)
	} else {
		log.Fatal(err)
	}
	for _, f := range []func(*database.SQLiteDB) (int, error){w.LoadCampSpawns, w.LoadSocialRules, w.LoadMoveRules, w.LoadRowWander} {
		if _, err := f(db); err != nil {
			log.Fatal(err)
		}
	}
	if _, _, err := w.LoadUndeadTags(db); err != nil {
		log.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		log.Fatal(err)
	}
	if err := w.LoadFactions(db); err != nil {
		log.Fatal(err)
	}
	return w, db
}

func facingByte(f float32) int {
	b := int(math.Round(float64(f) / (2 * math.Pi) * 256))
	return ((b % 256) + 256) % 256
}

// checkPlan says, for every row the plan moved, whether the server would have it wander now (its spot on the ground,
// its own wander row), and counts those that would not.
func checkPlan(rows []entity.DisperseRow, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var p struct {
		Moves []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"moves"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		log.Fatal(err)
	}
	by := map[int64]*entity.DisperseRow{}
	for i := range rows {
		by[int64(rows[i].ID)] = &rows[i]
	}
	bad := 0
	for _, m := range p.Moves {
		r := by[m.ID]
		if r == nil || !r.Wanders || !r.OnGround || r.InWater || r.InTown {
			bad++
			if r != nil {
				log.Printf("check: %d %s at (%.1f, %.1f, %.1f): wanders %v (%s), on the ground %v, in water %v, in town %v",
					r.ID, r.Name, r.X, r.Y, r.Z, r.Wanders, r.Why, r.OnGround, r.InWater, r.InTown)
			} else {
				log.Printf("check: %d %s: no such row", m.ID, m.Name)
			}
		}
	}
	log.Printf("check: %d rows, %d would not wander", len(p.Moves), bad)
}
