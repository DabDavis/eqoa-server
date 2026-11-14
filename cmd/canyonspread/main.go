// Command canyonspread surveys one stretch of a world for
// tools/data_1114c_20261114.py (the owner, 2026-11-14: "box canyon needs
// wildlife spread out"): the highest floor every -step units over -box, the
// part of it a creature walks to from -seed (rises under -rise a step, no
// wall between: cmd/wildspread's land), and each NPC there with the server's
// verdict on how it moves (entity's, as cmd/gameserver builds it at its start)
// and why another tool or table places it. It writes, into -out:
//
//	grid.bin    float32 heights, nx*nz, row by z (NaN: no floor)
//	reach.bin   one byte a sample: 1 reachable from -seed
//	grid.json   the grid's origin, step and size
//	npcs.json   the NPCs in -box
//
// -check FILE reads a plan (a JSON list of {"id", "x", "y", "z"}) and says for
// each place whether a creature stands there (the floor within 1 of y, no roof
// over it) and is joined to -seed; it exits 1 when one is not.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

type npcOut struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Level     int     `json:"level"`
	Type      int     `json:"type"`
	Movement  string  `json:"movement"`
	Protected string  `json:"protected,omitempty"`
	Floor     float64 `json:"floor"`
}

func main() {
	dbPath := flag.String("db", "", "the database (read only)")
	groundDir := flag.String("ground", "", "the ground files")
	world := flag.Int("world", 0, "the world")
	boxFlag := flag.String("box", "19600,19600,22400,22400", "x0,z0,x1,z1")
	seedFlag := flag.String("seed", "20957,20935", "x,z on the floor the reach is counted from")
	stepF := flag.Float64("step", 10, "the sample step")
	rise := flag.Float64("rise", 10, "the most two neighbouring samples may differ and join")
	out := flag.String("out", ".", "where the survey goes")
	check := flag.String("check", "", "a plan to check")
	flag.Parse()
	box := nums(*boxFlag, 4)
	seed := nums(*seedFlag, 2)
	g, err := ground.Open(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	w := uint8(*world)
	step := *stepF
	nx := int((box[2]-box[0])/step) + 1
	nz := int((box[3]-box[1])/step) + 1
	h := make([]float32, nx*nz)
	for j := 0; j < nz; j++ {
		for i := 0; i < nx; i++ {
			v, ok := g.Floor(w, float32(box[0]+float64(i)*step), 1e6, float32(box[1]+float64(j)*step))
			if !ok {
				v = float32(math.NaN())
			}
			h[j*nx+i] = v
		}
	}
	joins := func(a, b int) bool {
		ha, hb := h[a], h[b]
		if ha != ha || hb != hb || math.Abs(float64(ha-hb)) > *rise {
			return false
		}
		ax, az := box[0]+float64(a%nx)*step, box[1]+float64(a/nx)*step
		bx, bz := box[0]+float64(b%nx)*step, box[1]+float64(b/nx)*step
		return !g.Wall(w, float32(ax), ha+1, float32(az), float32(bx), hb+1, float32(bz))
	}
	reach := make([]byte, nx*nz)
	cell := func(x, z float64) int {
		i, j := int(math.Round((x-box[0])/step)), int(math.Round((z-box[1])/step))
		if i < 0 || j < 0 || i >= nx || j >= nz {
			return -1
		}
		return j*nx + i
	}
	s := cell(seed[0], seed[1])
	if s < 0 {
		log.Fatal("-seed is off -box")
	}
	stack := []int{s}
	reach[s] = 1
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		i, j := c%nx, c/nx
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			ii, jj := i+d[0], j+d[1]
			if ii < 0 || jj < 0 || ii >= nx || jj >= nz {
				continue
			}
			n := jj*nx + ii
			if reach[n] == 0 && joins(c, n) {
				reach[n] = 1
				stack = append(stack, n)
			}
		}
	}
	if *check != "" {
		os.Exit(checkPlan(g, w, *check, cell, reach))
	}
	must(os.MkdirAll(*out, 0o755))
	f, err := os.Create(filepath.Join(*out, "grid.bin"))
	must(err)
	must(binary.Write(f, binary.LittleEndian, h))
	must(f.Close())
	must(os.WriteFile(filepath.Join(*out, "reach.bin"), reach, 0o644))
	meta, _ := json.Marshal(map[string]any{"x0": box[0], "z0": box[1], "step": step, "nx": nx, "nz": nz, "seed": seed, "rise": *rise})
	must(os.WriteFile(filepath.Join(*out, "grid.json"), meta, 0o644))
	if *dbPath == "" {
		return
	}
	db, err := database.NewSQLite(*dbPath)
	must(err)
	defer db.Close()
	store := database.NewNPCStore(db)
	ew := entity.NewWorld()
	quiet(func() {
		players, err := store.PlayerModels()
		must(err)
		ew.SetPlayerModels(players)
		ew.LoadNPCs(store)
		ew.SetGround(g)
		if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
			ew.LoadNPCPaths(recs)
		}
		_, _, _ = ew.LoadUndeadTags(db)
		_, _ = ew.LoadMoveRules(db)
		_, _, _ = ew.LoadSpawnOnly(db)
	})
	protected := map[int64]string{}
	ids := func(why, q string) {
		rows, err := db.Query(q)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil && id > 0 && protected[id] == "" {
				protected[id] = why
			}
		}
	}
	ids("tools/seed_quest_npcs.py", `SELECT npc_row FROM quest_npc_seed_rows`)
	ids("a path (npcpaths)", `SELECT npcid FROM npcpaths`)
	ids("a path's seed (npcpaths_seed)", `SELECT npcid FROM npcpaths_seed`)
	ids("a sign (signtext)", `SELECT npcid FROM signtext`)
	ids("a quest step (queststeps)", `SELECT npcid FROM queststeps WHERE npcid > 0`)
	ids("a GM camp (gmcamps)", `SELECT npcid FROM gmcamps`)
	ids("a camp's spawn (campspawns)", `SELECT npcid FROM campspawns`)
	recs, err := store.GetAllNPCs()
	must(err)
	var list []npcOut
	for _, r := range recs {
		x, z := float64(r.XCoord), float64(r.ZCoord)
		if r.World != *world || x < box[0] || x > box[2] || z < box[1] || z > box[3] {
			continue
		}
		o := npcOut{ID: int64(r.ID), Name: r.NPCName, X: x, Y: float64(r.YCoord), Z: z, Level: r.NPCLevel, Type: r.NPCType,
			Protected: protected[int64(r.ID)], Floor: math.NaN()}
		if st, ok := ew.NPCStateOf(entity.EntityID(r.ID)); ok {
			o.Movement = st.Movement
		}
		if v, ok := g.Floor(w, float32(x), 1e6, float32(z)); ok {
			o.Floor = float64(v)
		}
		list = append(list, o)
	}
	for i := range list {
		if math.IsNaN(list[i].Floor) {
			list[i].Floor = -99999
		}
	}
	b, _ := json.MarshalIndent(list, "", " ")
	must(os.WriteFile(filepath.Join(*out, "npcs.json"), b, 0o644))
	fmt.Printf("%dx%d samples, %d reachable from the seed, %d NPCs\n", nx, nz, count(reach), len(list))
}

func checkPlan(g *ground.Ground, w uint8, path string, cell func(x, z float64) int, reach []byte) int {
	b, err := os.ReadFile(path)
	must(err)
	var raw []map[string]float64
	must(json.Unmarshal(b, &raw))
	bad := 0
	for _, p := range raw {
		x, y, z := p["x"], p["y"], p["z"]
		hh, _, roofed, ok := g.Stand(w, float32(x), float32(y)+1, float32(z))
		c := cell(x, z)
		why := ""
		switch {
		case !ok:
			why = "no floor"
		case math.Abs(float64(hh)-y) > 1:
			why = fmt.Sprintf("the floor is %.1f, not %.1f", hh, y)
		case roofed:
			why = "roofed"
		case c < 0 || reach[c] == 0:
			why = "not joined to the seed"
		}
		if why != "" {
			bad++
			fmt.Printf("row %v at (%.1f, %.1f, %.1f): %s\n", int64(p["id"]), x, y, z, why)
		}
	}
	fmt.Printf("%d places, %d bad\n", len(raw), bad)
	if bad > 0 {
		return 1
	}
	return 0
}

func count(b []byte) int {
	n := 0
	for _, v := range b {
		n += int(v)
	}
	return n
}

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

func quiet(f func()) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	f()
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
