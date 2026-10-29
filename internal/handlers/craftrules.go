package handlers

import (
	"time"

	"github.com/eqoa/server/internal/database"
)

// The tradeskills' rules. CLIENT shows a skill's level and rank and reports
// the XP a craft gave (claude-notes/tradeskill-protocol-20261029.md §3.3,
// §6.1); no record of SOE's server gives the curves, the odds or the XP, so
// every number here is OURS, the owner's choice of 2026-10-29.

// The skills, their slot index everywhere: the Recipe Book's skill, the
// reply's slot, the tool's wear slot less craftToolWear0.
const (
	craftWeapon = iota
	craftArmor
	craftTailoring
	craftJewel
	craftCarpentry
	craftAlchemy
)

// craftSkillNames are the skills' names in the server's lines (CLIENT's
// "You receive %d XP in Weapon Crafting." words 1560-1565).
var craftSkillNames = [database.CraftSkills]string{
	"Weapon Crafting", "Armor Crafting", "Tailoring", "Jewel Crafting", "Carpentry", "Alchemy",
}

func craftSkillName(skill int) string {
	if skill >= 0 && skill < len(craftSkillNames) {
		return craftSkillNames[skill]
	}
	return "that"
}

// craftLiveSkills is how many skills a character has with no row: 0-3.
// Carpentry and Alchemy were never live: with no row they are sent as
// Sandstorm sends them ({99, 99, 99, 98}) and refuse every craft; a row a GM
// writes (#craftskill) makes one live for that character.
const craftLiveSkills = 4

// Sandstorm's words of a slot that no CLIENT code reads (x00, x0C), and its
// level and rank of a skill that is not live.
const (
	craftSlotX00     uint32 = 99
	craftSlotX0C     uint32 = 98
	craftNotLiveRank        = 99
)

// The scale: level 1-10, rank 0-9; 100 × level XP to the next rank; ten ranks
// make the next level (rank 0); level 10 rank 9 is the top, where XP stops.
const (
	craftRanks         = 10
	craftRankMax       = craftRanks - 1
	craftXPPerRank     = 100 // × the level
	craftDiscoveryXP   = 10  // a discovery's XP is ten times a make's
	craftPointsPerRank = 1
	craftPointsPerLvl  = 10
)

// The roll: 0.60 at a recipe's difficulty, 0.05 a skill point either side,
// and the two stats of the craft's above 120 (a new character's 60 and 60)
// adding 1/2000 a point; never below 0.05 or above 0.95.
const (
	craftChanceBase     = 0.60
	craftChancePerPoint = 0.05
	craftChanceMin      = 0.05
	craftChanceMax      = 0.95
	craftStatBase       = 120
	craftStatDivisor    = 2000.0
)

// craftCooldown is how soon after an attempt the next may be made, per
// session (CLIENT's reply 5 says "You have not yet recovered.").
const craftCooldown = time.Second

// craftToolWearHP is what an attempt takes off its tool's condition.
const craftToolWearHP = 10

// craftSkill is one skill as the rules use it.
type craftSkill struct {
	level, rank, xp int
	live            bool // a craft of it may be made
}

// craftSkillOf is a character's skill from its row, or the default with
// none.
func craftSkillOf(rows [database.CraftSkills]database.CraftSkill, skill int) craftSkill {
	if skill < 0 || skill >= database.CraftSkills {
		return craftSkill{}
	}
	if r := rows[skill]; r.Row {
		s := craftSkill{level: r.Level, rank: r.Rank, xp: r.XP, live: true}
		// a row out of the scale (the panel refuses one; an old one may
		// hold it) is read inside it
		s.level = min(max(s.level, database.CraftLevelMin), database.CraftLevelMax)
		s.rank = min(max(s.rank, 0), craftRankMax)
		s.xp = max(s.xp, 0)
		return s
	}
	if skill >= craftLiveSkills {
		return craftSkill{level: craftNotLiveRank, rank: craftNotLiveRank}
	}
	return craftSkill{level: database.CraftLevelMin, live: true}
}

