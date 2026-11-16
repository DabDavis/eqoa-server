package handlers

import (
	"fmt"
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

// A door an NPC keeps (entity/questdoors.go, petition 2212) is passed as a
// wall is: past its quest's step, or holding its item; the engine says so to
// the World under the door's NPC.
func TestTheEngineTellsTheWorldTheDoorsPassed(t *testing.T) {
	f := newQuestFixture(t)
	scent := f.pattern("Cazic's Scent", 1)
	px, py, pz := f.tw.p.X, f.tw.p.Y, f.tw.p.Z
	f.tw.add(5, &entity.NPC{ServerID: 3003, Name: "The Guardian", SpawnX: px + 6, SpawnY: py, SpawnZ: pz})
	box := []database.WallBox{{X0: px - 1, Y0: py - 1, Z0: pz - 1, X1: px + 1, Y1: py + 1, Z1: pz + 1}}
	f.tw.w.SetQuestDoors([]database.QuestDoor{
		{NPCID: 3003, Item: scent, Inner: box, Outer: box, Outside: [3]float32{px, py, pz}},
		{NPCID: 3004, QuestID: 10010, Step: 1, Inner: box, Outer: box, Outside: [3]float32{px, py, pz}},
	})
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+" {
		t.Fatalf("with neither the World was told %q", got)
	}
	f.own(scent, 1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.qe.mu.Lock()
	f.qe.chars[f.sid].quests[10010].Step = 2
	f.qe.mu.Unlock()
	f.qe.syncQuestState()
	if got, want := f.qe.chars[f.sid].qstate.sent, fmt.Sprintf("+d3003,d3004,i%d", scent); got != want {
		t.Fatalf("with the scent and past the step the World was told %q, want %q", got, want)
	}
}
