package ground

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type holidayPlace struct {
	Name            string
	World           uint8
	X, Y, Z, Radius float32
}

func TestHolidayFootprints(t *testing.T) {
	dir, file := os.Getenv("EQOA_EVENT_GROUND"), os.Getenv("EQOA_EVENT_PLACES")
	if dir == "" || file == "" {
		t.Skip("requires the disc bake and event placements")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if worlds, _ := g.Walls(); worlds < 2 {
		t.Fatal("missing placed-object collision")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var points []holidayPlace
	if err = json.Unmarshal(raw, &points); err != nil {
		t.Fatal(err)
	}
	for _, p := range points {
		check := func(x, z float32) (float32, bool) {
			y, ok := g.Floor(p.World, x, p.Y+2, z)
			if !ok || math.Abs(float64(y-p.Y)) > 6 {
				return y, false
			}
			// Sweep the whole body and its approaches in both directions. The vertical ray
			// rejects furniture, roofs and other overlays even on otherwise flat ground.
			for ring := float32(0); ring <= p.Radius; ring += 1 {
				for i := 0; i < 24; i++ {
					a := float64(i) * math.Pi / 12
					px, pz := x+ring*float32(math.Cos(a)), z+ring*float32(math.Sin(a))
					py, ok := g.Floor(p.World, px, y+2, pz)
					if !ok || math.Abs(float64(py-y)) > 2 {
						return y, false
					}
					flags, ok := env.Flags(p.World, px, py+0.7, pz)
					if !ok || flags&(EnvWater|EnvSlime|EnvLava) != 0 {
						return y, false
					}
					if g.Placed(p.World, px, py+4, pz, px, py+0.15, pz) {
						return y, false
					}
					for _, h := range []float32{0.4, 1.6, 3} {
						if g.Wall(p.World, x, y+h, z, px, py+h, pz) || g.Wall(p.World, px, py+h, pz, x, y+h, z) {
							return y, false
						}
					}
				}
			}
			return y, true
		}
		if y, ok := check(p.X, p.Z); ok {
			t.Logf("%s at %.2f %.3f %.2f clear radius %.0f", p.Name, p.X, y, p.Z, p.Radius)
			continue
		}
		if os.Getenv("EQOA_EVENT_SEARCH") != "" {
			found := false
			for r := 2; r <= 90 && !found; r += 2 {
				for a := 0; a < 32; a++ {
					dx, dz := float32(math.Round(float64(r)*math.Cos(float64(a)*math.Pi/16))), float32(math.Round(float64(r)*math.Sin(float64(a)*math.Pi/16)))
					if y, ok := check(p.X+dx, p.Z+dz); ok {
						t.Logf("%s candidate %.2f %.3f %.2f", p.Name, p.X+dx, y, p.Z+dz)
						found = true
						break
					}
				}
			}
		}
		t.Errorf("%s footprint blocked at %.2f %.2f", p.Name, p.X, p.Z)
	}
}
