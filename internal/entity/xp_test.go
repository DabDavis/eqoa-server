package entity

import (
	"math"
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
// (totalXP 0) is at its level's start.
func TestALoginKeepsTheExperienceIntoTheLevel(t *testing.T) {
	w := NewWorld()
	p := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 7, CharName: "Kept", Level: 10, TotalXP: XPBase(10) + 500})
	if p == nil || p.Level != 10 || p.XPInLevel != 500 || p.TotalXP != XPBase(10)+500 {
		t.Fatalf("player %+v", p)
	}
	q := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 8, CharName: "Before", Level: 50})
	if q.XPInLevel != 0 || q.TotalXP != XPBase(50) {
		t.Fatalf("a row that kept nothing: %d in, %d total", q.XPInLevel, q.TotalXP)
	}
	if level, xp, ok := w.LiveProgress(7); !ok || level != 10 || xp != XPBase(10)+500 {
		t.Fatalf("LiveProgress = %d, %d, %v", level, xp, ok)
	}
}

// The level table is the chart's debt x 25 where it lists one: fifteen of
// its values, the forum copy's three, the page's quadratic fit for 1-5 and
// 15 (the fit within 0.1% of every other listed 6-19, 15's listed value 11%
// over it), and 56-59 rising from 55 toward the forum's 60. The control: the
// old table (CLIENT's) is not it.
func TestTheLevelTableIsTheLiveGamesChart(t *testing.T) {
	listed := map[int]int{6: 2195, 10: 5807, 14: 11519, 16: 15259, 19: 22079, 20: 49390, 21: 82473, 30: 738036,
		37: 1934308, 40: 2662682, 45: 4342005, 49: 6137969, 50: 6658528, 52: 7793276, 54: 9060444,
		51: 7209939, 53: 8409730, 55: 9746651}
	for l, debt := range listed {
		if got := CharXPDict[l]; got != debt*25 {
			t.Errorf("level %d: %d, want the chart's %d x 25", l, got, debt)
		}
	}
	fit := func(l int) int {
		x := math.Log(float64(l))
		return int(math.Round(math.Exp(8.125 + 1.283*x + 0.1521*x*x)))
	}
	for l := 1; l <= 5; l++ {
		if CharXPDict[l] != fit(l) {
			t.Errorf("level %d: %d, want the fit's %d", l, CharXPDict[l], fit(l))
		}
	}
	for l := 6; l <= 19; l++ {
		off := math.Abs(float64(fit(l))/float64(CharXPDict[l]) - 1)
		if l != 15 && off > 0.001 {
			t.Errorf("level %d: the fit is %.2f%% off the chart", l, 100*off)
		}
	}
	if math.Abs(float64(14778*25)/float64(fit(15))-1) < 0.1 || CharXPDict[15] != fit(15) {
		t.Errorf("15: the listed value must be the outlier and the fit used (%d)", CharXPDict[15])
	}
	for l := 55; l < 59; l++ {
		if CharXPDict[l+1] <= CharXPDict[l] || CharXPDict[59] >= 13752774*25 {
			t.Errorf("56-59 must rise from 55 toward the forum's 60: %d -> %d", CharXPDict[l], CharXPDict[l+1])
		}
	}
	if CharXPDict[10] == ExpForLevel(11) {
		t.Fatal("the control: the old table (CLIENT's) must differ")
	}
}

// CLIENT's table is the old one up to 49 (it switches formula at 51, the
// old table did not); the client is told the share of CLIENT's span that the
// player has of this server's, so its bar fills as this server levels. The
// control: an unscaled half-way is not half of CLIENT's bar.
func TestTheClientsBarFollowsTheTable(t *testing.T) {
	for l := 1; l < 50; l++ {
		if ExpForLevel(l+1) != charXPDictBefore(l) {
			t.Fatalf("level %d: CLIENT %d, the old table %d", l, ExpForLevel(l+1), charXPDictBefore(l))
		}
	}
	if ExpForLevel(51) == charXPDictBefore(50) {
		t.Fatal("from 51 CLIENT's formula changes")
	}
	for _, l := range []int{1, 10, 15, 30, 50, 59} {
		half := CharXPDict[l] / 2
		got := float64(ClientXP(l, half)) / float64(ExpForLevel(l+1))
		if want := float64(half) / float64(CharXPDict[l]); math.Abs(got-want) > 1/float64(ExpForLevel(l+1)) {
			t.Errorf("level %d half way: CLIENT's bar at %.7f, want %.7f", l, got, want)
		}
		if ClientXP(l, CharXPDict[l]-1) >= ExpForLevel(l+1) {
			t.Errorf("level %d: a point short shows a full bar", l)
		}
	}
	if ClientXP(60, 5000) != 0 {
		t.Error("60 holds none")
	}
	if float64(CharXPDict[10]/2)/float64(ExpForLevel(11)) > 0.4 {
		t.Fatal("the control: unscaled, the bar must be off")
	}
}

// A kept total is carried to the new table at the same level and the same
// share of it; a row at 60 or of none stays at its level's start. The
// control: read with the new table unconverted, a level 30 row half way is
// not half way.
func TestAKeptTotalKeepsItsLevelAndShare(t *testing.T) {
	oldBase := func(level int) int {
		n := 0
		for l := 1; l < level; l++ {
			n += charXPDictBefore(l)
		}
		return n
	}
	for _, l := range []int{1, 5, 15, 30, 50, 59} {
		old := oldBase(l) + charXPDictBefore(l)/2
		n := ConvertTotalXP(l, old)
		if XPInLevel(l, n) != CharXPDict[l]/2 && XPInLevel(l, n) != (CharXPDict[l]-1)/2 {
			t.Errorf("level %d half way: %d into %d", l, XPInLevel(l, n), CharXPDict[l])
		}
	}
	if ConvertTotalXP(60, 999999999) != XPBase(60) || ConvertTotalXP(30, 0) != XPBase(30) {
		t.Error("60, or a row of none, is its level's start")
	}
	old := oldBase(30) + charXPDictBefore(30)/2
	if XPInLevel(30, old) == CharXPDict[30]/2 {
		t.Fatal("the control: unconverted, the share must be lost")
	}
}
