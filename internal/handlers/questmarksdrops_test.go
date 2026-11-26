package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 3221: a give step's sashes are farmed off creatures through their loot
// tables; the step marks their camp as a hunt (at its place), not only the giver.
func TestGiveStepMarksItsItemsDropCamp(t *testing.T) {
	q := &QuestEngine{dropSrc: map[int][]string{701525: {"bandit", "bandit henchman"}}}
	give := &database.QuestStep{Kind: database.StepGive, Target: "Zarakiel", World: 0, X: 14700, Z: 14520,
		Items: []database.ItemCount{{PatternID: 701525, Count: 4}}}
	h := q.dropStep(give)
	if h == nil || h.Kind != database.StepLoot || h.Target != "bandit" || len(h.Also) != 1 || h.NPCID != 0 {
		t.Fatalf("dropStep: %+v", h)
	}
	if give.Kind != database.StepGive || give.Target != "Zarakiel" {
		t.Fatalf("the give step itself was changed: %+v", give)
	}
	spots := map[string][]entity.NPCSpot{"bandit": {
		spotAt(1, "bandit", 0, 14859, 14518), spotAt(2, "bandit", 0, 14857, 14590), spotAt(3, "bandit", 0, 14718, 14500),
		spotAt(4, "bandit", 0, 24000, 1000), spotAt(5, "bandit", 0, 24020, 1010), spotAt(6, "bandit", 0, 24030, 1020),
		spotAt(7, "bandit", 0, 24040, 1030),
	}, "bandit henchman": {spotAt(8, "bandit henchman", 0, 14800, 14540)}}
	kind, at := q.stepSpotsLocked(spots, h, 0, 0, 14084, 14525)
	if kind != markHunt || len(at) == 0 {
		t.Fatalf("no hunt mark: %v %v", kind, at)
	}
	for _, s := range at {
		if s.X > 15000 {
			t.Errorf("a mark at the far camp: %v", at)
		}
	}
	// the giver's own mark is unchanged
	if k, g := q.stepSpotsLocked(map[string][]entity.NPCSpot{"zarakiel": {spotAt(9, "Zarakiel", 0, 15000, 15000)}}, give, 0, 0, 0, 0); k != markStep || len(g) != 1 {
		t.Errorf("the giver: %v %v", k, g)
	}
	// controls: no creature drops it, or not a give step
	if q.dropStep(&database.QuestStep{Kind: database.StepGive, Target: "Zarakiel", Items: []database.ItemCount{{PatternID: 5, Count: 1}}}) != nil {
		t.Error("an item nothing drops marked")
	}
	if q.dropStep(&database.QuestStep{Kind: database.StepTalk, Target: "Zarakiel"}) != nil {
		t.Error("a talk step marked")
	}
}
