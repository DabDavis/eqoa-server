package entity

// mercroles.go — what each role's merc is made of (the owner, 2026-11-11:
// "give tank mercs max resists and high sta,agi,ac for their level. not the
// best damage. melee mercs high dex, str, agi and use melee").
//
// A merc's base stats are a player's of its class at its level (SimBaseStats:
// the create screen's and every training point its levels give), its points
// spent by its role here where the role says, each within the level's cap
// (StatCapBase: 350, 400 past 45):
//   - a tank (warrior, paladin, shadow knight) all of them on STA and AGI,
//     half each: at 60 its 482 points put both near the cap (a human
//     warrior's STA 350, AGI 330 before its heirlooms), none on STR, so its
//     melee stays a tank's, under a melee fighter's;
//   - a melee fighter (ranger, monk, bard, rogue: the melee archetype)
//     STR, DEX and AGI, 35/35/30;
//   - a healer and a caster as before (simTrainWeights).
// A tank's resists are at the cap (MercTankResists: ResistCap, 315, each of
// the six; the server's cap is flat, no level curve exists for it) and its
// armour class the higher of its heirlooms' and MercTankAC's for its level.

// mercTrainWeights are the roles' spending of their training points, STR..CHA
// (the file's doc).
var mercTrainWeights = map[mercRole][7]float64{
	roleTank:  {0, 0.5, 0.5, 0, 0, 0, 0},
	roleMelee: {0.35, 0, 0.3, 0.35, 0, 0, 0},
}

// MercBaseStats is a merc's base stats at a level (the file's doc).
func MercBaseStats(class, race, level int) [7]int {
	w, ok := mercTrainWeights[mercRoleOf(class)]
	if !ok {
		return SimBaseStats(class, race, level)
	}
	b := LookupBaseStats(race, class)
	base := [7]int{b.Str, b.Sta, b.Agi, b.Dex, b.Wis, b.Int, b.Cha}
	points := float64(TrainingPointsDue(level))
	cap := StatCapBase(level)
	for i := range base {
		base[i] = min(base[i]+int(points*w[i]+0.5), cap)
	}
	return base
}

// MercIsTank reports a class whose merc is a tank (warrior, paladin, shadow
// knight).
func MercIsTank(class int) bool { return mercRoleOf(class) == roleTank }

// MercTankResists puts a tank's resists at the cap (the file's doc): each of
// the six raised to ResistCap by its gear's share, what its WIS adds counted.
func MercTankResists(g *Gear, wis int) {
	need := max(ResistCap-wisResist(wis), 0)
	for _, r := range []*int{&g.PR, &g.DR, &g.FR, &g.CR, &g.LR, &g.AR} {
		*r = max(*r, need)
	}
}

// mercTankACPerLevel is a tank merc's least armour class a level
// (MercTankAC): 2,400 at 60, the top quarter of what the level 60 warriors,
// paladins and shadow knights of the Heirloom snapshot of 2026-11-11 wore
// (their worn AC: a quarter of the 24 wearing any at 2,387 or more, the most
// 3,300; their heirlooms alone 1,110), a straight line from none at level 0
// (ours: few players below 60 wore any). The catalog's own best for a level
// is no measure: a level 60 tank's best of each slot came to 12,815 (GMs'
// records among them).
const mercTankACPerLevel = 40

// MercTankAC is a tank merc's least armour class at a level.
func MercTankAC(level int) int { return mercTankACPerLevel * max(level, 1) }
