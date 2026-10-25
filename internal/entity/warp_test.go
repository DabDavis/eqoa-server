package entity

import "testing"

// The Freeport <-> guk route is not written down anywhere as a route: it is
// DERIVED from which warp NPCs exist, the same way coach routes are. That makes
// it cheap to extend and easy to break by accident — renaming an NPC or adding
// a world field in the wrong place silently empties a destination list, and the
// only symptom in game is a travel NPC that says nothing when you talk to it.
// These tests pin the derivation.

func gukWorld() *World {
	w := NewWorld()
	// The two warp NPCs, as the database now holds them.
	w.NPCs[1] = &NPC{
		ServerID: 1, Name: "Harbormaster Thand", World: 0,
		SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5,
	}
	w.NPCs[2] = &NPC{
		ServerID: 2, Name: "Gatekeeper Krup", World: 8,
		SpawnX: 1200, SpawnY: 0, SpawnZ: 1000,
	}
	// Coachmen, to prove the two systems stay separate.
	w.NPCs[3] = &NPC{ServerID: 3, Name: "Coachman Ronks", World: 0, SpawnX: 25273.7, SpawnZ: 15719.1}
	w.NPCs[4] = &NPC{ServerID: 4, Name: "Coachman Huff", World: 0, SpawnX: 18334.8, SpawnZ: 11357.8}
	// An ordinary NPC standing next to a coachman must not become a route.
	w.NPCs[5] = &NPC{ServerID: 5, Name: "Guard Kellin", World: 0}
	return w
}

func TestWarpNPCClassification(t *testing.T) {
	cases := []struct {
		name        string
		warp, coach bool
	}{
		{"Gatekeeper Krup", true, false},
		{"Harbormaster Thand", true, false},
		{"Coachman Ronks", false, true},
		{"Coachwoman Darva", false, true},
		{"Guard Kellin", false, false},
		// Substring, not prefix — must not match.
		{"The Gatekeeper", false, false},
	}
	for _, c := range cases {
		if got := IsWarpNPC(c.name); got != c.warp {
			t.Errorf("IsWarpNPC(%q) = %v, want %v", c.name, got, c.warp)
		}
		if got := IsCoachman(c.name); got != c.coach {
			t.Errorf("IsCoachman(%q) = %v, want %v", c.name, got, c.coach)
		}
	}
}

// Talking to Thand in Freeport must offer world 8 (guk's world while worlds
// came off the memory card; the hard-disk loader's hdd:world8.ESF now) to a
// console whose table has it, carrying world 8 — the byte the 0x00B2 Teleport
// sends raw and the only thing that reaches the loader.
func TestWarpFreeportOffersGuk(t *testing.T) {
	dests := gukWorld().GetWarpDestinations("Harbormaster Thand", nil)
	if len(dests) != 1 {
		t.Fatalf("expected exactly 1 destination from Freeport, got %d: %+v", len(dests), dests)
	}
	d := dests[0]
	if d.Name != "World 8 (HDD)" {
		t.Errorf("label = %q, want %q (friendly name, not the NPC name; not Guk, which is in Tunaria)", d.Name, "World 8 (HDD)")
	}
	if d.World != 8 {
		t.Errorf("world = %d, want 8 — a 0 here silently lands the player at guk's coordinates inside Tunaria", d.World)
	}
	if d.X != 1200 || d.Y != 0 || d.Z != 1000 {
		t.Errorf("destination = (%v, %v, %v), want guk's ground vertex (1200, 0, 1000)", d.X, d.Y, d.Z)
	}
}

// ...and the return leg, which is the half that is easy to forget.
func TestWarpGukOffersFreeport(t *testing.T) {
	dests := gukWorld().GetWarpDestinations("Gatekeeper Krup", nil)
	if len(dests) != 1 {
		t.Fatalf("expected exactly 1 destination from guk, got %d: %+v", len(dests), dests)
	}
	d := dests[0]
	if d.Name != "Freeport" {
		t.Errorf("label = %q, want %q", d.Name, "Freeport")
	}
	if d.World != 0 {
		t.Errorf("world = %d, want 0 — the player must come back to Tunaria", d.World)
	}
	if d.X != 25377.7 || d.Z != 15741.5 {
		t.Errorf("destination = (%v, %v), want Thand's spawn", d.X, d.Z)
	}
}

