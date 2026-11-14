package entity

import (
	"fmt"
	"strings"
	"testing"
)

// The players' "XP to next Level" chart (eqoa.forumotion.com/t279-calculations,
// "Unrezzed debt number for the level") is CLIENT's ExpCost as the live server
// worked it, to the unit, at every level but four (deathdebt.go): the chart
// below is the thread's as posted. The control: the four odd ones are not it,
// and the made-up values this server had at 51, 53 and 60 are not the thread's.
func TestADeathsDebtIsTheLiveServersExpCost(t *testing.T) {
	chart := map[int]int{
		6: 2195, 7: 2918, 8: 3758, 9: 4719, 10: 5807, 11: 7027, 12: 8382, 13: 9878, 14: 11519,
		16: 15259, 17: 17366, 18: 19638, 19: 22079, 20: 49390, 21: 82473,
		23: 168190, 24: 221994, 25: 283913, 26: 354584, 27: 434646, 28: 524780, 29: 625669,
		30: 738036, 31: 862615, 32: 1000132, 33: 1151370, 34: 1317104, 35: 1498159, 36: 1695366,
		38: 2141560, 40: 2662682, 41: 2953637, 42: 3266064, 43: 3600950,
		44: 3959254, 45: 4342005, 46: 4750228, 47: 5184910, 48: 5647140, 49: 6137969, 50: 6658528,
		51: 7209939, 52: 7793276, 53: 8409730, 54: 9060444, 55: 9746651, 60: 13752774,
	}
	for l, want := range chart {
		if got := DeathDebt(l); got != want {
			t.Errorf("level %d's debt %d, the thread's %d", l, got, want)
		}
	}
	odd := map[int]int{15: 14778, 22: 129735, 37: 1934308, 39: 2429523}
	for l, posted := range odd {
		if got := DeathDebt(l); got == posted || got <= DeathDebt(l-1) || got >= DeathDebt(l+1) {
			t.Errorf("level %d: %d (the thread's odd %d), between %d and %d", l, got, posted, DeathDebt(l-1), DeathDebt(l+1))
		}
	}
	for l := 1; l <= 5; l++ {
		if DeathDebt(l) != 0 {
			t.Errorf("level %d owes %d, want none below 6", l, DeathDebt(l))
		}
	}
	if DeathDebt(MaxGMLevel) != DeathDebt(60) {
		t.Errorf("a GM level %d owes %d, want 60's %d", MaxGMLevel, DeathDebt(MaxGMLevel), DeathDebt(60))
	}
	for l, old := range map[int]int{51: 7203593, 53: 8403008, 60: 13809547} {
		if DeathDebt(l) == old {
			t.Errorf("the control: level %d still owes the made-up %d", l, old)
		}
	}
}

// The kill experience by the player's level and the creature's against it
// (solo), as killexp.go gives it: CLIENT's model, 5% a level up or down, none
// below three quarters of the player's level (three under at the low levels),
// what the HUD's bar divides by beside it. Logged (go test -v) for the
// comparison; checked for the falloff's shape.
func TestTheKillExperienceTable(t *testing.T) {
	var b strings.Builder
	fmt.Fprintf(&b, "%5s", "lvl")
	for d := -5; d <= 5; d++ {
		fmt.Fprintf(&b, " %8s", fmt.Sprintf("%+d", d))
	}
	fmt.Fprintf(&b, " %11s %6s\n", "to level", "kills")
	for l := 1; l <= 60; l++ {
		fmt.Fprintf(&b, "%5d", l)
		prev := -1
		for d := -5; d <= 5; d++ {
			mob := l + d
			xp := 0
			if mob >= 1 {
				xp = groupExp(mob, l, []int{l})
			}
			fmt.Fprintf(&b, " %8d", xp)
			if xp < prev {
				t.Errorf("level %d: a creature %+d gives %d, less than one a level below (%d)", l, d, xp, prev)
			}
			prev = xp
		}
		even := groupExp(l, l, []int{l})
		need := ExpForLevel(l + 1)
		fmt.Fprintf(&b, " %11d %6d\n", need, (need+even-1)/even)
		// even con: LevelExp; +5 is 25% more of a higher level's worth
		if up := groupExp(l+5, l, []int{l}); l >= 10 && up <= even*5/4 {
			t.Errorf("level %d: a +5 gives %d, not past 1.25 x the even %d", l, up, even)
		}
		// a creature below three quarters of the level gives nothing
		if r := int(float32(l) * 0.75); l-r > 2 && groupExp(r-1, l, []int{l}) != 0 {
			t.Errorf("level %d: a level %d still gives experience", l, r-1)
		}
	}
	t.Log("\n" + b.String())
}

// The ding to MaxLevel leaves what is over it to the masteries (all experience
// past 60 is theirs, splitXP), not in a level that holds none: a level 59 one
// short of 60 kills a level 63 alone (its whole worth less one), with no share.
// The control: a level 58 the same kill leaves at 59 with all of it there.
func TestTheDingTo60LeavesTheRestToTheMasteries(t *testing.T) {
	npc := &NPC{ServerID: 5004, Name: "Froglok Slayer", Level: 63, X: 1000, Z: 1000}
	k := newKillWorld(t, 59)
	p := k.players[0]
	p.XPInLevel = CharXPDict[59] - 1
	p.TotalXP = TotalXP(59, p.XPInLevel)
	worth := groupExp(63, 59, []int{59})
	k.w.awardKillXPLocked(k.sess[0], p, npc)
	if p.Level != 60 || p.XPInLevel != 0 || p.TotalXP != XPBase(60) {
		t.Fatalf("level %d, %d into it, total %d; want 60, none, %d", p.Level, p.XPInLevel, p.TotalXP, XPBase(60))
	}
	over := worth - 1
	if got := p.Mastery.XP + sumCost(p.Mastery.Points); got != over {
		t.Errorf("mastery %+v: %d toward points, want the %d over 60", p.Mastery, got, over)
	}
	lines, _ := sentOf(k.sess[0])
	if !contains(lines, "You receive 1 XP.") || !contains(lines, fmt.Sprintf("You received %d class mastery XP.", over)) {
		t.Errorf("the chat %q", lines)
	}

	c := newKillWorld(t, 58)
	q := c.players[0]
	q.XPInLevel = CharXPDict[58] - 1
	c.w.awardKillXPLocked(c.sess[0], q, npc)
	if w := groupExp(63, 58, []int{58}); q.Level != 59 || q.XPInLevel != w-1 || q.Mastery.XP != 0 {
		t.Errorf("the control: level %d, %d into it, mastery %+v; want 59, %d", q.Level, q.XPInLevel, q.Mastery, w-1)
	}
}

func sumCost(points int) int {
	s := 0
	for n := 1; n <= points; n++ {
		s += CMPointXP(n)
	}
	return s
}

func contains(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}
