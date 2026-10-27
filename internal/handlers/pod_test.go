package handlers

import (
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// boxSaid is the text of the last dialogue box (0x0015) queued for s.
func boxSaid(s *session.Session) string {
	var box string
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) > 6 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeDialogueBox {
			box = wideText(d[6:])
		}
	}
	return box
}

// Necrologist Zizclank, in the Rathe's Dead Hills (world 1), sends a player
// of level 50 or more to the Plane of Disease (world 5), landing just in
// front of the portal players arrive at, and tells anyone below that they are
// too weak (F3 pass 17, "unable to travel to PoD"). The controls: No sends
// nobody, and his assistant, who has no route, has nothing to say.
func TestZizclankSendsTheStrongToPoD(t *testing.T) {
	tw := newTalkWorld(t, 4283, 6412)
	tw.p.World, tw.p.Y = 1, 97.8
	ziz := tw.add(1, &entity.NPC{ServerID: 1, Name: "Necrologist Zizclank", World: 1, SpawnX: 4280, SpawnY: 97.8, SpawnZ: 6412, SpawnFacing: math.Pi / 2})
	xep := tw.add(2, &entity.NPC{ServerID: 2, Name: "Necrologist Xeptip", World: 1, SpawnX: 4280, SpawnY: 97.2, SpawnZ: 6396, SpawnFacing: math.Pi / 2})
	tw.w.NPCs[3] = &entity.NPC{ServerID: 3, Name: "Portal to the Dead Hills", World: 5, SpawnX: 5063, SpawnY: -25.3, SpawnZ: 3881, SpawnFacing: math.Pi}

	tw.p.Level = 49
	tw.talk(ziz)
	if box := boxSaid(tw.s); !strings.Contains(box, "too weak") || !strings.Contains(box, "the Plane of Disease") {
		t.Fatalf("a level 49 was told %q, want too weak for the Plane of Disease", box)
	}
	tw.s.Queue().GetPending()
	tw.pick(ziz, 0)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("a level 49 was sent to the Plane of Disease")
	}

	tw.p.Level = 50
	tw.talk(ziz)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("a level 50 was offered %q, want Yes and No", got)
	}
	tw.pick(ziz, 1)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("the control: No sent the player")
	}
	tw.talk(ziz)
	tw.s.Queue().GetPending()
	tw.pick(ziz, 0)
	x, z, world, moved := teleported(tw.s)
	// the portal faces north (pi): its front is 3 to the north, lower z
	if !moved || world != 5 || math.Abs(float64(x-5063)) > 0.01 || math.Abs(float64(z-(3881-bindReach))) > 0.01 {
		t.Errorf("Yes sent the player to (%.1f, %.1f) world %d (%v), want (5063, %.0f) in world 5, in front of the portal",
			x, z, world, moved, 3881-bindReach)
	}
	if tw.p.World != 5 {
		t.Errorf("the server filed the player in world %d, want 5", tw.p.World)
	}

	tw.s.Queue().GetPending()
	tw.p.World = 1
	tw.talk(xep)
	if box, opts := boxSaid(tw.s), len(tw.s.Queue().PeekPending()); box != "" || opts != 0 {
		t.Errorf("the control: Xeptip, who has no route, said %q (%d messages)", box, opts)
	}
}

