package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
)

// maxDropNames bounds the creature names one item's marks count (a drop shared
// by a whole region's creatures would mark none usefully).
const maxDropNames = 8

// Petition 3221 (Paladin Exemplar's Bandit Justice): "it only shows the quest
// giver". A give step names the NPC it is handed in to, so its mark was that
// NPC alone, though the item (a bandit sash) is farmed off creatures through
// their loot tables (npcloot), not through a loot step. The marks now add, for
// a give step's item that creatures drop, the camp of those creatures as a
// hunt mark, at the step's own place when it has one (queststeps x, z, world:
// the camp east of Darvor Manor), else the nearest camps.

// dropSources reads, for the give steps' items of the quests, the creatures
// that drop each; none on an error (the marks stay the giver's).
func dropSources(store *database.QuestStore, quests []*database.Quest) map[int][]string {
	items := map[int]bool{}
	for _, qu := range quests {
		for i := range qu.Steps {
			if st := &qu.Steps[i]; st.Kind == database.StepGive {
				for _, it := range st.Items {
					items[it.PatternID] = true
				}
			}
		}
	}
	if len(items) == 0 || store == nil {
		return nil
	}
	src, err := store.DropSources(items)
	if err != nil {
		log.Printf("quest marks: drop sources: %v", err)
		return nil
	}
	for id, names := range src {
		if len(names) > maxDropNames {
			delete(src, id)
		}
	}
	return src
}

// dropStep is a hunt step standing for a give step's first item that creatures
// drop: a loot step for those creatures at the give step's place; nil for any
// other step or an item no creature drops.
func (q *QuestEngine) dropStep(st *database.QuestStep) *database.QuestStep {
	if st.Kind != database.StepGive {
		return nil
	}
	for _, it := range st.Items {
		if names := q.dropSrc[it.PatternID]; len(names) > 0 {
			h := *st
			h.Kind = database.StepLoot
			h.Target, h.Also = names[0], append([]string(nil), names[1:]...)
			h.NPCID = 0
			h.Items = []database.ItemCount{it}
			return &h
		}
	}
	return nil
}
