package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Gerntar Mines' gatekeeper sends a player to the Rathe, in front of Kuggin, and
// asks first; the doors in the cyclops' walls send a player through to each
// other's open side, in the other world.
func TestTheGatesBetweenTunariaAndTheRathe(t *testing.T) {
	tw := newTalkWorld(t, 13041.5, 27510)
	furyfoot := tw.add(1, &entity.NPC{ServerID: 701, Name: "Gatekeeper Furyfoot", NPCType: 384, SpawnX: 13041.5, SpawnY: 60, SpawnZ: 27506, X: 13041.5, Y: 60, Z: 27506})
	tw.add(2, &entity.NPC{ServerID: 702, Name: "Gatekeeper Kuggin", World: 1, NPCType: 384, SpawnX: 9617.25, SpawnY: 54.12, SpawnZ: 5705.45, X: 9617.25, Y: 54.12, Z: 5705.45})
	tw.talk(furyfoot)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("Furyfoot asks %q", got)
	}
	tw.pick(furyfoot, 0)
	x, z, world, moved := teleported(tw.s)
	if !moved || world != 1 || dist2(x, z, 9617.25, 5705.45) > 9 {
		t.Errorf("Furyfoot sent the player to (%.1f, %.1f) world %d (%v), want beside Kuggin in the Rathe", x, z, world, moved)
	}

	for _, c := range []struct {
		name     string
		world    uint8
		x, y, z  float32
		toWorld  uint8
		toX, toZ float32
	}{
		{"Door to Urglunt's Gate", 0, 11419.9, 80.06, 21408, 1, 9053.8, 2110.1},
		{"Door to Urglunt's Wall", 1, 9053.8, 52.55, 2106.08, 0, 11419.9, 21404},
	} {
		tw := newTalkWorld(t, c.x, c.z)
		tw.p.World, tw.p.Y = c.world, c.y
		door := tw.add(3, &entity.NPC{ServerID: 703, Name: c.name, World: c.world, NPCType: 384, SpawnX: c.x, SpawnY: c.y, SpawnZ: c.z, X: c.x, Y: c.y, Z: c.z})
		tw.talk(door)
		if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
			t.Fatalf("%s asks %q", c.name, got)
		}
		tw.pick(door, 0)
		x, z, world, moved := teleported(tw.s)
		if !moved || world != c.toWorld || x != c.toX || z != c.toZ {
			t.Errorf("%s sent the player to (%.1f, %.1f) world %d (%v)", c.name, x, z, world, moved)
		}
	}
}

func dist2(ax, az, bx, bz float32) float32 {
	return (ax-bx)*(ax-bx) + (az-bz)*(az-bz)
}

// The Archons' towers: two portals at the back of each tower's ground floor,
// one up to its Archon and one up to its Primeval (ticket 1196).
func TestTheArchonTowersPortals(t *testing.T) {
	for _, c := range []struct {
		name                string
		x, z, toX, toY, toZ float32
	}{
		{"Portal to Archon Aereth", 4692, 2512, 4702, 110.5, 2492},
		{"Earth Primeval Portal", 4708, 2512, 4700, 84, 2496},
		{"Portal to Archon Vetash", 4708, 2330, 4700, 106.5, 2357},
		{"Fire Primeval Portal", 4692, 2330, 4700, 85, 2361},
	} {
		tw := newTalkWorld(t, c.x, c.z)
		tw.p.World, tw.p.Y = 1, 66
		portal := tw.add(3, &entity.NPC{ServerID: 703, Name: c.name, World: 1, NPCType: 384, SpawnX: c.x, SpawnY: 66, SpawnZ: c.z})
		tw.talk(portal)
		if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
			t.Fatalf("%s asks %q", c.name, got)
		}
		tw.pick(portal, 0)
		x, z, world, moved := teleported(tw.s)
		if !moved || world != 1 || dist2(x, z, c.toX, c.toZ) > 1 {
			t.Errorf("%s sent the player to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f) in the Rathe", c.name, x, z, world, moved, c.toX, c.toZ)
		}
	}
}

// The Dead Hills' mausoleum portal is a travel NPC now, as Zizclank beside it is
// (ticket 1197): it answers, and takes a player of level 50 to the Plane of
// Disease (the wiki's level 51 since 2026-11-13). The control: below 51, it does not offer.
func TestTheMausoleumPortalTalks(t *testing.T) {
	if !IsWaysender("Mausoleum Portal") {
		t.Fatal("the portal is not a travel NPC")
	}
	r := wayRoutes["Mausoleum Portal"]
	if len(r) != 1 || r[0].To != "the Plane of Disease" || r[0].At != podArrival || r[0].MinLevel != 51 {
		t.Errorf("its route is %+v", r)
	}
	z := wayRoutes["Necrologist Zizclank"]
	if len(z) != 1 || z[0].To != r[0].To || z[0].At != r[0].At || z[0].MinLevel != r[0].MinLevel {
		t.Errorf("Zizclank's route %+v and the portal's %+v differ", z, r)
	}
}
