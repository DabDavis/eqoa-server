package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// coachQuestFixture is the quest fixture with the Homelands' three homes
// (homesBuilt), Freeport's Ronks, the homes' Harrek (Aviak Village) and Grumm
// (West Plateau), a camp of snow badgers in Aviak Village and quest 20061
// under way: kill three of them (Heirloom petition 3014's shape: Varg's
// badgers in Goldfeather Eyrie). The client holds world 10; no ledger is
// signed.
func coachQuestFixture(t *testing.T) *questFixture {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	entity.SetComposedWorld(10, true)
	SetServerWorlds([]worlds.Built{homesBuilt()})
	t.Cleanup(func() { SetServerWorlds(nil); entity.SetComposedWorld(10, false) })
	for _, n := range []*entity.NPC{
		{ServerID: 416, Name: "Coachman Ronks", SpawnX: 25273.7, SpawnY: 54.1, SpawnZ: 15719.1},
		{ServerID: 9001, Name: "Coachman Harrek", World: 10, SpawnX: 9211.7, SpawnY: 84.1, SpawnZ: 21547.1},
		{ServerID: 9002, Name: "Coachman Grumm", World: 10, SpawnX: 6947, SpawnY: 57.7, SpawnZ: 3300},
		{ServerID: 9101, Name: "snow badger", World: 10, SpawnX: 8500, SpawnY: 60, SpawnZ: 20500},
		{ServerID: 9102, Name: "snow badger", World: 10, SpawnX: 8530, SpawnY: 60, SpawnZ: 20520},
		{ServerID: 9103, Name: "snow badger", World: 10, SpawnX: 8480, SpawnY: 60, SpawnZ: 20540},
	} {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		f.tw.w.NPCs[n.ServerID] = n
	}
	f.tw.s.SetServerWorlds([]uint8{10})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (20061, 'Snow scent', 1, 'Malsis', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES
		(20061, 1, 'kill', 'snow badger', 3, 'Kill three snow badgers in Aviak Village.')`)
	f.reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 20061, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	return f
}

// sign writes town into the character's ledgers and lets the way marks read them again.
func (f *questFixture) sign(town string) {
	f.t.Helper()
	f.exec(`INSERT INTO charcoaches (serverid, town) VALUES (?, ?)`, f.sid, town)
	f.qe.mu.Lock()
	f.qe.signed = nil
	f.qe.mu.Unlock()
}

// wayMark is the set's way mark of the player's world (a step mark in it at a coachman), or nil.
func wayMark(t *testing.T, f *questFixture) *questMark {
	t.Helper()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	for i := range marks {
		if m := &marks[i]; m.kind == markStep && strings.HasPrefix(m.npc, "Coach") {
			return m
		}
	}
	return nil
}

// A step in a home of the Homelands has its way mark at the coachman whose
// coach goes there, saying the ride: in Freeport Ronks's (no waysender's trip
// leads into the Homelands, and before the coaches were ways there was no
// mark), in another home its own coachman's (the homes are one world, no road
// between them); none in the step's own home. The controls: a stop whose
// ledger is unsigned is no way, and a client without the Homelands has none.
func TestQuestWayIsTheCoach(t *testing.T) {
	f := coachQuestFixture(t)
	if m := wayMark(t, f); m != nil {
		t.Fatalf("no ledger signed: a way mark %+v", m)
	}
	f.sign("Aviak Village")
	m := wayMark(t, f)
	if m == nil || m.npc != "Coachman Ronks" || m.world != 0 || len(m.quests) != 1 ||
		m.quests[0].detail != "Ride Coachman Ronks's coach to Aviak Village" || m.quests[0].line != "Kill three snow badgers in Aviak Village." {
		t.Fatalf("in Freeport the way mark %+v, want Ronks's ride to Aviak Village", m)
	}

	// in West Plateau, a home of the same world
	f.tw.p.World, f.tw.p.X, f.tw.p.Z = 10, 6950, 3310
	m = wayMark(t, f)
	if m == nil || m.npc != "Coachman Grumm" || m.world != 10 || m.quests[0].detail != "Ride Coachman Grumm's coach to Aviak Village" {
		t.Fatalf("in West Plateau the way mark %+v, want Grumm's ride", m)
	}
	// in Aviak Village itself: the badgers alone
	f.tw.p.X, f.tw.p.Z = 9200, 21500
	if m := wayMark(t, f); m != nil {
		t.Errorf("in the step's own home a way mark %+v", m)
	}

	// the control: a client that does not hold the Homelands, in Freeport
	f.tw.p.World, f.tw.p.X, f.tw.p.Z = 0, 25403, 15725
	f.tw.s.SetServerWorlds(nil)
	if m := wayMark(t, f); m != nil {
		t.Errorf("a client without the Homelands was shown the coach %+v", m)
	}
}

// The coach menu puts " ?" after the stop of a step under way, in the
// coachman's own words (Ronks's "Get me a horse to Aviak Village ?."), and
// after no other; from a home with no stop in the step's home the stop the
// way goes on by (Freeport). The control: with the step done, no " ?".
func TestCoachMenuMarksTheQuestsStop(t *testing.T) {
	f := coachQuestFixture(t)
	was := questEngine.Load()
	questEngine.Store(f.qe)
	t.Cleanup(func() { questEngine.Store(was) })
	book := ledgerBook{f.tw.p.CharServerID: {"Freeport": true, "Aviak Village": true, "West Plateau": true}}
	f.tw.h.ledger = book
	f.tw.h.isGM = func(*session.Session) bool { return false }
	for _, town := range []string{"Freeport", "Aviak Village", "West Plateau"} {
		f.sign(town)
	}
	ronks := f.tw.at(1, 416)
	f.tw.s.Queue().GetPending()
	f.tw.talk(ronks)
	got := optionBox(t, f.tw.s)
	words := coachWordsByName["Coachman Ronks"]
	want := []string{words.labelTagged(entity.CoachDest{Name: "Aviak Village"}, questStopTag), words.label(entity.CoachDest{Name: "West Plateau"})}
	if !slices.Equal(got, want) {
		t.Fatalf("Ronks's menu %q, want %q", got, want)
	}
	if !strings.Contains(got[0], "Aviak Village ?") {
		t.Errorf("the tag after the place: %q", got[0])
	}

	// the stop the way goes on by: Grumm's coach runs to the homes and Freeport
	q := f.qe
	grumm := f.tw.w.NPCs[9002]
	dests := []entity.CoachDest{{Name: "Freeport", World: 0, X: 25273.7, Z: 15724}, {Name: "Gerntar Mines (the Homelands)", World: 10, X: 13000, Z: 27000}}
	q.CoachStops(f.tw.p, grumm, dests)
	if dests[0].QuestStop || dests[1].QuestStop {
		t.Errorf("neither stop is fewer trips from the badgers than Grumm's own home: %+v", dests)
	}
	dests = []entity.CoachDest{{Name: "Aviak Village", World: 10, X: 9211.7, Z: 21552}, {Name: "Freeport", World: 0, X: 25273.7, Z: 15724}}
	q.CoachStops(f.tw.p, grumm, dests)
	if !dests[0].QuestStop || dests[1].QuestStop {
		t.Errorf("Grumm's stops %+v, want Aviak Village alone marked", dests)
	}

	// done: no tag
	if _, err := f.qe.SetCharacterQuest(f.sid, 20061, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	dests = []entity.CoachDest{{Name: "Aviak Village", World: 10, X: 9211.7, Z: 21552}}
	q.CoachStops(f.tw.p, grumm, dests)
	if dests[0].QuestStop {
		t.Errorf("the quest done, still %+v", dests)
	}
}
