package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Petition 2530 (2026-11-20): the Cazic-Thule trials' step 3 is the second
// Kithgrin's (a talk pinned to one spawn); a player who talked to the third,
// another spawn of the name, heard nothing. Now the other Kithgrin says the
// step's journal line, "Not I." before it, and the pinned one still does the
// step. The control: with no step pinned elsewhere, a Kithgrin of no quest
// says nothing (Talk false).
func TestAnotherSpawnOfThePinnedNameSaysWhereToGo(t *testing.T) {
	f := newQuestFixture(t)
	third := &entity.NPC{ServerID: 800, Name: "Kithgrin", SpawnX: 25628, SpawnY: 29.9, SpawnZ: 3124}
	second := &entity.NPC{ServerID: 801, Name: "Kithgrin", SpawnX: 25603, SpawnY: 29.9, SpawnZ: 3096}
	f.tw.add(3, third)
	f.tw.add(4, second)
	if f.qe.Talk(f.tw.s, f.tw.p, third) {
		t.Fatal("the control: a Kithgrin of no quest answered")
	}
	f.msgs()

	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (701, 'The Trials', 1, 'Kithgrin', 1)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, npcid, journal, says) VALUES (701, 1, 'talk', 'Kithgrin', 801, ?, ?)`,
		"Go down to the second Kithgrin, at the head of the catacombs.", "Kithgrin: You came.")
	f.reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 701, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	if !f.qe.Talk(f.tw.s, f.tw.p, third) {
		t.Fatal("the third Kithgrin said nothing")
	}
	f.heardOnly(f.msgs(), "Kithgrin: Not I. Go down to the second Kithgrin, at the head of the catacombs.")

	if !f.qe.Talk(f.tw.s, f.tw.p, second) {
		t.Fatal("the second Kithgrin did not take the step")
	}
	f.want(f.msgs(), "box:Kithgrin: You came.", "box:"+questFinished)
}
