package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Plane of Sky's Snow Chain as the owner's source has it (t212; tools/snow_chain_20261118.py): the code its
// triggers need. A failed escort puts the questor back even when the encounter spawned its creature by a row key
// ("#119634", the Wind Mephit), its escort hidden an hour from her death (protect's minutes), and the raid near the
// boss's death set past it (questset: "If you are in the kill group, you will be flagged for Desolation Chain").

// snowRig is a quest fixture with a trigger engine of its own clock over the same database.
type snowRig struct {
	*questFixture
	e   *TriggerEngine
	now time.Time
}

func newSnowRig(t *testing.T) *snowRig {
	t.Helper()
	f := newQuestFixture(t)
	r := &snowRig{questFixture: f, now: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	r.e = NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	r.e.now = func() time.Time { return r.now }
	r.e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(r.e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(r.e, nil) })
	return r
}

// waitStep waits for the quest's step to be want (a trigger's queststep and questset run on a goroutine).
func (r *snowRig) waitStep(quest, want int) int {
	r.t.Helper()
	var step int
	for i := 0; i < 200; i++ {
		if step, _ = r.progress(quest); step == want {
			return step
		}
		time.Sleep(5 * time.Millisecond)
	}
	return step
}

// A walk whose creature is a row key's copy (the Snow chain's Wind Mephit, "#119634", a spawn-only original): the
// trigger is known to spawn the kill step's creature, so a run ended without the kill puts the quest back to the step
// that fired it. Until 2026-11-18 the encounter's look compared the row key with the creature's name and never saw it:
// a failed escort left the questor on the Mephit for good.
func TestARowKeyedEncounterEndedWithoutItsKillPutsTheQuestBack(t *testing.T) {
	r := newSnowRig(t)
	p := r.tw.p
	r.tw.w.NPCs[710] = &entity.NPC{ServerID: 710, Name: "Wind Mephit", HP: 100, MaxHP: 100, Level: 64, NPCType: 0x80,
		X: p.X + 30, Y: p.Y, Z: p.Z, World: p.World, SpawnOnly: true}
	r.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (30, 'The Walk', -1,
		'[{"do":"lock","lock":"walk"},{"do":"spawn","npc":"#710","hostile":true,"minutes":60},{"do":"awaitdead"}]')`)
	if err := r.e.Reload(); err != nil {
		t.Fatal(err)
	}
	r.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (81, 'The Walk', 1, 'Nobody', 'x', 'y')`)
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (81, 1, 'talk', 'Ryini', 'Speak with Ryini.',
		'[{"on":"done","do":"trigger","trigger":"The Walk","text":"She walks already."}]')`)
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 2, 'kill', 'Wind Mephit', 'Kill the mephit.')`)
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 3, 'talk', 'Ryini', 'Speak with her again.')`)
	// past the walk: a kill of the world's own creature, then a loot step off any creature, the walk's Mephit among
	// them (the Snow chain's herb): not the walk's kill
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 4, 'kill', 'Whirlwind', 'Kill the Whirlwind.')`)
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, items, chance, journal) VALUES (81, 5, 'loot', 'icebound gargoyle', '1', 0.15, 'A herb.')`)
	r.exec(`INSERT INTO queststeptargets (questid, step, target) VALUES (81, 5, 'Wind Mephit')`)
	r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 6, 'talk', 'Guenzi', 'The stew.')`)
	if err := r.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if !r.e.SpawnsNPC("The Walk", "wind mephit") {
		t.Fatal("the walk is not known to spawn the Wind Mephit (its row key)")
	}
	if _, err := r.qe.SetCharacterQuest(r.sid, 81, QuestGive, 1); err != nil {
		t.Fatal(err)
	}
	r.qe.Talk(r.tw.s, p, &entity.NPC{Name: "Ryini", ServerID: 901})
	if step, _ := r.progress(81); step != 2 || !r.e.RunningNamed("The Walk") {
		t.Fatalf("after the talk: step %d, running %v (%v)", step, r.e.RunningNamed("The Walk"), r.msgs())
	}
	if runs, _ := r.e.StopRuns(30); runs != 1 {
		t.Fatalf("%d runs stopped", runs)
	}
	t1 := time.Now().Add(time.Hour)
	r.qe.checkStranded(t1)
	r.qe.checkStranded(t1.Add(strandGrace + time.Second))
	if step, _ := r.progress(81); step != 1 {
		t.Fatalf("after the walk ended without the mephit: step %d, want 1 (%v)", step, r.msgs())
	}
	if all := strings.Join(r.msgs(), "|"); !strings.Contains(all, "The Walk: the encounter ended before Wind Mephit fell.") {
		t.Fatalf("not told: %s", all)
	}
	// on the herb's step (and on the talk past the walk), no walk running: left where they are
	for _, at := range []int{3, 5} {
		if _, err := r.qe.SetCharacterQuest(r.sid, 81, QuestSetStep, at); err != nil {
			t.Fatal(err)
		}
		t2 := t1.Add(time.Duration(at) * time.Hour)
		r.qe.checkStranded(t2)
		r.qe.checkStranded(t2.Add(strandGrace + time.Second))
		if step, _ := r.progress(81); step != at {
			t.Fatalf("on step %d past the walk: put back to %d", at, step)
		}
	}
}