// The portal players arrive at in the Plane of Disease sends them back to the
// Dead Hills, in front of Zizclank, at any level; the pockets' portals need
// level 60 and land in front of their other ends (the control: a level 59 is
// too weak for the Whirling Portal).
func TestPoDPortalsSendPlayersOn(t *testing.T) {
	tw := newTalkWorld(t, 5063, 3878)
	tw.p.World, tw.p.Y, tw.p.Level = 5, -25.3, 1
	back := tw.add(1, &entity.NPC{ServerID: 1, Name: "Portal to the Dead Hills", World: 5, SpawnX: 5063, SpawnY: -25.3, SpawnZ: 3881, SpawnFacing: math.Pi})
	whirl := tw.add(2, &entity.NPC{ServerID: 2, Name: "Whirling Portal", World: 5, SpawnX: 5040, SpawnY: -26.8, SpawnZ: 3662})
	tw.w.NPCs[3] = &entity.NPC{ServerID: 3, Name: "Necrologist Zizclank", World: 1, SpawnX: 4280, SpawnY: 97.8, SpawnZ: 6412, SpawnFacing: math.Pi / 2}
	tw.w.NPCs[4] = &entity.NPC{ServerID: 4, Name: "Portal to the Lake", World: 5, SpawnX: 5640, SpawnY: -20.6, SpawnZ: 3760, SpawnFacing: math.Pi / 2}

	tw.talk(back)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("a level 1 at the portal was offered %q, want Yes and No", got)
	}
	tw.pick(back, 0)
	x, z, world, moved := teleported(tw.s)
	if !moved || world != 1 || math.Abs(float64(x-(4280+bindReach))) > 0.01 || math.Abs(float64(z-6412)) > 0.01 {
		t.Errorf("the portal sent the player to (%.1f, %.1f) world %d (%v), want (%.0f, 6412) in world 1, in front of Zizclank",
			x, z, world, moved, 4280+bindReach)
	}

	// the trip changed worlds, which empties the session's channels: back
	// in PoD, beside the Whirling Portal, in view again
	tw.s.Queue().GetPending()
	tw.p.World, tw.p.Level = 5, 59
	tw.p.X, tw.p.Y, tw.p.Z = 5040, -26.4, 3665
	whirl = tw.add(2, tw.w.NPCs[2])
	tw.talk(whirl)
	if box := boxSaid(tw.s); !strings.Contains(box, "too weak") || !strings.Contains(box, "the wasp hive") {
		t.Fatalf("a level 59 at the Whirling Portal was told %q, want too weak", box)
	}
	tw.s.Queue().GetPending()
	tw.p.Level = 60
	tw.talk(whirl)
	tw.s.Queue().GetPending()
	tw.pick(whirl, 0)
	if x, z, world, moved := teleported(tw.s); !moved || world != 5 || math.Abs(float64(x-(5640+bindReach))) > 0.01 || math.Abs(float64(z-3760)) > 0.01 {
		t.Errorf("the Whirling Portal sent a level 60 to (%.1f, %.1f) world %d (%v), want in front of the Portal to the Lake",
			x, z, world, moved)
	}
}

// The Fuming Portal at the cauldron sends a level 60 to Noroxus's lair, in
// front of the Portal to the Cauldron in the north-west lake's lobe, and that
// portal sends anyone back in front of the Fuming Portal (F3 pass 21, jib:
// Noroxus stood "at his portal entrance, not his arena"). The control: a
// level 59 is too weak for the lair and is sent nowhere.
func TestFumingPortalSendsTheRaidToNoroxus(t *testing.T) {
	tw := newTalkWorld(t, 5623, 3061)
	tw.p.World, tw.p.Y, tw.p.Level = 5, -30.5, 59
	// facing bytes 54 and 174 (tools/seed_pod.py), in radians as the loader has them
	fuming := tw.add(1, &entity.NPC{ServerID: 1, Name: "Fuming Portal", World: 5, SpawnX: 5620, SpawnY: -30.54, SpawnZ: 3060,
		SpawnFacing: 54.0 / 256 * 2 * math.Pi})
	back := &entity.NPC{ServerID: 2, Name: "Portal to the Cauldron", World: 5, SpawnX: 4226, SpawnY: -24.16, SpawnZ: 2097,
		SpawnFacing: 174.0 / 256 * 2 * math.Pi}
	tw.w.NPCs[2] = back

	tw.talk(fuming)
	if box := boxSaid(tw.s); !strings.Contains(box, "too weak") || !strings.Contains(box, "Noroxus's lair") {
		t.Fatalf("a level 59 at the Fuming Portal was told %q, want too weak for Noroxus's lair", box)
	}
	tw.s.Queue().GetPending()
	tw.pick(fuming, 0)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("the control: a level 59 was sent to Noroxus's lair")
	}

	tw.p.Level = 60
	tw.talk(fuming)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("a level 60 at the Fuming Portal was offered %q, want Yes and No", got)
	}
	tw.pick(fuming, 0)
	fx, fz, _ := inFrontOf(back.SpawnX, back.SpawnZ, back.SpawnFacing)
	x, z, world, moved := teleported(tw.s)
	if !moved || world != 5 || math.Abs(float64(x-fx)) > 0.01 || math.Abs(float64(z-fz)) > 0.01 {
		t.Errorf("the Fuming Portal sent a level 60 to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f), in front of the Portal to the Cauldron",
			x, z, world, moved, fx, fz)
	}
	// in front of the way back is its west side, away from the lake
	if fx >= back.SpawnX-2 {
		t.Errorf("the landing (%.1f, %.1f) is not west of the Portal to the Cauldron", fx, fz)
	}

	// the trip stayed in world 5: the way back is in view, and leads to the cauldron
	tw.s.Queue().GetPending()
	tw.p.X, tw.p.Y, tw.p.Z, tw.p.Level = fx, -23.51, fz, 1
	way := tw.add(3, back)
	tw.talk(way)
	tw.s.Queue().GetPending()
	tw.pick(way, 0)
	cx, cz, _ := inFrontOf(5620, 3060, 54.0/256*2*math.Pi)
	if x, z, world, moved := teleported(tw.s); !moved || world != 5 || math.Abs(float64(x-cx)) > 0.01 || math.Abs(float64(z-cz)) > 0.01 {
		t.Errorf("the Portal to the Cauldron sent a level 1 to (%.1f, %.1f) world %d (%v), want (%.1f, %.1f), in front of the Fuming Portal",
			x, z, world, moved, cx, cz)
	}
}

