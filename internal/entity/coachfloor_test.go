package entity

import "testing"

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