// t212: "If she dies, you fail, and must wait one hour for the npc to respawn". The escort's row is hidden as the walk
// starts (an hour) and a copy walks; the copy's death fails the run and, with protect's minutes, keeps the row out of
// view that long from the death, not from the start. The control: a protect with no minutes leaves the start's hour.
func TestProtectMinutesHideTheEscortFromHerDeath(t *testing.T) {
	for _, tc := range []struct {
		minutes string
		want    time.Duration // after the death at 20 minutes
	}{{`,"minutes":60`, 60 * time.Minute}, {``, 40 * time.Minute}} {
		w := newTrigWorld(t)
		// the world's hiding is judged by the wall clock (NearestNPC): the engine's clock beside it
		start := time.Now().Truncate(time.Second)
		w.now = start
		row := &entity.NPC{ServerID: 720, Name: "Ryini", HP: 100, MaxHP: 100, Level: 60, X: w.gp.X, Y: w.gp.Y, Z: w.gp.Z,
			SpawnX: w.gp.X, SpawnY: w.gp.Y, SpawnZ: w.gp.Z, World: w.gp.World}
		w.gw.NPCs[row.ServerID] = row
		w.set(31, "Escort", "near", 400, `[{"do":"hide","npc":"Ryini","radius":10,"minutes":60},
			{"do":"spawn","npc":"#720","spread":1,"minutes":30},
			{"do":"protect","npc":"Ryini","radius":10,"text":"She fell."`+tc.minutes+`},
			{"do":"wait","seconds":1800}]`)
		if _, err := w.fire(31); err != nil {
			t.Fatal(err)
		}
		copies := w.spawned()
		if len(copies) != 1 || copies[0].Name != "Ryini" || !row.HiddenUntil.Equal(w.now.Add(time.Hour)) {
			t.Fatalf("at the start: copies %v, the row hidden until %v", copies, row.HiddenUntil)
		}
		w.now = start.Add(20 * time.Minute)
		w.e.Tick()
		w.kill(copies[0])
		w.now = start.Add(20*time.Minute + time.Second)
		w.e.Tick()
		if len(w.e.Runs()) != 0 {
			t.Fatalf("the run lives after her death")
		}
		death := start.Add(20*time.Minute + time.Second)
		if want := death.Add(tc.want); tc.minutes == "" {
			if !row.HiddenUntil.Equal(death.Add(-time.Second).Add(tc.want)) {
				t.Fatalf("no minutes: hidden until %v, want the start's hour", row.HiddenUntil)
			}
		} else if !row.HiddenUntil.Equal(want) {
			t.Fatalf("minutes 60: hidden until %v, want %v (an hour from her death)", row.HiddenUntil, want)
		}
	}
}

// t212: "If you are in the kill group, you will be flagged for Desolation Chain". A questset sets each player the
// run reaches who is on the quest at a step from its from up to before its step to that step (its journal line with
// it); one short of from, or at or past the step, is left as it is.
func TestQuestSetSetsTheRunsPlayersOnItsSteps(t *testing.T) {
	r := newSnowRig(t)
	r.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (82, 'The Chain', 1, 'Nobody', 'x', 'y')`)
	for s, j := range []string{"Speak with Ryini.", "Speak with Tamis.", "Bring the stew.", "Kill her.", "Go back to the Isle.", "Speak with the Mark."} {
		r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (82, ?, 'talk', 'Someone', ?)`, s+1, j)
	}
	if err := r.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	r.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (32, 'The Kill', -1, '[{"do":"questset","quest":82,"step":5,"from":2}]')`)
	if err := r.e.Reload(); err != nil {
		t.Fatal(err)
	}
	fire := func() {
		t.Helper()
		if _, err := r.e.Fire(32, TriggerFirer{Name: r.tw.p.Name, Sess: r.tw.s, Player: r.tw.p, Quiet: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ at, want int }{{1, 1}, {2, 5}, {3, 5}, {4, 5}, {5, 5}, {6, 6}} {
		if _, err := r.qe.SetCharacterQuest(r.sid, 82, QuestSetStep, tc.at); err != nil {
			t.Fatal(err)
		}
		r.msgs()
		fire()
		if tc.want != tc.at {
			if got := r.waitStep(82, tc.want); got != tc.want {
				t.Fatalf("from step %d: step %d, want %d", tc.at, got, tc.want)
			}
			if _, lines := r.journal(); !strings.Contains(strings.Join(lines, "|"), "Go back to the Isle.") {
				t.Fatalf("from step %d: the journal %v", tc.at, lines)
			}
			continue
		}
		time.Sleep(50 * time.Millisecond)
		if got, _ := r.progress(82); got != tc.want {
			t.Fatalf("from step %d: step %d, want it left", tc.at, got)
		}
	}
}

// A questset names its quest, its step and a from before it; a protect's minutes are 0 to the engine's most.
func TestQuestSetAndProtectMinutesProblems(t *testing.T) {
	for _, tc := range []struct {
		steps string
		want  string
	}{
		{`[{"do":"questset","quest":5,"step":4,"from":2}]`, ""},
		{`[{"do":"questset","quest":5,"step":4}]`, "questset names no quest, step and from"},
		{`[{"do":"questset","quest":5,"step":4,"from":4}]`, "is its step"},
		{`[{"do":"questset","quest":5,"step":4,"from":6}]`, ""},
		{`[{"do":"protect","npc":"Ryini","minutes":60}]`, ""},
		{`[{"do":"protect","npc":"Ryini","minutes":-1}]`, "protect's -1.0 minutes"},
	} {
		got := strings.Join(database.TriggerProblems(database.GMTrigger{Slot: 40, Name: "t", Scope: "near", World: -1, Steps: tc.steps}), "|")
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: problems %q, want %q", tc.steps, got, tc.want)
		}
	}
}
