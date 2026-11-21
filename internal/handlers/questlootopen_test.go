package handlers

import (
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The owner, 2026-11-21: "players dont need to be on a quest to obtain a quest drop. also no
// instanced quest items for each character, so all 4 players dont need to loot the corpse it
// slows things down too much" (questpreloot.go, questloot.go, lootshare.go).

// A quest's creature killed by a character with no quest at all drops the step's item on its
// corpse at the step's chance; a kill no player is credited with drops nothing.
func TestAQuestCreatureDropsItsItemWithNoOneOnTheQuest(t *testing.T) {
	f := newQuestFixture(t)
	blood := f.pattern("blood of the desert madman", 1)
	drops := lootQuest(f, blood, 0.4, 0.6)
	stranger := entity.KillPlayer{ServerID: f.sid + 500, Name: "Stranger"}
	f.qe.killed(entity.KillCredit{NPCName: "orc pawn", NPCSpawn: 990, NPCID: 990, Died: time.Now(), Players: []entity.KillPlayer{stranger}})
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{990, true, blood}) {
		t.Fatalf("no one on the quest, a draw of 0.4 at 0.5: drops %v, want the blood on 990", *drops)
	}
	f.qe.killed(entity.KillCredit{NPCName: "orc pawn", NPCSpawn: 991, NPCID: 991, Died: time.Now(), Players: []entity.KillPlayer{stranger}})
	if len(*drops) != 1 {
		t.Fatalf("a draw of 0.6 at 0.5 dropped: %v", *drops)
	}
	f.qe.roll = func() float64 { return 0 }
	f.qe.killed(entity.KillCredit{NPCName: "orc pawn", NPCSpawn: 992, NPCID: 992, Died: time.Now()})
	if len(*drops) != 1 {
		t.Fatalf("a kill no player is credited with dropped: %v", *drops)
	}
}

// A group of four kills the creature: ONE item goes on the corpse at ONE draw, whoever of them
// is on the quest (one here, on the step: the drop keeps the corpse). Two quests naming the
// same creature and item (chances 0.3 and 0.6) roll it once, at the higher: a draw of 0.5
// drops it, one of 0.7 does not, and each kill takes one draw.
func TestAGroupOfFourGetsOneQuestItem(t *testing.T) {
	f := newQuestFixture(t)
	blood := f.pattern("blood of the desert madman", 1)
	drops := lootQuest(f, blood, 0.5, 0.7, 0.1)
	f.exec(`UPDATE queststeps SET chance = 0.3 WHERE questid = 60 AND step = 1`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (61, 'Research', 1, 'Ilenar')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (61, 1, 'loot', 'Orc Pawn', '` + strconv.Itoa(blood) + `', 1, 0.6, 'A madman''s blood.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (61, 2, 'talk', 'Ilenar', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	four := []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"},
		{ServerID: f.sid + 1, Name: "B"}, {ServerID: f.sid + 2, Name: "C"}, {ServerID: f.sid + 3, Name: "D"}}
	kill := func(npc entity.EntityID) {
		f.qe.killed(entity.KillCredit{NPCName: "Orc Pawn", NPCSpawn: npc, NPCID: npc, Died: time.Now(), Players: four})
	}
	kill(993) // 0.5 under 0.6: one blood
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{993, false, blood}) {
		t.Fatalf("four killed it: drops %v, want ONE blood on 993", *drops)
	}
	kill(994) // 0.7: none
	if len(*drops) != 1 {
		t.Fatalf("a draw of 0.7 dropped: %v", *drops)
	}
	kill(995) // 0.1: one, the third draw (one a kill)
	if len(*drops) != 2 || (*drops)[1] != (lootDrop{995, false, blood}) {
		t.Fatalf("the third kill: drops %v, want one blood on 995", *drops)
	}
}

