package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A spiritmaster's bind does the player's quest step at it in the same go
// (answerBind): "Bound to Neriak"'s first step, "Have Spiritmaster Sharun bind
// your spirit to Neriak.", is done by the Yes that binds, where it waited for
// a second talk that new players did not know to make (F3 20261029-200655,
// 20261030-062009). The controls: No binds nothing and leaves the step; the
// bind of another spiritmaster (Friey, where cow bound) leaves Sharun's step.
func TestABindDoesItsQuestStep(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.h.binds = bindsKept{}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, offer, accept, decline, started)
		VALUES (10020, 'Bound to Neriak', 1, 'Theologist S''Tai', 1, 10, '10', 'Theologist S''Tai: Bind your spirit.',
			'I will.', 'Not now.', 'Theologist S''Tai: Go.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says) VALUES (10020, 1, 'talk', 'Spiritmaster Sharun',
		'Have Spiritmaster Sharun bind your spirit to Neriak.', 'Your spirit is bound to Neriak. Now see Coachman Lothi outside the city.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10020, 2, 'talk', 'Coachman Lothi', 'Sign Coachman Lothi''s ledger.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 10020, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	friey := f.tw.add(1, &entity.NPC{ServerID: 2477, Name: "Spiritmaster Friey", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	sharun := f.tw.add(2, &entity.NPC{ServerID: 2485, Name: "Spiritmaster Sharun", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})

	// the controls: Friey's bind is not Sharun's step; No at Sharun binds nothing
	f.tw.talk(friey)
	f.tw.s.Queue().GetPending()
	f.tw.pick(friey, 0)
	if b, bound := f.tw.w.BindOf(f.tw.p.ServerID); !bound || b.By != "Spiritmaster Friey" {
		t.Fatalf("Friey's Yes: bound %v by %q", bound, b.By)
	}
	if s, _ := f.progress(10020); s != 1 {
		t.Fatalf("the control: Friey's bind moved Sharun's step to %d", s)
	}
	f.msgs()
	f.tw.talk(sharun)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("Sharun offered %q, want his bind's Yes and No first", got)
	}
	f.tw.pick(sharun, 1)
	if b, _ := f.tw.w.BindOf(f.tw.p.ServerID); b.By != "Spiritmaster Friey" {
		t.Fatalf("the control: No at Sharun bound the player by %q", b.By)
	}
	if s, _ := f.progress(10020); s != 1 {
		t.Fatalf("the control: No at Sharun moved the step to %d", s)
	}
	f.msgs()

	// one talk and its Yes: bound by Sharun and the step done
	f.tw.talk(sharun)
	f.tw.s.Queue().GetPending()
	f.tw.pick(sharun, 0)
	if b, bound := f.tw.w.BindOf(f.tw.p.ServerID); !bound || b.By != "Spiritmaster Sharun" {
		t.Fatalf("Sharun's Yes: bound %v by %q", bound, b.By)
	}
	if s, d := f.progress(10020); s != 2 || d != 0 {
		t.Fatalf("after Sharun's Yes the quest is at step %d (done %d), want step 2 in the same go", s, d)
	}
	f.want(f.msgs(), "box:Spiritmaster Sharun: Your spirit is bound to Neriak. Now see Coachman Lothi outside the city.", "add:Sign Coachman Lothi's ledger.")
}
