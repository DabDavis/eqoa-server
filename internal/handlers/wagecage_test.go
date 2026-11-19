package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A quest given at a sign answers before the sign's words (petition 2418): the
// Wage Cage's one-answer question, repeatable, its "No." doing the step at once
// (an accept dostep) and giving ten snowballs; again on the next talk.
func TestASignsQuestAnswersFirst(t *testing.T) {
	f := newQuestFixture(t)
	snow := f.pattern("Snowball", 99)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable, offer, accept, decline, events)
		VALUES (60, 'Wage Cage', 1, 'Wage Cage', 1,
		'Wage Cage - if occupied a GM is 3.2% more productive.' || char(10) || 'Is the GM working hard enough?', 'No.', '',
		'[{"on":"accept","do":"dostep"}]')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, says, rewarditems)
		VALUES (60, 1, 'talk', 'Wage Cage', 'Answer the Wage Cage.',
		'10 snowballs have been deposited into your inventory. Please throw them at the GM to increase efficiency.', '%d*10')`, snow))
	f.qe.Reload()
	f.tw.h.SetSignTexts(func() (map[int]database.SignText, error) {
		return map[int]database.SignText{708: {NPCID: 708, Kind: database.SignKindQuest, Text: "Wage Cage - if occupied a GM is 3.2% more productive."}}, nil
	})
	sign := f.tw.add(4, &entity.NPC{ServerID: 708, Name: "Wage Cage", ModelID: SignBody, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
	for round := 1; round <= 2; round++ {
		f.msgs()
		f.tw.talk(sign)
		m := f.msgs()
		if !strings.Contains(strings.Join(m, "|"), "Is the GM working hard enough?") || boxID(m) == 0 {
			t.Fatalf("round %d: the sign's talk: %q, want the quest's question", round, m)
		}
		f.tw.pick(boxID(m), 0)
		m = f.msgs()
		if !strings.Contains(strings.Join(m, "|"), "10 snowballs have been deposited") {
			t.Fatalf("round %d: after No.: %q, want the snowball line", round, m)
		}
		if got := f.held(snow); got != 10*round {
			t.Fatalf("round %d: snowballs held %d, want %d", round, got, 10*round)
		}
	}
}
