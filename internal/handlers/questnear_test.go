package handlers

import (
	"strings"
	"testing"
)

// questnear.go's tests: a step's near events (Heirloom petition 3332, Tallon's
// fortress woken again by a player past its reach step; the owner 2026-11-27).

// nearFixture is quest 90 ("Tallon Zek", repeatable): step 1 a kill with no
// near event, steps 2 and 3 kills whose near events fire "Fortress" (lock
// "fortress", said "Awake already." while held) and "Post" (lock "post") within
// 50 of 100, 200 in world 0; its engine's own ticks stopped, the triggers' on
// the engine every look uses.
func nearFixture(t *testing.T) (*questFixture, *TriggerEngine) {
	t.Helper()
	f := newQuestFixture(t)
	f.qe.Stop()
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, x, y, z, steps) VALUES
		(30, 'Fortress', 0, 100, 0, 200, '[{"do":"lock","lock":"fortress"},{"do":"clock","seconds":7200,"empty":900},{"do":"wait","seconds":7200},{"do":"end"}]'),
		(31, 'Post', 0, 100, 0, 200, '[{"do":"lock","lock":"post"},{"do":"clock","seconds":7200,"empty":900},{"do":"wait","seconds":7200},{"do":"end"}]')`)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	near := `[{"on":"near","do":"trigger","trigger":"Fortress","world":0,"x":100,"z":200,"radius":50,"text":"Awake already."},` +
		`{"on":"near","do":"trigger","trigger":"Post","x":100,"z":200,"radius":50}]`
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (90, 'Tallon Zek', 1, 'Julaech', 1)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES
		(90, 1, 'kill', 'Zek troop', 'Kill a Zek troop.', ''),
		(90, 2, 'kill', 'Zek sentry', 'Kill the Zek sentry.', '` + near + `'),
		(90, 3, 'kill', 'Tallon Zek', 'Kill Tallon Zek.', '` + near + `')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	return f, e
}

// woke is which of the two triggers' locks are held.
func woke(e *TriggerEngine) (fort, post bool) { return e.BusyNamed("Fortress"), e.BusyNamed("Post") }

// A player on a near step standing in its place fires its triggers, both, once
// each coming in: not again while it stays (the runs ended under it), again
// when it steps out and in. The controls: on a step with no near event (step
// 1) nothing fires at the place; on the step, outside the radius, nothing; the
// quest moved on to a step of the same place while it stands there is no
// coming in; a login is one.
func TestANearStepFiresItsTriggersOnceEachComingIn(t *testing.T) {
	f, e := nearFixture(t)
	stop := func() { e.StopRuns(30); e.StopRuns(31) }

	f.at(110, 210) // at the place, on step 1
	f.qe.checkNear()
	if fort, post := woke(e); fort || post {
		t.Fatalf("off the step: fired (%v, %v)", fort, post)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestSetStep, 2); err != nil {
		t.Fatal(err)
	}
	f.at(100, 260) // 60 away
	f.qe.checkNear()
	if fort, post := woke(e); fort || post {
		t.Fatalf("outside the radius: fired (%v, %v)", fort, post)
	}
	f.at(100, 240) // 40 away
	f.qe.checkNear()
	if fort, post := woke(e); !fort || !post {
		t.Fatalf("on the step in the place: fired (%v, %v), want both", fort, post)
	}
	if m := f.msgs(); strings.Contains(strings.Join(m, "\n"), "Awake already.") {
		t.Errorf("told the lock is held by its own firing: %q", m)
	}

	stop()
	f.qe.checkNear()
	f.qe.checkNear()
	if fort, post := woke(e); fort || post {
		t.Fatalf("fired again while it stayed (%v, %v)", fort, post)
	}

	f.at(300, 300) // out
	f.qe.checkNear()
	f.at(95, 205) // and in again
	f.qe.checkNear()
	if fort, post := woke(e); !fort || !post {
		t.Fatalf("in again: fired (%v, %v), want both", fort, post)
	}

	// step 2 done in the fortress: step 3's place is the same, no coming in
	stop()
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	f.qe.checkNear()
	if fort, post := woke(e); fort || post {
		t.Fatalf("the next step at the same place fired again (%v, %v)", fort, post)
	}

	// a login there is a coming in
	f.qe.LoggedIn(f.sid)
	f.qe.checkNear()
	if fort, post := woke(e); !fort || !post {
		t.Fatalf("a login in the place: fired (%v, %v), want both", fort, post)
	}
}

// While a run holds the fortress's lock the player coming in is told the
// trigger event's text, once that coming in, and nothing fires (not the post
// either: the place's events run together, as step 4's reach waits on both);
// the lock free while it stays, they run then. The control: another coming in
// with the lock held is told again.
func TestANearStepWaitsForItsTriggersLock(t *testing.T) {
	f, e := nearFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestSetStep, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := e.FireNamed("Fortress", TriggerFirer{Name: "Other", Quiet: true}); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	told := func() int {
		n := 0
		for _, m := range f.msgs() {
			if strings.Contains(m, "Awake already.") {
				n++
			}
		}
		return n
	}

	f.at(100, 200)
	f.qe.checkNear()
	f.qe.checkNear()
	f.qe.checkNear()
	if n := told(); n != 1 {
		t.Errorf("told %d times in three looks, want once", n)
	}
	if _, post := woke(e); post {
		t.Fatal("the post fired while the fortress's lock was held")
	}

	f.at(400, 400)
	f.qe.checkNear()
	f.at(100, 200)
	f.qe.checkNear()
	if n := told(); n != 1 {
		t.Errorf("coming in again with the lock held: told %d times, want once", n)
	}

	e.StopRuns(30) // the other raid's run ends
	if fort, _ := woke(e); fort {
		t.Fatal("the lock still held after its run ended")
	}
	f.qe.checkNear()
	if fort, post := woke(e); !fort || !post {
		t.Fatalf("the lock free while it stayed: fired (%v, %v), want both", fort, post)
	}
	f.qe.checkNear()
	if n := told(); n != 0 {
		t.Errorf("told %d times after its own firing, want none", n)
	}
}

// A near event with no place of its own holds at its step's (its radius and
// height): the control, a step with neither, has none.
func TestANearEventTakesItsStepsPlace(t *testing.T) {
	f, e := nearFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (91, 'Fort Reach', 1, 'Julaech')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, world, x, y, z, radius, height, journal, events) VALUES
		(91, 1, 'kill', 'Zek sentry', 0, 500, 10, 500, 20, 5, 'Kill the sentry.', '[{"on":"near","do":"trigger","trigger":"Fortress"}]')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 91, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.tw.p.Y = 30 // 20 above the step's floor: out of its height
	f.at(505, 505)
	f.qe.checkNear()
	if fort, _ := woke(e); fort {
		t.Fatal("fired out of the step's height")
	}
	f.tw.p.Y = 12
	f.at(530, 500) // 30 away: past its 20
	f.qe.checkNear()
	if fort, _ := woke(e); fort {
		t.Fatal("fired outside the step's radius")
	}
	f.at(515, 500)
	f.qe.checkNear()
	if fort, _ := woke(e); !fort {
		t.Fatal("not fired within the step's place")
	}
}
