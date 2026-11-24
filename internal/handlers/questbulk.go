package handlers

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questbulk.go — "Hand in all" at a repeatable turn-in's giver (Heirloom
// petition 2847, the owner's "yes", 2026-11-24). A player holding several full
// sets of a repeatable hand-in (Chef Lukkif's Fiery Hot Peppers, four a set)
// took the quest, accepted and handed in once a set: three clicks a set. Now,
// when such a quest's hand-in at its giver finishes it and the pack still
// holds at least one more full set, the same talk asks one question box
// (0x0034, as every NPC menu here, so a PS2 and vanilla show it alike):
//
//	You hold 3 more sets for Fiery Hot Peppers. Hand them in too?
//	  Hand in all (3 sets) | Just one
//
// Two answers: CLIENT pages a box past three (welcome.go's notes), so it fits
// one page. "Just one" changes nothing (the one handed in stays handed in).
//
// "Hand in all" does each set through the ordinary path, one after another:
// the quest taken (startLocked: its journal line, its loot step counted off the
// held items, questpreloot.go), then its hand-in at the giver (atNPCLocked:
// the items taken, the rewards, experience with its boons and the masteries'
// share, faction, the finish count), so each set pays exactly what a separate
// turn-in would and nothing is paid twice or skipped. Quest marks are not paid:
// a repeatable quest pays none (marks.go's QuestDone), and a first finish only.
// Every set is checked again before it is done (the quest still takeable: a
// level past its maxlevel stops it; its items held; room in the pack for its
// rewards), and the run stops at the first that cannot be done, saying how
// many went in. At most bulkHandInMost sets a talk (a burst bounded: each set
// is its own handful of messages); more left are asked again at the next
// hand-in. A partial set stays in the pack and counts toward the next round, as
// before.
//
// Only a plain turn-in is offered it (bulkHandInStep): repeatable (not daily),
// no events on the quest or its steps (no trigger, spawn, popup or held
// step), steps of loot and give only, one give step, the last, at the giver,
// asking no tunar, with no choice and no any-order group; each loot step's
// item among the give step's (so the held items are the set); not a Plane of
// Sky chain (skygroup.go: a doer's step is its group's). Anything else, or one
// set held, is as before.

// bulkHandInMost is the most sets one "Hand in all" hands in.
const bulkHandInMost = 20

const (
	bulkJustOne = "Just one"
	bulkAskDef  = "Hand in the other sets too?"
)

// bulkHandInStep is a quest's last step when the quest is a plain repeatable
// turn-in that "Hand in all" may repeat (the file's doc), else nil.
func bulkHandInStep(qu *database.Quest) *database.QuestStep {
	if qu == nil || !qu.Repeatable || qu.Daily || qu.Source == "thanksgiving-2026" || skyChainQuests[qu.ID] ||
		len(qu.Events) > 0 || len(qu.Steps) == 0 {
		return nil
	}
	var give *database.QuestStep
	var loots []*database.QuestStep
	last := (*database.QuestStep)(nil)
	for st := qu.First(); st != nil; st = qu.Next(st.Step) {
		last = st
		if len(st.Events) > 0 || len(st.Choices.Options) > 0 || st.AnyOrder || st.Tunar > 0 {
			return nil
		}
		switch st.Kind {
		case database.StepGive:
			if give != nil {
				return nil
			}
			give = st
		case database.StepLoot:
			loots = append(loots, st)
		default:
			return nil
		}
	}
	if give == nil || give != last || len(give.Items) == 0 || foldName(give.Target) != foldName(qu.Giver) {
		return nil
	}
	for _, l := range loots {
		pid, _, ok := l.LootItem()
		if !ok {
			return nil
		}
		in := false
		for _, it := range give.Items {
			in = in || it.PatternID == pid
		}
		if !in {
			return nil
		}
	}
	return give
}