// Every trip of the Plane of Disease's lands on an NPC that has a trip back
// to where it began, so no portal strands a player; and an NPC is a waysender
// by its trips (the control: a name with none is not).
func TestPoDTripsGoBothWays(t *testing.T) {
	pod := []string{"Necrologist Zizclank", "Portal to the Dead Hills", "Whirling Portal", "Portal to the Lake",
		"Bloody Portal", "Portal of the Twins", "Fuming Portal", "Portal to the Cauldron"}
	for _, from := range pod {
		if !IsWaysender(from) {
			t.Errorf("%s is not a waysender", from)
		}
		routes := wayRoutes[from]
		if len(routes) != 1 {
			t.Fatalf("%s has %d trips, want 1", from, len(routes))
		}
		back := wayRoutes[routes[0].Arrive]
		if len(back) != 1 || back[0].Arrive != from {
			t.Errorf("%s lands on %s, whose trips %v do not lead back", from, routes[0].Arrive, back)
		}
	}
	if IsWaysender("Necrologist Xeptip") || IsWaysender("Necrologist Ipstok") {
		t.Error("the control: a necrologist with no trips counts as a waysender")
	}
}

// #tele pod and #tele deadhills land on the NPCs the trips land on, in their
// worlds; with the NPC not loaded, on the fallback, saying so. The control:
// a name not in the table resolves to nothing.
func TestPoDShortcuts(t *testing.T) {
	w := entity.NewWorld()
	w.NPCs[1] = &entity.NPC{ServerID: 1, Name: "Portal to the Dead Hills", World: 5, SpawnX: 5070, SpawnY: -25, SpawnZ: 3890}
	if x, _, z, world, via, ok := ResolveTeleDest(w, "pod"); !ok || world != 5 || x != 5070 || z != 3890 || strings.Contains(via, "fallback") {
		t.Errorf("pod: (%.1f, %.1f) world %d %q %v, want the portal's spawn in world 5", x, z, world, via, ok)
	}
	if x, _, z, world, via, ok := ResolveTeleDest(w, "deadhills"); !ok || world != 1 || x != 4280 || z != 6412 || !strings.Contains(via, "fallback") {
		t.Errorf("deadhills without Zizclank: (%.1f, %.1f) world %d %q %v, want the fallback in world 1", x, z, world, via, ok)
	}
	if _, _, _, _, _, ok := ResolveTeleDest(w, "plane of disease"); ok {
		t.Error("the control: a name not in the table resolved")
	}
}
