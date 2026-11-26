package entity

import (
	"math"
	"testing"
)

// A coach sets its rider down in front of its coachman, and on his own spot
// where there is no floor in front of him (Heirloom petition 2229: the
// Homelands' Guk, Coachman Inkcap on a floor at y -2 that ends beside him;
// the rider set down in front fell to -1000). The control: with floor there,
// in front of him as before.
func TestCoachSetsDownOnAFloor(t *testing.T) {
	for _, c := range []struct {
		name  string
		g     Ground
		front bool
	}{
		{"floor in front", flatGround{y: -2}, true},
		// beside him since petition 3301 (TestCoachSetsDownBesideWithNoFloorInFront),
		// never where the floor is missing
		{"no floor in front", flatGround{y: -2, holeX: 100, holeZ: 105, hole: 2}, true},
		{"the floor in front far under him", dropGround{}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := NewWorld()
			w.ground = c.g
			w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100}
			w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
			for _, d := range w.GetCoachDestinationsEveryWorld("Coachman Other") {
				front := distXZ(d.X, d.Z, 100, 100) > 4
				if front != c.front {
					t.Errorf("set down at (%.1f, %.1f): out from him %v, want %v", d.X, d.Z, front, c.front)
				}
				if distXZ(d.X, d.Z, 100, 105) < 2 && c.name != "floor in front" {
					t.Errorf("set down at (%.1f, %.1f), where the floor is missing", d.X, d.Z)
				}
				return
			}
			t.Fatal("no destination")
		})
	}
}

// dropGround has floor 30 under (100, 105), and at -2 elsewhere.
type dropGround struct{}

func (dropGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	if distXZ(x, z, 100, 105) < 2 {
		return -32, true
	}
	return -2, -2 <= y+2
}

// A rider set down in front of a coachman faces him (Heirloom petition 3091:
// "when you arrive at a coach you are not facing the coachman"): the landing's
// heading points from the landing at his spot, by the 0x00B2 heading's own
// convention (in front of a heading h is (sin h, cos h)), at every heading he
// may stand at. On his own spot (no floor in front) his way, as before. The
// control: the heading sent before, his own, points away from him.
func TestCoachRiderFacesTheCoachman(t *testing.T) {
	for _, h := range []float32{0, 0.7, 1.5708, 3.1, 4.2, 6.0} {
		w := NewWorld()
		w.ground = flatGround{y: -2}
		w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100, SpawnFacing: h}
		w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
		d := w.GetCoachDestinationsEveryWorld("Coachman Other")
		if len(d) != 1 {
			t.Fatalf("heading %v: %d destinations", h, len(d))
		}
		looks := func(face float32) float64 {
			// the cosine between the heading's way and the way to his spot
			fx, fz := math.Sin(float64(face)), math.Cos(float64(face))
			tx, tz := float64(100-d[0].X), float64(100-d[0].Z)
			return (fx*tx + fz*tz) / math.Hypot(tx, tz)
		}
		if c := looks(d[0].Facing); c < 0.999 {
			t.Errorf("heading %v: the rider at (%.2f, %.2f) faces %.3f, cosine to the coachman %.3f, want 1", h, d[0].X, d[0].Z, d[0].Facing, c)
		}
		if d[0].Facing < 0 || d[0].Facing >= 2*math.Pi {
			t.Errorf("heading %v: the rider's %.3f outside [0, 2pi)", h, d[0].Facing)
		}
		if c := looks(h); c > -0.999 {
			t.Errorf("the control, heading %v: his own heading's cosine %.3f, want -1 (away)", h, c)
		}
	}
	w := NewWorld()
	// no floor in front, beside or behind him (petition 3301 sets a rider down
	// beside him where there is floor): his own spot, his way
	w.ground = holesGround{{100, 105}, {105, 100}, {95, 100}, {100, 95}}
	w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100}
	w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
	if d := w.GetCoachDestinationsEveryWorld("Coachman Other"); len(d) != 1 || d[0].Facing != 0 || d[0].X != 100 {
		t.Errorf("on his own spot: %+v, want his heading 0 at his spot", d)
	}
}

// holesGround is floor at -2 but for holes of radius 2 at the given spots.
type holesGround []([2]float32)

func (g holesGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	for _, h := range g {
		if distXZ(x, z, h[0], h[1]) < 2 {
			return 0, false
		}
	}
	return -2, -2 <= y+2
}

// With no floor in front of a coachman, the rider is set down beside him where
// the floor is, facing him, not on his own spot inside him (Heirloom petition
// 3301: the Homelands' Guk, "when i coach to guk im inside the coachman"). The
// control: with no floor in front, beside or behind him, his own spot, as
// before (petition 2229's fall kept away).
func TestCoachSetsDownBesideWithNoFloorInFront(t *testing.T) {
	for _, c := range []struct {
		name  string
		holes holesGround
		own   bool
	}{
		{"floor beside him", holesGround{{100, 105}}, false},
		{"the control: no floor around him", holesGround{{100, 105}, {105, 100}, {95, 100}, {100, 95}}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := NewWorld()
			w.ground = c.holes
			w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100}
			w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
			d := w.GetCoachDestinationsEveryWorld("Coachman Other")
			if len(d) != 1 {
				t.Fatalf("%d destinations", len(d))
			}
			away := distXZ(d[0].X, d[0].Z, 100, 100)
			if c.own {
				if away > 0.01 {
					t.Errorf("set down at (%.1f, %.1f), %.2f from him; want his own spot", d[0].X, d[0].Z, away)
				}
				return
			}
			if math.Abs(float64(away-coachLandOut)) > 0.01 {
				t.Fatalf("set down at (%.1f, %.1f), %.2f from him; want %v beside him", d[0].X, d[0].Z, away, coachLandOut)
			}
			if distXZ(d[0].X, d[0].Z, 100, 105) < 2 {
				t.Errorf("set down in front of him, where there is no floor")
			}
			fx, fz := math.Sin(float64(d[0].Facing)), math.Cos(float64(d[0].Facing))
			tx, tz := float64(100-d[0].X), float64(100-d[0].Z)
			if cos := (fx*tx + fz*tz) / math.Hypot(tx, tz); cos < 0.999 {
				t.Errorf("the rider faces %.3f, cosine to the coachman %.3f, want 1", d[0].Facing, cos)
			}
		})
	}
}
