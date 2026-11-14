package handlers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The Plane of Sky's scripted events (tools/sky_events_20261114.py) on four more parts of
// the triggers: a trigger that fires itself on a talk or a player come near (start), a wait
// on kills of creatures of the world (awaitkill), a wait that fails its run when its time
// runs out (fail), and a spawn's max counted by the name a row key's copies carry.

// A talk to the start's NPC fires the trigger; with no words of its own the NPC answers as
// ever (Talk says false), with words they are its answer. While its run lives a talk fires
// nothing: its lock's words when it has some, else the NPC as ever. A class it does not
// take is refused. The controls: an NPC of the name far from the place, another world's.
func TestTriggerStartsOnATalk(t *testing.T) {
	w := newTrigWorld(t)
	npc := w.gw.NPCs[700] // Orc Pawn at (100, 100), world 0
	w.setAt(30, "Voice", "near", 80, 0, 100, 50, 100, `[{"do":"lock","lock":"voice","text":"The ancients already walk."},
		{"do":"awaittalk","npc":"Orc Pawn","start":true},{"do":"spawn","npc":"Orc Pawn","count":2,"minutes":10},{"do":"awaitdead"}]`, true)
	far := *npc
	far.X = 5000
	if w.e.Talk(w.gs, w.gp, &far) || len(w.spawned()) != 0 {
		t.Fatal("an NPC of the name far from the place fired it")
	}
	odd := *npc
	odd.World = 2
	if w.e.Talk(w.gs, w.gp, &odd) || len(w.spawned()) != 0 {
		t.Fatal("an NPC of the name in another world fired it")
	}
	if w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("a start with no words answered for the NPC")
	}
	if n := len(w.spawned()); n != 2 {
		t.Fatalf("%d spawns after the talk, want 2", n)
	}
	// its run lives: the lock's words, nothing more fired
	w.clear()
	if !w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("a busy start with lock words did not answer")
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "The ancients already walk.") {
		t.Fatalf("no lock words: %s", got)
	}
	if n := len(w.spawned()); n != 2 {
		t.Fatalf("%d spawns: a busy talk fired again", n)
	}
	for _, s := range w.spawned() {
		w.kill(s)
	}
	w.at(time.Second)
	if len(w.e.Runs()) != 0 {
		t.Fatal("the run outlived its spawns")
	}
	// words of its own, a class it takes
	w.setAt(30, "Voice", "near", 80, 0, 100, 50, 100, `[{"do":"awaittalk","npc":"Orc Pawn","start":true,"classes":[0],"refuse":"Only a warrior.","text":"Begin."},
		{"do":"spawn","npc":"Orc Pawn","minutes":10}]`, true)
	w.gp.Class = 9
	w.clear()
	if !w.e.Talk(w.gs, w.gp, npc) || len(w.spawned()) != 0 {
		t.Fatal("another class fired it, or was not answered")
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "Only a warrior.") {
		t.Fatalf("no refusal: %s", got)
	}
	w.gp.Class = 0
	if !w.e.Talk(w.gs, w.gp, npc) || len(w.spawned()) != 1 {
		t.Fatal("its words did not answer, or it did not fire")
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "Begin.") {
		t.Fatalf("no words: %s", got)
	}
	// no lock words: a busy start leaves the NPC to answer as ever
	if w.e.Talk(w.gs, w.gp, npc) || len(w.spawned()) != 1 {
		t.Fatal("a busy start without lock words answered or fired")
	}
	// a GM's firing passes its start by: the spawn at once
	w.e.StopRuns(0)
	if _, err := w.fire(30); err != nil || len(w.spawned()) != 1 {
		t.Fatalf("a GM's firing: %v, %d spawns", err, len(w.spawned()))
	}
}