// A warp NPC must never offer itself, or the option box lists the place you
// are already standing in.
func TestWarpExcludesSelf(t *testing.T) {
	for _, self := range []string{"Harbormaster Thand", "Gatekeeper Krup"} {
		for _, d := range gukWorld().GetWarpDestinations(self, nil) {
			if d.Name == self {
				t.Errorf("%s offered itself", self)
			}
		}
	}
}

// Coach routes must stay intra-world. A coach destination carries no world
// change, so an out-of-world coachman in this list would teleport the player to
// another world's coordinates while leaving them where they are.
func TestCoachRoutesAreIntraWorld(t *testing.T) {
	w := gukWorld()
	// A coachman inside guk: only reachable from guk, never offered in Tunaria.
	w.NPCs[6] = &NPC{ServerID: 6, Name: "Coachman Bogsworth", World: 8, SpawnX: 1300, SpawnZ: 1100}

	tun := w.GetCoachDestinations("Coachman Ronks", 0)
	for _, d := range tun {
		if d.World != 0 {
			t.Errorf("Tunaria coach list leaked world %d destination %q", d.World, d.Name)
		}
	}
	// Huff is listed under his friendly label, not his NPC name.
	if len(tun) != 1 || tun[0].Name != "Rivervale" {
		t.Errorf("Tunaria coach list = %+v, want just Rivervale (Coachman Huff)", tun)
	}

	guk := w.GetCoachDestinations("Coachman Bogsworth", 8)
	if len(guk) != 0 {
		t.Errorf("guk coach list = %+v, want empty (its only coachman is the one being talked to)", guk)
	}
}

// Coachmen and warp NPCs must not appear in each other's lists — they are
// different mechanisms with different teleport semantics.
func TestCoachAndWarpDoNotMix(t *testing.T) {
	w := gukWorld()
	for _, d := range w.GetWarpDestinations("Harbormaster Thand", nil) {
		if IsCoachman(d.Name) || d.Name == "Highpass Hold" {
			t.Errorf("warp list contains a coach destination: %q", d.Name)
		}
	}
	for _, d := range w.GetCoachDestinations("Coachman Ronks", 0) {
		if d.Name == "World 8 (HDD)" || d.Name == "Freeport" {
			t.Errorf("coach list contains a warp destination: %q", d.Name)
		}
	}
}

// A cross-world teleport commits its world index immediately — nothing
// acknowledges the arrival — so a world that fails to LOAD leaves the server
// believing the player moved while the client reports its old position. Saved,
// that pair strands the character: visibility filters to a world they are not
// in, no NPC is ever in range, and the travel NPC needed to get back is itself
// invisible. Observed for real on 2026-08-06 (world 8 at Freeport's
// coordinates) and reported as "NPCs are not populating".
func TestIncoherentWorldIsRescued(t *testing.T) {
	cases := []struct {
		name         string
		world        uint8
		x, z         float32
		want         uint8
		wantCoherent bool
	}{
		{"world 9 position in world 9", 9, 13000, 23000, 9, true},
		{"world 9 edges are inside", 9, 12000, 24000, 9, true},
		// The real stranding, moved to the world that still has a bound: a
		// world index carrying Freeport's coordinates.
		{"Freeport position under world 9", 9, 25370.9, 15750.0, 0, false},
		// Walking east out of the lifted zone: measured at x=14086, y=-1000.
		{"just outside world 9", 9, 14086, 23000, 0, false},
		// Disc worlds share one coordinate space and assert nothing, so a
		// position must never be second-guessed for them.
		{"tunaria is never second-guessed", 0, 25370.9, 15750.0, 0, true},
		{"odus is never second-guessed", 2, 7338.12, 11124.15, 2, true},
		// World 8 lost its bound when guk left the memory card: its table entry
		// is now a clone of Odus, so its positions are Odus positions and guk's
		// island extents would reject every one of them.
		{"world 8 no longer asserts guk's extents", 8, 7338.12, 11124.15, 8, true},
	}
	for _, c := range cases {
		got, ok := CoherentWorld(c.world, c.x, c.z)
		if got != c.want || ok != c.wantCoherent {
			t.Errorf("%s: CoherentWorld(%d, %.1f, %.1f) = (%d, %v), want (%d, %v)",
				c.name, c.world, c.x, c.z, got, ok, c.want, c.wantCoherent)
		}
	}
}

