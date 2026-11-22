package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Board entry 1292: a step with queststeps.exactbase on takes its items' own records alone. Fixture quests (not a
// live allowlist): 101299 gives two rings at Exacta, 101300 shows a ring at Showa, 101301 loots a ring off a wyvern
// for Lootra; each exactbase, beside the default quests of newAscendedQuestFixture (petition 1446's, unchanged).
const (
	exactGive = 101299 + iota
	exactShow
	exactLoot
)

func newExactBaseFixture(t *testing.T) (*ascendedQuestFixture, map[int]uint32) {
	t.Helper()
	f := newAscendedQuestFixture(t)
	npcs := map[int]uint32{}
	for i, q := range []struct {
		id           int
		giver, kind  string
		items, ofTgt string
	}{
		{exactGive, "Exacta", "give", fmt.Sprintf("%d*2", f.ring), ""},
		{exactShow, "Showa", "show", fmt.Sprint(f.ring), ""},
		{exactLoot, "Lootra", "loot", fmt.Sprint(f.ring), "wyvern"},
	} {
		npcs[q.id] = f.tw.add(5+i, &entity.NPC{ServerID: entity.EntityID(705 + i), Name: q.giver, SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, repeatable, offer, accept, decline, declined, started)
			VALUES (?, ?, 1, ?, 1, 10, '10', 1, ?, 'I will.', 'Not now.', 'Then not.', 'Go on.')`, q.id, "exact "+q.kind, q.giver, "Will you do exact "+q.kind+"?")
		target := q.giver
		if q.ofTgt != "" {
			target = q.ofTgt
		}
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, journal, says, reminder, rewarditems, byclass, exactbase)
			VALUES (?, 1, ?, ?, ?, 1, 'Do it.', 'There.', 'Not all of it yet.', ?, ?, 1)`,
			q.id, q.kind, target, q.items, fmt.Sprint(f.draught), fmt.Sprintf(`{"10": {"items": "%d"}}`, f.plate))
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	return f, npcs
}

// The column reads back (a reload, as a restart or a panel save makes) and is off on every other step.
func TestExactBaseReadsFromTheStep(t *testing.T) {
	f, _ := newExactBaseFixture(t)
	f.qe.mu.Lock()
	defer f.qe.mu.Unlock()
	for id, want := range map[int]bool{exactGive: true, exactShow: true, exactLoot: true, ascendedGive: false, ascendedLoot: false} {
		qu := f.qe.cat.byID[id]
		if qu == nil || qu.Step(1) == nil || qu.Step(1).ExactBase != want {
			t.Errorf("quest %d step 1 exactbase: want %v", id, want)
		}
	}
}

// A pack of Ascended rings alone does not meet an exactbase give step: nothing taken, no progress, no reward; a mixed
// pack gives only its base units, leaves the Ascended one, and the reward is the regular one (no Ascended record
// made).
func TestExactBaseGiveTakesOnlyTheBaseRecords(t *testing.T) {
	f, npcs := newExactBaseFixture(t)
	f.own(f.ascended, 1)
	f.own(f.ascended, 1)
	m := f.take(npcs[exactGive], "Will you do exact give?")
	f.want(m, "box:Exacta: Not all of it yet.")
	if f.held(f.ascended) != 2 || f.held(f.plate) != 0 || f.held(f.draught) != 0 {
		t.Fatalf("an Ascended-only hand-in: Ascended rings %d, plate %d, draught %d", f.held(f.ascended), f.held(f.plate), f.held(f.draught))
	}
	if step, done := f.progress(exactGive); step != 1 || done != 0 {
		t.Fatalf("an Ascended-only hand-in moved the step: %d %d", step, done)
	}
	f.own(f.ring, 1) // one base ring and two Ascended: still one short
	f.msgs()
	f.tw.talk(npcs[exactGive])
	f.want(f.msgs(), "box:Exacta: Not all of it yet.")
	f.own(f.ring, 1)
	f.msgs()
	f.tw.talk(npcs[exactGive])
	f.want(f.msgs(), "box:Exacta: There.", "box:You have given away 2 Ring of the Test.", "box:You have received Breastplate of the Test.")
	if f.held(f.ring) != 0 || f.held(f.ascended) != 2 || f.held(f.plate) != 1 || f.ascendedOf(f.plate) != 0 {
		t.Errorf("held: ring %d, Ascended ring %d, plate %d; Ascended plate made %d", f.held(f.ring), f.held(f.ascended),
			f.held(f.plate), f.ascendedOf(f.plate))
	}
}

// An exactbase show step is not met by an Ascended ring; a base ring is, kept, and the reward is regular.
func TestExactBaseShow(t *testing.T) {
	f, npcs := newExactBaseFixture(t)
	f.own(f.ascended, 1)
	m := f.take(npcs[exactShow], "Will you do exact show?")
	f.want(m, "box:Showa: Not all of it yet.")
	f.own(f.ring, 1)
	f.msgs()
	f.tw.talk(npcs[exactShow])
	f.want(f.msgs(), "box:Showa: There.", "box:You have received Breastplate of the Test.")
	if f.held(f.ring) != 1 || f.held(f.ascended) != 1 || f.ascendedOf(f.plate) != 0 {
		t.Errorf("held: ring %d, Ascended ring %d; Ascended plate %d", f.held(f.ring), f.held(f.ascended), f.ascendedOf(f.plate))
	}
}

// An exactbase loot step does not count an Ascended record looted, nor the tracker; the base record does.
func TestExactBaseLootAndTracker(t *testing.T) {
	f, npcs := newExactBaseFixture(t)
	f.take(npcs[exactLoot], "Will you do exact loot?")
	f.qe.Looted(f.tw.s, f.tw.p, f.ascended, 1)
	if step, done := f.progress(exactLoot); step != 1 || done != 0 {
		t.Fatalf("an Ascended ring looted counted: %d %d", step, done)
	}
	f.own(f.ascended, 1)
	l := sessionInvList(f.tw.s)
	f.qe.mu.Lock()
	exact, def := f.qe.cat.byID[exactLoot].Step(1), f.qe.cat.byID[ascendedLoot].Step(1)
	f.qe.mu.Unlock()
	if l.heldForStep(exact, f.ring) != 0 || l.heldForStep(def, f.ring) != 1 || l.heldForStep((*database.QuestStep)(nil), f.ring) != 1 {
		t.Errorf("the tracker: exact %d, default %d", l.heldForStep(exact, f.ring), l.heldForStep(def, f.ring))
	}
	f.qe.Looted(f.tw.s, f.tw.p, f.ring, 1)
	if step, done := f.progress(exactLoot); step != 0 || done != 1 {
		t.Errorf("the base ring looted: step %d, done %d; want finished", step, done)
	}
}