// A start needs a place of its own and is the first step but its locks.
func TestTriggerStartProblems(t *testing.T) {
	steps := `[{"do":"awaittalk","npc":"X","start":true},{"do":"broadcast","text":"x"}]`
	if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: -1, Steps: steps}); len(p) == 0 {
		t.Fatal("a start at the GM's place passed")
	}
	if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: 4, X: 1, Z: 1, Steps: steps}); len(p) != 0 {
		t.Fatalf("a start at a place of its own: %v", p)
	}
	late := `[{"do":"broadcast","text":"x"},{"do":"awaitnear","start":true}]`
	if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: 4, X: 1, Z: 1, Steps: late}); len(p) == 0 {
		t.Fatal("a start after another step passed")
	}
	bad := `[{"do":"awaitdead","fail":"x"},{"do":"spawn","npc":"X","start":true}]`
	if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: 4, X: 1, Z: 1, Steps: bad}); len(p) < 2 {
		t.Fatalf("a fail with no seconds and a start on a spawn: %v", p)
	}
	kill := `[{"do":"awaitkill"}]`
	if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: 4, X: 1, Z: 1, Steps: kill}); len(p) == 0 {
		t.Fatal("an awaitkill of no one passed")
	}
}

// A player come within the start's reach at about the place's height fires it once; a
// player above it (another isle) or far off does not, and while its run lives nobody
// fires it again.
func TestTriggerStartsOnAPlayerNear(t *testing.T) {
	w := newTrigWorld(t)
	w.setAt(31, "Popup", "near", 100, 0, 25300, 50, 15300, `[{"do":"lock","lock":"necro"},{"do":"awaitnear","start":true,"radius":30},
		{"do":"spawn","npc":"Orc Pawn","minutes":10},{"do":"awaitdead"},{"do":"wait","seconds":120}]`, true)
	w.e.StartNear()
	if len(w.spawned()) != 0 {
		t.Fatal("fired with nobody near")
	}
	w.gp.X, w.gp.Z, w.gp.Y = 25310, 15310, 50+database.TriggerNearHeight+10 // over it
	w.e.StartNear()
	if len(w.spawned()) != 0 {
		t.Fatal("a player on the isle above fired it")
	}
	w.gp.Y = 55
	w.e.StartNear()
	w.e.StartNear()
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns, want one firing", n)
	}
	w.kill(w.spawned()[0])
	w.at(time.Second)
	w.e.StartNear()
	if n := len(w.spawned()); n != 0 {
		t.Fatal("fired again during its last wait")
	}
	w.at(122 * time.Second)
	w.e.StartNear()
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns: its rest over, a player near fires it again", n)
	}
}

