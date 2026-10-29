package handlers

import (
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The tradeskills' rules (craftrules.go), the numbers the owner chose.

// XP: 100 x the level makes a rank, the tenth rank the next level at rank 0,
// a gain past several ranks takes them all, and level 10 rank 9 keeps none.
func TestCraftXPRollsRanksAndLevels(t *testing.T) {
	cases := []struct {
		name                string
		from                craftSkill
		gain                int
		level, rank, xp, ok int
	}{
		{"within a rank", craftSkill{level: 1, rank: 0, xp: 0}, 40, 1, 0, 40, 40},
		{"a rank exactly", craftSkill{level: 1, rank: 0, xp: 60}, 40, 1, 1, 0, 40},
		// rank 9 of level 1, 90 of 100: the tenth rank is level 2, rank 0
		{"the tenth rank is the next level", craftSkill{level: 1, rank: 9, xp: 90}, 20, 2, 0, 10, 20},
		// level 2 takes 200 a rank: 450 from 0 is two ranks and 50 over
		{"two ranks at once", craftSkill{level: 2, rank: 3, xp: 0}, 450, 2, 5, 50, 450},
		// 1,000 a rank at level 10: rank 8, 950 + 100 reaches rank 9, the top
		{"the top keeps nothing", craftSkill{level: 10, rank: 8, xp: 950}, 100, 10, 9, 0, 100},
		{"at the top no XP is taken", craftSkill{level: 10, rank: 9, xp: 0}, 500, 10, 9, 0, 0},
		{"a gain past the top stops there", craftSkill{level: 9, rank: 9, xp: 0}, 1000000, 10, 9, 0, 1000000},
	}
	for _, c := range cases {
		s := c.from
		got := s.addXP(c.gain)
		if s.level != c.level || s.rank != c.rank || s.xp != c.xp || got != c.ok {
			t.Errorf("%s: level %d rank %d xp %d, took %d; want level %d rank %d xp %d, took %d",
				c.name, s.level, s.rank, s.xp, got, c.level, c.rank, c.xp, c.ok)
		}
	}
	// the control: a rank is 100 x the LEVEL, so the same 150 at level 1 is a
	// rank and 50 over, at level 2 no rank at all
	a, b := craftSkill{level: 1}, craftSkill{level: 2}
	a.addXP(150)
	b.addXP(150)
	if a.rank != 1 || a.xp != 50 || b.rank != 0 || b.xp != 150 {
		t.Fatalf("150 XP: level 1 to rank %d xp %d, level 2 to rank %d xp %d", a.rank, a.xp, b.rank, b.xp)
	}
}

// Skill points are ten a level and one a rank.
func TestCraftSkillPoints(t *testing.T) {
	if p := (craftSkill{level: 1}).points(); p != 10 {
		t.Errorf("level 1 rank 0: %d points, want 10", p)
	}
	if p := (craftSkill{level: 10, rank: 9}).points(); p != 109 {
		t.Errorf("level 10 rank 9: %d points, want 109", p)
	}
	if p := (craftSkill{level: 3, rank: 4}).points(); p == 7 || p != 34 {
		t.Errorf("level 3 rank 4: %d points, want 34 (not the level and rank added)", p)
	}
}

// The XP of a success: the recipe's at its own level, half one level below
// the skill's, none two or more below, ten times for a discovery.
func TestCraftGainByTheRecipesLevel(t *testing.T) {
	for _, c := range []struct {
		recipe, skill int
		discovery     bool
		want          int
	}{
		{3, 3, false, 45},
		{2, 3, false, 22}, // half, the remainder dropped
		{1, 3, false, 0},
		{3, 3, true, 450},
		{2, 3, true, 220},
		{1, 3, true, 0},
	} {
		if got := craftGain(45, c.recipe, c.skill, c.discovery); got != c.want {
			t.Errorf("recipe level %d, skill %d, discovery %v: %d XP, want %d", c.recipe, c.skill, c.discovery, got, c.want)
		}
	}
}

// The roll: 0.60 at the difficulty, 0.05 a point either side, the two stats
// over 120 at 1/2000 a point, within 0.05 and 0.95; each skill its own pair.
func TestCraftChance(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	for _, c := range []struct {
		points, difficulty, stats int
		want                      float64
	}{
		{10, 10, 120, 0.60},
		{12, 10, 120, 0.70},
		{8, 10, 120, 0.50},
		{10, 10, 320, 0.70},
		{10, 10, 20, 0.55},
		{30, 10, 120, 0.95}, // capped
		{0, 30, 120, 0.05},  // floored
	} {
		if got := craftChance(c.points, c.difficulty, c.stats); !near(got, c.want) {
			t.Errorf("points %d, difficulty %d, stats %d: %.4f, want %.2f", c.points, c.difficulty, c.stats, got, c.want)
		}
	}
	// the pairs: STR 1, STA 2, AGI 4, DEX 8, WIS 16, INT 32, CHA 64 tell
	// every pair apart
	stats := [7]int{1, 2, 4, 8, 16, 32, 64}
	for skill, want := range []int{1 + 8, 1 + 2, 8 + 16, 32 + 64, 4 + 8, 16 + 32} {
		if got := craftStatSum(skill, stats); got != want {
			t.Errorf("%s reads %d, want %d", craftSkillName(skill), got, want)
		}
	}
}

// A tool's tier is its level / 5 + 1 (Sandstorm's ten hammers, levels 1, 6,
// ... 46, are tiers 1 to 10), and a recipe of level L needs tier L or more.
func TestCraftToolTier(t *testing.T) {
	for _, c := range []struct{ levelReq, tier int }{
		{1, 1}, {4, 1}, {6, 2}, {11, 3}, {16, 4}, {21, 5}, {26, 6}, {31, 7}, {36, 8}, {41, 9}, {46, 10}, {0, 1}, {-3, 1},
	} {
		if got := craftToolTier(c.levelReq); got != c.tier {
			t.Errorf("a level %d tool: tier %d, want %d", c.levelReq, got, c.tier)
		}
	}
	if !craftToolFits(46, 10) || !craftToolFits(6, 2) || !craftToolFits(6, 1) {
		t.Error("a tool of the recipe's tier or above refused")
	}
	// the control: one tier short refuses (a tier of levelreq / 5 alone, the
	// plausible slip, would refuse the novice's hammer even for level 1)
	if craftToolFits(1, 2) || craftToolFits(41, 10) || !craftToolFits(1, 1) {
		t.Error("a tool one tier short was let through, or the novice's hammer refused for a level 1 recipe")
	}
}

// A character with no rows: skills 0-3 at level 1 rank 0 and live; 4 and 5
// sent as Sandstorm sends them and not live; a row makes any skill its own.
func TestCraftSkillDefaults(t *testing.T) {
	var rows [database.CraftSkills]database.CraftSkill
	for i := 0; i < 4; i++ {
		if s := craftSkillOf(rows, i); !s.live || s.slot() != [4]uint32{99, 1, 0, 98} {
			t.Errorf("skill %d with no row: %+v %v", i, s, s.slot())
		}
	}
	for i := 4; i < 6; i++ {
		if s := craftSkillOf(rows, i); s.live || s.slot() != [4]uint32{99, 99, 99, 98} {
			t.Errorf("skill %d with no row: %+v %v", i, s, s.slot())
		}
	}
	rows[4] = database.CraftSkill{Level: 3, Rank: 2, Row: true}
	if s := craftSkillOf(rows, 4); !s.live || s.slot() != [4]uint32{99, 3, 2, 98} {
		t.Errorf("Carpentry with a row: %+v %v", s, s.slot())
	}
	// a row out of the scale is read inside it
	rows[1] = database.CraftSkill{Level: 40, Rank: -2, XP: -5, Row: true}
	if s := craftSkillOf(rows, 1); s.level != 10 || s.rank != 0 || s.xp != 0 {
		t.Errorf("a row out of the scale: %+v", s)
	}
}
