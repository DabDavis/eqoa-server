// Command dungeonfill plans a dungeon's population: the owner, 2026-11-15,
// "runnyeye dungeon is not populated yet". The open world's fill
// (cmd/wildspread) reads the land as its highest floor, which in a warren of
// halls over halls is the hill above it; this reads every floor (graph.go),
// keeps the pieces the dungeon's own creatures stand on, and fills its open
// floor by height band with the kinds each band holds now (plan.go).
//
// It never writes the database (it opens a copy read only): it writes the
// plan (plan.json, plan.csv), maps of each band before and after
// (before.png, after.png: the floor by height, the creatures standing as red
// dots, the plan's as green) and counts by kind and level, for a seed tool
// (tools/runnyeye_20261115.py) to apply with its own undo.
//
//	dungeonfill -db COPY.db -ground worlds/ground -world 0 -box x0,z0,x1,z1[;...]
//	    -bands lo:hi,... [-inside-above Y] [-camp-gap 24] [-group 40,40,20]
//	    [-beside "kind:Named:n;..."] [-seed N] [-out DIR]
package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

type row struct {
	id    int64
	name  string
	x, z  float64
	y     float32
	level int
	typ   int
}

func readBoxes(s string) ([]box, error) {
	var out []box
	for _, part := range strings.Split(s, ";") {
		f := strings.Split(part, ",")
		if len(f) != 4 {
			return nil, fmt.Errorf("box %q: x0,z0,x1,z1", part)
		}
		var v [4]float64
		for i := range f {
			n, err := strconv.ParseFloat(strings.TrimSpace(f[i]), 64)
			if err != nil {
				return nil, err
			}
			v[i] = n
		}
		out = append(out, box{math.Min(v[0], v[2]), math.Min(v[1], v[3]), math.Max(v[0], v[2]), math.Max(v[1], v[3])})
	}
	return out, nil
}

func readBands(s string) ([]band, error) {
	var out []band
	for _, part := range strings.Split(s, ",") {
		var b band
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%f:%f", &b.lo, &b.hi); err != nil || b.hi <= b.lo {
			return nil, fmt.Errorf("band %q: lo:hi", part)
		}
		out = append(out, b)
	}
	return out, nil
}

func readWeights(s string) ([]float64, error) {
	var out []float64
	for _, f := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("weight %q", f)
		}
		out = append(out, v)
	}
	return out, nil
}

// loadRows is world's npcs rows in the boxes; held are the names and rows a
// quest raises or seeds (npcspawnonly, quest_npc_seed_rows): never a
// template.
func loadRows(db *sql.DB, world int, boxes []box) (rows []row, held map[string]bool, err error) {
	held = map[string]bool{}
	if rs, err := db.Query(`SELECT name FROM npcspawnonly`); err == nil {
		for rs.Next() {
			var n string
			rs.Scan(&n)
			held[strings.ToLower(n)] = true
		}
		rs.Close()
	}
	seeded := map[int64]bool{}
	if rs, err := db.Query(`SELECT npc_row FROM quest_npc_seed_rows`); err == nil {
		for rs.Next() {
			var n int64
			rs.Scan(&n)
			seeded[n] = true
		}
		rs.Close()
	}
	rs, err := db.Query(`SELECT id, npc_name, x_coord, y_coord, z_coord, npc_level, COALESCE(npc_type,0) FROM npcs WHERE world = ? ORDER BY id`, world)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var r row
		var y float64
		if err := rs.Scan(&r.id, &r.name, &r.x, &y, &r.z, &r.level, &r.typ); err != nil {
			return nil, nil, err
		}
		r.y = float32(y)
		if seeded[r.id] || held["#"+strconv.FormatInt(r.id, 10)] {
			held["#"+strconv.FormatInt(r.id, 10)] = true
		}
		for _, b := range boxes {
			if b.in(r.x, r.z) {
				rows = append(rows, r)
				break
			}
		}
	}
	return rows, held, rs.Err()
}

