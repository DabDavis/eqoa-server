package entity

import "testing"

// stoneGround is a flat field at 0 with one knee-high stone on it (1.4 tall,
// x 4.5-5.5, z -1 to 1): the frost giants' camp's (petition 1883).
type stoneGround struct{}

func (stoneGround) Floor(_ uint8, _, _, _ float32) (float32, bool) { return 0, true }

// Placed is the stone's faces: the way meets them where it crosses x 4.5 or
// 5.5 within its z and below its top.
func (stoneGround) Placed(_ uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	for _, side := range []float32{4.5, 5.5} {
		if (x0 < side) == (x1 < side) {
			continue
		}
		t := (side - x0) / (x1 - x0)
		z, y := z0+(z1-z0)*t, y0+(y1-y0)*t
		if z >= -1 && z <= 1 && y < 1.4 {
			return true
		}
	}
	return false
}

// A creature as big as a frost giant (its body's edge 3.193, a man's 1.395)
// steps over a stone 1.4 tall in its chase's way, where a man-sized one still
// goes round it (petition 1883): the leg across it is one the giant runs, and
// its chase runs straight to its enemy past the stone without a route.
func TestABigCreatureStepsOverAKneeHighStone(t *testing.T) {
	const giantModel = 0x8117C627
	SetModelRadii(map[uint32]float32{giantModel: 3.193, creatureModel: 1.395})
	t.Cleanup(func() { SetModelRadii(nil) })
	w := roamWorld(t)
	w.SetGround(stoneGround{})
	giant := addNPC(w, 100, "tenacious frost giant", 0, 0, giantModel, 0)
	man := addNPC(w, 101, "orc pawn", 0, 0, creatureModel, 0)
	w.mu.Lock()
	_, giantOK := w.routeLegLocked(giant, 0, 0, 0, 10, 0)
	_, manOK := w.routeLegLocked(man, 0, 0, 0, 10, 0)
	w.mu.Unlock()
	if !giantOK {
		t.Error("the giant's leg across the stone is refused")
	}
	if manOK {
		t.Error("the control: a man-sized creature's leg across the stone is clear")
	}
	if k := bodyKnee(giant); k < 2.2 || k > 2.4 {
		t.Errorf("the giant's knee %.2f, want about 2.29", k)
	}
	if k := bodyKnee(man); k != legKnee {
		t.Errorf("a man's knee %.2f, want %v", k, legKnee)
	}

	ok, at := chaseOver(w, giant, 14, 0, 0, 200)
	if !ok {
		t.Fatalf("the giant never reached its enemy: at %v", at[len(at)-1])
	}
	for _, p := range at {
		if absf(p[2]) > 0.01 {
			t.Fatalf("the giant went round the stone: at %v", p)
		}
	}
}
