package handlers

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// The Thulite Anvil's shape (quest 100792, petition 2446; eqoa.forumotion.com t114: "with both golem parts walk
// toward the inanimate Thulite Golem ... Once you do, it will start attacking you"): the Thulite golem stands in the
// hut as a statue (the NPC bit: no one hurts it, it attacks no one); a reach step at it takes the two parts and fires
// a trigger that hides the statue and raises a hostile copy of it in its place, one at a time (its lock); the copy's
// loot step follows. Its run ended without the kill (its clock, a GM's stop), the character is put back to the reach
// step with the parts handed back (a reach step's items are set down, so they are its to have again), and setting
// them in again wakes it again; the kill then does the loot step.
func TestAStatueWokenByAReachStepsItems(t *testing.T) {
	f := newQuestFixture(t)
	partA, partB := f.pattern("Golem Part A", 1), f.pattern("Golem Part B", 1)
	chunks := f.pattern("Huge Chunks of Thulite", 1)
	statue := &entity.NPC{ServerID: 705, Name: "Thulite Golem", HP: 500, MaxHP: 500, Level: 50, NPCType: 0x80,
		X: 25403, Y: 50, Z: 15725, SpawnX: 25403, SpawnY: 50, SpawnZ: 15725, World: 0}
	f.tw.w.NPCs[statue.ServerID] = statue
	f.tw.w.SpatialIndex.Insert(statue.ServerID, statue.X, statue.Z)
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, x, y, z, radius, steps) VALUES (23, 'Thulite Golem wakes', 0, 25403, 50, 15725, 60,
		'[{"do":"lock","lock":"thulite golem"},{"do":"clock","seconds":1800,"empty":300},
		  {"do":"hide","npc":"Thulite Golem","radius":3,"minutes":31},
		  {"do":"spawn","npc":"Thulite Golem","hostile":true,"minutes":30,"spread":0.5,"world":0,"x":25403,"y":50,"z":15725,"face":"north"},
		  {"do":"awaitdead"},{"do":"end"},{"do":"despawn"},{"do":"hide","npc":"Thulite Golem","radius":3}]')`)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (82, 'The Thulite Anvil', 1, 'Grimm', 'x', 'y')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, items, world, x, z, radius, journal, reminder, events) VALUES
		(82, 1, 'reach', ?, 0, 25403, 15725, 5, 'Fit the two parts into the Thulite golem.', 'The golem needs both parts.',
		'[{"on":"done","do":"trigger","trigger":"Thulite Golem wakes","text":"The golem is awake already."},
		  {"on":"done","do":"popup","text":"The golem rises to fight!"}]')`, fmt.Sprintf("%d, %d", partA, partB))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, chance, journal) VALUES (82, 2, 'loot', 'Thulite Golem', ?, 1, 'Take its thulite.')`, chunks)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (82, 3, 'talk', 'Grimm', 'Back to Grimm.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	f.own(partA, 1)
	f.own(partB, 1)
	if _, err := f.qe.SetCharacterQuest(f.sid, 82, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	// before: the statue is the only golem, and no one may hurt it
	if statue.NPCType&0x80 == 0 {
		t.Fatal("the statue lost its NPC bit")
	}

	// the parts fitted: taken, the statue out of view, a hostile golem in its place
	wake := func(what string) *entity.NPC {
		t.Helper()
		f.qe.checkReach()
		if step, _ := f.progress(82); step != 2 || f.held(partA) != 0 || f.held(partB) != 0 {
			t.Fatalf("%s: step %d, parts held %d %d (%v)", what, step, f.held(partA), f.held(partB), f.msgs())
		}
		if !e.RunningNamed("Thulite Golem wakes") || len(e.runs) != 1 || len(e.runs[0].spawns) != 1 {
			t.Fatalf("%s: the trigger did not raise one golem", what)
		}
		if !time.Now().Before(statue.HiddenUntil) {
			t.Fatalf("%s: the statue still in view", what)
		}
		c := f.tw.w.NPCs[e.runs[0].spawns[0].id]
		if c == nil || c == statue || c.NPCType&0x80 != 0 || c.Dead || !strings.EqualFold(c.Name, "Thulite Golem") {
			t.Fatalf("%s: the woken golem %+v", what, c)
		}
		if d := math.Hypot(float64(c.X-statue.X), float64(c.Z-statue.Z)); d > 1 {
			t.Fatalf("%s: woken %.1f from the statue (%+v)", what, d, *c)
		}
		f.msgs()
		return c
	}
	wake("the parts fitted")

	// the run ends without the kill: put back to the reach step, the parts handed back
	if runs, _ := e.StopRuns(23); runs != 1 {
		t.Fatalf("%d runs stopped", runs)
	}
	t0 := time.Now().Add(time.Hour)
	f.qe.checkStranded(t0)
	f.qe.checkStranded(t0.Add(strandGrace + time.Second))
	m := f.msgs()
	if step, _ := f.progress(82); step != 1 || f.held(partA) != 1 || f.held(partB) != 1 {
		t.Fatalf("its run ended: step %d, parts %d %d (%v)", step, f.held(partA), f.held(partB), m)
	}
	if !strings.Contains(strings.Join(m, "|"), "the encounter ended before Thulite Golem fell") {
		t.Fatalf("not told: %v", m)
	}

	// fitted again: it wakes again; its kill does the loot step, and the run's end brings the statue back
	statue.HiddenUntil = time.Time{} // a stop leaves the hide to run out
	c := wake("fitted again")
	f.killFor("Thulite Golem", c.ServerID)
	c.Dead, c.HP, c.DeathTime = true, 0, time.Now()
	e.Tick()
	if e.RunningNamed("Thulite Golem wakes") {
		t.Fatal("the run outlived its golem")
	}
	if time.Now().Before(statue.HiddenUntil) {
		t.Error("the statue not back in view after the run")
	}
	if statue.NPCType&0x80 == 0 || statue.Dead {
		t.Error("the statue changed")
	}
}
