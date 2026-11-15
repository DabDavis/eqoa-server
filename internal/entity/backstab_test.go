package entity

import (
	"math"
	"testing"
)

// Petition 2196 (Hoofbite: "Assassinate or Backstab spells need a slightly
// larger 'behind enemy' radius 3 percent more to start"): a creature's rear
// for a backstab is 3% wider than its half, its half-angle 92.7 degrees, so a
// caster 88 degrees off its facing is behind it (until then not) and one 87
// degrees off is not, on either side.
func TestBehindWidenedThreePercent(t *testing.T) {
	npc := &NPC{X: 100, Z: 100}
	deg := func(d float64) float64 { return d * math.Pi / 180 }
	if got := behindFrom * 180 / math.Pi; math.Abs(got-87.3) > 1e-9 {
		t.Fatalf("behind from %.4f degrees, want 87.3", got)
	}
	for _, c := range []struct {
		off    float64
		behind bool
	}{{180, true}, {91, true}, {88, true}, {87.4, true}, {87.2, false}, {87, false}, {45, false}, {0, false}} {
		for _, side := range []float64{1, -1} {
			x, z := float32(103), float32(104)
			npc.Facing = headingTo(x-npc.X, z-npc.Z) - float32(side*deg(c.off))
			if got := behind(npc, x, z); got != c.behind {
				t.Errorf("%.1f degrees off its facing (side %v): behind %v, want %v", c.off, side, got, c.behind)
			}
		}
	}
}
