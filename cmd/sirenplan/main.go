// Command sirenplan plans the GM Jibrish's siren camps (petition 1741,
// 2026-11-12: "Disperse the sirens in groups of 4 that are agro, but do not
// assist. Spread them out."), for tools/misc1112_20261112.py. It reads a COPY
// of the database and the baked ground and prints, for every enchanting siren
// and the waterlogged chest of its group, a spot its wander could walk to from
// where it stands (entity.DisperseWalk: a floor all the way, no wall, no fall,
// the water it keeps to). It never writes the database.
//
// Each camp's two stacked groups of four are moved apart along the line
// between them (each half of -push further out from the camp's middle), so
// their middles end at least 2 x 21 (the aggro radius) + a margin apart, and
// within a group the four stand on a ring of -ring round their chest.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

type spot struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Group  string  `json:"group"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Z      float32 `json:"z"`
	Facing int     `json:"facing"`
}

// the groups: a chest and its four (live Heirloom, 2026-11-12)
var groups = []struct {
	name    string
	chest   int64
	members []int64
}{
	{"Vastly Deep north", 118491, []int64{118490, 118492, 118498, 118499}},
	{"Vastly Deep south", 118512, []int64{118533, 118534, 118535, 118536}},
	{"West Toxx east", 118283, []int64{118259, 118260, 118261, 118282}},
	{"West Toxx west", 118264, []int64{118262, 118263, 118265, 118266}},
	{"Kerra Isle east", 118268, []int64{118267, 118269, 118270, 118271}},
	{"Kerra Isle west", 118273, []int64{118272, 118274, 118275, 118276}},
}

func main() {
	dbPath := flag.String("db", "", "a COPY of the server's database")
	groundDir := flag.String("ground", "", "the baked ground")
	push := flag.Float64("push", 16, "how much further out from the camp's middle each group goes")
	ring := flag.Float64("ring", 5, "the ring a group's four stand on round their chest")
	flag.Parse()
	db, err := database.NewSQLite(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	store := database.NewNPCStore(db)
	w := entity.NewWorld()
	if m, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(m)
	}
	w.LoadNPCs(store)
	g, err := ground.Open(*groundDir)
	if err != nil {
		log.Fatal(err)
	}
	w.SetGround(g)
	if e, err := ground.OpenEnv(*groundDir); err == nil {
		w.SetEnv(e)
	}
	rows := map[int64]entity.DisperseRow{}
	for _, r := range w.DisperseSurvey() {
		rows[int64(r.ID)] = r
	}
	var out []spot
	for gi := 0; gi < len(groups); gi += 2 {
		a, b := groups[gi], groups[gi+1]
		ca, cb := rows[a.chest], rows[b.chest]
		mx, mz := (ca.X+cb.X)/2, (ca.Z+cb.Z)/2
		for _, grp := range []struct {
			gr int
			c  entity.DisperseRow
		}{{gi, ca}, {gi + 1, cb}} {
			gp := groups[grp.gr]
			dx, dz := grp.c.X-mx, grp.c.Z-mz
			l := float32(math.Hypot(float64(dx), float64(dz)))
			ux, uz := dx/l, dz/l
			cx, cz := grp.c.X+ux*float32(*push), grp.c.Z+uz*float32(*push)
			cy, ok := w.DisperseWalk(grp.c.ID, grp.c.X, grp.c.Y, grp.c.Z, cx, cz)
			if !ok {
				log.Fatalf("%s: the chest cannot go to (%.1f, %.1f)", gp.name, cx, cz)
			}
			out = append(out, spot{gp.chest, grp.c.Name, gp.name, cx, cy, cz, (int(math.Round(float64(grp.c.Facing)/(2*math.Pi)*256))%256 + 256) % 256})
			for i, id := range gp.members {
				r, ok := rows[id]
				if !ok {
					log.Fatalf("%s: row %d not in the survey", gp.name, id)
				}
				fmt.Fprintf(os.Stderr, "row %d %s: wanders %v (%s), in water %v, keeps %q\n", id, r.Name, r.Wanders, r.Why, r.InWater, r.Keeps)
				placed := false
				for try := 0; try < 24 && !placed; try++ {
					ang := (float64(i)+0.5)*math.Pi/2 + float64(try)*0.13
					rr := float32(*ring) * float32(1-0.03*float64(try))
					x, z := cx+rr*float32(math.Sin(ang)), cz+rr*float32(math.Cos(ang))
					y, ok := w.DisperseWalk(r.ID, r.X, r.Y, r.Z, x, z)
					if !ok || y-r.Y > 12 || r.Y-y > 12 {
						continue
					}
					f := entity.DisperseHeading(x-cx, z-cz)
					fb := int(math.Round(float64(f)/(2*math.Pi)*256)) % 256
					out = append(out, spot{id, r.Name, gp.name, x, y, z, (fb + 256) % 256})
					placed = true
				}
				if !placed {
					log.Fatalf("%s: no spot for row %d", gp.name, id)
				}
			}
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Group != out[j].Group {
				d := math.Hypot(float64(out[i].X-out[j].X), float64(out[i].Z-out[j].Z))
				if d < 2*float64(entity.DefaultAggroRadius)+2 {
					fmt.Fprintf(os.Stderr, "close: %d (%s) and %d (%s) %.1f apart\n", out[i].ID, out[i].Group, out[j].ID, out[j].Group, d)
				}
			}
		}
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", " ")
	e.Encode(out)
}
