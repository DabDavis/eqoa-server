package entity

import "testing"

// A homeland's gatekeeper (in a world the server composed of the disc's own
// zones: the races' homelands) flies its own people out by its quest, one
// way: no other warp NPC offers it and the account pages' routes leave it out;
// a GM's own list, with no one asking, still has it. A world that is not
// composed is offered as ever (the control).
func TestAHomelandGatekeeperIsNoStop(t *testing.T) {
	w := gukWorld()
	w.NPCs[7] = &NPC{ServerID: 7, Name: "Gatekeeper Windwhistle", World: 10, SpawnX: 9170.9, SpawnY: 84.1, SpawnZ: 21636.7}
	offered := func(by string) bool {
		for _, d := range w.GetWarpDestinations(by, nil) {
			if d.World == 10 {
				return true
			}
		}
		return false
	}
	routed := func() bool {
		for _, p := range w.WarpPoints(nil) {
			if p.World == 10 {
				return true
			}
		}
		return false
	}
	if !offered("Harbormaster Thand") || !routed() {
		t.Fatal("the control: a gatekeeper in a world nobody composed is a stop like any other")
	}
	SetComposedWorld(10, true)
	defer SetComposedWorld(10, false)
	if !Composed(10) || Composed(8) {
		t.Fatalf("Composed(10) %v, Composed(8) %v", Composed(10), Composed(8))
	}
	if offered("Harbormaster Thand") || offered("Gatekeeper Krup") {
		t.Error("a harbormaster or a gatekeeper offered a flight into a homeland")
	}
	if routed() {
		t.Error("the account pages' routes name a homeland's gatekeeper")
	}
	if !offered("") {
		t.Error("a GM's own list (no one asking) lost the homeland's gatekeeper")
	}
	if d := w.GetWarpDestinations("Harbormaster Thand", nil); len(d) != 1 || d[0].World != 8 {
		t.Errorf("Thand offers %+v, want world 8's Krup alone", d)
	}
}
