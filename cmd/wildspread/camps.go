package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// campPoint is one of EQOA-Data's map camps (Map/zones.json, the pois of
// category "camp": 403, the XP camps players go to), on its map's sheet.
type campPoint struct {
	name   string
	mx, my float64
}

// campFits lay EQOA-Data's map over a world: x = a*mx + b, z = c*my + d,
// tools/gen_camp_teles.py's least-squares fit of the map's named points to
// this server's NPCs of those names (2026-10-31: Tunaria 591 of 790 points,
// median 8.8 off; Odus 51 of 66, median 155.5). The sheet holds those two.
var campFits = map[int][4]float64{
	0: {2.13137, -3637.9, 2.13087, 3789.6},
	2: {2.35663, 3999.4, 2.32015, -16607.0},
}

func readCamps(dir string) ([]campPoint, error) {
	b, err := os.ReadFile(filepath.Join(dir, "Map", "zones.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Pois []struct {
			Name     string  `json:"name"`
			X        float64 `json:"x"`
			Y        float64 `json:"y"`
			Category string  `json:"category"`
		} `json:"pois"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	var out []campPoint
	for _, p := range doc.Pois {
		if p.Category == "camp" {
			out = append(out, campPoint{p.Name, p.X, p.Y})
		}
	}
	return out, nil
}

// at is the camp's place in world w by its fit, false for a world the sheet
// does not hold.
func (c campPoint) at(w int) (x, z float64, ok bool) {
	f, ok := campFits[w]
	if !ok {
		return 0, 0, false
	}
	return f[0]*c.mx + f[1], f[2]*c.my + f[3], true
}
