package handlers

import (
	"strings"
	"testing"
)

// The boats (petition 1034): every dockmaster's trip lands in front of the
// dockmaster of the harbour it names, who has trips of his own, so no boat
// strands a player; Freeport's two dockmasters offer the five harbours
// EQOA-Data's map gives them, and a boat back to Freeport lands on the one
// whose boats go that way (an Odus harbour's on Yarnning, the others' on
// Endol). The control: a dockmaster the map gives no boat stays one with
// nothing to say.
func TestTheBoatsLandOnDockmastersWithBoats(t *testing.T) {
	harbour := map[string]string{
		"Dockmaster Yarnning": "Freeport", "Dockmaster Endol": "Freeport", "Dockmaster D'Eleth": "Hazinak",
		"Dockmaster Thinda": "Highbourne", "Dockmaster Killeron": "Arcadin", "Dockmaster Bibbin": "Stone of Morathals",
		"Dockmaster Geben": "Bogman Village",
	}
	odus := map[string]bool{"Arcadin": true, "Stone of Morathals": true}
	for from := range harbour {
		if !IsWaysender(from) {
			t.Errorf("%s has no boats", from)
		}
		for _, r := range wayRoutes[from] {
			if harbour[r.Arrive] != r.To {
				t.Errorf("%s's boat to %s lands on %s, the dockmaster of %q", from, r.To, r.Arrive, harbour[r.Arrive])
			}
			if len(wayRoutes[r.Arrive]) == 0 {
				t.Errorf("%s's boat to %s lands on %s, who has no boats", from, r.To, r.Arrive)
			}
			if r.To == "Freeport" && (r.Arrive == "Dockmaster Yarnning") != odus[harbour[from]] {
				t.Errorf("%s's boat to Freeport lands on %s", from, r.Arrive)
			}
		}
	}
	var fp []string
	for _, d := range []string{"Dockmaster Yarnning", "Dockmaster Endol"} {
		for _, r := range wayRoutes[d] {
			fp = append(fp, r.To)
		}
	}
	if got := strings.Join(fp, ", "); got != "Arcadin, Stone of Morathals, Hazinak, Highbourne, Bogman Village" {
		t.Errorf("Freeport's boats: %s", got)
	}
	if IsWaysender("Dockmaster Twallny") || IsWaysender("dock master") {
		t.Error("the control: a dockmaster the map gives no boat has trips")
	}
}