// An awaitkill counts credited kills of creatures of the world by name within the run's
// reach, its own spawns' deaths once; each name its count, or with any all of them together.
// The controls: a kill out of reach, another name, a kill before the wait began.
func TestTriggerAwaitsKillsOfTheWorld(t *testing.T) {
	w := newTrigWorld(t)
	w.gw.NPCs[701] = &entity.NPC{ServerID: 701, Name: "Arch Bishop Millian", HP: 100, MaxHP: 100, Level: 60, X: 25010, Y: 50, Z: 15010, World: 0}
	w.gw.NPCs[702] = &entity.NPC{ServerID: 702, Name: "Arch Bishop Lancel", HP: 100, MaxHP: 100, Level: 60, X: 25020, Y: 50, Z: 14990, World: 0}
	w.gw.NPCs[703] = &entity.NPC{ServerID: 703, Name: "Arch Bishop Lancel", HP: 100, MaxHP: 100, Level: 60, X: 29000, Y: 50, Z: 14990, World: 0}
	w.setAt(32, "Maiden", "near", 100, 0, 25000, 50, 15000, `[{"do":"awaitkill","npc":"Arch Bishop Millian, Arch Bishop Lancel"},{"do":"broadcast","text":"saved"}]`, true)
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Millian", NPCID: 701}) // before any run: nothing
	if _, err := w.fire(32); err != nil {
		t.Fatal(err)
	}
	w.clear()
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Millian", NPCID: 701})
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Lancel", NPCID: 703}) // out of reach
	w.e.Killed(entity.KillCredit{NPCName: "a bat", NPCID: 700})
	w.at(time.Second)
	if len(yellow(t, w.gs)) != 0 {
		t.Fatal("went on with one of the two")
	}
	w.e.Killed(entity.KillCredit{NPCName: "arch bishop lancel", NPCID: 702})
	w.at(2 * time.Second)
	if got := yellow(t, w.gs); len(got) != 1 || got[0] != "saved" {
		t.Fatalf("after both: %v", got)
	}
	// any: three of either name; its own spawns' deaths counted once
	w.gw.NPCs[700].X, w.gw.NPCs[700].Z = 25030, 15030
	w.setAt(33, "Ring", "near", 300, 0, 25000, 50, 15000, `[{"do":"spawn","npc":"Orc Pawn","minutes":10},
		{"do":"awaitkill","npc":"orc pawn,erych scout","count":3,"any":true},{"do":"broadcast","text":"patroller"}]`, true)
	if _, err := w.fire(33); err != nil {
		t.Fatal(err)
	}
	w.clear()
	own := w.spawned()[0]
	w.kill(own)
	w.e.Killed(entity.KillCredit{NPCName: "Orc Pawn", NPCID: own.ServerID})
	w.at(3 * time.Second)
	w.e.Killed(entity.KillCredit{NPCName: "Orc Pawn", NPCID: 700})
	w.at(4 * time.Second)
	if len(yellow(t, w.gs)) != 0 {
		t.Fatal("went on at two kills (its own spawn counted twice?)")
	}
	w.e.Killed(entity.KillCredit{NPCName: "Orc Pawn", NPCID: 700})
	w.at(5 * time.Second)
	if got := yellow(t, w.gs); len(got) != 1 {
		t.Fatalf("after three: %v", got)
	}
}

// A wait with fail: its seconds out with its kills not made, the run fails (a window to its
// players, its spawns taken, the rest undone); made in time, it goes on.
func TestTriggerWaitFails(t *testing.T) {
	w := newTrigWorld(t)
	w.gw.NPCs[701] = &entity.NPC{ServerID: 701, Name: "Arch Bishop Millian", HP: 100, MaxHP: 100, Level: 60, X: 25010, Y: 50, Z: 15010, World: 0}
	w.set(34, "Race", "near", 100, `[{"do":"spawn","npc":"Orc Pawn","minutes":30},
		{"do":"awaitkill","npc":"Arch Bishop Millian","seconds":240,"fail":"The dukes reach the maiden."},{"do":"broadcast","text":"saved"}]`)
	if _, err := w.fire(34); err != nil {
		t.Fatal(err)
	}
	w.clear()
	w.at(241 * time.Second)
	if len(w.e.Runs()) != 0 || len(w.spawned()) != 0 {
		t.Fatal("a failed run lives on, or its spawns")
	}
	if box := sentAll(w.ps, protocol.OpcodeQuestPopupMulti); len(box) != 1 {
		t.Fatalf("%d windows to the player near", len(box))
	}
	if len(yellow(t, w.gs)) != 0 {
		t.Fatal("a failed run went on")
	}
	// the control: the kill in time
	if _, err := w.fire(34); err != nil {
		t.Fatal(err)
	}
	w.clear()
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Millian", NPCID: 701})
	w.at(300 * time.Second)
	if got := yellow(t, w.gs); len(got) != 1 {
		t.Fatalf("made in time: %v", got)
	}
}

// A spawn's max counts the copies of a row key by the name they carry.
func TestTriggerSpawnMaxByARowKey(t *testing.T) {
	w := newTrigWorld(t)
	w.set(35, "Patroller", "near", 100, `[{"do":"spawn","npc":"#700","count":3,"max":2,"minutes":10}]`)
	if _, err := w.fire(35); err != nil {
		t.Fatal(err)
	}
	if _, err := w.fire(35); err != nil && !errors.Is(err, ErrTriggerBusy) {
		t.Fatal(err)
	}
	if n := len(w.spawned()); n != 2 {
		t.Fatalf("%d copies of #700, want its max 2", n)
	}
}
