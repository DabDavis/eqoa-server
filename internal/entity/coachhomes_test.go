package entity

import (
	"slices"
	"testing"
)

// testHomes are three homes of a server's world 10: two whose zones' names are
// their own, and the minotaurs', whose zone bears the name of a coach town of
// the game (Coachman Ungar's Gerntar Mines, in Tunaria).
func testHomes() []CoachHome {
	return []CoachHome{
		{World: 10, WorldName: "the Homelands", Zone: "Aviak Village", MinX: 8000, MinZ: 20000, MaxX: 10000, MaxZ: 22000},
		{World: 10, WorldName: "the Homelands", Zone: "West Plateau", MinX: 6000, MinZ: 2000, MaxX: 8000, MaxZ: 4000},
		{World: 10, WorldName: "the Homelands", Zone: "Gerntar Mines", MinX: 12000, MinZ: 26000, MaxX: 14000, MaxZ: 28000},
	}
}

func withHomes(t *testing.T) {
	t.Helper()
	SetComposedWorld(10, true)
	SetCoachHomes(testHomes())
	t.Cleanup(func() { SetCoachHomes(nil); SetComposedWorld(10, false) })
}

// A home's stable is its zone's name, from the data given; one whose name a
// coach town of the game bears is told from it by its world's name. A home's
// coach runs to Freeport and to every other home, Freeport's to every home,
// and no other town's to any; the game's own routes are what they were. With
// no homes (the control) none of it is so.
func TestTheHomesStablesAreANetworkOfTheirOwn(t *testing.T) {
	if IsCoachHome("Aviak Village") || CoachRunsTo("Freeport", "Aviak Village") || OnCoachNetwork("Aviak Village") {
		t.Fatal("the control: a home's stable with no homes set")
	}
	before := CoachTownNames()
	withHomes(t)
	want := []string{"Aviak Village", "Gerntar Mines (the Homelands)", "West Plateau"}
	if got := CoachHomeNames(); !slices.Equal(got, want) {
		t.Fatalf("the homes' stables %q, want %q", got, want)
	}
	if w, ok := CoachHomeWorld("West Plateau"); !ok || w != "the Homelands" {
		t.Errorf("West Plateau's world %q (%v)", w, ok)
	}
	for _, h := range want {
		if !OnCoachNetwork(h) || !CoachRunsTo(h, CoachHomeHub) || !CoachRunsTo(CoachHomeHub, h) {
			t.Errorf("%s: no coach to and from %s", h, CoachHomeHub)
		}
		for _, o := range want {
			if got := CoachRunsTo(h, o); got != (h != o) {
				t.Errorf("a coach from %s to %s: %v", h, o, got)
			}
		}
		for _, town := range []string{"Halas", "Qeynos", "Highpass Hold", "Gerntar Mines", "Oggok"} {
			if CoachRunsTo(h, town) || CoachRunsTo(town, h) {
				t.Errorf("a coach runs between %s and %s: only Freeport's and the homes' do", h, town)
			}
		}
	}
	// the game's own are what they were
	if !CoachRunsTo("Freeport", "Highpass Hold") || CoachRunsTo("Freeport", "Qeynos") || !CoachRunsTo("Oggok", "Dark Solace") {
		t.Error("the live game's routes changed")
	}
	if IsCoachHome("Gerntar Mines") {
		t.Error("Tunaria's Gerntar Mines is a home's stable")
	}
	names := CoachTownNames()
	if len(names) != len(before)+3 {
		t.Errorf("%d coach towns with the homes, %d without: want three more", len(names), len(before))
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("two stables are named %q: a ledger names one", n)
		}
		seen[n] = true
	}
	if !slices.IsSorted(names) {
		t.Error("the coach towns are not sorted")
	}
}

// A coachman standing in a world the server made is the stable of the home he
// stands in, whatever he is called; one standing in no home there is no
// town's. The control: the same name in Tunaria is Freeport's Ronks.
func TestACoachmanOfAHomeIsItsStable(t *testing.T) {
	withHomes(t)
	for _, c := range []struct {
		npc   NPC
		town  string
		named bool
	}{
		{NPC{Name: "Coachman Harrek", World: 10, SpawnX: 9211.7, SpawnZ: 21547.1}, "Aviak Village", true},
		{NPC{Name: "Coachman Ronks", World: 10, SpawnX: 6947, SpawnZ: 3300}, "West Plateau", true},
		{NPC{Name: "Coachman Brakk", World: 10, SpawnX: 13000, SpawnZ: 27000}, "Gerntar Mines (the Homelands)", true},
		{NPC{Name: "Coachman Nowhere", World: 10, SpawnX: 100, SpawnZ: 100}, "Coachman Nowhere", false},
		{NPC{Name: "Coachman Ronks", World: 0, SpawnX: 25273.7, SpawnZ: 15719.1}, "Freeport", true},
		{NPC{Name: "Coachman Ungar", World: 0, SpawnX: 13038, SpawnZ: 27506}, "Gerntar Mines", true},
	} {
		if town, named := CoachTown(&c.npc); town != c.town || named != c.named {
			t.Errorf("%s of world %d at (%.0f, %.0f): %q (%v), want %q (%v)", c.npc.Name, c.npc.World, c.npc.SpawnX, c.npc.SpawnZ,
				town, named, c.town, c.named)
		}
	}
}

// Two homes of one name are told apart, a third that still has none of its
// own is left out; a zone with no name is no home.
func TestHomesOfOneNameAreToldApartOrLeftOut(t *testing.T) {
	SetCoachHomes([]CoachHome{
		{World: 10, WorldName: "the Homelands", Zone: "The Glade", MinX: 0, MinZ: 0, MaxX: 2000, MaxZ: 2000},
		{World: 12, WorldName: "the Far Homes", Zone: "The Glade", MinX: 0, MinZ: 0, MaxX: 2000, MaxZ: 2000},
		{World: 12, WorldName: "the Far Homes", Zone: "The Glade", MinX: 2000, MinZ: 0, MaxX: 4000, MaxZ: 2000},
		{World: 12, WorldName: "the Far Homes", Zone: "", MinX: 4000, MinZ: 0, MaxX: 6000, MaxZ: 2000},
	})
	t.Cleanup(func() { SetCoachHomes(nil) })
	if got, want := CoachHomeNames(), []string{"The Glade", "The Glade (the Far Homes)"}; !slices.Equal(got, want) {
		t.Errorf("the stables %q, want %q", got, want)
	}
}
