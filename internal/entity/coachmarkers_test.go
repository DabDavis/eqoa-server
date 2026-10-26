package entity

import (
	"slices"
	"testing"
)

// The map's coach stops: each coachman of the world at its spawn, the town
// it serves, the towns its coach menu offers (every other coach town of its
// world, as GetCoachDestinations lists them) and the live game's from that
// town (EQOA-Data's coach map). Another world's coachman and a GM's copy are
// not a stop of this one.
func TestCoachMarkersAreTheWorldsCoachmen(t *testing.T) {
	w := NewWorld()
	add := func(id EntityID, name string, world uint8, x, z float32, temp bool) {
		w.NPCs[id] = &NPC{ServerID: id, Name: name, World: world, X: x + 3, Z: z, SpawnX: x, SpawnY: 54, SpawnZ: z, Temporary: temp}
	}
	add(416, "Coachman Ronks", 0, 25274, 15719, false)
	add(101428, "Coachman Billfer", 0, 4813, 17144, false)
	add(101444, "Coachman Dilga", 1, 8962, 7214, false)
	add(9000, "Coachman Ronks", 0, 100, 100, true) // a GM's #spawn copy

	got := w.CoachMarkers(0)
	if len(got) != 2 {
		t.Fatalf("%d stops in Tunaria, want Ronks and Billfer: %+v", len(got), got)
	}
	ronks := got[0]
	if ronks.Town != "Freeport" || ronks.Name != "Coachman Ronks" || !ronks.Named || ronks.ID != 416 {
		t.Fatalf("first stop %+v, want Freeport's Coachman Ronks", ronks)
	}
	if ronks.X != 25274 || ronks.Z != 15719 {
		t.Errorf("Ronks at (%v, %v), want his spawn, not where he stands", ronks.X, ronks.Z)
	}
	if !slices.Equal(ronks.Routes, []string{"Qeynos"}) {
		t.Errorf("Ronks's coach menu %v, want the other coach town of the world", ronks.Routes)
	}
	if !slices.Contains(ronks.Live, "Highpass Hold") || slices.Contains(ronks.Live, "Freeport") {
		t.Errorf("Freeport's live routes %v, want EQOA-Data's (Highpass Hold among them)", ronks.Live)
	}
	if got[1].Town != "Qeynos" || !slices.Equal(got[1].Routes, []string{"Freeport"}) {
		t.Errorf("second stop %+v, want Qeynos with Freeport", got[1])
	}
	// the control: the Rathe's coachman is the Rathe's alone, with no route
	rathe := w.CoachMarkers(1)
	if len(rathe) != 1 || rathe[0].Name != "Coachman Dilga" || len(rathe[0].Routes) != 0 {
		t.Fatalf("the Rathe's stops %+v, want Dilga with no route", rathe)
	}
	if len(w.CoachMarkers(-1)) != 3 {
		t.Errorf("every world's: %d stops, want 3", len(w.CoachMarkers(-1)))
	}
}
