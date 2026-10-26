package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lootDrop is one drop the engine handed the corpse windows.
type lootDrop struct {
	npc     entity.EntityID
	char    uint32
	pattern int
}

// lootQuest is a loot step (two of an item off "orc pawn" at half a chance)
// then a talk to Malsis, given to the fixture's character; the engine's drops
// recorded and its draws taken in turn from rolls.
func lootQuest(f *questFixture, item int, rolls ...float64) *[]lootDrop {
	f.t.Helper()
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (60, 'Belts', 1, 'Malsis')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (60, 1, 'loot', 'Orc Pawn', '%d', 2, 0.5, 'Bring back two belts.')`, item))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (60, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.qe.Reload()
	var drops []lootDrop
	f.qe.drop = func(npc entity.EntityID, _ time.Time, char uint32, rec database.CharInventoryEntry) {
		drops = append(drops, lootDrop{npc, char, rec.Pattern.PatternID})
	}
	f.qe.roll = func() float64 {
		if len(rolls) == 0 {
			return 0.99
		}
		r := rolls[0]
		rolls = rolls[1:]
		return r
	}
	return &drops
}

// killFor is a kill of name credited to the fixture's character.
func (f *questFixture) killFor(name string, npc entity.EntityID) {
	f.qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: npc, NPCID: npc, Died: time.Now(),
		Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
}

// A loot step drops its item for the player on it, at the step's chance and
// only off its creature; a kill credited to nobody on it drops nothing. Taking
// the items counts them, and the last one needed does the step.
func TestALootStepDropsItsItemAtItsChance(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := lootQuest(f, belt, 0.7, 0.2, 0.1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	f.killFor("a bat", 801)
	if len(*drops) != 0 {
		t.Fatalf("a bat dropped a belt: %v", *drops)
	}
	f.killFor("Orc Pawn", 802) // 0.7: no drop at 0.5
	if len(*drops) != 0 {
		t.Fatalf("a draw above the chance dropped: %v", *drops)
	}
	f.killFor("orc pawn", 803) // 0.2: a drop
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{803, f.sid, belt}) {
		t.Fatalf("the drops: %v, want the belt on 803 for the character", *drops)
	}
	// the control: a kill credited to another character drops nothing for it
	f.qe.killed(entity.KillCredit{NPCName: "Orc Pawn", NPCID: 804, Died: time.Now(),
		Players: []entity.KillPlayer{{ServerID: f.sid + 50, Name: "Other"}}})
	if len(*drops) != 1 {
		t.Fatalf("a character not on the step got a drop: %v", *drops)
	}

	// taken: the count said; the second does the step
	f.qe.Looted(f.tw.s, f.tw.p, belt, 1)
	f.want(f.msgs(), "chat:slashed belt: 1 of 2.")
	f.qe.Looted(f.tw.s, f.tw.p, f.bone, 1) // another item: nothing
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("another item counted: %q", m)
	}
	f.killFor("Orc Pawn", 806) // 0.1: one of two taken, so another drops
	if len(*drops) != 2 {
		t.Fatalf("one of two taken, the next kill dropped nothing: %v", *drops)
	}
	f.qe.Looted(f.tw.s, f.tw.p, belt, 1)
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("two belts held, the step is %d", s)
	}
	// the step done, no more drop
	f.killFor("Orc Pawn", 805)
	if len(*drops) != 2 {
		t.Errorf("a drop after the step: %v", *drops)
	}
}

// A loot step counts what is taken while it is the step to do: items held
// from before do not do it, nor one taken before the quest; two taken on the
// step at once (a stack) do. The count outlives the engine.
func TestALootStepCountsWhatIsTakenOnIt(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	lootQuest(f, belt)
	f.own(belt, 2)
	f.qe.Looted(f.tw.s, f.tw.p, belt, 1) // before the quest: not counted
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.progress(60); s != 1 {
		t.Fatalf("two belts held before it, the step is %d, want 1", s)
	}
	f.qe.Looted(f.tw.s, f.tw.p, belt, 1)
	f.msgs()
	again, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, f.inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	again.items = f.qe.items
	if got := again.CharacterQuests(f.sid); len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("after a restart: %+v, want one taken", got)
	}
	f.qe.Looted(f.tw.s, f.tw.p, belt, 2)
	if s, _ := f.progress(60); s != 2 {
		t.Fatalf("three taken on the step, it is %d, want 2", s)
	}
}

// The events in their places and order: the quest's on taking it after the
// giver's words, its first step's start after them; the step's done after its
// words and rewards, then the quest's finish. A popup is a box with one
// answer, which changes nothing.
func TestQuestEventsInTheirOrder(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(fmt.Sprintf(`INSERT INTO quests (questid, name, enabled, giver, accept, started, events)
		VALUES (70, 'Told', 1, 'Malsis', 'Yes.', 'Go on.', '[{"on":"accept","do":"popup","text":"Welcome, %%p."},
			{"on":"accept","do":"say","npc":"Malsis","text":"Hurry."},{"on":"finish","do":"say","text":"The end."}]')`))
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, says, events)
		VALUES (70, 1, 'talk', 'Kellina', 'See Kellina.', 'Hello.', '[{"on":"start","do":"give","items":"%d"},
			{"on":"done","do":"popup","text":"Well done.","answer":"Thanks"}]')`, f.aura))
	f.qe.Reload()
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})

	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	m := f.msgs()
	at := func(want string) int {
		for i, x := range m {
			if x == want || (strings.HasSuffix(want, "*") && strings.HasPrefix(x, strings.TrimSuffix(want, "*"))) {
				return i
			}
		}
		t.Fatalf("no %q in %q", want, m)
		return -1
	}
	order := []int{at("box:Malsis: Go on."), at("ask#*"), at("box:Malsis: Hurry."), at("box:You have received Smoldering Aura.")}
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Fatalf("taken, out of order: %q", m)
		}
	}
	if !hasMsg(m, fmt.Sprintf("ask#%d:Welcome, Talker.|OK", boxID(m))) {
		t.Fatalf("the popup is not one answer with the player's name: %q", m)
	}
	if f.held(f.aura) != 1 {
		t.Fatalf("the start's give: %d auras", f.held(f.aura))
	}
	f.tw.pick(boxID(m), 0) // OK: nothing changes
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("the popup's answer did something: %q", m)
	}

	f.tw.talk(kellina)
	m = f.msgs()
	done, pop, end := at("box:"+questFinished), at("ask#*"), at("box:The end.")
	if !(at("box:Kellina: Hello.") < done && done < pop && pop < end) {
		t.Fatalf("done, out of order: %q", m)
	}
	if !hasMsg(m, fmt.Sprintf("ask#%d:Well done.|Thanks", boxID(m))) {
		t.Fatalf("the done popup and its answer: %q", m)
	}
}

