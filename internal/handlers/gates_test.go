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
