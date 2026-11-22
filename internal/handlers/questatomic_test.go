package handlers

import (
	"errors"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Board entry 1263: a hand-in whose doing fires an encounter is one outcome (questatomic.go).
// The fixture is lightQuests' (triggerraid_test.go): a repeatable give step that is done on
// its accept and fires the locked trigger "isle", as the Plane of Sky's shard quests do.

// handInOnce takes quest 71 (the rain's) and hands its shards in on the accept.
func handInOnce(t *testing.T, f *questFixture) []string {
	t.Helper()
	light := &entity.NPC{Name: "Light", ServerID: 900}
	if !f.qe.Talk(f.tw.s, f.tw.p, light) {
		t.Fatal("the light said nothing")
	}
	m := f.msgs()
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 0)
	return f.msgs()
}

func runsOf(e *TriggerEngine, name string) int {
	n := 0
	for _, r := range e.Runs() {
		if strings.EqualFold(r.Name, name) {
			n++
		}
	}
	return n
}

// The success: the shards gone, the quest counted in the database, exactly one run.
func TestAtomicHandInStartsOneRun(t *testing.T) {
	f := newQuestFixture(t)
	sun, rain := f.pattern("sun shard", 20), f.pattern("rain shard", 20)
	e := lightQuests(f, sun, rain)
	f.own(rain, 2)
	m := handInOnce(t, f)
	if f.held(rain) != 0 {
		t.Fatalf("%d rain shards left (%v)", f.held(rain), m)
	}
	if step, done := f.progress(71); step != 0 || done != 1 {
		t.Fatalf("the rain's row is step %d done %d, want finished once", step, done)
	}
	if n := runsOf(e, "Isle"); n != 1 {
		t.Fatalf("%d runs of the isle, want 1", n)
	}
	if !e.BusyNamed("isle") {
		t.Fatal("the run does not hold its lock")
	}
	if len(f.qe.reservedRuns) != 0 {
		t.Fatalf("a reservation left behind: %v", f.qe.reservedRuns)
	}
}

// A failure at any write (the row, the purse, the progress) leaves the shards, the row and
// the encounter as they were, and the lock free. The control: the same hand-in with no
// fault is done (TestAtomicHandInStartsOneRun).
func TestAtomicHandInFailureChangesNothing(t *testing.T) {
	for at := 0; at < 3; at++ {
		f := newQuestFixture(t)
		sun, rain := f.pattern("sun shard", 20), f.pattern("rain shard", 20)
		e := lightQuests(f, sun, rain)
		f.own(rain, 2)
		k := at
		f.qe.handInFault = func(i int) error {
			if i == k {
				return errors.New("the disk is full")
			}
			return nil
		}
		m := handInOnce(t, f)
		if f.held(rain) != 2 {
			t.Fatalf("fault at %d: %d rain shards held, want 2 (%v)", at, f.held(rain), m)
		}
		if _, done := f.progress(71); done != 0 {
			t.Fatalf("fault at %d: the quest counted done %d", at, done)
		}
		if n := runsOf(e, "Isle"); n != 0 || e.BusyNamed("isle") {
			t.Fatalf("fault at %d: %d runs, busy %v", at, n, e.BusyNamed("isle"))
		}
		if strings.Contains(strings.Join(m, "|"), "You have finished a quest!") {
			t.Fatalf("fault at %d: told it was finished: %v", at, m)
		}
		// the fault cleared, the same hand-in is done once
		f.qe.handInFault = nil
		f.qe.mu.Lock()
		c := f.qe.charLocked(f.sid)
		qu, cq := f.qe.cat.byID[71], c.quests[71]
		if cq == nil || !cq.Active() {
			f.qe.mu.Unlock()
			t.Fatalf("fault at %d: the quest is not under way to do again", at)
		}
		f.qe.atNPCLocked(f.tw.s, f.tw.p, c, qu, cq, qu.First(), true, -1)
		f.qe.mu.Unlock()
		if f.held(rain) != 0 || runsOf(e, "Isle") != 1 {
			t.Fatalf("fault at %d, then none: %d shards, %d runs", at, f.held(rain), runsOf(e, "Isle"))
		}
	}
}

