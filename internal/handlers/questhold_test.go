package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 1617 (Lickinfrogs, "collected items for froglok herbalist, turned in and
// no mobs spawned"): a held trigger (questhold.go). The herbalist talked to with the
// ingredients fires the brew and takes nothing; a second talk while it runs says so;
// the brew's queststep, at its end, takes the ingredients and gives the potion to the
// player on the step; the herbalist's death fails the run and leaves the ingredients
// and the step as they were.

type holdRig struct {
	*questFixture
	e                *TriggerEngine
	now              time.Time
	herb             *entity.NPC
	ing, potion, oth int
}

func newHoldRig(t *testing.T, steps string) *holdRig {
	t.Helper()
	f := newQuestFixture(t)
	r := &holdRig{questFixture: f, now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
	r.ing, r.potion = f.pattern("spotted tongue", 20), f.pattern("herbalist's potion", 1)
	r.e = NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	r.e.now = func() time.Time { return r.now }
	r.e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(r.e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(r.e, nil) })
	p := f.tw.p
	r.herb = &entity.NPC{ServerID: 950, Name: "froglok herbalist", HP: 1000, MaxHP: 1000, Level: 40, NPCType: 0x80,
		X: p.X + 5, Y: p.Y, Z: p.Z, SpawnX: p.X + 5, SpawnY: p.Y, SpawnZ: p.Z, World: p.World}
	f.tw.w.NPCs[r.herb.ServerID] = r.herb
	f.exec(`INSERT INTO gmtriggers (slot, name, scope, radius, world, x, z, steps) VALUES (161, 'The Brew', 'near', 60, -1, 0, 0, ?)`, steps)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline) VALUES (77, 'The Froglok Herbalist', 1, 'Palidan', 'Go.', 'I will.', 'No.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, events) VALUES (77, 1, 'give', 'froglok herbalist', ?, 'Bring the tongue.',
		'It is done. Drink this.', ?)`, fmt.Sprint(r.ing),
		`[{"on":"done","do":"trigger","trigger":"The Brew","hold":true,"says":"Guard me while I brew!","text":"I am brewing already: stay near."},
		  {"on":"done","do":"give","items":"`+fmt.Sprint(r.potion)+`"}]`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says) VALUES (77, 2, 'give', 'Palidan', ?, 'Bring the potion.', 'Done.')`, fmt.Sprint(r.potion))
	f.exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot) VALUES (?, 77, 1, 0, 0, 1)`, f.sid)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.qe.ReloadCharacter(f.sid)
	f.own(r.ing, 1)
	return r
}

func (r *holdRig) talk() string {
	r.t.Helper()
	if !r.qe.Talk(r.tw.s, r.tw.p, r.herb) {
		r.t.Fatal("the herbalist said nothing")
	}
	return strings.Join(r.msgs(), "|")
}

// tick moves the engine's clock on and ticks it.
func (r *holdRig) tick(d time.Duration) {
	r.now = r.now.Add(d)
	r.e.Tick()
}

const brewSteps = `[{"do":"lock","lock":"guk herbalist"},
	{"do":"protect","npc":"froglok herbalist","text":"The herbalist falls. Your ingredients are still yours."},
	{"do":"wait","seconds":300},
	{"do":"queststep","quest":77,"step":1},
	{"do":"wait","seconds":5}]`

func TestHeldTriggerTakesNothingUntilItsQuestStep(t *testing.T) {
	r := newHoldRig(t, brewSteps)
	m := r.talk()
	if !strings.Contains(m, "Guard me while I brew!") {
		t.Fatalf("the herbalist's words: %s", m)
	}
	if r.held(r.ing) != 1 || r.held(r.potion) != 0 {
		t.Fatalf("taken at the talk: tongue %d potion %d", r.held(r.ing), r.held(r.potion))
	}
	if step, _ := r.progress(77); step != 1 {
		t.Fatalf("step %d, want 1 until the brew is done", step)
	}
	if runs := r.e.Runs(); len(runs) != 1 || runs[0].Slot != 161 {
		t.Fatalf("runs %+v, want the brew's one", runs)
	}
	// a second talk while it brews: the lock's words, nothing taken, no second run
	if m := r.talk(); !strings.Contains(m, "I am brewing already: stay near.") {
		t.Fatalf("the second talk: %s", m)
	}
	if len(r.e.Runs()) != 1 || r.held(r.ing) != 1 {
		t.Fatalf("a second talk: %d runs, tongue %d", len(r.e.Runs()), r.held(r.ing))
	}
	// the brew's end: the step done for the player near who holds the tongue
	r.tick(301 * time.Second)
	deadline := time.Now().Add(3 * time.Second)
	for r.held(r.potion) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r.held(r.ing) != 0 || r.held(r.potion) != 1 {
		t.Fatalf("after the brew: tongue %d potion %d", r.held(r.ing), r.held(r.potion))
	}
	if step, _ := r.progress(77); step != 2 {
		t.Fatalf("step %d, want 2 (bring the potion)", step)
	}
	m = strings.Join(r.msgs(), "|")
	if !strings.Contains(m, "It is done. Drink this.") {
		t.Fatalf("the step's words at the brew's end: %s", m)
	}
	r.tick(6 * time.Second)
	if len(r.e.Runs()) != 0 {
		t.Fatalf("the run lives on: %+v", r.e.Runs())
	}
}

// The control: the herbalist dead, the run fails, the ingredients and the step stay,
// and the brew can be begun again.
func TestProtectedNPCsDeathFailsTheRun(t *testing.T) {
	r := newHoldRig(t, brewSteps)
	r.talk()
	r.tick(10 * time.Second)
	r.herb.Dead, r.herb.HP = true, 0
	r.tick(time.Second)
	if len(r.e.Runs()) != 0 {
		t.Fatalf("the run lives on with its herbalist dead: %+v", r.e.Runs())
	}
	if m := strings.Join(r.msgs(), "|"); !strings.Contains(m, "The herbalist falls. Your ingredients are still yours.") {
		t.Fatalf("no word of the failure: %s", m)
	}
	r.tick(400 * time.Second)
	time.Sleep(50 * time.Millisecond)
	if r.held(r.ing) != 1 || r.held(r.potion) != 0 {
		t.Fatalf("after a failed brew: tongue %d potion %d", r.held(r.ing), r.held(r.potion))
	}
	if step, _ := r.progress(77); step != 1 {
		t.Fatalf("step %d, want 1", step)
	}
	r.herb.Dead, r.herb.HP = false, 1000
	if m := r.talk(); !strings.Contains(m, "Guard me while I brew!") || len(r.e.Runs()) != 1 {
		t.Fatalf("begun again: %s (%d runs)", m, len(r.e.Runs()))
	}
}

// A player on the step who lacks the ingredients gets nothing at the brew's end.
func TestQuestStepPassesOverOneWhoLacks(t *testing.T) {
	r := newHoldRig(t, brewSteps)
	r.talk()
	r.exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid = ?`, r.sid, r.ing)
	r.tick(301 * time.Second)
	time.Sleep(100 * time.Millisecond)
	if r.held(r.potion) != 0 {
		t.Fatal("a potion for one who handed in nothing")
	}
	if step, _ := r.progress(77); step != 1 {
		t.Fatalf("step %d, want 1", step)
	}
}

func TestHeldTriggerAndNewStepsProblems(t *testing.T) {
	if p := database.StepProblems([]database.TriggerStep{{Do: database.TriggerProtect, NPC: "x"}, {Do: database.TriggerQuestStep, Quest: 1, QuestStep: 4}}); len(p) != 0 {
		t.Fatalf("problems with good steps: %v", p)
	}
	if p := database.StepProblems([]database.TriggerStep{{Do: database.TriggerProtect}, {Do: database.TriggerQuestStep, Quest: 1}}); len(p) != 2 {
		t.Fatalf("problems %v, want 2", p)
	}
	if p := database.EventProblems(`[{"on":"start","do":"trigger","trigger":"x","hold":true}]`, true); len(p) != 1 {
		t.Fatalf("a held trigger on a step's start: %v", p)
	}
	if p := database.EventProblems(`[{"on":"done","do":"trigger","trigger":"x","hold":true}]`, true); len(p) != 0 {
		t.Fatalf("a held trigger on a step's done: %v", p)
	}
}
