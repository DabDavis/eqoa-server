package entity

import "testing"

// IsTourHost names the host alone (petition 2368: HandleInteract asks the
// quests before the host's sleigh ride, and only for the host).
func TestIsTourHost(t *testing.T) {
	r := newTourRig(t)
	if !r.w.IsTourHost(r.npc("Santa Claus")) {
		t.Fatal("Santa Claus is not the tour's host")
	}
	for _, n := range r.w.NPCs {
		if n.Name != "Santa Claus" && r.w.IsTourHost(n) {
			t.Fatalf("%s hosts the tour", n.Name)
		}
	}
	if r.w.IsTourHost(nil) {
		t.Fatal("nil hosts the tour")
	}
}
