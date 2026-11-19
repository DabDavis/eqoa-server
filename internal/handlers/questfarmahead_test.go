package handlers

import (
	"strconv"
	"testing"
)

// The owner, 2026-11-18 (petition 2383): a REPEATABLE quest's item farms ahead past
// the count one round wants (the SK exemplar's Screaming Souls: ten a round, the souls stack to
// 999), before the quest is taken and past its loot step too (on its give step: the next
// round's). A one-time quest stops at its count, as before. Each round's give step takes only
// its own count (TestAFullPackWaitsForRoom: three held, two taken).
func TestARepeatableQuestFarmsAhead(t *testing.T) {
	f := newQuestFixture(t)
	soul := f.pattern("screaming soul", 999)
	drops := lootQuest(f, f.pattern("unused", 1), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 60`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (72, 'Screaming Souls', 1, 'Urglauh', 1)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (72, 1, 'loot', 'a paladin', '` + strconv.Itoa(soul) + `', 2, 1.0, 'Two souls.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, journal)
		VALUES (72, 2, 'give', 'Urglauh', '` + strconv.Itoa(soul) + `*2', 1, 'Feed the souls.')`)
	f.qe.Reload()

	f.own(soul, 2)
	f.killFor("a paladin", 901) // not taken, the two one round wants held: a repeatable quest's drops on
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{901, f.sid, soul}) {
		t.Fatalf("repeatable, two held of two: drops %v, want a soul", *drops)
	}

	f.exec(`UPDATE quests SET repeatable = 0 WHERE questid = 72`)
	f.qe.Reload()
	*drops = nil
	f.killFor("a paladin", 902) // the control: one-time, two held: none
	if len(*drops) != 0 {
		t.Fatalf("one-time, two held of two: dropped %v", *drops)
	}

	f.exec(`UPDATE quests SET repeatable = 1 WHERE questid = 72`)
	f.qe.Reload()
	f.own(soul, 1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 72, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(72); s != 2 {
		t.Fatalf("taken with three souls held: step %d, want 2 (the loot step done at once)", s)
	}
	*drops = nil
	f.killFor("a paladin", 903) // on the give step, past the loot step: the next round's soul
	if len(*drops) != 1 || (*drops)[0].pattern != soul {
		t.Fatalf("repeatable, on its give step: drops %v, want a soul", *drops)
	}

	f.exec(`UPDATE quests SET repeatable = 0 WHERE questid = 72`)
	f.qe.Reload()
	*drops = nil
	f.killFor("a paladin", 904) // the control: one-time, past its loot step: none
	if len(*drops) != 0 {
		t.Fatalf("one-time, on its give step: dropped %v", *drops)
	}
}
