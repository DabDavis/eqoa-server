package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

func warpWorld(t *testing.T) *entity.World {
	t.Helper()
	w := entity.NewWorld()
	for i, n := range []struct {
		name  string
		world uint8
		x     float32
	}{
		{"Harbormaster Thand", 0, 25377},
		{"Harbormaster Machado", 9, 13000},
		{"Coachman Ronks", 0, 25273}, // NOT a warp NPC
	} {
		w.NPCs[entity.EntityID(i+1)] = &entity.NPC{
			Name: n.name, World: n.world,
			SpawnX: n.x, SpawnY: 54, SpawnZ: 15000,
		}
	}
	return w
}

// The account pages offer what a Harbormaster offers. Routes are keyed by the
// NPC's own NAME, not its label: a label is display text and is rewritten the
// moment a world gets a real name, which would break every saved route.
func TestWarpDestinationsAreKeyedByNPCName(t *testing.T) {
	dests := WarpDestinations(warpWorld(t))

	var names []string
	for _, d := range dests {
		names = append(names, d.Name)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"Harbormaster Thand", "Harbormaster Machado"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q missing from the warp list: %v", want, names)
		}
	}
	if strings.Contains(joined, "Coachman") {
		t.Error("a coachman is not a cross-world route and must not be offered")
	}
}

// A Harbormaster leaves ITSELF out, because a player standing at one cannot
// usefully travel to it. A web page is not standing anywhere, so Freeport has to
// be reachable too.
func TestWarpDestinationsIncludeEveryStop(t *testing.T) {
	if len(WarpDestinations(warpWorld(t))) != 2 {
		t.Error("the web list must contain every warp stop, including Freeport")
	}
}

// Resolving keys off the travel NPCs, and ONLY off them — otherwise this would
// be "teleport to any NPC by name", which is a different and much larger power
// than the one a Harbormaster's menu grants.
func TestResolveTeleDestAcceptsWarpNPCsOnly(t *testing.T) {
	gw := warpWorld(t)

	x, _, _, world, via, ok := ResolveTeleDest(gw, "Harbormaster Machado")
	if !ok {
		t.Fatal("a warp NPC route did not resolve")
	}
	if world != 9 || x != 13000 {
		t.Errorf("resolved to world %d x=%.0f, want world 9 at the NPC's spawn", world, x)
	}
	if via == "" {
		t.Error("the reply must name where the player went")
	}

	if _, _, _, _, _, ok := ResolveTeleDest(gw, "Coachman Ronks"); ok {
		t.Error("resolved a non-warp NPC; the allowed set is the travel NPCs")
	}
	if _, _, _, _, _, ok := ResolveTeleDest(gw, "Archivist Vaal"); ok {
		t.Error("resolved an arbitrary NPC by name")
	}
}

// The #tele shortcuts still work, and still win over the NPC lookup — the web
// route was added to that resolver rather than replacing it.
func TestResolveTeleDestStillHonoursShortcuts(t *testing.T) {
	if _, _, _, _, _, ok := ResolveTeleDest(warpWorld(t), "freeport"); !ok {
		t.Error("the #tele shortcut stopped resolving")
	}
}

// TestWorldSixTakesTheOrdinaryGuards replaces two tests that pinned the setup
// screen's exemptions from them.
//
// World 6 used to be a DESTINATION and not a PLACE: ValidateTeleport waved it
// past the world floor and CoherentWorld, because a screen that replaces the
// game has no terrain to stand on. With the screen gone the exemption must go
// too -- otherwise slot 6 stays the one world where a position under the floor
// is accepted, and the first person to notice is a player who falls forever.
func TestWorldSixTakesTheOrdinaryGuards(t *testing.T) {
	if _, err := ValidateTeleport(0, entity.WorldFloorY-100, 0, 6); err == nil {
		t.Error("ValidateTeleport accepted a position under the world floor in " +
			"world 6; the setup-screen exemption outlived the setup screen")
	}
	// The control, so this is testing the guard and not a typo in the world id.
	if _, err := ValidateTeleport(0, entity.WorldFloorY-100, 0, 0); err == nil {
		t.Error("ValidateTeleport accepted an under-floor position in world 0")
	}
}