// The rescue must land somewhere the player can actually recover from: world 0
// is the only world always present, and its NPCs include the travel NPCs.
func TestRescueLandsInAReachableWorld(t *testing.T) {
	got, _ := CoherentWorld(9, 25370.9, 15750.0)
	if got != 0 {
		t.Errorf("stranded player rescued into world %d, want 0", got)
	}
	w := gukWorld()
	dests := w.GetWarpDestinations("", nil)
	found := false
	for _, d := range dests {
		if d.World == got {
			found = true
		}
	}
	if !found {
		t.Errorf("world %d has no warp NPC, so a rescued player could not travel out", got)
	}
}

// The destination list is built by ranging over w.NPCs, and Go randomises map
// iteration order on every call — while the player picks by INDEX. Live on
// 2026-08-06 two lists one second apart came out [Odus (control) Guk] then
// [Guk Odus (control)], so a player aiming for Odus was teleported to guk and
// the server logged a perfectly sensible teleport to the wrong place. Nothing
// errored; the only symptom was arriving somewhere else.
func TestDestinationOrderIsStable(t *testing.T) {
	w := gukWorld()
	w.NPCs[7] = &NPC{ServerID: 7, Name: "Gatekeeper Sulon", World: 2, SpawnX: 7338, SpawnZ: 11124}

	first := w.GetWarpDestinations("Harbormaster Thand", nil)
	if len(first) < 2 {
		t.Fatalf("need at least 2 destinations to detect reordering, got %d", len(first))
	}
	// Many rebuilds: a map-order bug shows up within a handful of iterations.
	for i := 0; i < 200; i++ {
		got := w.GetWarpDestinations("Harbormaster Thand", nil)
		if len(got) != len(first) {
			t.Fatalf("iteration %d: length changed %d -> %d", i, len(first), len(got))
		}
		for j := range got {
			if got[j].Name != first[j].Name {
				t.Fatalf("iteration %d: order changed at %d: %q != %q — the player "+
					"picks by index, so this teleports them somewhere they did not choose",
					i, j, got[j].Name, first[j].Name)
			}
		}
	}

	// Sorted by the DISPLAYED label, which is the thing the player reads.
	for i := 1; i < len(first); i++ {
		if first[i-1].Name > first[i].Name {
			t.Errorf("not sorted by label: %q before %q", first[i-1].Name, first[i].Name)
		}
	}

	// Coach lists share the defect and the fix.
	c1 := w.GetCoachDestinations("Coachman Ronks", 0)
	for i := 0; i < 200; i++ {
		c := w.GetCoachDestinations("Coachman Ronks", 0)
		for j := range c {
			if c[j].Name != c1[j].Name {
				t.Fatalf("coach order changed at %d: %q != %q", j, c[j].Name, c1[j].Name)
			}
		}
	}
}

// --- World 9: Widows Peak, off the PS2 hard disk ---

// world9World is the pair the DB now holds for the Freeport <-> world 9 route.
// Machado's coordinates are the MEASURED ground at the zone centre, so a test
// that quietly rounds them is a test that stops describing the world.
func world9World() *World {
	w := NewWorld()
	w.NPCs[1] = &NPC{
		ServerID: 1, Name: "Harbormaster Thand", World: 0,
		SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5,
	}
	w.NPCs[2] = &NPC{
		ServerID: 2, Name: "Harbormaster Machado", World: 9,
		SpawnX: 13000, SpawnY: 66.5, SpawnZ: 23000,
	}
	return w
}

