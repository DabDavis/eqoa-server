// Command hazardscan reads where a world's zone actors of the given sprites stand and writes each as
// a hazards row (database/hazards.go): a capsule along the placed mesh's long axis across (its
// principal axis in x/z), its radius the distance across that holds most of the mesh, its height
// the actor's. tools/seed_hazards.py writes the rows (petition 1937: the Toxxulia cave's spiky
// vines, the disc's SimpleSprites B9F5C1A9, B9F5C1AA and B9F5C1AB, every one of them in Odus, zone
// 20).
//
//	hazardscan -iso <disc> -file ODUS.ESF -world 2 -sprites 0xb9f5c1a9,0xb9f5c1aa,0xb9f5c1ab \
//	    -name "Spiky vines" -damage 1000 -every 2000 -words "The spiky vines tear at you for %d." > vines.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/pkg/pkg/esf"
)

// row is a hazards row as tools/seed_hazards.py reads it.
type row struct {
	Name    string  `json:"name"`
	World   int     `json:"world"`
	X1      float64 `json:"x1"`
	Y1      float64 `json:"y1"`
	Z1      float64 `json:"z1"`
	X2      float64 `json:"x2"`
	Y2      float64 `json:"y2"`
	Z2      float64 `json:"z2"`
	Radius  float64 `json:"radius"`
	Below   float64 `json:"below"`
	Above   float64 `json:"above"`
	Damage  int     `json:"damage"`
	EveryMS int     `json:"every_ms"`
	Words   string  `json:"words"`
	Note    string  `json:"note"`
}

func main() {
	iso := flag.String("iso", "", "the disc image (or -esf)")
	file := flag.String("file", "ODUS.ESF", "the world file on the disc")
	path := flag.String("esf", "", "a world file on disk, in place of -iso")
	world := flag.Int("world", 2, "the server's world number")
	sprites := flag.String("sprites", "", "the sprites' DictIDs, commas")
	name := flag.String("name", "Spiky vines", "the rows' name")
	damage := flag.Int("damage", 1000, "a hit's damage")
	every := flag.Int("every", 2000, "ms from one hit to the next")
	words := flag.String("words", "", "what a hit tells, %d its damage")
	share := flag.Float64("share", 0.8, "the share of the mesh's points across within the radius")
	minR := flag.Float64("min-radius", 1.0, "the least radius")
	maxR := flag.Float64("max-radius", 3.0, "the most radius")
	flag.Parse()
	want := map[int32]bool{}
	for _, s := range strings.Split(*sprites, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(s), 0, 32)
		if err != nil {
			log.Fatalf("-sprites %q: %v", s, err)
		}
		want[int32(uint32(v))] = true
	}
	var f *esf.ObjFile
	var err error
	if *path != "" {
		f, err = esf.Open(*path)
	} else {
		f, err = esf.OpenISOFile(*iso, *file)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	root, err := f.Root()
	if err != nil {
		log.Fatal(err)
	}
	w := root.Child(esf.TypeWorld)
	if w == nil {
		log.Fatal("no world in it")
	}
	var out []row
	for zn, zi := range w.ChildrenOfType(esf.TypeZone) {
		obj, err := f.GetObject(zi)
		if err != nil {
			continue
		}
		z, ok := obj.(*esf.Zone)
		if !ok {
			continue
		}
		acts, err := z.GetZoneActors(f)
		if err != nil || acts == nil {
			continue
		}
		rae, err := acts.GetActorsWithRoomIDs(f)
		if err != nil {
			continue
		}
		for _, a := range rae {
			pl := a.Actor.Placement
			if !want[pl.SpriteID] {
				continue
			}
			o, err := f.FindObject(pl.SpriteID)
			s, ok := o.(*esf.SimpleSprite)
			if err != nil || !ok {
				continue
			}
			pb, err := s.GetPrimBuffer(f)
			if err != nil || pb == nil {
				continue
			}
			var pts [][2]float64
			for _, l := range pb.VertexLists {
				for _, v := range l.Vertices {
					p := esf.Point{X: v.X, Y: v.Y, Z: v.Z}
					pl.Transform(&p)
					pts = append(pts, [2]float64{float64(p.X), float64(p.Z)})
				}
			}
			if len(pts) == 0 {
				continue
			}
			r := capsule(pts, *share, *minR, *maxR)
			y := float64(pl.Pos.Y)
			out = append(out, row{Name: *name, World: *world, X1: r[0], Y1: y, Z1: r[1], X2: r[2], Y2: y, Z2: r[3], Radius: r[4],
				Below: 2, Above: 4, Damage: *damage, EveryMS: *every, Words: *words,
				Note: fmt.Sprintf("hazardscan: %s's actor of sprite %#08x, zone %d, room %d, at (%.1f, %.1f, %.1f), scale %.2f",
					*file, uint32(pl.SpriteID), zn, a.RoomID, pl.Pos.X, pl.Pos.Y, pl.Pos.Z, pl.Scale)})
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d rows", len(out))
}

// capsule is the points' capsule across: x1, z1, x2, z2, radius. Its axis is their principal axis
// through their middle, its ends their farthest along it, its radius the distance across that holds
// share of them, within [lo, hi]; the ends come in by the radius (the round ends hold them).
func capsule(pts [][2]float64, share, lo, hi float64) [5]float64 {
	var mx, mz float64
	for _, p := range pts {
		mx += p[0]
		mz += p[1]
	}
	n := float64(len(pts))
	mx, mz = mx/n, mz/n
	var sxx, sxz, szz float64
	for _, p := range pts {
		dx, dz := p[0]-mx, p[1]-mz
		sxx += dx * dx
		sxz += dx * dz
		szz += dz * dz
	}
	a := 0.5 * math.Atan2(2*sxz, sxx-szz)
	ux, uz := math.Cos(a), math.Sin(a)
	t0, t1 := math.Inf(1), math.Inf(-1)
	across := make([]float64, 0, len(pts))
	for _, p := range pts {
		dx, dz := p[0]-mx, p[1]-mz
		t := dx*ux + dz*uz
		t0, t1 = math.Min(t0, t), math.Max(t1, t)
		across = append(across, math.Abs(-dx*uz+dz*ux))
	}
	sort.Float64s(across)
	r := across[min(len(across)-1, int(share*float64(len(across))))]
	r = math.Min(math.Max(r, lo), hi)
	if t1-t0 > 2*r {
		t0, t1 = t0+r, t1-r
	} else {
		t0, t1 = (t0+t1)/2, (t0+t1)/2
	}
	round := func(v float64) float64 { return math.Round(v*100) / 100 }
	return [5]float64{round(mx + t0*ux), round(mz + t0*uz), round(mx + t1*ux), round(mz + t1*uz), round(r)}
}
