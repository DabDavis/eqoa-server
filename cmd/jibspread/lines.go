package main

// Copied from cmd/disperseplan (its line finding and its rows left alone).

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

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

const laneWidth = 2.5

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
			if pts[k].v-lane[0].v > laneWidth { // a lane is a line, laneWidth across at most
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
