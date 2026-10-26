package entity

// MaxLevel is the highest level a character reaches: what experience goes to
// past it is mastery points (mastery.go splitXP).
const MaxLevel = 60

// CharXPDict maps level -> XP required to advance past that level: CLIENT's
// ExpForLevel(level+1) (killexp.go), what the HUD's experience bar divides by.
// The C# table it was copied from (CharacterUtilities.CharXPDict) took the
// formula's lower branch past level 50 too, 4% above the game's own from 50 to
// 59 (F3 20261026-031118), so the bar filled before the level did.
var CharXPDict = func() map[int]int {
	m := map[int]int{0: 0}
	for l := 1; l < 60; l++ {
		m[l] = ExpForLevel(l + 1)
	}
	return m
}()

// XPBase is the experience a character has at the start of level: every
// level below it whole (CharXPDict holds what each level takes to pass).
func XPBase(level int) int {
	total := 0
	for l := 1; l < level && l < 60; l++ {
		total += CharXPDict[l]
	}
	return total
}

// TotalXP is the experience the characters table keeps (totalXP): the
// levels below and what is into this one.
func TotalXP(level, xpInLevel int) int {
	return XPBase(level) + max(0, xpInLevel)
}

// XPInLevel is how far into its level a kept total puts a character. The
// level column decides the level, so a total that does not reach it (a row
// from before experience was kept, a level a GM set) is no way into it, and
// one past it (a level a GM lowered) is short of the next: what a level holds
// is never more than it takes, nor anything at 60.
func XPInLevel(level, totalXP int) int {
	if level >= 60 {
		return 0
	}
	return min(max(0, totalXP-XPBase(level)), max(0, CharXPDict[level]-1))
}
