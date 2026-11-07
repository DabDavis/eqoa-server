package entity

// race.go — a race the server has and CLIENT does not (the user, 2026-11-07:
// "rename giantdab to Lagerbottoms, race is Hill Giant").
//
// CLIENT's gRaces has names, cameras, a create page and every item's race bit
// for the races it ships, and a slot past them (or an item's mask without its
// bit) leaves a character with no camera and nothing it may wear, in vanilla and
// on a PS2 alike. So a Hill Giant is an Ogre on the wire (WireRace: the nearest
// build) and its own race here: its stats (raceBonus), its masteries
// (cmcatalog.races, bit 11), its standings (factionstart) and its name in what
// the server writes itself (a GM's lines, the target box's title). Its body is
// the character's own model id, as an illusion's is.

// RaceHillGiant is the Hill Giant's race on the server. Only a GM's hand makes
// one: character creation offers the races CLIENT has.
const RaceHillGiant = 11

// WireRace is the race CLIENT is told, and an item's mask is asked about, for a
// race of the server's.
func WireRace(race int) int {
	if race == RaceHillGiant {
		return raceOgre
	}
	return race
}

// allRacesMask is cmcatalog.races' "every race": the eleven bits CLIENT's races
// and the froglok fill.
const allRacesMask = 2047

// raceInMask reports a race in a bit set of races: its own bit, or a race past
// the eleven in a set of every race.
func raceInMask(mask, race int) bool {
	if race > 10 && mask&allRacesMask == allRacesMask {
		return true
	}
	return mask&(1<<uint(race)) != 0
}
