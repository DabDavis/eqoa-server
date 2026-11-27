package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
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
	f.sign(homesMenuAviak)
	m := wayMark(t, f)
	if m == nil || m.npc != "Coachman Ronks" || m.world != 0 || len(m.quests) != 1 ||
		m.quests[0].detail != "Ride Coachman Ronks's coach to "+homesMenuAviak || m.quests[0].line != "Kill three snow badgers in Aviak Village." {
		t.Fatalf("in Freeport the way mark %+v, want Ronks's ride to Aviak Village", m)
	}

	// in West Plateau, a home of the same world
	f.tw.p.World, f.tw.p.X, f.tw.p.Z = 10, 6950, 3310
	m = wayMark(t, f)
	if m == nil || m.npc != "Coachman Grumm" || m.world != 10 || m.quests[0].detail != "Ride Coachman Grumm's coach to "+homesMenuAviak {
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

// The coach menu puts " ?" at the end of the stop of a step under way, in the
// coachman's own words after his period (Ronks's "Get me a horse to Aviak
// Village. ?": petition 3191), and
// after no other; from a home with no stop in the step's home the stop the
// way goes on by (Freeport). The control: with the step done, no " ?".
func TestCoachMenuMarksTheQuestsStop(t *testing.T) {
	f := coachQuestFixture(t)
	was := questEngine.Load()
	questEngine.Store(f.qe)
	t.Cleanup(func() { questEngine.Store(was) })
	book := ledgerBook{f.tw.p.CharServerID: {"Freeport": true, homesMenuAviak: true, homesMenuWest: true}}
	f.tw.h.ledger = book
	f.tw.h.isGM = func(*session.Session) bool { return false }
	for _, town := range []string{"Freeport", homesMenuAviak, homesMenuWest} {
		f.sign(town)
	}
	ronks := f.tw.at(1, 416)
	f.tw.s.Queue().GetPending()
	f.tw.talk(ronks)
	got := optionBox(t, f.tw.s)
	words := coachWordsByName["Coachman Ronks"]
	want := []string{words.labelTagged(entity.CoachDest{Name: homesMenuAviak}, questStopTag), words.label(entity.CoachDest{Name: homesMenuWest})}
	if !slices.Equal(got, want) {
		t.Fatalf("Ronks's menu %q, want %q", got, want)
	}
	if got[0] != "Get me a horse to Aviak Village (Homelands). ?" {
		t.Errorf("the tag at the line's end, after the period: %q", got[0])
	}

	// the stop the way goes on by: Grumm's coach runs to the homes and Freeport
	q := f.qe
	grumm := f.tw.w.NPCs[9002]
	dests := []entity.CoachDest{{Name: "Freeport", World: 0, X: 25273.7, Z: 15724}, {Name: homesMenuGerntar, World: 10, X: 13000, Z: 27000}}
	q.CoachStops(f.tw.p, grumm, dests)
	if dests[0].QuestStop || dests[1].QuestStop {
		t.Errorf("neither stop is fewer trips from the badgers than Grumm's own home: %+v", dests)
	}
	dests = []entity.CoachDest{{Name: homesMenuAviak, World: 10, X: 9211.7, Z: 21552}, {Name: "Freeport", World: 0, X: 25273.7, Z: 15724}}
	q.CoachStops(f.tw.p, grumm, dests)
	if !dests[0].QuestStop || dests[1].QuestStop {
		t.Errorf("Grumm's stops %+v, want Aviak Village alone marked", dests)
	}

	// done: no tag
	if _, err := f.qe.SetCharacterQuest(f.sid, 20061, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	dests = []entity.CoachDest{{Name: homesMenuAviak, World: 10, X: 9211.7, Z: 21552}}
	q.CoachStops(f.tw.p, grumm, dests)
	if dests[0].QuestStop {
		t.Errorf("the quest done, still %+v", dests)
	}
}

// A stop's tag ends the line in every form (petition 3191): after a filmed
// coachman's period or question mark, and after a plain stop's fee, so the
// client's yellow trailing " ?" (vanilla) is always the line's last two runes.
func TestQuestStopTagEndsTheLine(t *testing.T) {
	d := entity.CoachDest{Name: "Fayspire", Fee: 40}
	for _, c := range []struct{ got, want string }{
		{coachWordsByName["Coachman Ronks"].labelTagged(entity.CoachDest{Name: "Aviak Village"}, questStopTag), "Get me a horse to Aviak Village. ?"},
		{coachWordsByName["Coachman Huff"].labelTagged(d, questStopTag), "How about one to Fayspires (40 tunar)? ?"},
		{coachWordsByName["Coachman Frender"].labelTagged(d, questStopTag), "Fayspire (40 tunar) ?"},
		{coachWordsByName["Coachman Huff"].label(d), "How about one to Fayspires (40 tunar)?"},
		// past 63 with the mark: the period goes, then the tail, never the mark
		{coachWordsByName["Coachman Dudley"].labelTagged(entity.CoachDest{Name: "Castle Lightwolf", Fee: 40}, questStopTag), "Travel sir, I would like to go to Castle Lightwolf (40 tunar) ?"},
		{coachWordsByName["Coachman Dudley"].labelTagged(entity.CoachDest{Name: "Gerntar Mines (the Homelands)", Fee: 120}, questStopTag), "Travel sir, I would like to go to Gerntar Mines (the Homeland ?"},
		{coachWordsByName["Coachman Dudley"].label(entity.CoachDest{Name: "Castle Lightwolf", Fee: 40}), "Travel sir, I would like to go to Castle Lightwolf (40 tunar)."},
	} {
		if c.got != c.want {
			t.Errorf("%q, want %q", c.got, c.want)
		}
		if len(c.got) > 63 || strings.Contains(c.got, "|") {
			t.Errorf("%q breaks the PS2 TextBox's rules", c.got)
		}
	}
}

// A coachman whose town's faction holds the player kill-on-sight (at or below
// -350: he would refuse it) is no quest's way for it (Heirloom petition 3191:
// "the questionmark should not appear on good coaches if you are evil,
// likewise if you are good, it should not direct you to evil coaches"): no way
// mark over him, and no " ?" on a stop whose coachman would refuse it. The
// controls: at -349 both are back.
func TestQuestWayAvoidsCoachmenWhoRefuse(t *testing.T) {
	f := coachQuestFixture(t)
	f.sign(homesMenuAviak)
	f.tw.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "coachman ronks", World: -1, Faction: "Citizens of Freeport"},
		{Name: "coachman harrek", World: -1, Faction: "Citizens of Bastable"},
	})
	f.tw.p.Factions = map[string]int{"Citizens of Freeport": -350}
	if m := wayMark(t, f); m != nil {
		t.Errorf("Ronks would refuse the player: the way mark %+v", m)
	}
	f.tw.p.Factions["Citizens of Freeport"] = -349
	if m := wayMark(t, f); m == nil || m.npc != "Coachman Ronks" {
		t.Fatalf("the control: at -349 the way mark %+v, want Ronks's", m)
	}
	// the stop's coachman at Aviak Village holds the player kill-on-sight: no way there either
	f.tw.p.Factions["Citizens of Bastable"] = -500
	if m := wayMark(t, f); m != nil {
		t.Errorf("Harrek would refuse the player at the stop: the way mark %+v", m)
	}

	// the menu: Grumm's stop at Aviak Village, Harrek's, unmarked while he refuses
	grumm := f.tw.w.NPCs[9002]
	dests := []entity.CoachDest{{Name: homesMenuAviak, World: 10, X: 9211.7, Z: 21552, Coachman: 9001}}
	f.qe.CoachStops(f.tw.p, grumm, dests)
	if dests[0].QuestStop {
		t.Errorf("a refusing coachman's stop marked: %+v", dests)
	}
	f.tw.p.Factions["Citizens of Bastable"] = 0
	f.qe.CoachStops(f.tw.p, grumm, dests)
	if !dests[0].QuestStop {
		t.Errorf("the control: Harrek serves the player, the stop %+v unmarked", dests)
	}
}
