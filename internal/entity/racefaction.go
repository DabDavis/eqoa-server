package entity

// Standings under a race illusion (a GM, 2026-11-12, work board 1258: an item's
// "Illusion: Elf" that shows its bearer to the world as an Elf, faction too).
// An illusion of a player race whose line says "Race Faction" (Rules.
// FactionRace) makes every standing the world reads of its bearer
// (standingLocked: aggro on sight, the target's face, dialog's mood, quests'
// faction gates) its own standing plus what the race's start with the faction
// differs from its own race's (the bearer's class and human type), held to
// -2000..2000. The faction window shows its own standings, and every hit (a
// kill's, a quest's, an item's) moves its own (ownStandingLocked). It ends with
// the illusion: its time, clicked off, a death, another illusion.

// raceFactionOf is the Rules.FactionRace a spell gives what lasts of it: its
// illusion's race + 1 when its line says "Race Faction" and the illusion is of
// a player race, else 0.
func raceFactionOf(d *SpellDef) int {
	if !d.RaceFaction || d.FormKind == FormNone || d.FormKind == FormEnd {
		return 0
	}
	if race, ok := illusionRaces[d.Form]; ok {
		return race + 1
	}
	return 0
}

// factionRace is the race whose standings p shows (a race illusion with
// "Race Faction" on it: the latest landed), false none.
func (p *Player) factionRace() (int, bool) {
	race, ok := 0, false
	for _, e := range p.Effects {
		if e.Rules.FactionRace > 0 && e.Form != 0 {
			race, ok = e.Rules.FactionRace-1, true
		}
	}
	return race, ok
}