// slot is the skill as CLIENT's VICraftSlot holds it: {x00, level, rank,
// x0C}, Sandstorm's 99 and 98 in the two words nothing reads.
func (s craftSkill) slot() [4]uint32 {
	return [4]uint32{craftSlotX00, uint32(s.level), uint32(s.rank), craftSlotX0C}
}

// row is the skill as charcraftskills keeps it.
func (s craftSkill) row() database.CraftSkill {
	return database.CraftSkill{Level: s.level, Rank: s.rank, XP: s.xp, Row: true}
}

// points is the skill points the roll counts: ten a level and one a rank.
func (s craftSkill) points() int {
	return craftPointsPerLvl*s.level + craftPointsPerRank*s.rank
}

// capped is whether the skill is at the top, where XP stops.
func (s craftSkill) capped() bool {
	return s.level >= database.CraftLevelMax && s.rank >= craftRankMax
}

// addXP gives the skill n XP: 100 × its level make a rank, the tenth rank
// the next level at rank 0, and the top keeps none. It returns the XP the
// skill took: n, or 0 at the top.
func (s *craftSkill) addXP(n int) int {
	if n <= 0 || s.capped() {
		return 0
	}
	s.xp += n
	for !s.capped() && s.xp >= craftXPPerRank*s.level {
		s.xp -= craftXPPerRank * s.level
		s.rank++
		if s.rank >= craftRanks {
			s.level++
			s.rank = 0
		}
	}
	if s.capped() {
		s.level, s.rank, s.xp = database.CraftLevelMax, craftRankMax, 0
	}
	return n
}

// craftGain is the XP a success of a recipe gives a skill: its xp at the
// recipe's own level, half of it one level below the skill's, none two or
// more below; a discovery ten times that. (A recipe above the skill's level
// is never made.)
func craftGain(xp, recipeLevel, skillLevel int, discovery bool) int {
	g := 0
	switch skillLevel - recipeLevel {
	case 0:
		g = xp
	case 1:
		g = xp / 2
	}
	if discovery {
		g *= craftDiscoveryXP
	}
	return g
}

// craftStatPairs are the two stats each skill's roll reads, as indices of a
// CraftView's Stats (STR, STA, AGI, DEX, WIS, INT, CHA): Weapon STR+DEX,
// Armor STR+STA, Tailoring DEX+WIS, Jewel INT+CHA, Carpentry AGI+DEX, Alchemy
// WIS+INT.
var craftStatPairs = [database.CraftSkills][2]int{
	{0, 3}, {0, 1}, {3, 4}, {5, 6}, {2, 3}, {4, 5},
}

// craftStatSum is the two stats of the skill's, out of the seven.
func craftStatSum(skill int, stats [7]int) int {
	if skill < 0 || skill >= database.CraftSkills {
		return craftStatBase
	}
	p := craftStatPairs[skill]
	return stats[p[0]] + stats[p[1]]
}

// craftChance is the chance an attempt makes its recipe: 0.60, plus 0.05 a
// skill point over the recipe's difficulty (less one under), plus the two
// stats over 120 at 1/2000 a point, kept within 0.05 and 0.95.
func craftChance(points, difficulty, statSum int) float64 {
	c := craftChanceBase + craftChancePerPoint*float64(points-difficulty) + float64(statSum-craftStatBase)/craftStatDivisor
	return min(max(c, craftChanceMin), craftChanceMax)
}

// craftToolTier is a tool's tier by its level: levelreq / 5 + 1, so
// Sandstorm's ten hammers (levels 1, 6, ..., 46, the Novice's for crude
// iron to the Grandmaster's for seralite) are tiers 1 to 10.
func craftToolTier(levelReq int) int {
	return max(levelReq, 0)/5 + 1
}

// craftToolFits is whether a tool of that level may make a recipe of that
// trade level: its tier at least the recipe's level.
func craftToolFits(levelReq, recipeLevel int) bool {
	return craftToolTier(levelReq) >= recipeLevel
}
