package handlers

import (
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A sphere of light known by its place (lightsphere.go, work board 773): a
// talk to the nameless NPC standing at a row's place opens its line with
// "Touch the light.", and the answer sends the player in front of the other
// sphere, facing it. The controls: a nameless NPC at no row's place has
// nothing to say, and Exit sends no one.
func TestALightSphereSendsToItsPair(t *testing.T) {
	r := newPortalRig(t)
	rows := []database.LightSphere{{ID: 1, World: 0, X: 5001, Z: 5000, Line: "This sphere of light is used to teleport to the test house.",
		Answer: "Touch the light.", ToWorld: 0, ToX: 6000, ToY: 60, ToZ: 6000, ToFacing: 64}}
	r.ih.SetLightSpheres(func() ([]database.LightSphere, error) { return rows, nil })
	sphere := &entity.NPC{ServerID: 9101, Name: "", NPCType: 384, X: 5001.5, Y: 50, Z: 5000, SpawnX: 5001.5, SpawnZ: 5000,
		Level: 1, HP: 100, MaxHP: 100}
	other := &entity.NPC{ServerID: 9102, Name: "", NPCType: 384, X: 4996, Y: 50, Z: 5000, SpawnX: 4996, SpawnZ: 5000,
		Level: 1, HP: 100, MaxHP: 100}
	r.gw.NPCs[sphere.ServerID], r.gw.NPCs[other.ServerID] = sphere, other

	r.talk(r.anna, other)
	r.want("Anna, at a nameless NPC at no sphere's place", r.msgs(r.anna))

	r.talk(r.anna, sphere)
	got := r.msgs(r.anna)
	r.want("Anna, at the sphere", got, "ask#0:"+rows[0].Line+"|Touch the light.")
	r.pick(r.anna, 0, 255) // Exit
	r.want("Anna, Exit", r.msgs(r.anna))

	r.talk(r.anna, sphere)
	r.msgs(r.anna)
	r.pick(r.anna, 0, 0)
	r.want("Anna, touching the light", r.msgs(r.anna), "teleport")
	// in front of the other sphere (facing 64: +x), facing back at it
	if math.Abs(float64(r.pa.X-(6000+bindReach))) > 0.01 || math.Abs(float64(r.pa.Z-6000)) > 0.01 {
		t.Errorf("landed at (%.2f, %.2f), want (%.2f, 6000)", r.pa.X, r.pa.Z, 6000+bindReach)
	}
	if f := float64(r.pa.Facing); math.Abs(f-1.5*math.Pi) > 0.01 {
		t.Errorf("facing %.3f, want 3/2 pi", f)
	}
}

// An NPC beside a sphere's place is not the sphere (Heirloom petition 2849):
// Saerk stands on the floor over his tower's upper orb of light, 2.1 from its
// place across, and a talk to him opened the orb's box instead of his quest.
// The orb itself still answers. (The rig's places, near Anna: Saerk 2.1 from
// the orb's place across, as on the tower.)
func TestANPCBesideASphereIsNotIt(t *testing.T) {
	r := newPortalRig(t)
	rows := []database.LightSphere{{ID: 8, World: 0, X: 5001, Z: 5000,
		Line: "This orb of light carries you down to the ground floor of the tower.", Answer: "Touch the light.",
		ToWorld: 0, ToX: 21248.572, ToY: 245, ToZ: 13515.552, ToFacing: 208}}
	r.ih.SetLightSpheres(func() ([]database.LightSphere, error) { return rows, nil })
	orb := &entity.NPC{ServerID: 9201, Name: "orb of light", NPCType: 384, X: 5001, Y: 50, Z: 5000,
		SpawnX: 5001, SpawnZ: 5000, Level: 1, HP: 100, MaxHP: 100}
	saerk := &entity.NPC{ServerID: 9202, Name: "Saerk", NPCType: 384, X: 5001.1, Y: 54, Z: 5002.1,
		SpawnX: 5001.1, SpawnZ: 5002.1, Level: 50, HP: 100, MaxHP: 100}
	r.gw.NPCs[orb.ServerID], r.gw.NPCs[saerk.ServerID] = orb, saerk

	r.talk(r.anna, saerk)
	for _, m := range r.msgs(r.anna) {
		if m == "ask#0:"+rows[0].Line+"|Touch the light." {
			t.Fatalf("Anna, at Saerk: got the orb's box")
		}
	}
	r.talk(r.anna, orb)
	r.want("Anna, at the orb", r.msgs(r.anna), "ask#0:"+rows[0].Line+"|Touch the light.")
}
