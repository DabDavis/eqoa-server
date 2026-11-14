package entity

import "testing"

// A trigger's walk by a route (Ryini down her mountain): the route's points then its end, at
// the step's pace, walked (not run) under pathRunFrom; with none, the run's straight line as
// ever. The control: an NPC of another name is not set walking.
func TestWalkNPCsRouteWalksTheRouteAtItsPace(t *testing.T) {
	w := NewWorld()
	w.NPCs[900] = &NPC{ServerID: 900, Name: "Ryini", HP: 10, MaxHP: 10, X: 100, Z: 100, World: 4}
	w.NPCs[901] = &NPC{ServerID: 901, Name: "Tamis", HP: 10, MaxHP: 10, X: 101, Z: 100, World: 4}
	n := w.WalkNPCsRoute("ryini", 4, 100, 100, 50, 4, [][3]float32{{110, 5, 100}, {120, 4, 110}}, 130, 3, 130, 4)
	if n != 1 {
		t.Fatalf("%d set walking, want Ryini alone", n)
	}
	p := w.paths.byNPC[900]
	if p == nil || len(p.Points) != 3 || p.Run || p.speed() != 4 || p.Points[2].X != 130 || p.Points[0].Z != 100 {
		t.Fatalf("route %+v", p)
	}
	if w.paths.byNPC[901] != nil {
		t.Fatal("another name set walking")
	}
	w.WalkNPCsTo("ryini", 4, 100, 100, 50, 4, 140, 3, 140)
	if p := w.paths.byNPC[900]; p == nil || len(p.Points) != 1 || !p.Run {
		t.Fatalf("straight walk %+v", p)
	}
}
