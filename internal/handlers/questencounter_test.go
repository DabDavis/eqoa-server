package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// rallosShapeQuest is The Mark of Rallos's shape (101441): step 1 at the signpost fires the encounter (and a portal trigger
// that spawns no creature a step kills), step 2 hands the Mark to the portal, step 3 kills the boss the encounter
// spawns. It returns the Mark's patternid and the engine.
func rallosShapeQuest(f *questFixture) (int, *TriggerEngine) {
	f.t.Helper()
	mark := f.pattern("Mark of Rallos", 1)
	f.tw.w.NPCs[700] = &entity.NPC{ServerID: 700, Name: "Orc Pawn", HP: 100, MaxHP: 100, Level: 5, NPCType: 0x80, X: 100, Y: 50, Z: 100, World: 0}
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	f.t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (20, 'Isle Boss', -1,
		'[{"do":"lock","lock":"isle"},{"do":"clock","seconds":7200,"empty":600},{"do":"spawn","npc":"Orc Pawn","hostile":true,"minutes":120},{"do":"awaitdead","npc":"Orc Pawn"},{"do":"wait","seconds":300},{"do":"end"}]')`)
	f.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (21, 'Isle Portal', -1, '[{"do":"broadcast","text":"The portal wakes."}]')`)
	if err := e.Reload(); err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable, offer, accept) VALUES (80, 'The Mark', 1, 'Nobody', 1, 'x', 'y')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (80, 1, 'talk', 'Signpost', 'Press the Mark to the signpost.',
		'[{"on":"done","do":"trigger","trigger":"Isle Boss","text":"Another raid fights him."},{"on":"done","do":"trigger","trigger":"Isle Portal"}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, answer) VALUES (80, 2, 'give', 'Portal', ?, 'Give the portal the Mark.', 'Take the Mark.')`, mark)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (80, 3, 'kill', 'Orc Pawn', 'Kill the pawn.')`)
	if err := f.qe.Reload(); err != nil {
		f.t.Fatal(err)
	}
	return mark, e
}

// toTheKill takes the fixture's character from step 1 to the kill: the signpost (the encounter fires), the Mark into
// the portal.
func toTheKill(t *testing.T, f *questFixture, e *TriggerEngine, mark int) {
	t.Helper()
	if step, _ := f.progress(80); step != 1 {
		if _, err := f.qe.SetCharacterQuest(f.sid, 80, QuestGive, 1); err != nil {
			t.Fatal(err)
		}
	}
	f.qe.Talk(f.tw.s, f.tw.p, &entity.NPC{Name: "Signpost", ServerID: 901})
	if step, _ := f.progress(80); step != 2 || !e.RunningNamed("Isle Boss") {
		t.Fatalf("at the signpost: step %d, encounter running %v (%v)", step, e.RunningNamed("Isle Boss"), f.msgs())
	}
	f.qe.Talk(f.tw.s, f.tw.p, &entity.NPC{Name: "Portal", ServerID: 902})
	if m := f.msgs(); boxID(m) != 0 {
		f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 0)
	}
	if step, _ := f.progress(80); step != 3 || f.held(mark) != 0 {
		t.Fatalf("at the portal: step %d, %d Marks held (%v)", step, f.held(mark), f.msgs())
	}
	f.msgs()
}

