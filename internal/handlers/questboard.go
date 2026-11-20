package handlers

import (
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A giver of repeatable work lets the player choose (petition 2388: "every
// time you talk to him again he offers you a new quest and theres nothing to
// hand in ... it would be nice if you had an option to choose which one your
// looking to do", of Chef Lukkif's thirteen Hunt and Gather quests). A giver
// whose every quest in the game is repeatable, with two or more the player may
// take now (mayTakeLocked), asks which, a quest's name an answer; the one
// picked is offered as ever (its offer, or taken at once with no answer to
// take it). More than a box holds go in pages, the last answer of each the
// next page. A giver of one takeable quest, of any quest that is not
// repeatable (a chain), or of a quest whose first step hands it items, offers
// as before (offerAtLocked, readyHandInLocked).

// questAskBoard is the board's question (ours: no recording has one), and
// questBoardMore the answer that turns the page.
const (
	questAskBoard  = "Which will you do?"
	questBoardMore = "Something else."
)

// boardLocked is the quests a giver of repeatable work offers the player now,
// in the catalog's order: nil when it is not such a giver or fewer than two
// may be taken.
func (q *QuestEngine) boardLocked(look *factionLook, p *entity.Player, c *questChar, giver string) []*database.Quest {
	all := q.cat.givers[foldName(giver)]
	if len(all) < 2 {
		return nil
	}
	var out []*database.Quest
	for _, qu := range all {
		if !qu.Enabled {
			continue
		}
		if !qu.Repeatable {
			return nil
		}
		// a hand-in at the giver (the Plane of Sky's armourers, a quest a
		// piece): offered by what the player holds (readyHandInLocked), a
		// no moving on to the next (offerAtLocked), as before
		if st := qu.First(); st != nil && (st.Kind == database.StepGive || st.Kind == database.StepShow) && foldName(st.Target) == foldName(giver) {
			return nil
		}
		if q.mayTakeLocked(look, p, c, qu) == "" {
			out = append(out, qu)
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// askBoardLocked opens the board's page (from 0; past the last, the first
// again): its quests' names, and the page turn when they do not all fit.
func (q *QuestEngine) askBoardLocked(sess *session.Session, c *questChar, giver string, list []*database.Quest, page int) {
	room := dialogRoom(sess)
	per := len(list)
	if per > room {
		per = max(room-1, 1)
	}
	pages := (len(list) + per - 1) / per
	if page < 0 || page >= pages {
		page = 0
	}
	var choices []questChoice
	for _, qu := range list[page*per : min((page+1)*per, len(list))] {
		choices = append(choices, questChoice{kind: choiceBoard, quest: qu.ID, label: qu.Name})
	}
	if pages > 1 {
		choices = append(choices, questChoice{kind: choiceBoardMore, quest: list[0].ID, option: page + 1, label: questBoardMore})
	}
	q.askLocked(sess, c, giver, "", questAskBoard, choices)
}