// One DB row wires BOTH directions — that is the whole reason the route table
// is derived from the NPCs rather than kept separately, and it is what makes
// adding a world a one-row change.
func TestWarpWorld9BothDirections(t *testing.T) {
	w := world9World()

	out := w.GetWarpDestinations("Harbormaster Thand", nil)
	if len(out) != 1 {
		t.Fatalf("Freeport should offer exactly world 9, got %d: %+v", len(out), out)
	}
	if out[0].World != 9 {
		t.Errorf("outbound world = %d, want 9 — the byte 0x00B2 sends raw", out[0].World)
	}
	if out[0].X != 13000 || out[0].Y != 66.5 || out[0].Z != 23000 {
		t.Errorf("outbound lands at (%v, %v, %v), want Machado's measured ground "+
			"(13000, 66.5, 23000)", out[0].X, out[0].Y, out[0].Z)
	}
	if out[0].Name != "Widows Peak (world 9, HDD)" {
		t.Errorf("outbound label = %q, want the friendly name", out[0].Name)
	}

	back := w.GetWarpDestinations("Harbormaster Machado", nil)
	if len(back) != 1 {
		t.Fatalf("world 9 should offer exactly Freeport, got %d: %+v", len(back), back)
	}
	if back[0].World != 0 || back[0].Name != "Freeport" {
		t.Errorf("return trip = %q world %d, want Freeport world 0",
			back[0].Name, back[0].World)
	}
}

// The destination must be INSIDE the bound, or arriving there would be undone
// by the very check meant to rescue a failed world change.
func TestWorld9DestinationIsInsideItsBound(t *testing.T) {
	b, known := worldBounds[9]
	if !known {
		t.Fatal("world 9 has no bounds — a failed load there would strand the character")
	}
	if got, ok := CoherentWorld(9, 13000, 23000); !ok || got != 9 {
		t.Errorf("CoherentWorld(9, 13000, 23000) = %d, %v; want 9, true — "+
			"Machado stands there", got, ok)
	}
	// Where the character actually fell out of the world on 2026-08-10.
	if got, ok := CoherentWorld(9, 14086, 23000); ok || got != 0 {
		t.Errorf("CoherentWorld(9, 14086, 23000) = %d, %v; want 0, false — "+
			"x=14086 is past the zone's %v edge", got, ok, b.MaxX)
	}
}

// #tele resolves through the NPC so its coordinates cannot drift from the
// world. SpawnOfNPC reports the SPAWN, not the live position.
func TestSpawnOfNPC(t *testing.T) {
	w := world9World()
	w.NPCs[2].X, w.NPCs[2].Y, w.NPCs[2].Z = 1, 2, 3 // wandered

	x, y, z, _, world, ok := w.SpawnOfNPC("Harbormaster Machado")
	if !ok {
		t.Fatal("Machado not found")
	}
	if x != 13000 || y != 66.5 || z != 23000 || world != 9 {
		t.Errorf("SpawnOfNPC = (%v, %v, %v) world %d, want the SPAWN "+
			"(13000, 66.5, 23000) world 9, not the live position", x, y, z, world)
	}
	if _, _, _, _, _, ok := w.SpawnOfNPC("Harbormaster Nobody"); ok {
		t.Error("SpawnOfNPC found an NPC that does not exist")
	}
}

// --- World 22: an authored world, fetched on demand ---

// freeportWorld is the route table as the DB now holds it: Freeport plus the
// two hard-disk worlds a player can reach from it.
func freeportWorld() *World {
	w := world9World()
	w.NPCs[3] = &NPC{
		ServerID: 3, Name: "Harbormaster Deuces", World: 22,
		SpawnX: 9000, SpawnY: 59.3, SpawnZ: 9000,
	}
	return w
}

// The ask was "world 22 AND world 9 on Thand" — world 9 was already there, and
// this asserts BOTH survive, because the failure mode of a derived route table
// is that a new NPC displaces rather than joins.
func TestFreeportOffersBothHDDWorlds(t *testing.T) {
	out := freeportWorld().GetWarpDestinations("Harbormaster Thand", nil)

	got := map[uint8]CoachDest{}
	for _, d := range out {
		if _, dup := got[d.World]; dup {
			t.Errorf("world %d offered twice", d.World)
		}
		got[d.World] = d
	}
	for _, want := range []struct {
		world   uint8
		label   string
		x, y, z float32
	}{
		{9, "Widows Peak (world 9, HDD)", 13000, 66.5, 23000},
		{22, "World 22 (authored, HDD)", 9000, 59.3, 9000},
	} {
		d, ok := got[want.world]
		if !ok {
			t.Errorf("Freeport does not offer world %d at all", want.world)
			continue
		}
		if d.Name != want.label {
			t.Errorf("world %d label = %q, want %q", want.world, d.Name, want.label)
		}
		// The landing point is the NPC's SPAWN, and for world 22 that spawn is
		// an observed in-world ground height (a player dropped from y=400 came
		// to rest at 59.3). Rounding it here would be rounding the measurement.
		if d.X != want.x || d.Y != want.y || d.Z != want.z {
			t.Errorf("world %d lands at (%v, %v, %v), want (%v, %v, %v)",
				want.world, d.X, d.Y, d.Z, want.x, want.y, want.z)
		}
	}
}