// The owner, 2026-11-13: a quest is never left stuck when its encounter ends without the kill (questencounter.go).
// The Mark of Rallos's shape: the character on the kill step, the Mark taken. While the encounter runs nothing
// changes, however long. Its run stopped (a clock, the isle empty, a GM, a restart that could not carry it), the
// character is put back to the signpost's step after strandGrace, the Mark handed back and said; the signpost then
// fires the encounter again. The control: one who got the kill is finished and left so (no Mark back) when the run
// ends. And one on the kill step with no run at all (a cold start, Lonewullf's row today) is put back the same way.
func TestAnEncounterEndedWithoutItsKillPutsTheQuestBack(t *testing.T) {
	f := newQuestFixture(t)
	mark, e := rallosShapeQuest(f)
	f.own(mark, 1)
	toTheKill(t, f, e, mark)
	t0 := time.Now().Add(time.Hour) // past any check the engine's own second makes
	for _, d := range []time.Duration{0, 30 * time.Second, 10 * time.Minute} {
		f.qe.checkStranded(t0.Add(d))
	}
	if step, _ := f.progress(80); step != 3 {
		t.Fatalf("put back while its encounter runs: step %d", step)
	}

	// the run ends without the kill
	if runs, _ := e.StopRuns(20); runs != 1 {
		t.Fatalf("%d runs stopped", runs)
	}
	t1 := t0.Add(time.Hour)
	f.qe.checkStranded(t1)
	f.qe.checkStranded(t1.Add(5 * time.Second))
	if step, _ := f.progress(80); step != 3 {
		t.Fatalf("put back within the grace: step %d", step)
	}
	f.qe.checkStranded(t1.Add(strandGrace + time.Second))
	m := f.msgs()
	if step, _ := f.progress(80); step != 1 || f.held(mark) != 1 {
		t.Fatalf("after its encounter ended: step %d, %d Marks (%v)", step, f.held(mark), m)
	}
	all := strings.Join(m, "|")
	if !strings.Contains(all, "The Mark: the encounter ended before Orc Pawn fell.") || !strings.Contains(all, "You have received Mark of Rallos.") {
		t.Fatalf("not told: %v", m)
	}
	if _, lines := f.journal(); !strings.Contains(strings.Join(lines, "|"), "Press the Mark to the signpost.") {
		t.Fatalf("the journal: %v", lines)
	}

	// the control: the encounter again, the kill got; its run ending leaves the quest finished, no Mark back
	toTheKill(t, f, e, mark)
	f.killFor("Orc Pawn", 5000)
	if step, done := f.progress(80); step != 0 || done != 1 {
		t.Fatalf("the kill: step %d done %d", step, done)
	}
	e.StopRuns(20)
	for _, d := range []time.Duration{0, time.Minute} {
		f.qe.checkStranded(t1.Add(time.Hour + d))
	}
	if step, done := f.progress(80); step != 0 || done != 1 || f.held(mark) != 0 {
		t.Fatalf("after the kill and its run's end: step %d done %d, %d Marks", step, done, f.held(mark))
	}

	// a cold start: on the kill step (taken again and set there, as Lonewullf's row stands), no run of it at all
	f.msgs()
	if _, err := f.qe.SetCharacterQuest(f.sid, 80, QuestGive, 3); err != nil {
		t.Fatal(err)
	}
	t2 := t1.Add(3 * time.Hour)
	f.qe.checkStranded(t2)
	f.qe.checkStranded(t2.Add(strandGrace + time.Second))
	if step, _ := f.progress(80); step != 1 || f.held(mark) != 1 {
		t.Fatalf("a cold start's kill step: step %d, %d Marks", step, f.held(mark))
	}
}

// A loot step's encounter (Jarek's Creation's shape) ends at its creature's death, and the looting comes after: while
// the creature's corpse lies the character is left on the step, however long; the corpse gone unlooted, it is put back
// to the step that fires the encounter.
func TestALootStepWaitsForItsCorpse(t *testing.T) {
	f := newQuestFixture(t)
	shell := f.pattern("creation's shell", 1)
	f.tw.w.NPCs[704] = &entity.NPC{ServerID: 704, Name: "Orc Grunt", HP: 50, MaxHP: 50, Level: 3, NPCType: 0x80, X: 100, Y: 50, Z: 100, World: 0}
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (22, 'Creation', -1, '[{"do":"spawn","npc":"Orc Grunt","hostile":true,"minutes":30}]')`)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (81, 'Creation', 1, 'Nobody', 'x', 'y')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (81, 1, 'talk', 'Jarek', 'Talk to Jarek.',
		'[{"on":"done","do":"trigger","trigger":"Creation"}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, chance, journal) VALUES (81, 2, 'loot', 'Orc Grunt', ?, 1, 'Take its shell.')`, shell)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 81, QuestGive, 1); err != nil {
		t.Fatal(err)
	}
	f.qe.Talk(f.tw.s, f.tw.p, &entity.NPC{Name: "Jarek", ServerID: 903})
	if step, _ := f.progress(81); step != 2 || !e.RunningNamed("Creation") || len(e.runs) != 1 || len(e.runs[0].spawns) != 1 {
		t.Fatalf("at Jarek: step %d, running %v", step, e.RunningNamed("Creation"))
	}
	c := f.tw.w.NPCs[e.runs[0].spawns[0].id]
	c.Dead, c.HP, c.DeathTime = true, 0, time.Now()
	e.Tick()
	if e.RunningNamed("Creation") {
		t.Fatal("the run outlived its creature")
	}
	t0 := time.Now().Add(time.Hour)
	for _, d := range []time.Duration{0, time.Minute, 2 * time.Minute} {
		f.qe.checkStranded(t0.Add(d))
	}
	if step, _ := f.progress(81); step != 2 {
		t.Fatalf("put back while the creature's corpse lies: step %d", step)
	}
	c.CorpseGone = time.Now()
	f.qe.checkStranded(t0.Add(3 * time.Minute))
	f.qe.checkStranded(t0.Add(3*time.Minute + strandGrace + time.Second))
	if step, _ := f.progress(81); step != 1 {
		t.Fatalf("its corpse gone unlooted: step %d", step)
	}
}
