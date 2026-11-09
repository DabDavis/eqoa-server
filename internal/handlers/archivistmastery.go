package handlers

import "github.com/eqoa/server/internal/entity"

// archivistmastery.go — what the Spell Archivist leaves to the masteries
// (petition 1381, Statler, 2026-11-09: "spell archivist gives necs cm version
// of mastery spells instead or regular. this prevents buying cm upgrade to
// get mc"). A mastery rank's purchase gives spells (entity.CMRank.BookChange:
// its adds, less those it improves in place). Since petition 1361 his basic
// grant has written the quests' side spells too, and the necromancer's class
// quests at 23, 33 and 43 taught Teachings of Morthalis 1-3's own spells
// (Flesh, Decay and Death Mastery +), where EQOA-Data's lists and its ranks
// name the plain ones those ranks replace: so he wrote the mastery's spell,
// and the ranks, which need the plain spells, could not be bought, nor the
// Lich, which needs the fourth. His merchants' part wrote such spells as well
// (Gloom Covenant, Deadly Covenant 1's; the wizard's Annihilations and
// Explodes): merchants here sell scrolls of them.
//
// A spell is the masteries' for a class and race (masteryOwn) when a rank
// offered to them gives it and either
//
//   - the rank takes out or needs another spell he gives them: the rank is
//     the way up from it (Gloom Pact to Gloom Covenant, Explode to Fiery
//     Explode, Flesh Mastery to Flesh Mastery +), or
//   - no merchant sells them its scroll: a rank (or a quest's wrong record)
//     is the only way to it (Flesh Mastery +, the shaman's Acidic Storm).
//
// He writes none of them, in the merchants' part or the quests', nor puts back
// a spell a rank the character owns took out (takenByOwned). A spell a
// merchant sells them that a rank only adds stays his (the rogue's Hide,
// which every class's Hide mastery gives; the paladin's Smite Undead), as
// does one a rank gives in place of spells he does not give (the magician's
// Fire Elemental, for Fire Elemental (34); the shadowknight's Cursed Kiss):
// it is the class's own way to it. The paths' spells are the reward quests'
// (none of them a rank's).

// takenByOwned is the spells the ranks a character owns took out of its book
// (entity.CMRank.BookChange's takes: Gloom Pact for Gloom Covenant, Life Draw
// for Life Draw +): his basic grant puts none of them back, which undid the
// purchase's change (Statler's book held Life Draw beside Chardith's Boon 1's
// Life Draw +).
func takenByOwned(arch *spellArchive, owned map[int]bool) map[int]bool {
	if arch == nil || len(owned) == 0 {
		return nil
	}
	out := map[int]bool{}
	for _, r := range arch.ranks {
		if !owned[r.ID] {
			continue
		}
		_, takes := r.BookChange()
		for _, a := range takes {
			if a.SpellID > 0 {
				out[a.SpellID] = true
			}
		}
	}
	return out
}

// masteryOwn is the spells he leaves to the masteries for a character of
// the class and race (the notes at the top): the ranks offered them, as
// buying each changes a book, against what he would give them at level 60.
func masteryOwn(qe *QuestEngine, arch *spellArchive, class, race int) map[int]bool {
	if arch == nil || len(arch.ranks) == 0 {
		return nil
	}
	sold, gives := map[int]bool{}, map[int]bool{} // the merchants' part; the rest he gives
	for _, s := range arch.basic(class, race, archivistLevelCap) {
		sold[s.SpellID], gives[s.SpellID] = true, true
	}
	for _, s := range sideSpells(qe, arch, class, archivistLevelCap) {
		gives[s.SpellID] = true
	}
	if qe != nil {
		paths, common := classPaths(qe, arch, class)
		for _, pa := range paths {
			for _, s := range pa.spells {
				gives[s.id] = true
			}
		}
		for _, s := range common {
			gives[s.id] = true
		}
	}
	type change struct{ gives, others []int }
	var changes []change
	ranked := map[int]bool{} // what a rank offered them gives
	for _, r := range arch.ranks {
		if !r.OfferedTo(class, race) {
			continue
		}
		in, takes := r.BookChange()
		var c change
		mine := map[int]bool{}
		for _, a := range in {
			if a.SpellID > 0 {
				c.gives = append(c.gives, a.SpellID)
				mine[a.SpellID], ranked[a.SpellID] = true, true
			}
		}
		for _, list := range [][]entity.CMAbility{takes, r.Requires} {
			for _, a := range list {
				if a.SpellID > 0 && !mine[a.SpellID] {
					c.others = append(c.others, a.SpellID)
				}
			}
		}
		changes = append(changes, c)
	}
	own := map[int]bool{}
	for _, c := range changes {
		up := false // the rank is the way up from a spell he gives
		for _, id := range c.others {
			up = up || (gives[id] && !ranked[id])
		}
		for _, id := range c.gives {
			if up || !sold[id] {
				own[id] = true
			}
		}
	}
	return own
}
