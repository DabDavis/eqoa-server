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

// What jib's quest walk of 2026-10-30 found in the engine: a quest asking one
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

// A giver whose quests are all repeatable offers its first, then, that one
// finished, the next it has never finished, and once every one is finished
// the one finished fewest times: the player goes through them all. Before,
// the first was offered for ever (Chef Lukkif's eleven). A never-finished
// quest of the giver comes before a finished repeatable one whatever their
// ids (the control: 82 comes before 81 once 81 is finished).
func TestARepeatableGiverGoesOnThroughItsQuests(t *testing.T) {
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
	round := func() string {
		t.Helper()
		f.tw.talk(chef) // taken at once: no answer to take it
		m := f.msgs()
		var line string
		for _, s := range m {
			if strings.HasPrefix(s, "add:Cook dish") {
				line = strings.TrimSuffix(strings.TrimPrefix(s, "add:Cook dish "), ".")
			}
		}
		if line == "" {
			t.Fatalf("no dish offered: %q", m)
		}
		f.tw.talk(chef) // its one step, at the chef: done
		f.want(f.msgs(), "box:"+questFinished)
		return line
	}
	var got []string
	for i := 0; i < 5; i++ {
		got = append(got, round())
	}
	if strings.Join(got, " ") != "81 82 83 81 82" {
		t.Fatalf("the dishes in turn: %q, want 81 82 83 81 82", got)
	}
}

// A loot step whose item the player holds a full stack of counts each drop
// at the kill (CLIENT keeps no more past a full first record, opening no
// second), and is done at its count; the controls: a stack with room takes
// the drop onto the corpse as ever, and an item that does not stack does too.
func TestALootStepCountsWhatAFullStackCannotTake(t *testing.T) {
	f := newQuestFixture(t)
	wand := f.pattern("froglok summoner's wand", 2)
	drops := lootQuest(f, wand, 0.1, 0.1, 0.1, 0.1)
	f.own(wand, 2) // a full stack, held from before
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.killFor("Orc Pawn", 811)
	if len(*drops) != 0 {
		t.Fatalf("a drop a full stack cannot take went on the corpse: %v", *drops)
	}
	f.want(f.msgs(), "chat:You can carry no more froglok summoner's wand: this one counts.", "chat:froglok summoner's wand: 1 of 2.")
	f.killFor("Orc Pawn", 812)
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("two counted, the step is %d", s)
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
