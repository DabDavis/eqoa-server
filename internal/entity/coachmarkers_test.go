package entity

import (
	"slices"
	"testing"
)

// The map's coach stops: each coachman of the world at its spawn, the town
// it serves, the towns its coach menu may offer (the other coach towns of its
// world its coach runs to, CoachMenu; every other coach town of its world for
// a coachman of no town on the network) and the live game's from that town
// (EQOA-Data's coach map). Another world's coachman and a GM's copy are not a
// stop of this one.
func TestCoachMarkersAreTheWorldsCoachmen(t *testing.T) {
	w := NewWorld()
	add := func(id EntityID, name string, world uint8, x, z float32, temp bool) {
		w.NPCs[id] = &NPC{ServerID: id, Name: name, World: world, X: x + 3, Z: z, SpawnX: x, SpawnY: 54, SpawnZ: z, Temporary: temp}
	}
	add(416, "Coachman Ronks", 0, 25274, 15719, false)
	add(101428, "Coachman Billfer", 0, 4813, 17144, false)
	add(415, "Coachman Quicksteed", 0, 16781, 15356, false)
	add(8000, "Coachman Wanderer", 0, 100, 100, false) // a GM's, in no town
	add(101444, "Coachman Dilga", 1, 8962, 7214, false)
	add(9000, "Coachman Ronks", 0, 100, 100, true) // a GM's #spawn copy

	got := w.CoachMarkers(0)
	if len(got) != 4 {
		t.Fatalf("%d stops in Tunaria, want Ronks, Billfer, Quicksteed and the Wanderer: %+v", len(got), got)
	}
	stop := map[string]CoachMarker{}
	for _, m := range got {
		stop[m.Name] = m
	}
	ronks := got[1] // by town: Coachman Wanderer, Freeport, Highpass Hold, Qeynos
	if ronks.Town != "Freeport" || ronks.Name != "Coachman Ronks" || !ronks.Named || ronks.ID != 416 {
		t.Fatalf("second stop %+v, want Freeport's Coachman Ronks", ronks)
	}
	if ronks.X != 25274 || ronks.Z != 15719 {
		t.Errorf("Ronks at (%v, %v), want his spawn, not where he stands", ronks.X, ronks.Z)
	}
	if !slices.Equal(ronks.Routes, []string{"Highpass Hold"}) || !ronks.Routed {
		t.Errorf("Ronks's coach menu %v (routed %v), want Highpass Hold alone: Freeport's coach never ran to Qeynos", ronks.Routes, ronks.Routed)
	}
	if !slices.Contains(ronks.Live, "Highpass Hold") || slices.Contains(ronks.Live, "Freeport") {
		t.Errorf("Freeport's live routes %v, want EQOA-Data's (Highpass Hold among them)", ronks.Live)
	}
	if q := stop["Coachman Quicksteed"]; q.Town != "Highpass Hold" || !slices.Equal(q.Routes, []string{"Freeport"}) {
		t.Errorf("Quicksteed %+v, want Highpass Hold with Freeport", q)
	}
	if b := stop["Coachman Billfer"]; b.Town != "Qeynos" || len(b.Routes) != 0 || !b.Routed {
		t.Errorf("Billfer %+v, want Qeynos with no stop here its coach runs to", b)
	}
	// the control: a coachman of no town offers every other coach town of the
	// world, and none of the network's runs to him
	if wd := stop["Coachman Wanderer"]; wd.Named || wd.Routed || !slices.Equal(wd.Routes, []string{"Freeport", "Highpass Hold", "Qeynos"}) {
		t.Errorf("the Wanderer %+v, want every other coach town of the world", wd)
	}
	// the Rathe's coachman is the Rathe's alone, with no route
	rathe := w.CoachMarkers(1)
	if len(rathe) != 1 || rathe[0].Name != "Coachman Dilga" || len(rathe[0].Routes) != 0 || rathe[0].Routed {
		t.Fatalf("the Rathe's stops %+v, want Dilga with no route", rathe)
	}
	if len(w.CoachMarkers(-1)) != 5 {
		t.Errorf("every world's: %d stops, want 5", len(w.CoachMarkers(-1)))
	}
}
