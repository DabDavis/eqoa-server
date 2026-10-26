package entity

import "testing"

// flatGround is a terrain at one height everywhere but inside a hole (no floor).
type flatGround struct {
	y            float32
	holeX, holeZ float32
	hole         float32
}

func (g flatGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	if g.hole > 0 && distXZ(x, z, g.holeX, g.holeZ) < g.hole {
		return 0, false
	}
	return g.y, g.y <= y+2
}

// roofed is a ground with another floor (a ceiling) above it everywhere.
type roofed struct {
	flatGround
	roof float32
}

func (g roofed) Floor(world uint8, x, y, z float32) (float32, bool) {
	if y+2 >= g.roof {
		return g.roof, true
	}
	return g.flatGround.Floor(world, x, y, z)
}

func campWorld(g Ground) *World {
	w := NewWorld()
	w.ground = g
	// a camp of five round (1000, 1000), the anchor its eastmost
	for i, p := range [][2]float32{{1000, 1000}, {1010, 1000}, {990, 1005}, {1000, 990}, {1020, 1000}} {
		w.NPCs[EntityID(i+1)] = &NPC{ServerID: EntityID(i + 1), Name: "orc", World: 0,
			X: p[0], Y: 50, Z: p[1], SpawnX: p[0], SpawnY: 50, SpawnZ: p[1]}
	}
	return w
}

// A camp on the ground lands beside it: off the side away from its middle,
// clear of every creature's notice, on the terrain.
func TestCampLandingBesideTheCamp(t *testing.T) {
	w := campWorld(flatGround{y: 50})
	x, y, z, ok := w.CampLanding("orc", 0, 1020, 1000, 300)
	if !ok || y != 50 {
		t.Fatalf("landed (%.0f, %.1f, %.0f) %v", x, y, z, ok)
	}
	for _, npc := range w.NPCs {
		if d := distXZ(npc.SpawnX, npc.SpawnZ, x, z); d < campClear {
			t.Errorf("landed %.1f from a creature at (%.0f, %.0f)", d, npc.SpawnX, npc.SpawnZ)
		}
	}
	if x <= 1020 {
		t.Errorf("landed at x %.0f: not on the side away from the camp's middle (east)", x)
	}
	if d := distXZ(x, z, 1020, 1000); d < 44.9 || d > 150.1 {
		t.Errorf("landed %.0f off the anchor, want 45 to 150", d)
	}
}

// A camp on a platform (a keep's floor, a tree's) lands on the terrain
// beside it.
func TestCampLandingBesideACampOnAPlatform(t *testing.T) {
	w := campWorld(flatGround{y: 20}) // the camp 30 above the terrain
	x, y, z, ok := w.CampLanding("orc", 0, 1020, 1000, 300)
	if !ok || y != 20 || (x == 1020 && z == 1000) {
		t.Fatalf("landed (%.0f, %.1f, %.0f) %v, want the terrain beside the platform", x, y, z, ok)
	}
}

// The negative controls: no ground, an anchor off the terrain (a cave), a
// camp with no clear spot round it land on the anchor's own spot; no NPC of
// the name is no landing.
func TestCampLandingOnTheAnchorOtherwise(t *testing.T) {
	anchor := func(w *World) {
		t.Helper()
		x, y, z, ok := w.CampLanding("orc", 0, 1020, 1000, 300)
		if !ok || x != 1020 || y != 50 || z != 1000 {
			t.Errorf("landed (%.0f, %.1f, %.0f) %v, want the anchor's spawn", x, y, z, ok)
		}
	}
	anchor(campWorld(nil))
	anchor(campWorld(flatGround{y: 80}))  // the terrain 30 above: the camp is under it
	anchor(campWorld(flatGround{y: -20})) // 70 below: no terrain within reach under it
	// the terrain round the camp a hole: no floor for a landing
	anchor(campWorld(flatGround{y: 50, holeX: 1020, holeZ: 1000, hole: 200}))
	// creatures every 20 out past the furthest landing: no spot with
	// campCrowded's room (the most is half a diagonal, 14)
	ringed := campWorld(flatGround{y: 50})
	for i := 0; i < 30*30; i++ {
		x, z := 720+float32(i%30)*20, 700+float32(i/30)*20
		ringed.NPCs[EntityID(100+i)] = &NPC{ServerID: EntityID(100 + i), Name: "rat", World: 0, X: x, Z: z, SpawnX: x, SpawnZ: z, SpawnY: 50}
	}
	anchor(ringed)
	// with room for one creature's notice and no more, the spot with the most
	sparse := campWorld(flatGround{y: 50})
	for i := 0; i < 15*15; i++ {
		x, z := 720+float32(i%15)*40, 700+float32(i/15)*40
		sparse.NPCs[EntityID(100+i)] = &NPC{ServerID: EntityID(100 + i), Name: "rat", World: 0, X: x, Z: z, SpawnX: x, SpawnZ: z, SpawnY: 50}
	}
	if x, _, z, ok := sparse.CampLanding("orc", 0, 1020, 1000, 300); !ok || (x == 1020 && z == 1000) {
		t.Errorf("a crowded camp with room between its creatures landed on the anchor (%.0f, %.0f)", x, z)
	} else {
		for _, npc := range sparse.NPCs {
			if d := distXZ(npc.SpawnX, npc.SpawnZ, x, z); d < campCrowded {
				t.Errorf("landed %.1f from a creature", d)
			}
		}
	}
	if _, _, _, ok := campWorld(flatGround{y: 50}).CampLanding("gnoll", 0, 1020, 1000, 300); ok {
		t.Error("a name with no NPC there landed")
	}
	if _, _, _, ok := campWorld(flatGround{y: 50}).CampLanding("orc", 0, 1600, 1000, 300); ok {
		t.Error("an anchor further than reach landed")
	}
}
