package entity

// CharXPDict maps level -> XP required to advance past that level.
// From C# CharacterUtilities.CharXPDict.
var CharXPDict = map[int]int{
	0: 0, 1: 424, 2: 2187, 3: 6976, 4: 17125, 5: 35640, 6: 66199, 7: 113152, 8: 181521,
	9: 277000, 10: 405955, 11: 575424, 12: 793117, 13: 1067416, 14: 1407375, 15: 1822720,
	16: 2323849, 17: 2921832, 18: 3628411, 19: 4456000, 20: 5417685, 21: 6527224, 22: 7799047,
	23: 9248256, 24: 10890625, 25: 12742600, 26: 14821299, 27: 17144512, 28: 19730701, 29: 22599000,
	30: 25769215, 31: 29261824, 32: 33097977, 33: 37299496, 34: 41888875, 35: 46889280, 36: 52324549,
	37: 58219192, 38: 64598391, 39: 71488000, 40: 78914545, 41: 86905224, 42: 95487907, 43: 104691136,
	44: 114544125, 45: 125076760, 46: 136319599, 47: 148303872, 48: 161061481, 49: 174625000, 50: 189027675,
	51: 204303424, 52: 220486837, 53: 237613176, 54: 255718375, 55: 274839040, 56: 295012449, 57: 316276552,
	58: 338669971, 59: 362232000,
}

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
