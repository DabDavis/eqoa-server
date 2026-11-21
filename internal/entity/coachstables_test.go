package entity

import (
	"slices"
	"testing"
)

// Blackvein Deep's stable is there only while world 10 is one the server
// made: a coachman in the cave's tunnels is its stable (not the orcs' home
// whose zone it shares), its coach runs to Moradhim and Moradhim's to it, no
// other town's, home's or Freeport's; its town is a ledger of its own. The
// control: with world 10 not made, none of it.
func TestBlackveinDeepsStableRunsToMoradhim(t *testing.T) {
	deep := &NPC{Name: "Coachman Test", World: 10, SpawnX: 18760, SpawnZ: 9150}
	if CoachRunsTo("Moradhim", "Blackvein Deep") || OnCoachNetwork("Blackvein Deep") || slices.Contains(CoachTownNames(), "Blackvein Deep") {
		t.Fatal("the control: Blackvein Deep's stable with world 10 not made")
	}
	if _, named := coachLabel(deep); named {
		t.Fatal("the control: a coachman of world 10 named for a town with world 10 not made")
	}
	before := CoachTownNames()
	SetComposedWorld(10, true)
	SetCoachHomes([]CoachHome{{World: 10, WorldName: "the Homelands", Zone: "Thedruk", MinX: 18000, MinZ: 8000, MaxX: 20000, MaxZ: 10000}})
	t.Cleanup(func() { SetCoachHomes(nil); SetComposedWorld(10, false) })

	if town, named := coachLabel(deep); !named || town != "Blackvein Deep" {
		t.Errorf("the cave's coachman is %q (%v), want Blackvein Deep", town, named)
	}
	orcs := &NPC{Name: "Coachman Orc", World: 10, SpawnX: 19804, SpawnZ: 8850}
	if town, _ := coachLabel(orcs); town != "Thedruk" {
		t.Errorf("the orcs' camp's coachman is %q, want Thedruk", town)
	}
	if !OnCoachNetwork("Blackvein Deep") || !CoachRunsTo("Blackvein Deep", "Moradhim") || !CoachRunsTo("Moradhim", "Blackvein Deep") {
		t.Error("no coach between Blackvein Deep and Moradhim")
	}
	for _, town := range []string{"Freeport", "Thedruk", "Halas", "Neriak", "Rivervale"} {
		if CoachRunsTo("Blackvein Deep", town) || CoachRunsTo(town, "Blackvein Deep") {
			t.Errorf("a coach runs between Blackvein Deep and %s", town)
		}
	}
	if IsCoachHome("Blackvein Deep") {
		t.Error("Blackvein Deep is a race's home: Freeport's coach and the homes' would run to it")
	}
	// Moradhim's own routes are what they were
	for _, town := range []string{"Halas", "Rivervale", "Fayspire", "Darvar Manor"} {
		if !CoachRunsTo("Moradhim", town) {
			t.Errorf("Moradhim's coach no longer runs to %s", town)
		}
	}
	names := CoachTownNames()
	if !slices.Contains(names, "Blackvein Deep") || len(names) != len(before)+2 || !slices.IsSorted(names) {
		t.Errorf("the coach towns %d (before %d): want Blackvein Deep and Thedruk more, sorted", len(names), len(before))
	}
}

// A home whose zone bears Blackvein Deep's name is told from the stable by its
// world's name, as one bearing a game town's.
func TestAHomeNamedAsBlackveinDeepIsToldApart(t *testing.T) {
	SetComposedWorld(10, true)
	SetCoachHomes([]CoachHome{{World: 10, WorldName: "the Homelands", Zone: "Blackvein Deep", MinX: 0, MinZ: 0, MaxX: 2000, MaxZ: 2000}})
	t.Cleanup(func() { SetCoachHomes(nil); SetComposedWorld(10, false) })
	if got := CoachHomeNames(); !slices.Equal(got, []string{"Blackvein Deep (the Homelands)"}) {
		t.Errorf("the home's stable %q", got)
	}
}