// setsHeldLocked is how many full sets of a give step's items the pack holds:
// an item's Ascended records count for it, as the hand-in counts them
// (planHandInLocked).
func (q *QuestEngine) setsHeldLocked(p *entity.Player, st *database.QuestStep) int {
	need := map[int]int{}
	for _, it := range st.Items {
		need[it.PatternID] += max(it.Count, 1)
	}
	if len(need) == 0 {
		return 0
	}
	var asc map[int]int
	if !st.ExactBase {
		asc = q.ascendedMapLocked()
	}
	have := map[int]int{}
	for _, r := range q.heldItems(p) {
		have[r.Pattern.PatternID] += r.StackLeft
		if b, ok := asc[r.Pattern.PatternID]; ok {
			have[b] += r.StackLeft
		}
	}
	n := -1
	for id, k := range need {
		if s := have[id] / k; n < 0 || s < n {
			n = s
		}
	}
	return max(n, 0)
}

// offerBulkLocked asks "Hand in all" after a hand-in (st, done at the giver)
// finished a plain repeatable turn-in, when the pack holds another full set
// and nothing else is asked.
func (q *QuestEngine) offerBulkLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) {
	if sess == nil || p == nil || c.bulking || c.box != nil || cq == nil || cq.Active() || cq.Done < 1 || bulkHandInStep(qu) != st {
		return
	}
	if q.mayTakeLocked(q.lookAt(sess), p, c, qu) != "" {
		return
	}
	held := q.setsHeldLocked(p, st)
	if held < 1 {
		return
	}
	n := min(held, bulkHandInMost)
	words := fmt.Sprintf("You hold %d more %s for %s. Hand them in too?", held, plural(held, "set", "sets"), qu.Name)
	if held > n {
		words = fmt.Sprintf("You hold %d more sets for %s. Hand in %d now?", held, qu.Name, n)
	}
	q.askLocked(sess, c, "", words, bulkAskDef, []questChoice{
		{kind: choiceBulk, quest: qu.ID, step: st.Step, option: n, label: fmt.Sprintf("Hand in all (%d %s)", n, plural(n, "set", "sets"))},
		{kind: choiceNothing, label: bulkJustOne},
	})
	log.Printf("Quests: %s holds %d more sets of %q: asked to hand in %d", p.Name, held, qu.Name, n)
}

// bulkHandInLocked is "Hand in all" answered: up to want sets (at most
// bulkHandInMost), each taken and handed in as a separate turn-in, until one
// cannot be.
func (q *QuestEngine) bulkHandInLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, want int) {
	st := bulkHandInStep(qu)
	if st == nil || p == nil {
		return
	}
	want = min(want, bulkHandInMost)
	c.bulking = true
	defer func() { c.bulking = false }()
	got, why := 0, ""
	for got < want {
		cq := c.quests[qu.ID]
		if cq != nil && cq.Active() {
			why = "it is under way"
			break
		}
		if why = q.mayTakeLocked(q.lookAt(sess), p, c, qu); why != "" {
			break
		}
		if lack := q.lackingLocked(p, st); lack != "" {
			why = "it lacks " + lack
			break
		}
		before := 0
		if cq != nil {
			before = cq.Done
		}
		if why = q.startLocked(sess, p, c, qu, 0, false); why != "" {
			break
		}
		cq = c.quests[qu.ID]
		if cq == nil || !cq.Active() || cq.Step != st.Step {
			why = "its hand-in was not the step to do once taken"
			break
		}
		q.atNPCLocked(sess, p, c, qu, cq, st, true, -1)
		if cq.Done != before+1 {
			why = "its hand-in was not done"
			break
		}
		got++
	}
	if got < want {
		log.Printf("Quests: %s handed in %d of %d sets of %q: %s", p.Name, got, want, qu.Name, why)
	} else {
		log.Printf("Quests: %s handed in %d sets of %q at once", p.Name, got, qu.Name)
	}
	if got > 0 {
		q.tellLocked(c, sess, "", fmt.Sprintf("You handed in %d more %s.", got, plural(got, "set", "sets")))
	}
}
