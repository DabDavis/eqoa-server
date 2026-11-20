package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What a GM's quest walk of 2026-10-30 found in the engine: a quest asking one
// of several quests finished (a recruiter's token each class gets from its
// own), a shop's offer once a session, a giver of repeatable quests going on
// through them, and a loot step whose item the player holds a full stack of.
// The words and numbers are the tests' own.

// A quest that requires "71|72" is offered once either is finished, and not
// before; "71, 72" (the control) still asks both, and a group beside a plain
// quest asks that one too.
func TestAQuestRequiringOneOfAGroup(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	ruddish := f.tw.add(1, &entity.NPC{ServerID: 731, Name: "Ruddish", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	for _, id := range []int{71, 72, 73} {
		f.exec(fmt.Sprintf(`INSERT INTO quests (questid, name, enabled, giver) VALUES (%d, 'Recruiter %d', 1, 'Nobody')`, id, id))
		f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (%d, 1, 'talk', 'Nobody', 'x')`, id))
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (75, 'Wands', 1, 'Ruddish', '71|72')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (75, 1, 'talk', 'Kellina', 'Wands for Kellina.')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (76, 'Both', 1, 'Ruddish', '71, 72')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (76, 1, 'talk', 'Kellina', 'Both for Kellina.')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (77, 'Group and one', 1, 'Ruddish', '73, 71|72')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (77, 1, 'talk', 'Kellina', 'Third for Kellina.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	offered := func() []string {
		t.Helper()
		var out []string
		f.qe.mu.Lock()
		c := f.qe.charLocked(f.sid)
		for _, id := range []int{75, 76, 77} {
			if f.qe.mayTakeLocked(f.qe.lookAt(f.tw.s), f.tw.p, c, f.qe.cat.byID[id]) == "" {
				out = append(out, f.qe.cat.byID[id].Name)
			}
		}
		f.qe.mu.Unlock()
		return out
	}

	if got := offered(); len(got) != 0 {
		t.Fatalf("with neither finished: %q", got)
	}
	f.tw.talk(ruddish)
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("Ruddish offered a quest with neither finished: %q", m)
	}
	f.finish(72)
	if got := offered(); strings.Join(got, "/") != "Wands" {
		t.Fatalf("72 finished: %q, want Wands alone (Both asks 71 too, Group and one 73)", got)
	}
	f.tw.talk(ruddish)
	f.want(f.msgs(), "add:Wands for Kellina.")
	f.finish(71)
	f.finish(73)
	if got := offered(); strings.Join(got, "/") != "Both/Group and one" {
		t.Fatalf("71, 72 and 73 finished, Wands under way: %q", got)
	}
}

// openShop is a merchant's window asked for (0x004C) through a window
// handler that knows the one NPC: the messages it queued.
func openShop(f *questFixture, npc *entity.NPC, id uint32) []string {
	f.t.Helper()
	wh := NewNPCWindowHandler(f.tw.mgr, f.inv)
	wh.playerOf = func(*session.Session) *entity.Player { return f.tw.p }
	wh.find = func(_ *session.Session, oid uint32) (entity.Entity, bool) { return npc, oid == id }
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeMerchantDiag)
	msg.SetData(build(protocol.OpcodeMerchantDiag, 8, func(w *buffer.Writer) { w.WriteUint32(id) })[2:])
	if err := wh.HandleMerchantOpen(f.tw.s, msg); err != nil {
		f.t.Fatal(err)
	}
	return f.msgs()
}

// A shop's quest offered and given up, or offered and left, is offered again
// at the character's next login, and at once when given up; the control: in
// the same session, left unanswered, the next request opens the shop.
func TestAShopOffersAQuestOnceASession(t *testing.T) {
	f := newQuestFixture(t)
	ulla := &entity.NPC{ServerID: 741, Name: "Spellsmith Ulla", NPCID: 741, NPCType: 0x81, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726}
	id := f.tw.add(7, ulla)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline)
		VALUES (97, 'The Orb', 1, 'Spellsmith Ulla', 'Spellsmith Ulla: Fetch my orb?', 'I will.', 'Not now.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (97, 1, 'talk', 'Malsis', 'Ask Malsis about the orb.')`)
	f.qe.Reload()
	f.msgs()
	shop := fmt.Sprintf("op:%04X", uint16(protocol.OpcodeMerchantBox))
	isOffer := func(m []string) bool { return strings.Contains(strings.Join(m, "\n"), "Fetch my orb?") }

	if m := openShop(f, ulla, id); !isOffer(m) {
		t.Fatalf("the first request: %q, want the offer", m)
	}
	// the control: left unanswered, the next request this session is the shop
	f.want(openShop(f, ulla, id), shop)
	// a new login: offered again
	f.qe.LoggedIn(f.sid)
	m := openShop(f, ulla, id)
	if !isOffer(m) {
		t.Fatalf("after a login: %q, want the offer again", m)
	}
	f.tw.pick(boxID(m), 0)
	f.want(f.msgs(), "add:Ask Malsis about the orb.")
	// given up (the journal's delete): offered again in this session
	token, _ := f.journal()
	drop := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDeleteQuest)
	drop.SetData(build(protocol.OpcodeDeleteQuest, 16, func(w *buffer.Writer) {
		w.WriteUint32(token)
		w.Write7BitEncodedInt64(0)
	})[2:])
	if err := f.qe.HandleDrop(f.tw.s, drop); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	if m := openShop(f, ulla, id); !isOffer(m) {
		t.Fatalf("given up: %q, want the offer again", m)
	}
	f.want(openShop(f, ulla, id), shop)
}

// A giver whose quests are all repeatable asks which the player will do
// (petition 2388, Chef Lukkif's Hunt and Gather: until then each talk took the
// next one, in turn, filling the journal): every one it may take now, by name;
// the one picked is taken (no answer to take it here), its step done at the
// chef, and the board comes again with all three. The controls: one under way
// is left off the board; a giver with a quest that is not repeatable offers
// as before (no board).
func TestARepeatableGiverAsksWhichQuest(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	chef := f.tw.add(1, &entity.NPC{ServerID: 751, Name: "Chef Lukkif", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	for _, id := range []int{81, 82, 83} {
		f.exec(fmt.Sprintf(`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (%d, 'Dish %d', 1, 'Chef Lukkif', 1)`, id, id))
		f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, rewardtunar) VALUES (%d, 1, 'talk', 'Chef Lukkif', 'Cook dish %d.', 1)`, id, id))
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	board := func(want string) []string {
		t.Helper()
		f.tw.talk(chef)
		m := f.msgs()
		if len(m) != 1 || !strings.HasSuffix(m[0], ":"+questAskBoard+"|"+want) {
			t.Fatalf("the board: %q, want %q", m, want)
		}
		return m
	}
	f.tw.pick(boxID(board("Dish 81|Dish 82|Dish 83")), 1)
	f.want(f.msgs(), "add:Cook dish 82.")
	f.tw.talk(chef) // its one step, at the chef: done
	f.want(f.msgs(), "box:"+questFinished)
	f.tw.pick(boxID(board("Dish 81|Dish 82|Dish 83")), 2)
	f.want(f.msgs(), "add:Cook dish 83.")
	// 83 under way: its step comes first at the chef, so take 81 with it
	// under way by asking the board's own quests (the control)
	f.qe.mu.Lock()
	list := f.qe.boardLocked(nil, f.tw.p, f.qe.charLocked(f.sid), "Chef Lukkif")
	f.qe.mu.Unlock()
	if len(list) != 2 || list[0].ID != 81 || list[1].ID != 82 {
		t.Fatalf("the board with 83 under way: %v", list)
	}
	f.tw.talk(chef)
	f.want(f.msgs(), "box:"+questFinished)
	// a chain's quest at the giver: no board, the old offer
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (84, 'Dish 84', 1, 'Chef Lukkif', 0)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, rewardtunar) VALUES (84, 1, 'talk', 'Chef Lukkif', 'Cook dish 84.', 1)`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(chef)
	for _, s := range f.msgs() {
		if strings.Contains(s, questAskBoard) {
			t.Fatalf("a board at a giver of a chain's quest: %q", s)
		}
	}
}

// A board of more quests than a box holds goes in pages: five names and
// "Something else." in the PS2's six, the next page the rest, and past the
// last the first again.
func TestARepeatableGiversBoardTurnsItsPages(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	chef := f.tw.add(1, &entity.NPC{ServerID: 751, Name: "Chef Lukkif", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	for id := 81; id <= 88; id++ {
		f.exec(fmt.Sprintf(`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (%d, 'Dish %d', 1, 'Chef Lukkif', 1)`, id, id))
		f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, rewardtunar) VALUES (%d, 1, 'talk', 'Chef Lukkif', 'Cook dish %d.', 1)`, id, id))
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(chef)
	m := f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], ":"+questAskBoard+"|Dish 81|Dish 82|Dish 83|Dish 84|Dish 85|"+questBoardMore) {
		t.Fatalf("the first page: %q", m)
	}
	f.tw.pick(boxID(m), 5)
	m = f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], ":"+questAskBoard+"|Dish 86|Dish 87|Dish 88|"+questBoardMore) {
		t.Fatalf("the second page: %q", m)
	}
	f.tw.pick(boxID(m), 3)
	m = f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], "|Dish 81|Dish 82|Dish 83|Dish 84|Dish 85|"+questBoardMore) {
		t.Fatalf("past the last, the first again: %q", m)
	}
	f.tw.pick(boxID(m), 4)
	f.want(f.msgs(), "add:Cook dish 85.")
}

// A loot step whose item the player holds a full stack of counts each drop
// at the kill (CLIENT keeps no more past a full first record, opening no
// second), and is done at its count; the controls: a stack with room takes
// the drop onto the corpse as ever, and an item that does not stack does too.
// The full stack of two held from before counts two of the step's three when
// it is taken (questpreloot.go).
func TestALootStepCountsWhatAFullStackCannotTake(t *testing.T) {
	f := newQuestFixture(t)
	wand := f.pattern("froglok summoner's wand", 2)
	drops := lootQuest(f, wand, 0.1, 0.1, 0.1, 0.1)
	f.exec(`UPDATE queststeps SET count = 3 WHERE questid = 60 AND step = 1`)
	f.qe.Reload()
	f.own(wand, 2) // a full stack, held from before
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), "chat:froglok summoner's wand: 2 of 3.")
	f.killFor("Orc Pawn", 811)
	if len(*drops) != 0 {
		t.Fatalf("a drop a full stack cannot take went on the corpse: %v", *drops)
	}
	f.want(f.msgs(), "chat:You can carry no more froglok summoner's wand: this one counts.", "drop:0", "add:Return to Malsis.")
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("three counted, the step is %d", s)
	}
	if f.held(wand) != 2 {
		t.Errorf("the stack held: %d", f.held(wand))
	}
}

func TestALootStepDropsOntoTheCorpseWhileTheStackHasRoom(t *testing.T) {
	f := newQuestFixture(t)
	wand := f.pattern("froglok summoner's wand", 2)
	drops := lootQuest(f, wand, 0.1, 0.1)
	f.own(wand, 1) // room for one more
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.killFor("Orc Pawn", 821)
	if len(*drops) != 1 {
		t.Fatalf("a stack with room: the drops %v, want one on the corpse", *drops)
	}
	if s, _ := f.progress(60); s != 1 {
		t.Fatalf("a drop on the corpse is not taken yet: the step is %d", s)
	}
	f.not(f.msgs(), "chat:You can carry no more*")

	// an item that does not stack is a record each: never a full stack
	g := newQuestFixture(t)
	box := g.pattern("locked box", 1)
	boxes := lootQuest(g, box, 0.1)
	g.own(box, 1)
	if _, err := g.qe.SetCharacterQuest(g.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	g.killFor("Orc Pawn", 822)
	if len(*boxes) != 1 {
		t.Fatalf("an item that does not stack: the drops %v, want one on the corpse", *boxes)
	}
}
