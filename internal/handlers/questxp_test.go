package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// world.quest_xp_pct (the GM Jibwiz's petition 1915: Heirloom's one-time quests
// four times their experience): a one-time quest's 430 is 1720 at 400; a
// repeatable's and a hunt and gather's (by its source, made one-time) stay;
// unset or 100, every quest's stays.
func TestQuestXPShare(t *testing.T) {
	t.Cleanup(func() { SetQuestXPPct(0) })
	once := &database.Quest{Name: "The pincers", Source: "EQOA-Data 1-20/freeport/warrior/L1 (tools/import_quests.py)"}
	again := &database.Quest{Name: "Skins", Repeatable: true}
	hunt := &database.Quest{Name: "Hajyd skins", Source: "EQOA-Data HuntAndGather/hajyd-patron-skins (tools/import_quests.py)"}
	for _, c := range []struct {
		pct               int
		once, again, hunt int
	}{{0, 430, 430, 430}, {100, 430, 430, 430}, {400, 1720, 430, 430}} {
		SetQuestXPPct(c.pct)
		if got := [3]int{questXPShare(once, 430), questXPShare(again, 430), questXPShare(hunt, 430)}; got != [3]int{c.once, c.again, c.hunt} {
			t.Errorf("at %d%%: %v, want %v", c.pct, got, [3]int{c.once, c.again, c.hunt})
		}
	}
	if questXPShare(once, 0) != 0 {
		t.Error("no experience got some")
	}
}
