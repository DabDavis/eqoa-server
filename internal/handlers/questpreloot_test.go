package handlers

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
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

// Petition 2098 (Heirloom), Harbingers of Khalimet: two loot steps in a row off one
// creature (Armis's tome, then his wand), the creature spawned by the first step's start and
// gone once looted. On the first step one kill drops both: its own item, and the next step's
// ahead of it. Taken in either order, the tome does step 1 and the wand held does step 2 at once.
func TestTwoLootStepsOffOneCreatureDropBothOnOneKill(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	tome, wand := f.pattern("Tome of Khalimet", 1), f.pattern("Wand of Shattered Souls", 1)
	drops := lootQuest(f, belt, 0.1, 0.1)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (62, 'Harbingers', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (62, 1, 'loot', 'Armis', '` + strconv.Itoa(tome) + `', 1, 1.0, 'The tome.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (62, 2, 'loot', 'Armis', '` + strconv.Itoa(wand) + `', 1, 1.0, 'The wand.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (62, 3, 'talk', 'Malsis', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 62, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("Armis", 820)
	if len(*drops) != 2 || (*drops)[0] != (lootDrop{820, f.sid, tome}) || (*drops)[1] != (lootDrop{820, f.sid, wand}) {
		t.Fatalf("one kill on the tome's step dropped %v, want the tome and the wand", *drops)
	}
	f.own(wand, 1) // the wand taken first: step 1 still wants the tome
	f.own(tome, 1)
	f.qe.Looted(f.tw.s, f.tw.p, tome, 1)
	if s, _ := f.progress(62); s != 3 {
		t.Fatalf("both taken: step %d, want 3 (the wand held does step 2 at once)", s)
	}
}

// The Split Ruby (Oggok, 100535): a ruby half off one creature at step 1, one off another at
// step 2, both handed in at step 3. Step 2, become the step to do, does not count step 1's half
// as its own (it would be done at once and the hand-in short of a half the quest never drops
// again): it waits for its own, which its kill drops, and the two do the hand-in. The control:
// a quest whose earlier step hands the item in counts the half held at the later step.
func TestALaterLootStepOfTheSameItemWantsItsOwn(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	ruby := f.pattern("ruby half", 1)
	drops := lootQuest(f, belt, 0.1, 0.1, 0.1, 0.1)
	r := strconv.Itoa(ruby)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (63, 'The Split Ruby', 1, 'Oligan')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (63, 1, 'loot', 'wandering forager', '` + r + `', 1, 1.0, 'A half.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (63, 2, 'loot', 'fat piranha', '` + r + `', 1, 1.0, 'The other half.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal) VALUES (63, 3, 'give', 'Oligan', '` + r + `*2', 'Both.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 63, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("wandering forager", 830)
	f.own(ruby, 1)
	f.qe.Looted(f.tw.s, f.tw.p, ruby, 1)
	if s, _ := f.progress(63); s != 2 {
		t.Fatalf("the first half taken: step %d, want 2 (waiting for its own half)", s)
	}
	*drops = nil
	f.killFor("fat piranha", 831)
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{831, f.sid, ruby}) {
		t.Fatalf("the piranha dropped %v, want the second half", *drops)
	}
	f.own(ruby, 1)
	f.qe.Looted(f.tw.s, f.tw.p, ruby, 1)
	if s, _ := f.progress(63); s != 3 {
		t.Fatalf("the second half taken: step %d, want 3", s)
	}

	// the control: the first half handed in before the second loot step
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (64, 'Halves Apart', 1, 'Oligan')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (64, 1, 'loot', 'wandering forager', '` + r + `', 1, 1.0, 'A half.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal) VALUES (64, 2, 'give', 'Oligan', '` + r + `', 'One.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (64, 3, 'loot', 'fat piranha', '` + r + `', 1, 1.0, 'Another.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (64, 4, 'talk', 'Oligan', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 64, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(64); s != 4 {
		t.Fatalf("a half held at a loot step after the hand-in: step %d, want 4 (counted at once)", s)
	}
}

// Live petition 5248: an item a loot step wants, handed over in a trade while the
// step is under way, counts as one taken off a corpse does. One belt in: 1 of 2, the step
// stays; the second in: the step is done.
func TestATradedItemCountsOnItsLootStep(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	lootQuest(f, belt)
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.own(belt, 1)
	f.qe.Received(f.tw.s, f.tw.p)
	if s, _ := f.progress(60); s != 1 {
		t.Fatalf("one belt traded in: step %d, want 1 (one of two)", s)
	}
	f.qe.Received(f.tw.s, f.tw.p) // told again with nothing new: still one of two
	if s, _ := f.progress(60); s != 1 {
		t.Fatalf("told again: step %d, want 1", s)
	}
	f.own(belt, 1)
	f.qe.Received(f.tw.s, f.tw.p)
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("both belts traded in: step %d, want 2", s)
	}
}

// The owner, 2026-11-18: "allow pre farming of quest items". prefarmQuest is quest 70: a talk
// to Malsis, a kill of a bat, a loot step of two belts off orc pawns at half a chance, a talk;
// the engine's drops recorded (as lootQuest's) and its draws taken from rolls.
func prefarmQuest(f *questFixture, belt int, rolls ...float64) *[]lootDrop {
	f.t.Helper()
	drops := lootQuest(f, f.pattern("unused", 1), rolls...) // quest 60 off "Orc Pawn" too: take it out of the game
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 60`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (70, 'Belts Ahead', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (70, 1, 'talk', 'Malsis', 'See Malsis.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (70, 2, 'kill', 'a bat', 'Kill a bat.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (70, 3, 'loot', 'Orc Pawn', '` + strconv.Itoa(belt) + `', 2, 0.5, 'Two belts.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (70, 4, 'talk', 'Malsis', 'Back.')`)
	f.qe.Reload()
	return drops
}

// On step 1, a kill of step 3's creature drops its item at the step's chance (0.7 none, 0.2 a
// belt), up to the two the step wants (two held: no draw at all); the tracker shows them
// "(ahead)"; reaching step 3 (the bat killed) counts the two at once and moves on to step 4.
func TestPreFarmALaterStepsItemCountsWhenItsStepComes(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := prefarmQuest(f, belt, 0.7, 0.2, 0.2)
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 840) // 0.7: none
	f.killFor("Orc Pawn", 841) // 0.2: a belt
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{841, f.sid, belt}) {
		t.Fatalf("on step 1 the drops are %v, want a belt ahead of step 3 on 841", *drops)
	}
	f.own(belt, 1)
	f.killFor("Orc Pawn", 842) // 0.2: the second
	f.own(belt, 1)
	f.killFor("Orc Pawn", 843) // two held of two: no draw, none
	if len(*drops) != 2 {
		t.Fatalf("drops %v, want two: no more than the step will take", *drops)
	}
	f.qe.mu.Lock()
	tracker := show(f.qe.progressLocked(sessionInvList(f.tw.s), f.tw.p, f.qe.namedLocked(f.sid, "Talker")))
	f.qe.mu.Unlock()
	if !strings.Contains(tracker, "slashed belt (ahead) 2/2") {
		t.Errorf("the tracker %q shows no belts ahead", tracker)
	}
	if s, _ := f.progress(70); s != 1 {
		t.Fatalf("the step is %d, want 1 (held items count only at their step)", s)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestSetStep, 2); err != nil {
		t.Fatal(err)
	}
	f.killFor("a bat", 844)
	if s, _ := f.progress(70); s != 4 {
		t.Fatalf("the bat killed with the two belts held: step %d, want 4 (step 3 counted at once)", s)
	}
	// the control: one belt held of two counts one, the step waits for the other
	f.exec(`DELETE FROM charinventory WHERE serverid = ?`, f.sid)
	f.own(belt, 1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(70); s != 3 {
		t.Fatalf("one of two held at step 3: step %d, want 3", s)
	}
}

// The owner, 2026-11-18: "do not require quest at all, just drops". A quest not taken drops
// its loot steps' items ahead as one under way does: quest 70's step 3, after a kill step, and
// quest 71's tusk though the quest begins at level 20 and the player is level 5. Taken with the
// tusk held, its step is done at once; finished (not repeatable), none drops again.
func TestPreFarmANotTakenQuestDropsToo(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	tusk := f.pattern("boar tusk", 20)
	drops := prefarmQuest(f, belt, 0, 0, 0, 0, 0, 0)
	f.killFor("Orc Pawn", 850)
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{850, f.sid, belt}) {
		t.Fatalf("not taken, a loot step after a kill: drops %v, want the belt", *drops)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (71, 'Tusks', 1, 'Malsis', 20)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (71, 1, 'loot', 'a boar', '` + strconv.Itoa(tusk) + `', 1, 1.0, 'A tusk.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (71, 2, 'talk', 'Malsis', 'Back.')`)
	f.qe.Reload()
	*drops = nil
	f.killFor("a boar", 852)
	if len(*drops) != 1 || (*drops)[0].pattern != tusk {
		t.Fatalf("level 5, a quest from level 20, not taken: drops %v, want the tusk", *drops)
	}
	f.own(tusk, 1)
	*drops = nil
	f.killFor("a boar", 853) // one held of one: none
	if len(*drops) != 0 {
		t.Fatalf("the tusk held: dropped %v", *drops)
	}
	f.tw.p.Level = 20
	if _, err := f.qe.SetCharacterQuest(f.sid, 71, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(71); s != 2 {
		t.Fatalf("taken with the tusk held: step %d, want 2", s)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 71, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.exec(`DELETE FROM charinventory WHERE serverid = ?`, f.sid)
	f.tw.s.SetInventoryList(newInvList(nil))
	f.killFor("a boar", 854)
	if len(*drops) != 0 {
		t.Fatalf("finished, not repeatable: dropped %v", *drops)
	}
}

// The exclusions (questpreloot.go's doc), each against a control that drops: a loot step whose
// doing fires a trigger; a key an event reads (Tallon's Key); a creature the quest brings (the
// mirrored soul, spawned by step 3: none on step 1, its item on step 3's own kill for step 4);
// an item an earlier loot step still to do wants too (the Spider Queen's shards).
func TestPreFarmExclusions(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	key := f.pattern("Tallon's Key", 1)
	scepter, orb := f.pattern("Scepter of Reflection", 1), f.pattern("mirror orb", 1)
	shard := f.pattern("Crystal Spider Shard", 20)
	ember := f.pattern("ember", 20)
	drops := prefarmQuest(f, belt, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (72, 'Ahead Or Not', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (72, 1, 'talk', 'Malsis', 'See Malsis.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (72, 2, 'kill', 'a bat', 'A bat.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal, events)
		VALUES (72, 3, 'loot', 'mirrored soul', '` + strconv.Itoa(orb) + `', 1, 1.0, 'The orb.',
		'[{"on":"start","do":"spawn","npc":"mirrored soul","hostile":true}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (72, 4, 'loot', 'mirrored soul', '` + strconv.Itoa(scepter) + `', 1, 1.0, 'The scepter.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal, events)
		VALUES (72, 5, 'loot', 'fire beetle', '` + strconv.Itoa(ember) + `', 1, 1.0, 'An ember.',
		'[{"on":"done","do":"trigger","trigger":"Fire Lord"}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (72, 6, 'loot', 'Zek Sergeant', '` + strconv.Itoa(key) + `', 1, 1.0, 'The key.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (72, 7, 'loot', 'Grosp', '` + strconv.Itoa(shard) + `', 1, 1.0, 'Grosp''s shard.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (72, 8, 'loot', 'Fengzel', '` + strconv.Itoa(shard) + `', 1, 1.0, 'Fengzel''s shard.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (72, 9, 'talk', 'Malsis', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 72, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"mirrored soul", "fire beetle", "Zek Sergeant", "Fengzel"} {
		f.killFor(name, entity.EntityID(860+i))
		if len(*drops) != 0 {
			t.Fatalf("on step 1, %s dropped %v ahead: excluded", name, *drops)
		}
	}
	f.killFor("Grosp", 865) // the control: the first of the shards' steps drops ahead
	if len(*drops) != 1 || (*drops)[0].pattern != shard {
		t.Fatalf("on step 1, Grosp dropped %v, want his shard ahead", *drops)
	}
	// on the step that brings the mirrored soul: its own kill drops the next step's scepter too
	*drops = nil
	if _, err := f.qe.SetCharacterQuest(f.sid, 72, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	f.killFor("mirrored soul", 866)
	if len(*drops) != 2 || (*drops)[0].pattern != orb || (*drops)[1].pattern != scepter {
		t.Fatalf("on step 3, the mirrored soul dropped %v, want the orb and the scepter", *drops)
	}
	// past Grosp's step, Fengzel's shard drops ahead (Grosp's held, kept for his step)
	*drops = nil
	f.exec(`DELETE FROM charinventory WHERE serverid = ?`, f.sid)
	f.own(shard, 1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 72, QuestSetStep, 7); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(72); s != 8 {
		t.Fatalf("Grosp's shard held at step 7: step %d, want 8", s)
	}
	f.killFor("Fengzel", 867)
	if len(*drops) != 1 || (*drops)[0].pattern != shard {
		t.Fatalf("on step 8, Fengzel dropped %v, want his shard", *drops)
	}
}

// A shared kill drops each player its own belt, at its own draw, on its own view of the corpse,
// the quest not asked (the owner's "just drops"): the first on the quest, the second without it.
func TestPreFarmASharedKillDropsEachQuesterItsOwn(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := prefarmQuest(f, belt, 0.2, 0.7, 0.1)
	rec := &database.CharacterRecord{CharName: "Second", AccountID: 2}
	if err := database.NewSQLiteCharacterStore(f.db).Create(rec); err != nil {
		t.Fatal(err)
	}
	s2 := f.tw.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7010}, 10, 2, 3, 4, true)
	p2 := &entity.Player{ServerID: 1010, Name: "Second", Level: 5, Class: 10, CharServerID: rec.ServerID}
	f.tw.w.Players[p2.ServerID] = p2
	s2.SetECSEntity(p2)
	s2.SetCharacterInWorld(true)
	s2.SetInventoryList(newInvList(nil))
	both := func(npc entity.EntityID) {
		f.qe.killed(entity.KillCredit{NPCName: "Orc Pawn", NPCSpawn: npc, NPCID: npc, Died: time.Now(),
			Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}, {Session: s2, ServerID: rec.ServerID, Name: "Second"}}})
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	both(870) // 0.2 for the first: its belt; 0.7 for the second: none
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{870, f.sid, belt}) {
		t.Fatalf("drops %v, want the first's belt alone", *drops)
	}
	*drops = nil
	both(871) // 0.1 for the first: a belt; the second's draw the default 0.99: none
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{871, f.sid, belt}) {
		t.Fatalf("drops %v, want the first's second belt", *drops)
	}
	f.qe.roll = func() float64 { return 0 }
	*drops = nil
	both(872) // every draw 0: a belt each, the second without the quest too
	got := map[uint32]bool{}
	for _, d := range *drops {
		got[d.char] = true
		if d.npc != 872 || d.pattern != belt {
			t.Fatalf("drop %v", d)
		}
	}
	if len(*drops) != 2 || !got[f.sid] || !got[rec.ServerID] {
		t.Fatalf("drops %v, want one belt each", *drops)
	}
}
