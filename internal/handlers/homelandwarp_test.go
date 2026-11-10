package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A homeland's gatekeeper is no stop of the travel network (warpRoutes): with
// the Homelands built, Odus's gatekeeper offers a client that holds them
// Freeport and not the homeland's, and the homeland's offers nothing (its
// quest flies its race to its city, one way). The control: with no server
// world built, slot 10 is an ordinary world and both are stops, as
// GetWarpDestinations alone makes them.
func TestHomelandGatekeeperIsNoStop(t *testing.T) {
	w := entity.NewWorld()
	thand := &entity.NPC{ServerID: 1, Name: "Harbormaster Thand", World: 0, SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5}
	sulon := &entity.NPC{ServerID: 2, Name: "Gatekeeper Sulon", World: 2, SpawnX: 5000, SpawnY: 60, SpawnZ: 5000}
	home := &entity.NPC{ServerID: 3, Name: "Gatekeeper Windwhistle", World: 10, SpawnX: 9170.9, SpawnY: 84.1, SpawnZ: 21636.7}
	w.NPCs[1], w.NPCs[2], w.NPCs[3] = thand, sulon, home
	every := func(uint8) bool { return true }
	labels := func(ds []entity.CoachDest) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.Name)
		}
		slices.Sort(out)
		return out
	}

	// the control: no world of the server's own
	if got := labels(warpRoutes(w, sulon, every)); !slices.Equal(got, []string{"Freeport", "Gatekeeper Windwhistle"}) {
		t.Fatalf("control: Sulon's trips %v, want Freeport and the gatekeeper of an ordinary world 10", got)
	}
	if got := labels(warpRoutes(w, home, every)); !slices.Equal(got, []string{"Freeport", "Odus"}) {
		t.Fatalf("control: an ordinary world's gatekeeper offers %v, want Freeport and Odus", got)
	}

	withServerWorlds(t, homelands())
	if got := labels(warpRoutes(w, sulon, every)); !slices.Equal(got, []string{"Freeport"}) {
		t.Errorf("Sulon offers %v with the Homelands built: a homeland is no stop", got)
	}
	if got := warpRoutes(w, home, every); got != nil {
		t.Errorf("the homeland's gatekeeper offers %v: none (its quest flies the player, one way)", labels(got))
	}
	// a client whose table lacks Odus's world: only what it can load, as before
	if got := labels(warpRoutes(w, thand, func(world uint8) bool { return world == 0 || world == 10 })); len(got) != 0 {
		t.Errorf("Thand offers %v to a client of Tunaria and the Homelands: nothing it can load is a stop", got)
	}
}
