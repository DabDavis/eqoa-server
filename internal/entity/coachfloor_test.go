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
		{"no floor in front", flatGround{y: -2, holeX: 100, holeZ: 105, hole: 2}, false},
		{"the floor in front far under him", dropGround{}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := NewWorld()
			w.ground = c.g
			w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100}
			w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
			for _, d := range w.GetCoachDestinationsEveryWorld("Coachman Other") {
				front := distXZ(d.X, d.Z, 100, 100) > 4
				if front != c.front {
					t.Errorf("set down at (%.1f, %.1f): in front %v, want %v", d.X, d.Z, front, c.front)
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
	w.ground = flatGround{y: -2, holeX: 100, holeZ: 105, hole: 2}
	w.NPCs[1] = &NPC{ServerID: 1, Name: "Coachman Inkcap", World: 10, SpawnX: 100, SpawnY: -2, SpawnZ: 100}
	w.NPCs[2] = &NPC{ServerID: 2, Name: "Coachman Other", World: 10, SpawnX: 900, SpawnY: -2, SpawnZ: 900}
	if d := w.GetCoachDestinationsEveryWorld("Coachman Other"); len(d) != 1 || d[0].Facing != 0 || d[0].X != 100 {
		t.Errorf("on his own spot: %+v, want his heading 0 at his spot", d)
	}
}
