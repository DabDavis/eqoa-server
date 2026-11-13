package handlers

import (
	"log"
	"strings"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
)

// questxp.go — a server's share of its one-time quests' experience (world.quest_xp_pct; the GM Jibwiz's
// petition 1915, for the Heirloom Test Server: "On heirloom increase NON hunt and gather or repeatable quest
// XP by 4x its current amount"). Every step's experience of a quest that is not repeatable and not a hunt
// and gather is the share's (400: four times), as it is given (quests.go) and as the quest helper says it
// (questmarks.go); the rows stay as they are, so the share is undone by the config alone.
//
// A hunt and gather is EQOA-Data's HuntAndGather import (quests.source "EQOA-Data HuntAndGather/...": the
// Hajyd patrons' skins and the rest, 60 rows on 2026-11-13, every one repeatable): left out by its source as
// well as by its repeatable flag, so one a GM makes one-time still gives what its row says.

var questXPPct atomic.Int32

// SetQuestXPPct sets the share (0 or 100: none).
func SetQuestXPPct(pct int) {
	questXPPct.Store(int32(max(pct, 0)))
	if pct > 0 && pct != 100 {
		log.Printf("Quests: one-time quests give %d%% of their experience (world.quest_xp_pct)", pct)
	}
}

// huntAndGather is whether a quest is a hunt and gather (the file's doc).
func huntAndGather(qu *database.Quest) bool {
	return strings.Contains(qu.Source, "HuntAndGather/")
}

// questXPShare is xp, a step's experience of qu, after the server's share.
func questXPShare(qu *database.Quest, xp int) int {
	pct := int(questXPPct.Load())
	if xp <= 0 || pct <= 0 || pct == 100 || qu == nil || qu.Repeatable || huntAndGather(qu) {
		return xp
	}
	return int(int64(xp) * int64(pct) / 100)
}
