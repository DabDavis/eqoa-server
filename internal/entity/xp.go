package entity

// CharXPDict is the experience a level takes to pass, the live game's as
// eqoa.fandom.com's Formulas chart gives it: each level's "unrezzed debt"
// times 25 ("Multiply by 25 for an estimated amount of XP to get to next
// level"), levels 6-50, 52 and 54 listed there and 51, 53 and 55 in its 2010
// copy on the EQOA forum (t279). 1-5 come from the page's quadratic fit for
// 1-19 (ln XP = 8.125 + 1.283 ln L + 0.1521 ln^2 L), which as printed is
// within 0.07% of every listed 6-19 but 15 (its cubic, as printed, 0.46%).
// 15 comes from that fit too: its listed 14,778 is 11% over it, and its
// steps (+481 after +1,641, then +3,259) are the only break in 6-19's
// smooth growth. 20's 49,390 is kept, because the page says a second curve
// starts at 20. 56-59 are ESTIMATES, log-linear in the debt between 55 and
// the forum's 60 (13,752,774, the debt for dying at 60, used for nothing
// else). Until 2026-10-25 the table was CLIENT's own (ExpForLevel below) up to
// 49, and the C# reference's above.
var CharXPDict = map[int]int{
	1:  3378,      // the page's fit
	2:  8843,      // the page's fit
	3:  16616,     // the page's fit
	4:  26794,     // the page's fit
	5:  39494,     // the page's fit
	6:  54875,     // listed
	7:  72950,     // listed
	8:  93950,     // listed
	9:  117975,    // listed
	10: 145175,    // listed
	11: 175675,    // listed
	12: 209550,    // listed
	13: 246950,    // listed
	14: 287975,    // listed
	15: 332654,    // the page's fit: the listed 14,778 (369,450) is 11% over it
	16: 381475,    // listed
	17: 434150,    // listed
	18: 490950,    // listed
	19: 551975,    // listed
	20: 1234750,   // listed
	21: 2061825,   // listed
	22: 3243375,   // listed
	23: 4204750,   // listed
	24: 5549850,   // listed
	25: 7097825,   // listed
	26: 8864600,   // listed
	27: 10866150,  // listed
	28: 13119500,  // listed
	29: 15641725,  // listed
	30: 18450900,  // listed
	31: 21565375,  // listed
	32: 25003300,  // listed
	33: 28784250,  // listed
	34: 32927600,  // listed
	35: 37453975,  // listed
	36: 42384150,  // listed
	37: 48357700,  // listed
	38: 53539000,  // listed
	39: 60738075,  // listed
	40: 66567050,  // listed
	41: 73840925,  // listed
	42: 81651600,  // listed
	43: 90023750,  // listed
	44: 98981350,  // listed
	45: 108550125, // listed
	46: 118755700, // listed
	47: 129622750, // listed
	48: 141178500, // listed
	49: 153449225, // listed
	50: 166463200, // listed
	51: 180248475, // the forum's copy
	52: 194831900, // listed
	53: 210243250, // the forum's copy
	54: 226511100, // listed
	55: 243666275, // the forum's copy
	56: 261037200, // ESTIMATE
	57: 279646500, // ESTIMATE
	58: 299582450, // ESTIMATE
	59: 320939625, // ESTIMATE
}

// ExpForLevel is CLIENT's own table (decomp viexperience.cpp, 0x00CC68D8):
// the experience a level needs, l^3 ((l-1) 28 + 25), and from 51 l^3 ((l-1)
// 27 + 20). The HUD's experience bar is the dump's (and 0x0020's) experience
// over ExpForLevel(level + 1) (VIWndGame's gauges), so what the client is told
// is scaled to it (ClientXP).
func ExpForLevel(l int) int {
	if l >= 51 {
		return l * l * l * ((l-1)*27 + 20)
	}
	return l * l * l * ((l-1)*28 + 25)
}

// ClientXP is the experience into a level as the client is told it: the
// same share of CLIENT's own span (ExpForLevel(level + 1)) as of this
// server's (CharXPDict), so its bar fills as this server levels.
func ClientXP(level, xpInLevel int) int {
	need := CharXPDict[level]
	if level >= 60 || need <= 0 {
		return 0
	}
	return int(int64(max(0, xpInLevel)) * int64(ExpForLevel(level+1)) / int64(need))
}

// XPTableVersion names CharXPDict for the one-time carry of every kept
// total (database.MigrateTotalXP with ConvertTotalXP).
const XPTableVersion = "fandom-formulas-2026-10-25"

// charXPDictBefore is the table the server kept until 2026-10-25 (CLIENT's
// up to 49, the C# reference's level^3 ((level-1) 28 + 25) of level + 1
// above), for ConvertTotalXP alone.
func charXPDictBefore(level int) int {
	l := level + 1
	return l * l * l * ((l-1)*28 + 25)
}

// ConvertTotalXP carries a kept total experience (characters.totalXP) from
// the old table to this one: the same level, and the same share of it.
func ConvertTotalXP(level, oldTotal int) int {
	if level < 1 || level >= 60 {
		return TotalXP(level, 0)
	}
	base := 0
	for l := 1; l < level; l++ {
		base += charXPDictBefore(l)
	}
	span := charXPDictBefore(level)
	into := min(max(0, oldTotal-base), span-1)
	return TotalXP(level, int(int64(into)*int64(CharXPDict[level])/int64(span)))
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