// A trigger that is off (or gone) takes nothing and says so; on again, the hand-in is done.
func TestAtomicHandInRefusedTriggerTakesNothing(t *testing.T) {
	f := newQuestFixture(t)
	sun, rain := f.pattern("sun shard", 20), f.pattern("rain shard", 20)
	e := lightQuests(f, sun, rain)
	f.own(rain, 2)
	f.exec(`UPDATE gmtriggers SET enabled = 0 WHERE slot = 20`)
	m := handInOnce(t, f)
	if f.held(rain) != 2 || runsOf(e, "Isle") != 0 {
		t.Fatalf("an off trigger's hand-in took %d shards, ran %d", 2-f.held(rain), runsOf(e, "Isle"))
	}
	if !strings.Contains(strings.Join(m, "|"), handInCannot) {
		t.Fatalf("no word on why: %v", m)
	}
	f.exec(`DELETE FROM gmtriggers WHERE slot = 20`)
	handInOnce(t, f)
	if f.held(rain) != 2 {
		t.Fatal("a missing trigger's hand-in took the shards")
	}
}

// A reservation holds its lock against every other firing (a GM's, a death's), and its
// own activation runs once; a release frees the lock with nothing run.
func TestATriggerReservationCannotBeStolen(t *testing.T) {
	f := newQuestFixture(t)
	sun, rain := f.pattern("sun shard", 20), f.pattern("rain shard", 20)
	e := lightQuests(f, sun, rain)
	by := TriggerFirer{Name: "hand-in", Player: f.tw.p, Quiet: true}
	res, _, err := e.ReserveNamed("isle", by)
	if err != nil {
		t.Fatal(err)
	}
	if !e.BusyNamed("isle") {
		t.Fatal("a reserved lock reads free")
	}
	if _, err := e.FireNamed("isle", TriggerFirer{Name: "a GM", Player: f.tw.p}); !errors.Is(err, ErrTriggerBusy) {
		t.Fatalf("a GM's firing took a reserved lock: %v", err)
	}
	if _, _, err := e.ReserveNamed("isle", by); !errors.Is(err, ErrTriggerBusy) {
		t.Fatalf("a second reservation of the lock: %v", err)
	}
	if _, err := e.Activate(res); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activate(res); err == nil {
		t.Fatal("a reservation activated twice")
	}
	if n := runsOf(e, "Isle"); n != 1 {
		t.Fatalf("%d runs, want 1", n)
	}
	e.StopRuns(0)
	res2, _, err := e.ReserveNamed("isle", by)
	if err != nil {
		t.Fatal(err)
	}
	e.Release(res2)
	if e.BusyNamed("isle") || runsOf(e, "Isle") != 0 {
		t.Fatal("a released reservation still holds its lock or ran")
	}
}

// A give step that fires no trigger is taken as ever, all its rows or none: a fault at the
// second of two rows leaves the first.
func TestATakeIsAllItsRowsOrNone(t *testing.T) {
	f := newQuestFixture(t)
	f.own(f.pincer, 1)
	f.own(f.pincer, 1) // two rows of one
	f.qe.handInFault = func(i int) error {
		if i == 1 {
			return errors.New("refused")
		}
		return nil
	}
	f.qe.mu.Lock()
	qu := f.qe.cat.byID[10010]
	_, ok := f.qe.takeLocked(f.tw.s, f.tw.p, qu.Step(2))
	f.qe.mu.Unlock()
	if ok || f.held(f.pincer) != 2 {
		t.Fatalf("a take refused at its second row took %d (ok %v)", 2-f.held(f.pincer), ok)
	}
	f.qe.handInFault = nil
	f.qe.mu.Lock()
	_, ok = f.qe.takeLocked(f.tw.s, f.tw.p, qu.Step(2))
	f.qe.mu.Unlock()
	if !ok || f.held(f.pincer) != 0 {
		t.Fatalf("the take with no fault: ok %v, %d left", ok, f.held(f.pincer))
	}
}
