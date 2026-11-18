package entity

import (
	"math"
	"testing"
)

// A broken weapon still hits: a fifth of the roll, a third of the floor.
func TestBrokenWeaponWorth(t *testing.T) {
	if r, f := rollWorth(0), floorWorth(0); math.Abs(r-0.195) > 0.001 || math.Abs(f-0.343) > 0.001 {
		t.Errorf("a broken weapon's worth: roll %.3f, floor %.3f", r, f)
	}
	if r, f := rollWorth(1), floorWorth(1); r != 1 || f != 1 {
		t.Errorf("a weapon in repair's worth: roll %v, floor %v", r, f)
	}
}
