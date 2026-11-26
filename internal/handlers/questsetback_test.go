package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 3144 (Night 3's race for the maiden): a bishop killed in a lost race counted on the quest, so a
// raid killed one, let the time run out, started again and killed the other. A questset with its from past its step
// puts those on the steps after it back to it, and a fail's onfail does it as the run fails.

// questSetBackRig is a snow rig with quest 82 of six talk steps.
func questSetBackRig(t *testing.T) *snowRig {
	t.Helper()
	r := newSnowRig(t)
	r.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (82, 'The Chain', 1, 'Nobody', 'x', 'y')`)
	for s := 1; s <= 6; s++ {
		r.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (82, ?, 'talk', 'Someone', ?)`, s, "Step "+string(rune('0'+s))+".")
	}
	if err := r.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestQuestSetPutsBackThosePastItsStep(t *testing.T) {
	r := questSetBackRig(t)
	r.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (33, 'Back', -1, '[{"do":"questset","quest":82,"step":2,"from":4}]')`)
	if err := r.e.Reload(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ at, want int }{{1, 1}, {2, 2}, {3, 2}, {4, 2}, {5, 5}} {
		if _, err := r.qe.SetCharacterQuest(r.sid, 82, QuestSetStep, tc.at); err != nil {
			t.Fatal(err)
		}
		r.msgs()
		if _, err := r.e.Fire(33, TriggerFirer{Name: r.tw.p.Name, Sess: r.tw.s, Player: r.tw.p, Quiet: true}); err != nil {
			t.Fatal(err)
		}
		if tc.want != tc.at {
			if got := r.waitStep(82, tc.want); got != tc.want {
				t.Fatalf("from step %d: step %d, want %d", tc.at, got, tc.want)
			}
			continue
		}
		time.Sleep(50 * time.Millisecond)
		if got, _ := r.progress(82); got != tc.want {
			t.Fatalf("from step %d: step %d, want it left", tc.at, got)
		}
	}
}

func TestAFailsOnFailPutsTheRacesKillsBack(t *testing.T) {
	r := questSetBackRig(t)
	steps := `[{"do":"awaitkill","npc":"Arch Bishop Millian, Arch Bishop Lancel","seconds":60,"fail":"The dukes carry her off.",` +
		`"onfail":[{"do":"questset","quest":82,"step":2,"from":4}]},{"do":"broadcast","text":"Saved."}]`
	r.exec(`INSERT INTO gmtriggers (slot, name, world, steps) VALUES (34, 'Race', -1, ?)`, steps)
	if err := r.e.Reload(); err != nil {
		t.Fatal(err)
	}
	if p := database.TriggerProblems(database.GMTrigger{Slot: 34, Name: "Race", Scope: "near", World: -1, Steps: steps}); len(p) > 0 {
		t.Fatalf("problems %v", p)
	}
	// the first bishop's kill took the player to step 3; the time runs out
	if _, err := r.qe.SetCharacterQuest(r.sid, 82, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := r.e.Fire(34, TriggerFirer{Name: r.tw.p.Name, Sess: r.tw.s, Player: r.tw.p, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	r.now = r.now.Add(30 * time.Second)
	r.e.Tick()
	time.Sleep(50 * time.Millisecond)
	if got, _ := r.progress(82); got != 3 {
		t.Fatalf("before the time is out: step %d, want 3", got)
	}
	r.now = r.now.Add(31 * time.Second)
	r.e.Tick()
	if got := r.waitStep(82, 2); got != 2 {
		t.Fatalf("the race lost: step %d, want 2 (the first bishop again)", got)
	}
	if len(r.e.Runs()) != 0 {
		t.Fatal("the lost race's run lives on")
	}
}

func TestOnFailProblems(t *testing.T) {
	for _, tc := range []struct{ steps, want string }{
		{`[{"do":"awaitkill","npc":"a","seconds":5,"fail":"x","onfail":[{"do":"questset","quest":5,"step":2,"from":4}]}]`, ""},
		{`[{"do":"awaitkill","npc":"a","seconds":5,"onfail":[{"do":"questset","quest":5,"step":2,"from":4}]}]`, "onfail is a fail's"},
		{`[{"do":"awaitkill","npc":"a","seconds":5,"fail":"x","onfail":[{"do":"broadcast","text":"y"}]}]`, "questsets alone"},
		{`[{"do":"awaitkill","npc":"a","seconds":5,"fail":"x","onfail":[{"do":"questset","quest":5,"step":2,"from":2}]}]`, "is its step"},
		{`[{"do":"awaitkill","npc":"a","seconds":5},{"do":"awaitkill","npc":"a","seconds":5,"keep":true}]`, ""},
		{`[{"do":"awaitdead","keep":true}]`, "keep is an awaitkill's"},
	} {
		got := strings.Join(database.TriggerProblems(database.GMTrigger{Slot: 40, Name: "t", Scope: "near", World: -1, Steps: tc.steps}), "|")
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: problems %q, want %q", tc.steps, got, tc.want)
		}
	}
}
