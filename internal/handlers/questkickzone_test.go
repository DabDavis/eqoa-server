package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Work board 709: the quest engine tells the World which guarded creature (a kick zone that
// spares questers) a character's step to do is at: the Anvil's talk and show steps are, the
// return to Arios between them is not, nor a quest finished, nor a creature whose zone spares
// none (not among the names asked about).
func TestQuestAtNamesTheGuardedCreatureOfTheStepToDo(t *testing.T) {
	qu := &database.Quest{ID: 101029, Name: "Sarek the Betrayer", Steps: []database.QuestStep{
		{Step: 3, Kind: database.StepTalk, Target: "Anvil of Sarek"},
		{Step: 4, Kind: database.StepTalk, Target: "Arios the Battered"},
		{Step: 10, Kind: database.StepShow, Target: "Anvil of Sarek"},
		{Step: 19, Kind: database.StepKill, Target: "Anvil of Sarek"},
	}}
	q := &QuestEngine{cat: questCatalog{byID: map[int]*database.Quest{qu.ID: qu}}}
	c := &questChar{quests: map[int]*database.CharQuest{}}
	names := []string{"anvil of sarek"}
	for _, tc := range []struct {
		step, done int
		names      []string
		want       bool
	}{
		{3, 0, names, true},
		{10, 0, names, true},
		{4, 0, names, false},
		{19, 0, names, false}, // a kill step is no talk at it
		{0, 1, names, false},  // finished
		{3, 0, nil, false},    // no zone spares anyone
	} {
		c.quests[qu.ID] = &database.CharQuest{QuestID: qu.ID, Step: tc.step, Done: tc.done}
		got := q.questAtLocked(c, tc.names)["anvil of sarek"]
		if got != tc.want {
			t.Errorf("step %d done %d names %v: at the Anvil %v, want %v", tc.step, tc.done, tc.names, got, tc.want)
		}
	}
}
