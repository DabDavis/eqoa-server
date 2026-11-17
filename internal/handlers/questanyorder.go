package handlers

// Steps done in any order (the owner, 2026-11-17: "for the xmas quests it has
// multiple quest npcs to talk to for singing and giving gifts, lets make it
// completeable in any order, players can speak with bowyer koll or tailor
// bariston in any order").
//
// A run of steps at an NPC next to one another, each with queststeps.anyorder
// set, is a group (database.Quest.AnyOrderGroup): while the player is on it,
// every step of it not yet done may be done, at its own NPC, with its own
// answer, words and rewards; the quest goes on past the group once all are.
// The player's row keeps the group's first step as its step and which of the
// group are done in its count, a bit each in the group's order (a talk step
// counts nothing else). A row from before (its step a later one of the group,
// count 0: the steps before it done in order) reads as those done.

import (
	"strings"

	"github.com/eqoa/server/internal/database"
)

// anyOrderState is the group cq's step is in and which of it are done (a bit
// each, in the group's order); nil when the step is no group's.
func anyOrderState(qu *database.Quest, cq *database.CharQuest) ([]*database.QuestStep, int) {
	if qu == nil || cq == nil || cq.Step == 0 {
		return nil, 0
	}
	g := qu.AnyOrderGroup(cq.Step)
	if g == nil {
		return nil, 0
	}
	done := cq.Count
	for k, s := range g {
		if s.Step == cq.Step {
			// a row from before the group: the steps before its step done
			done |= 1<<k - 1
			break
		}
	}
	return g, done & (1<<len(g) - 1)
}

// openSteps is what the player may do now of quest qu: its group's steps not
// yet done, else its step (none when finished).
func openSteps(qu *database.Quest, cq *database.CharQuest) []*database.QuestStep {
	if qu == nil || cq == nil {
		return nil
	}
	if g, done := anyOrderState(qu, cq); g != nil {
		var out []*database.QuestStep
		for k, s := range g {
			if done&(1<<k) == 0 {
				out = append(out, s)
			}
		}
		return out
	}
	if st := qu.Step(cq.Step); st != nil {
		return []*database.QuestStep{st}
	}
	return nil
}

// isOpen is whether step n of qu may be done now.
func isOpen(qu *database.Quest, cq *database.CharQuest, n int) bool {
	for _, s := range openSteps(qu, cq) {
		if s.Step == n {
			return true
		}
	}
	return false
}

// anyOrderDone marks step st of its group done in cq: whether the whole group
// is now (the quest goes on past it), and the group's last step (what comes
// after it is the quest's next). ok is false when st is no group's.
func anyOrderDone(qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) (all bool, last *database.QuestStep, ok bool) {
	g, done := anyOrderState(qu, cq)
	if g == nil {
		return false, nil, false
	}
	k := -1
	for i, s := range g {
		if s.Step == st.Step {
			k = i
		}
	}
	if k < 0 {
		return false, nil, false
	}
	done |= 1 << k
	full := 1<<len(g) - 1
	if done == full {
		return true, g[len(g)-1], true
	}
	cq.Step, cq.Count = g[0].Step, done
	return false, g[len(g)-1], true
}

// anyOrderLine is the journal's line while a group is under way: the one step
// left's own line, else the quest's name and who is left ("Gifts for
// Freeport: Tailor Bariston, Bowyer Koll"); "" when cq is on no group.
func anyOrderLine(qu *database.Quest, cq *database.CharQuest) string {
	g, _ := anyOrderState(qu, cq)
	if g == nil {
		return ""
	}
	left := openSteps(qu, cq)
	if len(left) == 1 && strings.TrimSpace(left[0].Journal) != "" {
		return strings.TrimSpace(left[0].Journal)
	}
	var names []string
	for _, s := range left {
		names = append(names, strings.TrimSpace(s.Target))
	}
	name := strings.TrimSpace(qu.Name)
	if name == "" {
		return strings.Join(names, ", ")
	}
	return name + ": " + strings.Join(names, ", ")
}
