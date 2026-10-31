package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// report writes the plan: plan.json, moves.csv, fills.csv and each world's
// before.png and after.png (land by its distance from the nearest creature:
// green near, yellow, orange, red far; water blue, towns cyan, land the tool
// does not reach grey; creatures white, new ones magenta).
func (p *plan) report(dir string) error {
	type zoneStat struct {
		Zone         string
		Land         int
		Before300    float64 `json:"before_beyond_300_pct"`
		After300     float64 `json:"after_beyond_300_pct"`
		Moved, Added int
		AddedKinds   map[string]int `json:",omitempty"`
	}
	type worldOut struct {
		World      int
		Clumps     int
		Moved      int
		Added      int
		NoWildNear int `json:"gap_samples_with_no_wildlife_near"`
		Kept       map[string]int
		Before     coverage
		After      coverage
		AddedKinds map[string]int
		Zones      []*zoneStat `json:",omitempty"`
		MoveDist   [3]float64  `json:"move_distance_median_p90_max"`
	}
	var outs []worldOut
	mf, err := os.Create(filepath.Join(dir, "moves.csv"))
	if err != nil {
		return err
	}
	defer mf.Close()
	mw := csv.NewWriter(mf)
	mw.Write([]string{"world", "id", "name", "level", "clump", "from_x", "from_y", "from_z", "to_x", "to_y", "to_z", "distance"})
	ff, err := os.Create(filepath.Join(dir, "fills.csv"))
	if err != nil {
		return err
	}
	defer ff.Close()
	fw := csv.NewWriter(ff)
	fw.Write([]string{"world", "name", "level", "place_level", "source", "x", "y", "z", "facing", "template_row", "zone"})
	for _, wp := range p.worlds {
		o := worldOut{World: wp.w, Clumps: wp.clumps, Moved: len(wp.moves), Added: len(wp.fills), NoWildNear: wp.empty,
			Kept: wp.kept, Before: wp.before, After: wp.after, AddedKinds: map[string]int{}}
		var dists []float64
		for _, m := range wp.moves {
			d := math.Hypot(m.x-m.row.x, m.z-m.row.z)
			dists = append(dists, d)
			mw.Write([]string{fmt.Sprint(wp.w), fmt.Sprint(m.row.id), m.row.name, fmt.Sprint(m.row.level), fmt.Sprint(m.clump),
				f1(m.row.x), f1(m.row.y), f1(m.row.z), f1(m.x), f1(m.y), f1(m.z), f1(d)})
		}
		sort.Float64s(dists)
		if len(dists) > 0 {
			o.MoveDist = [3]float64{dists[len(dists)/2], dists[len(dists)*9/10], dists[len(dists)-1]}
		}
		for _, f := range wp.fills {
			o.AddedKinds[f.kind.spelling]++
			fw.Write([]string{fmt.Sprint(wp.w), f.kind.spelling, fmt.Sprint(f.level), fmt.Sprint(f.placeLevel), f.source,
				f1(f.x), f1(f.y), f1(f.z), fmt.Sprint(f.facing), fmt.Sprint(f.template.id), p.zoneName(wp.w, f.x, f.z)})
		}
		if wp.w == 0 {
			zs := map[string]*zoneStat{}
			get := func(x, z float64) *zoneStat {
				n := p.zoneName(0, x, z)
				if zs[n] == nil {
					zs[n] = &zoneStat{Zone: n, AddedKinds: map[string]int{}}
				}
				return zs[n]
			}
			bix, aix := newIndex(100), newIndex(100)
			for _, q := range wp.pointsBefore {
				bix.add(q.x, q.z, 0)
			}
			for _, q := range wp.pointsAfter {
				aix.add(q.x, q.z, 0)
			}
			land := wp.land
			for k := range land.kind {
				if !land.placeable(k) {
					continue
				}
				x, z := land.xz(k)
				s := get(x, z)
				s.Land++
				if d, _ := bix.nearest(x, z, 301); d > 300 {
					s.Before300++
				}
				if d, _ := aix.nearest(x, z, 301); d > 300 {
					s.After300++
				}
			}
			for _, m := range wp.moves {
				get(m.x, m.z).Moved++
			}
			for _, f := range wp.fills {
				s := get(f.x, f.z)
				s.Added++
				s.AddedKinds[f.kind.spelling]++
			}
			for _, s := range zs {
				if s.Land > 0 {
					s.Before300 = math.Round(1000*s.Before300/float64(s.Land)) / 10
					s.After300 = math.Round(1000*s.After300/float64(s.Land)) / 10
				}
				o.Zones = append(o.Zones, s)
			}
			sort.Slice(o.Zones, func(a, b int) bool { return o.Zones[a].Before300 > o.Zones[b].Before300 })
		}
		outs = append(outs, o)
		if err := p.drawMap(wp, filepath.Join(dir, fmt.Sprintf("%d-before.png", wp.w)), wp.pointsBefore, nil); err != nil {
			return err
		}
		if err := p.drawMap(wp, filepath.Join(dir, fmt.Sprintf("%d-after.png", wp.w)), wp.pointsAfter, wp.fills); err != nil {
			return err
		}
	}
	mw.Flush()
	fw.Flush()
	b, err := json.MarshalIndent(outs, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plan.json"), b, 0o644)
}

func f1(v float64) string { return fmt.Sprintf("%.1f", v) }

// zoneName is Tunaria's zone at (x, z) by WORDS.TXT, else its word's number.
func (p *plan) zoneName(w int, x, z float64) string {
	if w != 0 {
		return ""
	}
	word := tunariaZone(x, z)
	if word < len(p.words) {
		return strings.TrimSpace(p.words[word])
	}
	return fmt.Sprint(word)
}

func (p *plan) drawMap(wp *worldPlan, path string, points []pt, fills []fill) error {
	land := wp.land
	img := image.NewRGBA(image.Rect(0, 0, land.nx, land.nz))
	ix := newIndex(100)
	for _, q := range points {
		ix.add(q.x, q.z, 0)
	}
	for k := range land.kind {
		i, j := k%land.nx, k/land.nx
		var c color.RGBA
		switch land.kind[k] {
		case sNone:
			c = color.RGBA{0, 0, 0, 255}
		case sBad:
			c = color.RGBA{10, 30, 110, 255}
		case sTown:
			c = color.RGBA{0, 170, 170, 255}
		case sZone:
			c = color.RGBA{50, 50, 70, 255}
		default:
			if !land.placeable(k) {
				c = color.RGBA{70, 70, 70, 255}
				break
			}
			x, z := land.xz(k)
			d, _ := ix.nearest(x, z, 1001)
			switch {
			case d <= 150:
				c = color.RGBA{40, 200, 40, 255}
			case d <= 250:
				c = color.RGBA{30, 130, 30, 255}
			case d <= 300:
				c = color.RGBA{200, 200, 40, 255}
			case d <= 500:
				c = color.RGBA{230, 130, 20, 255}
			default:
				c = color.RGBA{220, 40, 20, 255}
			}
		}
		img.SetRGBA(i, j, c)
	}
	dot := func(x, z float64, c color.RGBA) {
		k := land.cell(x, z)
		if k >= 0 {
			img.SetRGBA(k%land.nx, k/land.nx, c)
		}
	}
	for _, q := range points {
		if q.v >= 0 {
			dot(q.x, q.z, color.RGBA{255, 255, 255, 255})
		}
	}
	for _, f := range fills {
		dot(f.x, f.z, color.RGBA{255, 0, 255, 255})
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
