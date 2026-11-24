package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// bulkFixture is a giver of one repeatable hunt and gather turn-in, four
// peppers a set: a loot step of them off lava, then a give step of them at the
// giver paying experience, tunar, an item and faction (as Chef Lukkif's Fiery
// Hot Peppers).
type bulkFixture struct {
	*questFixture
	giver          uint32
	pepper, reward int
}

const bulkQuest = 30001

func newBulkFixture(t *testing.T, repeatable int) *bulkFixture {
	t.Helper()
	f := &bulkFixture{questFixture: newQuestFixture(t)}
	f.giver = f.tw.add(1, &entity.NPC{ServerID: 702, Name: "Chef Test", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.pepper = f.pattern("fiery hot pepper", 200)
	f.reward = f.pattern("pepper token", 200)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable, offer, accept, decline, declined, started)
		VALUES (?, 'Hot Peppers', 1, 'Chef Test', 1, 60, ?, 'Bring me four.' || char(10) || 'Will you?', 'I will do it.', 'Not now.', 'Then go.', 'Off you go.')`,
		bulkQuest, repeatable)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, journal, chance)
		VALUES (?, 1, 'loot', 'animated lava', ?, 4, 'Kill animated lava for four peppers.', 0)`, bulkQuest, fmt.Sprintf("%d*4", f.pepper))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, xp, rewardtunar, rewarditems, factions)
		VALUES (?, 2, 'give', 'Chef Test', ?, 'Bring four peppers to Chef Test.', 'Hot hot hot!', 100, 7, ?, 'Test Chefs:5')`,
		bulkQuest, fmt.Sprintf("%d*4", f.pepper), fmt.Sprint(f.reward))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	return f
}

type bulkPay struct{ xp, tunar, faction, reward, done int }

func (f *bulkFixture) pay() bulkPay {
	_, done := f.progress(bulkQuest)
	return bulkPay{int(f.tw.p.TotalXP), f.tw.p.Tunar, f.tw.p.Factions["Test Chefs"], f.held(f.reward), max(done, 0)}
}

func (b bulkPay) minus(o bulkPay) bulkPay {
	return bulkPay{b.xp - o.xp, b.tunar - o.tunar, b.faction - o.faction, b.reward - o.reward, b.done - o.done}
}

// firstHandIn takes the quest (the held peppers count its loot step) and
// hands in at the giver; it returns what the hand-in's talk was told.
func (f *bulkFixture) firstHandIn() []string {
	f.t.Helper()
	f.msgs()
	f.tw.talk(f.giver)
	m := f.msgs()
	if !asks(m, "Will you?") {
		f.t.Fatalf("not offered: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if step, _ := f.progress(bulkQuest); step != 2 {
		f.t.Fatalf("the quest is at step %d once taken, not its hand-in", step)
	}
	f.tw.talk(f.giver)
	return f.msgs()
}

func bulkAsked(m []string) string {
	for _, s := range m {
		if strings.HasPrefix(s, "ask#") {
			return s
		}
	}
	return ""
}

// Three sets handed in at once pay exactly three single hand-ins, and the
// partial set stays in the pack.
func TestHandInAllPaysEachSet(t *testing.T) {
	// one hand-in, one set held: what one set pays, and no question
	one := newBulkFixture(t, 1)
	one.own(one.pepper, 6)
	before := one.pay()
	if m := one.firstHandIn(); bulkAsked(m) != "" {
		t.Fatalf("asked with no more sets held: %q", m)
	}
	single := one.pay().minus(before)
	if single.done != 1 || single.xp <= 0 || single.tunar != 7 || single.faction != 5 || single.reward != 1 {
		t.Fatalf("one hand-in paid %+v", single)
	}
	if one.held(one.pepper) != 2 {
		t.Fatalf("one hand-in left %d peppers, not 2", one.held(one.pepper))
	}

	f := newBulkFixture(t, 1)
	f.own(f.pepper, 14) // three sets and two over
	before = f.pay()
	m := f.firstHandIn()
	ask := bulkAsked(m)
	want := "You hold 2 more sets for Hot Peppers. Hand them in too?|Hand in all (2 sets)|Just one"
	if !strings.HasSuffix(ask, ":"+want) {
		t.Fatalf("asked %q, not %q (%q)", ask, want, m)
	}
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	f.want(m, "box:You handed in 2 more sets.")
	got := f.pay().minus(before)
	if want := (bulkPay{3 * single.xp, 3 * single.tunar, 3 * single.faction, 3 * single.reward, 3}); got != want {
		t.Errorf("three sets paid %+v, not three times one's %+v", got, want)
	}
	if n := f.held(f.pepper); n != 2 {
		t.Errorf("%d peppers left, not the partial set's 2", n)
	}
	if step, _ := f.progress(bulkQuest); step != 0 {
		t.Errorf("the quest is at step %d after the sets, not finished", step)
	}
	if _, lines := f.journal(); len(lines) != 0 {
		t.Errorf("the journal holds %q after the sets", lines)
	}
	if bulkAsked(m) != "" {
		t.Errorf("asked again during the sets: %q", m)
	}
}

// "Just one" hands in nothing more.
func TestHandInAllJustOne(t *testing.T) {
	f := newBulkFixture(t, 1)
	f.own(f.pepper, 12)
	m := f.firstHandIn()
	if bulkAsked(m) == "" {
		t.Fatalf("not asked: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	f.msgs()
	if _, done := f.progress(bulkQuest); done != 1 || f.held(f.pepper) != 8 {
		t.Errorf("after Just one: done %d, peppers %d", done, f.held(f.pepper))
	}
}

// At most bulkHandInMost sets a talk; the rest stay held.
func TestHandInAllCap(t *testing.T) {
	f := newBulkFixture(t, 1)
	sets := bulkHandInMost + 5
	f.own(f.pepper, 4*sets+1)
	m := f.firstHandIn()
	ask := bulkAsked(m)
	more := sets - 1
	want := fmt.Sprintf("You hold %d more sets for Hot Peppers. Hand in %d now?|Hand in all (%d sets)|Just one", more, bulkHandInMost, bulkHandInMost)
	if !strings.HasSuffix(ask, ":"+want) {
		t.Fatalf("asked %q, not %q", ask, want)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if _, done := f.progress(bulkQuest); done != 1+bulkHandInMost {
		t.Errorf("done %d times, not %d", done, 1+bulkHandInMost)
	}
	if n, want := f.held(f.pepper), 4*(more-bulkHandInMost)+1; n != want {
		t.Errorf("%d peppers left, not %d", n, want)
	}
}

// A quest that is not repeatable is handed in once, as ever, whatever is held.
func TestHandInAllNotForOneTimeQuests(t *testing.T) {
	f := newBulkFixture(t, 0)
	f.own(f.pepper, 12)
	if m := f.firstHandIn(); bulkAsked(m) != "" {
		t.Fatalf("a one-time quest asked: %q", m)
	}
	if _, done := f.progress(bulkQuest); done != 1 || f.held(f.pepper) != 8 {
		t.Errorf("done %d, peppers %d", done, f.held(f.pepper))
	}
}

// Only plain turn-ins: an event, a tunar price or a talk step keeps the
// question away.
func TestBulkHandInStepPlainOnly(t *testing.T) {
	f := newBulkFixture(t, 1)
	qu := f.qe.cat.byID[bulkQuest]
	if bulkHandInStep(qu) == nil {
		t.Fatal("the plain turn-in is not offered it")
	}
	cp := *qu
	cp.Steps = append(cp.Steps[:0:0], qu.Steps...)
	cp.Steps[1].Tunar = 5
	if bulkHandInStep(&cp) != nil {
		t.Error("a hand-in that costs tunar is offered it")
	}
	cp.Steps[1].Tunar = 0
	cp.Steps[0].Kind = "talk"
	if bulkHandInStep(&cp) != nil {
		t.Error("a quest with a talk step is offered it")
	}
	cp.Steps[0].Kind = qu.Steps[0].Kind
	cp.Daily = true
	if bulkHandInStep(&cp) != nil {
		t.Error("a daily quest is offered it")
	}
}

// A PS2 is asked the same box, and its close sent before the pick (interact.go)
// does not end it.
func TestHandInAllOnAPS2(t *testing.T) {
	f := newBulkFixture(t, 1)
	f.tw.s.SetDiscVersion(protocol.GameVersionEQOAFrontiers)
	f.own(f.pepper, 8)
	m := f.firstHandIn()
	if !strings.HasSuffix(bulkAsked(m), "|Hand in all (1 set)|Just one") {
		t.Fatalf("a PS2 was not asked: %q", m)
	}
	f.tw.pick(boxID(m), 255)
	f.tw.pick(boxID(m), 0)
	f.want(f.msgs(), "box:You handed in 1 more set.")
	if _, done := f.progress(bulkQuest); done != 2 || f.held(f.pepper) != 0 {
		t.Errorf("done %d, peppers %d", done, f.held(f.pepper))
	}
}
