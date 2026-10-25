package entity

import (
	"slices"
	"testing"
)

// A coach menu lists towns, not coachmen (F3 20261025-171410: Coachman
// Ronks offered "Coachman Droogra", "Coachman Dudley", "Coachman Frender"):
// every coachman the server's npcs holds is named for its town, the two of
// Hazinak apart. The control: a coachman of no town still shows its name.
func TestCoachMenusListTowns(t *testing.T) {
	w := NewWorld()
	for i, n := range []*NPC{
		{Name: "Coachman Ronks", SpawnX: 25273.7, SpawnZ: 15719.1},
		{Name: "Coachman Droogra", SpawnX: 25129, SpawnZ: 27156},
		{Name: "Coachman Dudley", SpawnX: 12971, SpawnZ: 4684},
		{Name: "Coachman Frender", SpawnX: 9136, SpawnZ: 12737},
		{Name: "Coachman Zelik", SpawnX: 24875, SpawnZ: 27607},
		{Name: "Coachman Wanderer", SpawnX: 1000, SpawnZ: 1000},
	} {
		n.ServerID = EntityID(i + 1)
		w.NPCs[n.ServerID] = n
	}
	var got []string
	for _, d := range w.GetCoachDestinations("Coachman Ronks", 0) {
		got = append(got, d.Name)
	}
	want := []string{"Coachman Wanderer", "Halas", "Hazinak (Evil)", "Hazinak (Good)", "Surefall Glade"}
	if !slices.Equal(got, want) {
		t.Errorf("Freeport's coach menu = %q, want %q", got, want)
	}
}

// A coachman a GM adds is named for the town it stands in (TownAt); in
// another world, or out in the wild, it keeps its own name.
func TestAGMsCoachmanIsNamedByItsTown(t *testing.T) {
	for _, tt := range []struct {
		npc  NPC
		want string
		town bool
	}{
		{NPC{Name: "Coachman Newcomer", SpawnX: 23470, SpawnZ: 6500}, "Klik'Anon", true},
		{NPC{Name: "Coachwoman Newcomer", SpawnX: 25300, SpawnZ: 15650}, "Freeport", true},
		{NPC{Name: "Coachman Lost", SpawnX: 20000, SpawnZ: 20000}, "Coachman Lost", false},
		{NPC{Name: "Coachman Abroad", World: 2, SpawnX: 23470, SpawnZ: 6500}, "Coachman Abroad", false},
	} {
		if got, town := coachLabel(&tt.npc); got != tt.want || town != tt.town {
			t.Errorf("%s at (%.0f, %.0f) world %d: %q (a town: %v), want %q (%v)",
				tt.npc.Name, tt.npc.SpawnX, tt.npc.SpawnZ, tt.npc.World, got, town, tt.want, tt.town)
		}
	}
}

// Every coachman of the table has a town of its own, and each town is one
// of the map's: two lines of one menu never read alike.
func TestCoachTownsAreDistinctAndOnTheMap(t *testing.T) {
	seen := map[string]string{}
	for coachman, town := range coachLocationNames {
		if other, dup := seen[town]; dup {
			t.Errorf("%s and %s are both %q", coachman, other, town)
		}
		seen[town] = coachman
		if !slices.ContainsFunc(coachTowns, func(c coachTown) bool { return c.name == town }) {
			t.Errorf("%s's town %q is not one of coachTowns", coachman, town)
		}
	}
	if len(coachLocationNames) != 30 {
		t.Errorf("%d coachmen named, want the server's 30 (all but Rathe's Dilga)", len(coachLocationNames))
	}
}
