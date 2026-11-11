package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The engine tells the World which walls a character in the world passes (a
// quest under way past the wall's step, or finished), and PassedStep answers
// an item's rule (itemhide.go) by the same reading.
func TestTheEngineTellsTheWorldTheWallsPassed(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.add(5, &entity.NPC{ServerID: 3001, Name: "invisible wall", NPCType: 0x80, ModelID: int64(entity.BlockerBody), SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15730})
	f.tw.w.SetQuestWalls([]database.QuestWall{{NPCID: 3001, QuestID: 10010, Step: 1}})
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if !f.qe.PassedStep(f.sid, 10010, 0) || f.qe.PassedStep(f.sid, 10010, 1) {
		t.Fatal("at step 1: past step 0 and not step 1, wanted")
	}
	f.qe.syncQuestState()
	if f.qe.chars[f.sid].qstate.sent != "+" {
		t.Fatalf("at step 1 the wall of step 1 is passed: %q", f.qe.chars[f.sid].qstate.sent)
	}
	f.qe.mu.Lock()
	f.qe.chars[f.sid].quests[10010].Step = 2
	f.qe.mu.Unlock()
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+w3001" {
		t.Fatalf("at step 2 the World was told %q, want the wall", got)
	}
}
