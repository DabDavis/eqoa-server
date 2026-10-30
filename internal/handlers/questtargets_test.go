package handlers

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A kill or loot step counts several creatures (queststeptargets: the user,
// 2026-10-30, "dust was farming beaks for the weapon quest and it never
// drops": the step named the ravens, its journal the ravens and magpies).

// otherCreatures gives a step of a quest more creatures, as the table holds
// them, and reads the quests again.
func (f *questFixture) otherCreatures(quest, step int, names ...string) {
	f.t.Helper()
	for _, n := range names {
		f.exec(`INSERT INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)`, quest, step, n)
	}
	if err := f.qe.Reload(); err != nil {
		f.t.Fatal(err)
	}
}

// recordDrops has the engine's drops recorded and its draws taken in turn
// from rolls (0.99 once they run out).
func recordDrops(f *questFixture, rolls ...float64) *[]lootDrop {
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

// A kill step counts each creature it names, any case and spaces round the
// name, and is done at its count. The controls: a creature it does not name
// counts for nothing, nor one whose name only holds one of its names
// ("Bloodfeather Raven" is no raven to a step that names "raven").
func TestAKillStepCountsEveryCreatureItNames(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (20, 'Birds', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (20, 1, 'kill', 'raven', 3, 'Kill ravens and magpies.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.otherCreatures(20, 1, "Magpie ")
	if _, err := f.qe.SetCharacterQuest(f.sid, 20, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	kill := func(name string) {
		f.qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: 900, Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
	}
	kill("Crow ")
	kill("Bloodfeather Raven")
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("a crow and a Bloodfeather Raven counted: %q", m)
	}
	kill("Raven")
	f.want(f.msgs(), "chat:Raven: 1 of 3.")
	kill("magpie")
	f.want(f.msgs(), "chat:magpie: 2 of 3.")
	kill(" MAGPIE ")
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
	if s, _ := f.progress(20); s != 2 {
		t.Errorf("after a raven and two magpies the step is %d, want 2", s)
	}
	got := f.qe.CharacterQuests(f.sid)
	if len(got) != 1 || got[0].Targets != nil {
		t.Errorf("at the talk step the panel's state names creatures: %+v", got)
	}
}

// A step pinned to one spawn takes its own creature at that spawn only, and
// its others at any of theirs.
func TestAPinnedStepTakesItsOthersAnywhere(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (21, 'The raven', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, npcid, count, journal) VALUES (21, 1, 'kill', 'raven', 900, 2, 'Kill the raven.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (21, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.otherCreatures(21, 1, "magpie")
	if _, err := f.qe.SetCharacterQuest(f.sid, 21, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	kill := func(name string, spawn entity.EntityID) {
		f.qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: spawn, Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
	}
	kill("raven", 901)
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("a raven of another spawn counted: %q", m)
	}
	kill("magpie", 905)
	f.want(f.msgs(), "chat:magpie: 1 of 2.")
	kill("raven", 900)
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
}

// A loot step drops its item off every creature it names, at its chance; the
// control: a creature it does not name drops nothing and takes no draw.
func TestALootStepDropsOffEveryCreatureItNames(t *testing.T) {
	f := newQuestFixture(t)
	beak := f.pattern("chipped raven beak", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (60, 'New Gear', 1, 'Malsis')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal)
		VALUES (60, 1, 'loot', 'raven', '%d', 2, 0.5, 'Take the chipped raven beak from the ravens and magpies outside the city.')`, beak))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (60, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.otherCreatures(60, 1, "Magpie")
	got := recordDrops(f, 0.2, 0.7, 0.3)
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.killFor("Crow ", 801)
	if len(*got) != 0 {
		t.Fatalf("a crow dropped a beak: %v", *got)
	}
	f.killFor("Magpie ", 802) // 0.2: a drop
	if len(*got) != 1 || (*got)[0] != (lootDrop{802, f.sid, beak}) {
		t.Fatalf("the drops %v, want the beak on the magpie 802", *got)
	}
	f.killFor("raven", 803) // 0.7: none at 0.5
	f.killFor("raven", 804) // 0.3: a drop (the crow took no draw)
	if len(*got) != 2 || (*got)[1] != (lootDrop{804, f.sid, beak}) {
		t.Fatalf("the drops %v, want a second beak on the raven 804", *got)
	}
	f.qe.Looted(f.tw.s, f.tw.p, beak, 2)
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
}

// A talk step's rows in the table name no one: the step is done at its own
// NPC only (the control: the row's NPC talked to does nothing of it).
func TestATalkStepsOtherNamesCountForNothing(t *testing.T) {
	f := newQuestFixture(t)
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.otherCreatures(10010, 1, "Malsis")
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.talk(malsis)
	f.not(f.msgs(), "drop:0")
	if s, _ := f.progress(10010); s != 1 {
		t.Fatalf("Malsis did the talk step to Kellina: it is %d", s)
	}
	f.tw.talk(kellina)
	f.want(f.msgs(), "drop:0", "add:Bring Kellina two cracked ant pincers.")
}

// The marks and the tracker name every creature a step counts: the hunting
// marks at the nearest spawns of all its names (two magpies before a raven
// farther off), a kill step's detail and tracker line "raven or magpie", and
// the panel's state its creatures.
func TestMarksAndTrackerNameEveryCreature(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.add(3, &entity.NPC{ServerID: 800, Name: "raven", SpawnX: 26200, SpawnZ: 15725, Level: 2})
	f.tw.add(4, &entity.NPC{ServerID: 801, Name: "Magpie ", SpawnX: 25450, SpawnZ: 15725, Level: 2})
	f.tw.add(5, &entity.NPC{ServerID: 802, Name: "Magpie ", SpawnX: 25600, SpawnZ: 15725, Level: 2})
	f.tw.add(6, &entity.NPC{ServerID: 803, Name: "crow", SpawnX: 25410, SpawnZ: 15725, Level: 2})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (30, 'Birds', 1, 'Malsis', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (30, 1, 'kill', 'raven', 3, 'Kill ravens and magpies.')`)
	f.otherCreatures(30, 1, "magpie")
	if _, err := f.qe.SetCharacterQuest(f.sid, 30, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	var hunt []questMark
	for _, m := range marks {
		if m.kind == markHunt {
			hunt = append(hunt, m)
		}
	}
	if len(hunt) != maxMarkSpots || hunt[0].x != 25450 || hunt[1].x != 25600 || hunt[0].npc != "Magpie" {
		t.Fatalf("hunting marks %+v, want the two nearest magpies (the crow nearer still is no bird of the step's)", hunt)
	}
	if d := hunt[0].quests[0].detail; d != "raven or magpie: 0 of 3 killed" {
		t.Errorf("its detail %q", d)
	}
	f.qe.SetProgress(f.tw.s, f.tw.p, true)
	if got, _ := sentProgress(t, f.tw.s); show(got) != "Birds: raven or magpie 0/3" {
		t.Errorf("the tracker %q", show(got))
	}
	if got := f.qe.CharacterQuests(f.sid); len(got) != 1 || !reflect.DeepEqual(got[0].Targets, []string{"raven", "magpie"}) {
		t.Errorf("the panel's state %+v", got)
	}
}
