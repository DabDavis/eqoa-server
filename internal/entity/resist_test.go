package entity

import (
	"math"
	"math/rand"
	"testing"
)

// A resist takes its share in proportion up to 90% at the cap of 315, and
// no more past it (the owner's 90%, 2026-11-11; jib's 75% before); none below 0.
func TestResistShare(t *testing.T) {
	for _, tt := range []struct {
		resist int
		want   float64
	}{
		{-20, 0}, {0, 0}, {105, 0.3}, {210, 0.6}, {315, 0.9}, {600, 0.9}, {100000, 0.9},
	} {
		if got := ResistShare(tt.resist); math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("resist %d: %v, want %v", tt.resist, got, tt.want)
		}
	}
}

// Below the cap a resist never fails (no dice are thrown); at it, a throw
// under 2.5% fails it altogether, the next 5% in part (a second throw
// picks the share in 54-88.8%, jib's 45-74% of 75% in proportion to 90%),
// and anything else keeps its 90%.
func TestRollResistAtTheCap(t *testing.T) {
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { t.Fatal("dice thrown below the cap"); return 0 }
	if share, how := RollResist(314); how != ResistHeld || math.Abs(share-0.9*314/315) > 1e-12 {
		t.Errorf("below the cap: %v %v", share, how)
	}
	for _, tt := range []struct {
		dice  []float64
		share float64
		how   ResistOutcome
	}{
		{[]float64{0}, 0, ResistFailed},
		{[]float64{0.0249}, 0, ResistFailed},
		{[]float64{0.025, 0}, 0.54, ResistPartial},
		{[]float64{0.0749, 0.999999999}, 0.888, ResistPartial},
		{[]float64{0.075}, 0.9, ResistHeld},
		{[]float64{0.9}, 0.9, ResistHeld},
	} {
		dice := append([]float64(nil), tt.dice...)
		resistRand = func() float64 { d := dice[0]; dice = dice[1:]; return d }
		share, how := RollResist(ResistCap)
		if how != tt.how || math.Abs(share-tt.share) > 1e-6 || len(dice) != 0 {
			t.Errorf("dice %v: %v %v, want %v %v (dice left %d)", tt.dice, share, how, tt.share, tt.how, len(dice))
		}
	}
}

// Over many spells at the cap: about 2.5% fail altogether, 5% in part, the
// rest hold; every partial share is in 54-88.8%.
func TestRollResistOdds(t *testing.T) {
	defer func(f func() float64) { resistRand = f }(resistRand)
	r := rand.New(rand.NewSource(17))
	resistRand = r.Float64
	const n = 200000
	var failed, partial int
	for i := 0; i < n; i++ {
		share, how := RollResist(400)
		switch how {
		case ResistFailed:
			failed++
		case ResistPartial:
			partial++
			if share < 0.54 || share > 0.888 {
				t.Fatalf("a partial share of %v", share)
			}
		}
	}
	if f, p := float64(failed)/n, float64(partial)/n; math.Abs(f-0.025) > 0.002 || math.Abs(p-0.05) > 0.003 {
		t.Errorf("failed %.4f (want 0.025), partial %.4f (want 0.05)", f, p)
	}
}

// What the chat says of each.
func TestResistWords(t *testing.T) {
	for _, tt := range []struct {
		share float64
		how   ResistOutcome
		want  string
	}{
		{0.9, ResistHeld, "your resist took 90%"},
		{0.52, ResistPartial, "your resist took only 52%"},
		{0, ResistFailed, "your resist failed"},
	} {
		if got := resistWords(tt.share, tt.how); got != tt.want {
			t.Errorf("%v %v: %q, want %q", tt.share, tt.how, got, tt.want)
		}
	}
}
