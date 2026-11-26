package handlers

import (
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questpathpick.go — a path quest's pick against the path the Spell Archivist wrote (Heirloom
// petition 3200, 2026-11-26: a necromancer set to level 20 by Dabrish took the path of Blood
// Squall from the archivist, Blood Gale written in, then picked Power Gale at the level 20
// class quest, "Specialization", and held both paths).
//
// The archivist writes one path at a time (archivistpaths.go, petition 1325), and a quest's spell
// is kept beside it ("unless they do the quests manually"): a character who did the quests keeps
// what each pick gave. What that let through is a pick made AFTER the archivist's path, of
// another: two paths for one quest's worth of work. So a path quest's option (its spells, or its
// scrolls, a scroll being named as its spell) of one of the class's lines (archivistPathLines) is
// refused while the book holds a spell of another line that no quest of the character gave
// (spellsKeptButLocked): the archivist's. The step stays to do; the player picks the path held,
// or asks the archivist to change it first (which takes his path's spells out). A path held only
// by quests' picks refuses nothing, as before.

// questPathHeld is the giver's refusal (ours): %s is the path held, by its line's first spell.
const questPathHeld = "The Spell Archivist wrote the path of %s into your book. Choose that path, or ask him to change it first."

// pathLineOf is which of the class's path lines a spell or scroll of this name is of: its index,
// false for none.
func pathLineOf(class int, name string) (int, bool) {
	key := exactKey(name)
	if key == "" {
		return 0, false
	}
	for i, line := range archivistPathLines[class] {
		for _, n := range line {
			if exactKey(n) == key {
				return i, true
			}
		}
	}
	return 0, false
}

// pathPickRefusedLocked is whether opt, picked by p at a step, is of a path other than one the
// archivist wrote into p's book, and that path's name. Must be called with q.mu held.
func (q *QuestEngine) pathPickRefusedLocked(p *entity.Player, opt *database.StepOption) (string, bool) {
	if p == nil || opt == nil || q.spells == nil || len(archivistPathLines[p.Class]) == 0 {
		return "", false
	}
	picked, ok := -1, false
	for _, id := range opt.Reward.Spells {
		if pat, err := q.spells.SpellPattern(id); err == nil && pat != nil {
			if k, is := pathLineOf(p.Class, pat.SpellName); is {
				picked, ok = k, true
				break
			}
		}
	}
	if !ok && q.items != nil {
		for _, it := range opt.Reward.Items {
			if pat, err := q.items.GetItemPattern(it.PatternID); err == nil && pat != nil {
				if k, is := pathLineOf(p.Class, pat.ItemName); is {
					picked, ok = k, true
					break
				}
			}
		}
	}
	if !ok {
		return "", false
	}
	book, err := q.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		return "", false
	}
	var kept map[int]bool
	for _, e := range book {
		k, is := pathLineOf(p.Class, e.Pattern.SpellName)
		if !is || k == picked {
			continue
		}
		if kept == nil {
			kept = q.spellsKeptButLocked(p.CharServerID, p.Class, 0, 0)
		}
		if !kept[e.Pattern.SpellID] {
			return archivistPathLines[p.Class][k][0], true
		}
	}
	return "", false
}