// A step's spawn is a copy of its NPC before the player for the step: hostile,
// attackable and at the player; gone when the step is done, or when its
// character is away. The control: a spawn not hostile keeps its NPC bit and
// fights no one.
func TestASpawnIsTheSteps(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // alive, to be attacked
	f.tw.add(5, &entity.NPC{ServerID: 706, Name: "Delvin", NPCType: 0x80, MaxHP: 100, SpawnX: 26000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (80, 'Delvin', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (80, 1, 'kill', 'Delvin', 'Kill Delvin.',
		'[{"on":"start","do":"spawn","npc":"Delvin","hostile":true}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (80, 2, 'talk', 'Malsis', 'Return to Malsis.',
		'[{"on":"start","do":"spawn","npc":"Delvin"}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (80, 3, 'talk', 'Malsis', 'Again.')`)
	f.qe.Reload()
	copies := func() []*entity.NPC {
		var out []*entity.NPC
		for _, n := range f.tw.w.NPCs {
			if n.QuestSpawn {
				out = append(out, n)
			}
		}
		return out
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 80, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	cs := copies()
	if len(cs) != 1 {
		t.Fatalf("%d copies, want one", len(cs))
	}
	c := cs[0]
	if d := entity.DistanceBetween(f.tw.p, c); d > 5 || !c.Temporary || c.NPCType&0x80 != 0 || c.CurrentEnemy != f.tw.p.ServerID {
		t.Fatalf("the hostile copy: %.1f off, temporary %v, type %#x, enemy %d", d, c.Temporary, c.NPCType, c.CurrentEnemy)
	}
	f.killFor("Delvin", 0)
	if s, _ := f.progress(80); s != 2 {
		t.Fatalf("Delvin killed, the step is %d", s)
	}
	cs = copies()
	if len(cs) != 1 || cs[0] == c {
		t.Fatalf("after the step: %d copies, the first gone %v", len(cs), len(cs) == 0 || cs[0] != c)
	}
	if n := cs[0]; n.NPCType&0x80 == 0 || n.CurrentEnemy != 0 {
		t.Fatalf("the friendly copy: type %#x, enemy %d", n.NPCType, n.CurrentEnemy)
	}
	// away: the character's session gone, its spawn goes
	f.qe.sessionOf = func(uint32) *session.Session { return nil }
	f.qe.expireSpawns()
	if len(copies()) != 0 {
		t.Fatal("a spawn outlived its character's leaving")
	}
}

// A visual is a cast's 0x00B1 to the player; a teleport is a travel NPC's warp.
func TestAVisualAndATeleport(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.s.SetPlayerObjectID(500000001) // the id a cast names its player by
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (90, 'Away', 1, 'Malsis',
		'[{"on":"accept","do":"visual","fx":"0x1DAEB5B6"},{"on":"accept","do":"teleport","world":0,"x":25500,"y":60,"z":15800}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (90, 1, 'talk', 'Malsis', 'Talk.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	m := f.msgs()
	f.want(m, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeCastingCombat)), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	if f.tw.p.X != 25500 || f.tw.p.Z != 15800 {
		t.Errorf("after the teleport the player stands at %.0f, %.0f", f.tw.p.X, f.tw.p.Z)
	}
}
