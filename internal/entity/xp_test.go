package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The experience a row keeps (totalXP) and the experience into the level the
// game holds turn into each other: every level below whole plus what is into
// this one. The level column decides the level: a total short of it is no
// way in, one past it is short of the next, and 60 holds none. The control
// is XPBase's sums, read off CharXPDict one by one.
func TestTheKeptExperienceIsTheLevelsBelowAndWhatIsIntoThisOne(t *testing.T) {
	if XPBase(1) != 0 || XPBase(2) != CharXPDict[1] || XPBase(3) != CharXPDict[1]+CharXPDict[2] {
		t.Fatalf("XPBase(1..3) = %d, %d, %d", XPBase(1), XPBase(2), XPBase(3))
	}
	sum := 0
	for l := 1; l <= 60; l++ {
		if XPBase(l) != sum {
			t.Fatalf("XPBase(%d) = %d, want %d", l, XPBase(l), sum)
		}
		sum += CharXPDict[l]
	}
	for l := 1; l < 60; l++ {
		for _, in := range []int{0, 1, CharXPDict[l] / 2, CharXPDict[l] - 1} {
			if got := XPInLevel(l, TotalXP(l, in)); got != in {
				t.Errorf("level %d, %d in: kept as %d, read back %d", l, in, TotalXP(l, in), got)
			}
		}
		if got := XPInLevel(l, 0); got != 0 {
			t.Errorf("level %d from a row that kept no experience: %d in", l, got)
		}
		if got := XPInLevel(l, XPBase(l+5)); got != CharXPDict[l]-1 {
			t.Errorf("level %d from a total five levels on: %d in, want %d", l, got, CharXPDict[l]-1)
		}
	}
	if got := XPInLevel(60, XPBase(60)+12345); got != 0 {
		t.Errorf("level 60 holds %d experience", got)
	}
}

// A login puts the kept experience back into the level: a level 10 row that
// keeps 500 past level 10's start is 500 into it, and a row of before
// (totalXP 0) is at its level's start; and the debt the row keeps, never
// below nothing.
func TestALoginKeepsTheExperienceIntoTheLevel(t *testing.T) {
	w := NewWorld()
	p := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 7, CharName: "Kept", Level: 10, TotalXP: XPBase(10) + 500, Debt: 1234})
	if p == nil || p.Level != 10 || p.XPInLevel != 500 || p.TotalXP != XPBase(10)+500 || p.Debt != 1234 {
		t.Fatalf("player %+v", p)
	}
	q := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 8, CharName: "Before", Level: 50, Debt: -5})
	if q.XPInLevel != 0 || q.TotalXP != XPBase(50) || q.Debt != 0 {
		t.Fatalf("a row that kept nothing: %d in, %d total, debt %d", q.XPInLevel, q.TotalXP, q.Debt)
	}
	if level, xp, debt, ok := w.LiveProgress(7); !ok || level != 10 || xp != XPBase(10)+500 || debt != 1234 {
		t.Fatalf("LiveProgress = %d, %d, %d, %v", level, xp, debt, ok)
	}
}
