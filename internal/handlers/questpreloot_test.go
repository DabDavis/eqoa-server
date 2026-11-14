package handlers

import (
	"strconv"
	"testing"
)

// The owner, 2026-11-14: "allow pre-looting for all quests" (questpreloot.go). Before the
// quest is taken, a kill of its loot step's creature drops the step's item at the step's
// chance (0.7 nothing, 0.2 a belt); one kill drops one belt at most though the roll is
// drawn once. Holding the two the step wants, no more drop. Taken with the two in the pack,
// the step is done at once. Finished (not repeatable), no drop; another creature none ever.
func TestAQuestsItemDropsBeforeItsStep(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := lootQuest(f, belt, 0.7, 0.2, 0.2, 0.1, 0.1)
	f.killFor("a bat", 801)
	f.killFor("Orc Pawn", 802) // 0.7: none
	if len(*drops) != 0 {
		t.Fatalf("drops before: %v", *drops)
	}
	f.killFor("orc pawn", 803) // 0.2: a belt, not on the quest
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{803, f.sid, belt}) {
		t.Fatalf("the drops: %v, want the belt on 803 before the quest", *drops)
	}
	f.own(belt, 2)
	f.killFor("Orc Pawn", 804) // two held: no roll, no drop
	if len(*drops) != 1 {
		t.Fatalf("two held of two, another dropped: %v", *drops)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("taken with the two belts held, the step is %d, want 2 (done at once)", s)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.tw.s.SetInventoryList(newInvList(nil))
	f.killFor("Orc Pawn", 805) // finished, not repeatable: none at a roll of 0.2
	if len(*drops) != 1 {
		t.Fatalf("a finished quest's item dropped: %v", *drops)
	}
}

// A character on an earlier step (a talk first) gets the later loot step's item too; one on
// the step itself gets one drop a kill, as ever, not two.
func TestAQuestsItemDropsOnAnEarlierStep(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (61, 'Belts Later', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (61, 1, 'talk', 'Malsis', 'See Malsis.')`)
	drops := lootQuest(f, belt, 0.1, 0.1, 0.1)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (61, 2, 'loot', 'Orc Pawn', '` + strconv.Itoa(belt) + `', 1, 0.5, 'One belt.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 61, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 810)
	if len(*drops) != 1 {
		t.Fatalf("on the talk before the loot step: drops %v, want one", *drops)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 811) // on quest 60's step and before 61's: one belt
	if len(*drops) != 2 {
		t.Fatalf("on one step and before another of the same item: drops %v, want one more", *drops)
	}
}
