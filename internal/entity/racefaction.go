package entity

import "log"

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

// Spells too (a GM's request, the owner's approval, Heirloom petition 2792:
// a dark elf in an enchanter's Glamour: Halfling still drew the Rivervale
// coach's red face): a race illusion or glamour whose effects carry the
// workbook's Faction Change (`"kind":"faction"`, EffectDisguise) shifts the
// shown standings as the item's "Race Faction" line does, by the race of its
// name ("Glamour: Halfling") or, for one named otherwise, of its body
// (raceBodyLocked: the workbook's race sprites). A form that is no race
// (Brownie, Clay Golem) keeps the disguise alone (no attack on sight by
// standing, workbookmore.go). Nothing else changes: the face, a merchant's or
// coachman's refusal, dialog's mood, quest gates and #stats read the shown
// standing (standingLocked); the faction window, kill, quest and raise hits and
// the Testing Heroes read the real one (ownStandingLocked).
//
// Seen through (ours: no record says what an attack did): the bearer's first
// harmful act on a creature (an engaged attack, a harmful spell at one, a
// thrown potion) ends the regard and the disguise of every such effect on it,
// its body and stats kept to their time, and says disguiseSeenThrough.

// disguiseSeenThrough is the line a disguise's end by a harmful act says (ours).
const disguiseSeenThrough = "Your disguise is seen through."

// raceFactionOf is the Rules.FactionRace a spell gives what lasts of it: its
// illusion's race + 1 when its line says "Race Faction", or its effects carry a
// faction change, and the illusion is of a player race by its name, else 0
// (one of a race body by another name: factionRaceLocked).
func raceFactionOf(d *SpellDef) int {
	if d.FormKind == FormNone || d.FormKind == FormEnd || !(d.RaceFaction || d.changesFaction()) {
		return 0
	}
	if race, ok := illusionRaces[d.Form]; ok {
		return race + 1
	}
	return 0
}

// changesFaction is whether a spell's effects carry a faction change (the
// workbook's, EffectDisguise).
func (d *SpellDef) changesFaction() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectDisguise {
			return true
		}
	}
	return false
}

// factionRaceLocked is the race whose standings p shows: a race illusion with
// "Race Faction" or a faction change on it (the latest landed), or a disguise
// whose body is a race's; false none. Must be called with the lock held.
func (w *World) factionRaceLocked(p *Player) (int, bool) {
	race, ok := 0, false
	for _, e := range p.Effects {
		switch {
		case e.Form == 0:
		case e.Rules.FactionRace > 0:
			race, ok = e.Rules.FactionRace-1, true
		case e.Rules.Disguise:
			if _, r, isRace := w.raceBodyLocked(e.Form, p.Sex); isRace {
				race, ok = r, true
			}
		}
	}
	return race, ok
}

// seenThroughLocked ends the regard and the disguise of every effect on p
// that gives either (the file's doc), at its harmful act on a creature, and
// tells it once. Must be called with the lock held.
func (w *World) seenThroughLocked(p *Player, why string) {
	seen := false
	for _, e := range p.Effects {
		if e.Rules.Disguise || e.Rules.FactionRace > 0 {
			e.Rules.Disguise, e.Rules.FactionRace = false, 0
			seen = true
		}
	}
	if !seen {
		return
	}
	log.Printf("Faction: %s's disguise is seen through (%s)", p.Name, why)
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendChatToSession(sess, disguiseSeenThrough)
	}
}
