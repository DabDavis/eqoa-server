package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// The owner, 2026-11-13, after Lonewulf lost two Rallos Zek fights to server updates: a scripted encounter survives
// a keep restart (gmtriggerkeep.go, entity/keeptrigger.go). A run with a lock, a clock (with its empty), a hostile
// spawn set on Mr. Moogie and an attackable add, waiting on the boss's death: the boss fought down to 40 of 100 and
// off its spawn, the add killed and lying. Written and read back as JSON, then made again in a fresh process (its own
// World, engine and clock, 5 s later): the run is there under its id at the same step, its lock held, its clock's end
// and its spawn's stay moved on by the 5 s, the same boss under the same id where it stood at its hit points, still
// hostile and still on Mr. Moogie, the add's corpse found by the id its corpse came back under. Its awaitdead still
// watches the boss: his death sends the run on to its next step. The control: a cold start (no file) has no run and
// the lock is free.
func TestATriggerRunGoesThroughAKeepRestart(t *testing.T) {
	setup := func(w *trigWorld) {
		gp := w.gp
		w.gw.NPCs[701] = &entity.NPC{ServerID: 701, Name: "Mr. Moogie", HP: 1000, MaxHP: 1000, Level: 50, X: gp.X + 6, Y: gp.Y, Z: gp.Z, World: gp.World}
		w.gw.NPCs[703] = &entity.NPC{ServerID: 703, Name: "Orc Grunt", HP: 50, MaxHP: 50, Level: 3, NPCType: 0x80, X: 120, Y: 50, Z: 100, World: 0}
		w.set(30, "Pawn Attacks Mr. Moogie", "near", 100, `[{"do":"lock","lock":"pawn vs moogie"},{"do":"clock","seconds":3600,"empty":600},
			{"do":"spawn","npc":"Orc Pawn","hostile":true,"minutes":60,"spread":2},{"do":"spawn","npc":"Orc Grunt","attackable":true,"minutes":60,"spread":8},
			{"do":"sic","npc":"Orc Pawn","target":"Mr. Moogie","radius":60,"casts":true},
			{"do":"awaitdead","npc":"Orc Pawn"},{"do":"broadcast","text":"The pawn falls."},{"do":"wait","seconds":30},{"do":"end"}]`)
	}
	a := newTrigWorld(t)
	setup(a)
	if _, err := a.fire(30); err != nil {
		t.Fatal(err)
	}
	var boss, add *entity.NPC
	for _, n := range a.spawned() {
		switch n.Name {
		case "Orc Pawn":
			boss = n
		case "Orc Grunt":
			add = n
		}
	}
	if boss == nil || add == nil || boss.CurrentEnemy != 701 {
		t.Fatalf("the run's spawns: boss %+v add %+v", boss, add)
	}
	a.at(10 * time.Second)
	boss.HP = 40
	boss.X += 5
	add.Dead, add.HP, add.DeathTime = true, 0, time.Now()
	runA := a.e.runs[0]
	deadlineA, untilA, nextA, idA := runA.deadline, runA.spawns[0].until, runA.next, runA.id

	// the restart: the run and its living spawn written, the add's corpse as keepcopy.go writes it
	written := a.now
	k := a.e.keep(written)
	copies := a.gw.KeepCopies()
	if k == nil || len(k.Runs) != 1 || len(k.NPCs) != 1 || len(copies) != 1 {
		t.Fatalf("written: %+v, %d corpses", k, len(copies))
	}
	raw, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	var back KeptTriggers
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}

	b := newTrigWorld(t) // the new process: its World, engine and clock fresh
	setup(b)
	b.now = written.Add(5 * time.Second)
	cids, lost := b.gw.RestoreCopies(copies)
	if lost != 0 {
		t.Fatalf("the add's corpse lost")
	}
	_, r := RestoreTriggers(&back, cids, b.now)
	if r.Runs != 1 || r.Alive != 1 || r.Corpses != 1 || r.Lost != 0 || r.RunsLost != 0 {
		t.Fatalf("restored %+v", r)
	}
	if len(b.e.runs) != 1 {
		t.Fatalf("%d runs", len(b.e.runs))
	}
	run := b.e.runs[0]
	if run.id != idA || run.next != nextA || run.await == nil || run.await.NPC != "Orc Pawn" {
		t.Fatalf("the run: id %d next %d await %+v; want id %d next %d awaiting the pawn", run.id, run.next, run.await, idA, nextA)
	}
	if !b.e.BusyNamed("Pawn Attacks Mr. Moogie") {
		t.Fatal("its lock is free after the restart")
	}
	if got := run.deadline.Sub(deadlineA); got != 5*time.Second {
		t.Fatalf("the clock's end moved %v, want the restart's 5 s (the time it had left)", got)
	}
	if got := run.spawns[0].until.Sub(untilA); got != 5*time.Second {
		t.Fatalf("the spawn's stay moved %v", got)
	}
	nb := b.gw.NPCs[boss.ServerID]
	if nb == nil || nb.Dead || !nb.Temporary || nb.CopyOf != 700 || nb.HP != 40 || nb.X != boss.X || !nb.EventAggro || nb.NPCType&0x80 != 0 {
		t.Fatalf("the boss after the restart: %+v (was id %d at %v, 40 hp)", nb, boss.ServerID, boss.X)
	}
	if nb.CurrentEnemy != 701 || nb.Threats[701] <= 0 {
		t.Fatalf("the boss is off Mr. Moogie: enemy %d threats %v", nb.CurrentEnemy, nb.Threats)
	}
	if !nb.Temporary || !b.gw.NPCAlive(nb.ServerID) {
		t.Fatal("the boss is not alive")
	}
	var spawnIDs []entity.EntityID
	for _, s := range run.spawns {
		spawnIDs = append(spawnIDs, s.id)
	}
	if len(spawnIDs) != 2 || spawnIDs[0] != boss.ServerID || spawnIDs[1] != cids[add.ServerID] {
		t.Fatalf("the run's spawns %v, want the boss %d and the add's corpse %d", spawnIDs, boss.ServerID, cids[add.ServerID])
	}

	// it still watches: a tick with the boss alive waits; his death sends it on
	b.clear()
	b.at(20 * time.Second)
	if len(b.e.runs) != 1 || b.e.runs[0].next != nextA {
		t.Fatal("the run went on with its boss alive")
	}
	nb.Dead, nb.HP = true, 0
	b.at(21 * time.Second)
	if got := strings.Join(yellow(t, b.ps), "|"); !strings.Contains(got, "The pawn falls.") {
		t.Fatalf("the boss's death did not send the run on: %q", got)
	}

	// the control: a cold start carries nothing
	c := newTrigWorld(t)
	setup(c)
	if len(c.e.runs) != 0 || c.e.BusyNamed("Pawn Attacks Mr. Moogie") {
		t.Fatal("a cold start has a run")
	}
}