func main() {
	dbPath := flag.String("db", "eqoa.db", "a COPY of the server's database (opened read only)")
	groundDir := flag.String("ground", "worlds/ground", "the ground files")
	world := flag.Int("world", 0, "the world")
	boxFlag := flag.String("box", "", "the dungeon's boxes, x0,z0,x1,z1[;...]")
	bandsFlag := flag.String("bands", "", "the height bands, lo:hi,...: each filled with its own kinds")
	insideY := flag.Float64("inside-above", 1e9, "floor at or over this height is filled only where roofed (a hall's, not the hill's)")
	minPiece := flag.Int("min-piece", 100, "the dungeon's floor comes in joined pieces of at least this many floors (a canopy's few left out)")
	cell := flag.Float64("cell", 3, "the floor grid's step")
	campGap := flag.Float64("camp-gap", 24, "a camp's centre from any creature, at least")
	memberGap := flag.Float64("member-gap", 4, "a camp's creatures from each other, at least")
	rowGap := flag.Float64("row-gap", 9, "a camp's others from a creature standing, at least")
	groupFlag := flag.String("group", "40,40,20", "a camp's size by weight: 1,2,3...")
	excludeFlag := flag.String("exclude", `(?i)(spirit|prisoner|treasure|protector|gem thief|hoarder|royal guard|surly|^filthbat$)`, "kinds never filled (a regexp)")
	wandersFlag := flag.String("wanders", `(?i)bat\b`, "kinds the server walks on the terrain's floor (placed only on it)")
	minDeg := flag.Int("min-joins", 5, "open floor: joined to this many of its 8 neighbours (8: a room's; 5: a 2-cell tunnel's middle)")
	beside := flag.String("beside", "", "camps of a kind beside a named creature: kind:Named:n[;...]")
	seed := flag.Int64("seed", 1115, "the plan's random seed")
	out := flag.String("out", "dungeonfill", "the report's folder")
	px := flag.Float64("px", 4, "a map's pixels a unit")
	check := flag.String("check", "", "check a plan.json's creatures against -world's ground (a wall within a body or no floor) and stop")
	flag.Parse()
	if *check != "" {
		os.Exit(checkPlan(*check, *groundDir, uint8(*world)))
	}
	boxes, err := readBoxes(*boxFlag)
	if err != nil {
		log.Fatal(err)
	}
	bands, err := readBands(*bandsFlag)
	if err != nil {
		log.Fatal(err)
	}
	gw, err := readWeights(*groupFlag)
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.OpenReadOnly(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	rows, held, err := loadRows(db, *world, boxes)
	if err != nil {
		log.Fatal(err)
	}
	gr, err := ground.Open(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	env, _ := ground.OpenEnv(*groundDir)
	g := build(gr, env, uint8(*world), boxes, *cell)
	seeds := map[int32]int{}
	for _, r := range rows {
		if k := g.nearest(r.x, float64(r.y), r.z, 4); k >= 0 {
			seeds[g.nodes[k].comp]++
		}
	}
	keep := map[int32]bool{}
	for c, n := range seeds {
		if n >= 5 { // a dungeon's piece, not a lone creature's ledge
			keep[c] = true
		}
	}
	exclude := regexp.MustCompile(*excludeFlag)
	var templ []row
	for _, r := range rows {
		if !held[strings.ToLower(r.name)] && !held["#"+strconv.FormatInt(r.id, 10)] {
			templ = append(templ, r)
		}
	}
	in := g.insideOf(keep, float32(*insideY), *minPiece)
	p := &planner{g: g, rows: templ, bands: bands, keep: keep, in: in, campGap: *campGap,
		memberGap: *memberGap, minDeg: *minDeg, rowGap: *rowGap, groupW: gw, exclude: exclude,
		wanders: regexp.MustCompile(*wandersFlag), rnd: rand.New(rand.NewSource(*seed))}
	plan := p.plan()
	if *beside != "" {
		plan = append(plan, p.besides(*beside, rows, len(plan))...)
	}
	plan = clearOfWalls(gr, uint8(*world), plan)
	for i := range plan {
		if plan[i].template == 0 {
			log.Fatalf("no template row of %q", plan[i].name)
		}
	}
	os.MkdirAll(*out, 0o755)
	if err := report(*out, newView(g, keep, in, *px), rows, plan, bands, map[string]any{
		"world": *world, "box": *boxFlag, "bands": *bandsFlag, "inside_above": *insideY, "cell": *cell, "min_piece": *minPiece,
		"camp_gap": *campGap, "min_joins": *minDeg, "member_gap": *memberGap, "row_gap": *rowGap, "group": *groupFlag,
		"exclude": *excludeFlag, "wanders": *wandersFlag, "beside": *beside, "seed": *seed, "db": filepath.Base(*dbPath),
	}); err != nil {
		log.Fatal(err)
	}
}

// besides places camps of a kind round a named creature (Runnyeye: King
// Merdid's royal guards, board entry 766 §2), on the floor two to three steps
// from him, each a copy of the kind's nearest row at its level.
func (p *planner) besides(spec string, rows []row, n int) []placed {
	var out []placed
	for _, part := range strings.Split(spec, ";") {
		f := strings.Split(part, ":")
		if len(f) != 3 {
			log.Fatalf("-beside %q: kind:Named:n", part)
		}
		count, _ := strconv.Atoi(f[2])
		var who *row
		for i := range rows {
			if rows[i].name == f[1] {
				who = &rows[i]
			}
		}
		if who == nil {
			log.Fatalf("-beside: no %q", f[1])
		}
		t := p.template(f[0], who.x, float64(who.y), who.z)
		var tr *row
		for i := range rows {
			if rows[i].id == t {
				tr = &rows[i]
			}
		}
		if tr == nil {
			log.Fatalf("-beside: no %q row", f[0])
		}
		k := p.g.nearest(who.x, float64(who.y), who.z, 4)
		all := make([]bool, len(p.g.nodes))
		for i := range all {
			all[i] = p.in[i] && len(p.g.adj[i]) >= p.minDeg
		}
		ring := p.ringTo(k, all, 6)
		var spots []node
		for _, s := range ring {
			nd := p.g.nodes[s]
			if math.Abs(float64(nd.y-who.y)) > 1 {
				continue
			}
			ok := true
			for _, o := range spots {
				if math.Hypot(o.x-nd.x, o.z-nd.z) < p.memberGap {
					ok = false
				}
			}
			for _, r := range rows {
				if math.Hypot(r.x-nd.x, r.z-nd.z) < 3.5 && math.Abs(float64(r.y-nd.y)) < 2 {
					ok = false
				}
			}
			if ok {
				spots = append(spots, nd)
			}
			if len(spots) == count {
				break
			}
		}
		for _, s := range spots {
			out = append(out, placed{name: f[0], template: t, level: tr.level, x: round2(s.x), y: round2(float64(s.y)), z: round2(s.z),
				facing: facingToward(s.x, s.z, who.x, who.z) ^ 128, band: p.bandOf(s.y), camp: -1, terrain: s.terrain})
		}
		if len(spots) < count {
			log.Printf("-beside %s: %d of %d placed", part, len(spots), count)
		}
	}
	return out
}

// clearOfWalls drops a planned creature with a wall within clearR of it
// (the way from its spot a body's height up to clearR off, eight ways,
// meets a wall) or no floor under it (ground.Support within 0.3): never a
// creature in a wall, whatever the grid's columns missed.
func clearOfWalls(gr *ground.Ground, world uint8, plan []placed) []placed {
	const clearR = 0.8 // a goblin's body (CLIENT's capsule for a small creature: creature-collision-20261024.md)
	var out []placed
	for _, p := range plan {
		x, y, z := float32(p.x), float32(p.y), float32(p.z)
		ok := true
		if f, has := gr.Support(world, x, y, z, 1, 1); !has {
			ok = false
		} else {
			y, p.y = f, round2(float64(f)) // the floor at the spot itself, not the column's middle
		}
		for a := 0; ok && a < 8; a++ {
			h := float64(a) * math.Pi / 4
			tx, tz := x+float32(clearR*math.Sin(h)), z+float32(clearR*math.Cos(h))
			if gr.Wall(world, x, y+bodyUp, z, tx, y+bodyUp, tz) {
				ok = false
			}
		}
		if ok {
			out = append(out, p)
		} else {
			log.Printf("dropped %s at (%.2f, %.2f, %.2f): a wall within %.1f or no floor", p.name, p.x, p.y, p.z, clearR)
		}
	}
	return out
}

// checkPlan reports a plan's creatures that clearOfWalls would drop on
// world's ground (Runnyeye's instance, world 14, is the citadel again): 1
// when any would be.
func checkPlan(path, groundDir string, world uint8) int {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var raw struct {
		Adds []map[string]any `json:"adds"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Fatal(err)
	}
	gr, err := ground.Open(groundDir)
	if err != nil {
		log.Fatal(err)
	}
	var plan []placed
	for _, a := range raw.Adds {
		plan = append(plan, placed{name: a["name"].(string), x: a["x"].(float64), y: a["y"].(float64), z: a["z"].(float64)})
	}
	kept := clearOfWalls(gr, world, plan)
	fmt.Printf("%d of %d clear on world %d's ground\n", len(kept), len(plan), world)
	if len(kept) != len(plan) {
		return 1
	}
	return 0
}

// facingToward is the facing byte (256 a turn, 0 toward +z) from (x, z) to (tx, tz).
func facingToward(x, z, tx, tz float64) int {
	d := math.Atan2(tx-x, tz-z) * 180 / math.Pi
	return int(math.Round(math.Mod(d+360, 360)/360*256)) % 256
}

// report writes the plan and the maps, and prints the counts.
func report(dir string, v *view, rows []row, plan []placed, bands []band, params map[string]any) error {
	type planRow struct {
		Name     string  `json:"name"`
		Template int64   `json:"template_row"`
		Level    int     `json:"level"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Z        float64 `json:"z"`
		Facing   int     `json:"facing"`
		Band     int     `json:"band"`
		Camp     int     `json:"camp"`
		Terrain  bool    `json:"terrain_floor"`
	}
	var pr []planRow
	for _, p := range plan {
		pr = append(pr, planRow{p.name, p.template, p.level, p.x, p.y, p.z, p.facing, p.band, p.camp, p.terrain})
	}
	tmpl := map[int64]map[string]any{}
	for _, r := range rows {
		for _, p := range plan {
			if p.template == r.id {
				tmpl[r.id] = map[string]any{"name": r.name, "x": r.x, "y": r.y, "z": r.z}
			}
		}
	}
	js, _ := json.MarshalIndent(map[string]any{"params": params, "templates": tmpl, "adds": pr}, "", " ")
	if err := os.WriteFile(filepath.Join(dir, "plan.json"), js, 0o644); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "plan.csv"))
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"name", "level", "x", "y", "z", "facing", "band", "camp", "template_row", "terrain_floor"})
	for _, p := range plan {
		w.Write([]string{p.name, strconv.Itoa(p.level), fmt.Sprint(p.x), fmt.Sprint(p.y), fmt.Sprint(p.z), strconv.Itoa(p.facing),
			strconv.Itoa(p.band), strconv.Itoa(p.camp), strconv.FormatInt(p.template, 10), strconv.FormatBool(p.terrain)})
	}
	w.Flush()
	f.Close()

	// counts
	type kl struct {
		name  string
		level int
	}
	byKL := map[kl]int{}
	byBand := map[int]int{}
	for _, p := range plan {
		byKL[kl{p.name, p.level}]++
		byBand[p.band]++
	}
	var keys []kl
	for k := range byKL {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].level < keys[j].level
	})
	fmt.Printf("%d adds\n", len(plan))
	for i, b := range bands {
		have := 0
		for _, r := range rows {
			if r.y >= b.lo && r.y < b.hi {
				have++
			}
		}
		fmt.Printf("band %v-%v: %d standing, %d added\n", b.lo, b.hi, have, byBand[i])
	}
	for _, k := range keys {
		fmt.Printf("  %-22s L%-3d x%d\n", k.name, k.level, byKL[k])
	}

	// maps: a sheet of the bands, before and after
	for _, which := range []string{"before", "after"} {
		var tiles []image.Image
		for _, b := range bands {
			var marks []mark
			for _, r := range rows {
				marks = append(marks, mark{r.x, r.z, r.y, color.RGBA{255, 50, 50, 255}})
			}
			if which == "after" {
				for _, p := range plan {
					marks = append(marks, mark{p.x, p.z, float32(p.y), color.RGBA{60, 255, 60, 255}})
				}
			}
			tmp := filepath.Join(dir, fmt.Sprintf("%s_band_%03.0f_%03.0f.png", which, b.lo, b.hi))
			if err := v.renderBand(tmp, marks, b.lo, b.hi); err != nil {
				return err
			}
			fi, err := os.Open(tmp)
			if err != nil {
				return err
			}
			im, err := png.Decode(fi)
			fi.Close()
			if err != nil {
				return err
			}
			tiles = append(tiles, im)
		}
		cols := 4
		tw, th := tiles[0].Bounds().Dx(), tiles[0].Bounds().Dy()
		rowsN := (len(tiles) + cols - 1) / cols
		sheet := image.NewRGBA(image.Rect(0, 0, cols*(tw+8), rowsN*(th+8)))
		draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{255, 255, 255, 255}), image.Point{}, draw.Src)
		for i, t := range tiles {
			at := image.Pt((i%cols)*(tw+8), (i/cols)*(th+8))
			draw.Draw(sheet, image.Rectangle{at, at.Add(image.Pt(tw, th))}, t, image.Point{}, draw.Src)
		}
		fo, err := os.Create(filepath.Join(dir, which+".png"))
		if err != nil {
			return err
		}
		if err := png.Encode(fo, sheet); err != nil {
			fo.Close()
			return err
		}
		fo.Close()
	}
	return nil
}