// And the return leg, which the same single row buys: Deuces offers everywhere
// else, never himself.
func TestWorld22OffersTheWayBack(t *testing.T) {
	back := freeportWorld().GetWarpDestinations("Harbormaster Deuces", nil)
	if len(back) != 2 {
		t.Fatalf("world 22 should offer Freeport and world 9, got %d: %+v", len(back), back)
	}
	for _, d := range back {
		if d.World == 22 {
			t.Errorf("Deuces offers a trip to where the player already stands: %+v", d)
		}
	}
}

// A world with no bounds entry must still be REACHABLE. CoherentWorld treats
// an unknown world as coherent on purpose: world 22's extents have never been
// measured in world space (both readings taken off the ESF were zone-local), and
// a guessed bound would send every arrival straight back to Tunaria.
func TestWorld22IsReachableWithoutBounds(t *testing.T) {
	if _, known := worldBounds[22]; known {
		t.Fatal("worldBounds[22] exists now — this test guards its ABSENCE; " +
			"if it was measured in WORLD space, assert the extents instead")
	}
	if got, ok := CoherentWorld(22, 9000, 9000); !ok || got != 22 {
		t.Errorf("CoherentWorld(22, 9000, 9000) = %d, %v; want 22, true — "+
			"Deuces stands there and the route would be undone on arrival", got, ok)
	}
}

// A travel menu, the GM's places and the warp points offer only the worlds
// the asking client loads (handlers.LoadableWorlds): from Freeport a game
// with the disc's six is offered Odus, never the patched consoles' worlds 7,
// 8, 9 and 22, which it cannot load (picked, the client stayed where it was
// while the server filed the player there: F3 20261025-000749). The control:
// the whole table (nil, a patched PS2's) offers all five.
func TestTravelOffersOnlyLoadableWorlds(t *testing.T) {
	w := freeportWorld()
	w.NPCs[4] = &NPC{ServerID: 4, Name: "Gatekeeper Sulon", World: 2, SpawnX: 7338, SpawnY: 54.2, SpawnZ: 11124}
	w.NPCs[5] = &NPC{ServerID: 5, Name: "Gatekeeper Krup", World: 8, SpawnX: 1125, SpawnY: -62.2, SpawnZ: 1143.7}
	w.NPCs[6] = &NPC{ServerID: 6, Name: "Gatekeeper Rilis", World: 7, SpawnX: 4562, SpawnY: -12.3, SpawnZ: 3663.8}
	disc := func(world uint8) bool { return world < 6 }
	worlds := func(d []CoachDest) (out []uint8) {
		for _, x := range d {
			out = append(out, x.World)
		}
		return
	}
	if got := worlds(w.GetWarpDestinations("Harbormaster Thand", disc)); len(got) != 1 || got[0] != 2 {
		t.Errorf("the disc's six from Freeport: worlds %v, want just Odus (2)", got)
	}
	if got := worlds(w.GetWarpDestinations("Harbormaster Thand", nil)); len(got) != 5 {
		t.Errorf("the control, the whole table: worlds %v, want 2, 7, 8, 9 and 22", got)
	}
	for _, p := range w.Places(disc) {
		if p.World >= 6 {
			t.Errorf("Places offered %q in world %d to the disc's six", p.Name, p.World)
		}
	}
	for _, wp := range w.WarpPoints(disc) {
		if wp.World >= 6 {
			t.Errorf("WarpPoints offered %q in world %d to the disc's six", wp.NPC, wp.World)
		}
	}
	if n := len(w.Places(nil)); n != 6 {
		t.Errorf("the control: %d places in the whole table, want 6", n)
	}
	// world 8 is not Guk any more, and Odus is not a control
	for _, d := range w.GetWarpDestinations("Harbormaster Thand", nil) {
		if d.Name == "Guk" || d.Name == "Odus (control)" {
			t.Errorf("a stale label is still offered: %q (world %d)", d.Name, d.World)
		}
	}
}