// questPattern adds an item that lies on no creature (a quest's drop), and is its id.
func (r *raidLootRig) questPattern(name string, stack, trade int) int {
	r.t.Helper()
	res, err := r.f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, ?, 1, 0, 0, 0, 0, 0, ?, 0, 0, 32767, 0, 0, 0, 0, 0, ?, 'a quest piece', 0, 0)`, trade, stack, name)
	if err != nil {
		r.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	r.pat[name] = int(id)
	return int(id)
}

// The one blood on a group of four's corpse: every member's window lists it, the first to
// take it has it, and then no one else sees it or holds one; the corpse goes with it.
func TestAQuestItemOnACorpseIsLootedOnce(t *testing.T) {
	r := newRaidLootRig(t, 4)
	blood := r.questPattern("blood of the desert madman", 1, 1)
	r.f.h.heardKillRight(entity.KillCredit{NPCID: r.f.npc.ServerID, Died: r.f.npc.DeathTime, Players: []entity.KillPlayer{
		{ServerID: r.p[0].CharServerID}, {ServerID: r.p[1].CharServerID}, {ServerID: r.p[2].CharServerID}, {ServerID: r.p[3].CharServerID}}})
	r.f.h.AddCorpseQuestDrop(r.f.npc.ServerID, r.f.npc.DeathTime, r.f.dropOf(blood), false)

	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	for i := range r.p {
		if got := r.view(i)["blood of the desert madman"]; got != 1 {
			t.Fatalf("%s's view lists %d bloods, want the one", r.p[i].Name, got)
		}
	}
	r.lootCall(1, protocol.OpcodeLootTakeAll, nil)
	r.lootCall(1, protocol.OpcodeClientCloseLoot, nil)
	for i := range r.p {
		want := 0
		if i == 1 {
			want = 1
		}
		if got := r.owns(i, "blood of the desert madman"); got != want {
			t.Fatalf("%s holds %d bloods, want %d", r.p[i].Name, got, want)
		}
		if got := r.view(i)["blood of the desert madman"]; got != 0 {
			t.Fatalf("%s's view still lists %d bloods after it was taken", r.p[i].Name, got)
		}
	}
	if len(r.f.cleared) != 1 {
		t.Fatalf("the one item taken, the corpse did not go: cleared %v", r.f.cleared)
	}
}

// A drop with no player of the kill on its step keeps the corpse only aheadKeep, as a drop
// ahead did; one a player of the kill is on the step for keeps it as any item; the same item
// added both ways is one stack that keeps it.
func TestACorpseQuestDropAheadKeepsTheCorpseAWhile(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.h.AddCorpseQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.dropOf(f.bait), true)
	ls := f.h.loots()
	c := &corpse{died: f.npc.DeathTime}
	later := f.npc.DeathTime.Add(aheadKeep + time.Second)
	if ls.emptyLocked(f.npc.ServerID, c, f.npc.DeathTime) || !ls.emptyLocked(f.npc.ServerID, c, later) {
		t.Fatal("a drop with no one on its step: want it to keep the corpse until aheadKeep, then not")
	}
	f.h.AddCorpseQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.dropOf(f.bait), false)
	if ls.emptyLocked(f.npc.ServerID, c, later) {
		t.Fatal("the same item dropped for one on its step: the stack keeps the corpse no longer")
	}
	if got := ls.dropsLocked(f.npc.ServerID, c, f.charSID); len(got) != 1 || got[0].StackLeft != 2 {
		t.Fatalf("the corpse's drops %v, want one stack of two", got)
	}
}

// A quest item's No Trade stack on a corpse (an npcitem row) is one record, one looter's; a
// No Trade stack no quest names is shared one to a character as before (lootshare.go), and so
// is a key an event reads, a quest's or not.
func TestAQuestNoTradeStackIsNotShared(t *testing.T) {
	r := newRaidLootRig(t, 4)
	quest := r.item("madman's eye", 8, 0, 1, 0, 0, 1) // no trade, 4 on the corpse
	r.item("Jeweled Fang", 8, 0, 1, 0, 0, 1)          // no trade, no quest's
	key := r.skyKey("Rain Key 3", 20)
	r.f.h.questItem = func(p int) bool { return p == quest || p == key }
	r.kill(entity.LootNeedGreed)
	c := r.corpse()
	shared := map[string]bool{}
	for _, s := range c.shares {
		shared[s.rec.Pattern.ItemName] = true
	}
	if shared["madman's eye"] || !shared["Jeweled Fang"] || !shared["Rain Key 3"] {
		t.Fatalf("shares %v: want the fang and the key shared, the quest's eyes not", shared)
	}
	for i := range r.p {
		if got := r.view(i)["madman's eye"]; got != 4 {
			t.Fatalf("%s's view lists %d eyes, want the one stack of 4", r.p[i].Name, got)
		}
	}
	if !sharesItem(&database.CharInventoryEntry{StackLeft: 2, Pattern: database.ItemPatternRecord{PatternID: quest, Trade: 1}}, nil) {
		t.Error("with no quests known, a No Trade stack is not shared")
	}
}
