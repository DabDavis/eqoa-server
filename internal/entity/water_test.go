package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// beach is a shore along x: land at height 60 up to x 1990, a beach falling
// 1 a unit to the sea floor at 40 from x 2010; the water lies below 50
// everywhere (seaWater), so the beach goes under at x 2000.5.
type beach struct{}

func beachFloor(x float32) float32 {
	switch {
	case x < 1990:
		return 60
	case x > 2010:
		return 40
	}
	return 60 - (x - 1990)
}

func (beach) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	f := beachFloor(x)
	return f, f <= y+2
}

type seaWater struct{}

func (seaWater) Flags(_ uint8, _, y, _ float32) (uint32, bool) {
	if y < 50 {
		return envWater, true
	}
	return 0, true
}

// beachWorld is a roam world on the beach, its water known (or not), its
// viewer standing on the shore.
func beachWorld(t *testing.T, water bool) *World {
	t.Helper()
	w := roamWorld(t)
	w.SetGround(beach{})
	if water {
		w.SetEnv(seaWater{})
	}
	p := w.Players[1]
	p.X, p.Y, p.Z = 1990, 60, 1000
	w.SpatialIndex.Update(p.ServerID, 1000, 1000, p.X, p.Z)
	return w
}

// legs is where a creature's next legs end, n of them, from where it stands.
func legs(w *World, npc *NPC, n int) [][2]float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out [][2]float32
	for i := 0; i < n; i++ {
		x, z := w.roamPointLocked(npc)
		if x != npc.X || z != npc.Z {
			out = append(out, [2]float32{x, z})
		}
	}
	return out
}

// A creature of the land wanders up to the water's edge but never into the
// water; told it goes in and out of the water (npcmove's "both"), it does.
// The control: with no water known, its legs go into the sea.
func TestALandCreatureNeverWandersIntoTheWater(t *testing.T) {
	into := func(w *World, npc *NPC) (in, total int) {
		for _, l := range legs(w, npc, 300) {
			total++
			if beachFloor(l[0])+bodyAbove < 50 {
				in++
			}
		}
		return in, total
	}
	w := beachWorld(t, true)
	orc := addNPC(w, 100, "orc pawn", 1985, 1000, creatureModel, 0)
	orc.Y, orc.SpawnY = 60, 60
	if in, total := into(w, orc); in != 0 || total < 100 {
		t.Errorf("a land creature: %d of %d legs into the water, want none of a hundred and more", in, total)
	}
	w.SetMoveRules(map[string]database.NPCMoveRule{"orc pawn": {Roam: true, Water: keepsToBoth}})
	if in, _ := into(w, orc); in == 0 {
		t.Error("told it goes in and out of the water, no leg went in")
	}
	w = beachWorld(t, false)
	orc = addNPC(w, 100, "orc pawn", 1985, 1000, creatureModel, 0)
	orc.Y, orc.SpawnY = 60, 60
	if in, _ := into(w, orc); in == 0 {
		t.Error("with no water known no leg went in: the test proves nothing")
	}
}

// A creature that spawned in the water keeps to it, wandering; told it
// keeps to the land too (npcmove's "both"), it comes out.
func TestACreatureOfTheWaterKeepsToIt(t *testing.T) {
	w := beachWorld(t, true)
	shark := addNPC(w, 100, "mako shark", 2030, 1000, creatureModel, 0)
	shark.Y, shark.SpawnY = 40, 40
	ls := legs(w, shark, 300)
	out := 0
	for _, l := range ls {
		if beachFloor(l[0])+bodyAbove >= 50 {
			out++
		}
	}
	if out != 0 || len(ls) < 100 {
		t.Errorf("a creature of the water: %d of %d legs out of it, want none of a hundred and more", out, len(ls))
	}
	w.SetMoveRules(map[string]database.NPCMoveRule{"mako shark": {Roam: true, Water: keepsToBoth}})
	out = 0
	for _, l := range legs(w, shark, 300) {
		if beachFloor(l[0])+bodyAbove >= 50 {
			out++
		}
	}
	if out == 0 {
		t.Error("told it goes in and out of the water, no leg came out")
	}
}

// No damage crosses the water's edge: a harmful cast at a creature in the
// water from the shore is refused (and one from the water at the shore),
// and a creature's melee on a player across it does nothing; on the same
// side both land.
func TestNoDamageCrossesTheWatersEdge(t *testing.T) {
	w := beachWorld(t, true)
	w.SessionManager = session.NewManager() // npcAttackPlayer looks the player's session up
	p := w.Players[1]
	p.HP, p.MaxHP = 500, 500
	croc := addNPC(w, 100, "crocodile", 2030, 1000, creatureModel, 0)
	croc.Y, croc.SpawnY = 40, 40
	w.mu.Lock()
	defer w.mu.Unlock()
	if got := w.waterRefusalLocked(p, []EntityID{croc.ServerID}); got != castTargetInWater {
		t.Errorf("from the shore at a creature in the water: %q, want %q", got, castTargetInWater)
	}
	w.npcAttackPlayer(croc, p.ServerID, time.Now())
	if p.HP != 500 {
		t.Errorf("a creature in the water hit a player on the shore: %d of 500 left", p.HP)
	}
	p.X, p.Y = 2025, 40 // in the water with it
	if got := w.waterRefusalLocked(p, []EntityID{croc.ServerID}); got != "" {
		t.Errorf("both in the water: %q, want no refusal", got)
	}
	w.npcAttackPlayer(croc, p.ServerID, time.Now())
	if p.HP == 500 {
		t.Error("both in the water, the creature's hit did nothing: the test proves nothing")
	}
	croc.X, croc.Y = 1985, 60 // out on the land; the player still in the water
	if got := w.waterRefusalLocked(p, []EntityID{croc.ServerID}); got != castTargetOutWater {
		t.Errorf("from the water at a creature on the shore: %q, want %q", got, castTargetOutWater)
	}
}

// A creature of the water chasing a player onto the shore stops at the
// water's edge; one that goes in and out of the water follows.
func TestACreatureOfTheWaterStopsAtItsEdge(t *testing.T) {
	chase := func(rule *database.NPCMoveRule) *NPC {
		w := beachWorld(t, true)
		if rule != nil {
			w.SetMoveRules(map[string]database.NPCMoveRule{"mako shark": *rule})
		}
		shark := addNPC(w, 100, "mako shark", 2030, 1000, creatureModel, 0)
		shark.Y, shark.SpawnY = 40, 40
		p := w.Players[1]
		now := time.Now()
		w.mu.Lock()
		defer w.mu.Unlock()
		shark.chase.on, shark.chase.fromX, shark.chase.fromZ = true, shark.X, shark.Z
		for i := 0; i < 60; i++ {
			now = now.Add(100 * time.Millisecond)
			shark.chase.last = now.Add(-100 * time.Millisecond)
			w.chaseLocked(shark, p.X, p.Z, 0, 0, now)
		}
		return shark
	}
	shark := chase(nil)
	if beachFloor(shark.X)+bodyAbove >= 50 || shark.X > 2010 {
		t.Errorf("the shark stood at x %.2f (floor %.2f): want it at the water's edge, in the water", shark.X, beachFloor(shark.X))
	}
	both := chase(&database.NPCMoveRule{Roam: true, Water: keepsToBoth})
	if both.X > 1995 {
		t.Errorf("told it goes in and out of the water it stopped at x %.2f, want it out on the shore", both.X)
	}
}

// A crocodile recorded 1.68 above the sand (F3 20261026-021425) stands on
// the ground as it moves; one 3 above it (a dock the ground leaves out)
// does not wander, keeps its height near its spawn and stands on the terrain
// beyond (platformReach). The control: the 1.5 the rule was put the first off
// the ground.
func TestACreatureRecordedALittleHighWalksOnTheGround(t *testing.T) {
	w := beachWorld(t, true)
	croc := addNPC(w, 100, "crocodile", 1900, 1000, creatureModel, 0)
	croc.Y, croc.SpawnY = 61.68, 61.68
	dock := addNPC(w, 101, "dock rat", 1900, 1020, creatureModel, 0)
	dock.Y, dock.SpawnY = 63, 63
	w.mu.Lock()
	if !w.wanders(croc) || w.wanders(dock) {
		t.Errorf("wanders: the crocodile %v, want true; the dock's %v, want false", w.wanders(croc), w.wanders(dock))
	}
	for i := 0; i < 20; i++ {
		w.stepNPCToward(croc, croc.SpawnX-30, croc.SpawnZ, NPCWalkSpeed, 0.1)
	}
	if croc.Y != 60 {
		t.Errorf("the crocodile walked at %.2f, want the floor's 60", croc.Y)
	}
	for i := 0; i < 16; i++ { // 8 off its spawn
		w.stepNPCToward(dock, dock.SpawnX-8, dock.SpawnZ, NPCWalkSpeed, 0.1)
	}
	if dock.Y != 63 {
		t.Errorf("8 off its spawn the dock's creature stood at %.2f, want its 63", dock.Y)
	}
	for i := 0; i < 16; i++ { // 16 off it
		w.stepNPCToward(dock, dock.SpawnX-16, dock.SpawnZ, NPCWalkSpeed, 0.1)
	}
	if dock.Y != 60 {
		t.Errorf("16 off its spawn the dock's creature stood at %.2f, want the terrain's 60", dock.Y)
	}
	w.mu.Unlock()
	if 61.68-beachFloor(1900) <= 1.5 {
		t.Error("the crocodile is no more than 1.5 above the ground: the test proves nothing")
	}
}

// A wander leg never crosses a cliff the creature would fall off, nor goes
// where there is no floor; a gentle slope it walks.
func TestALegNeverCrossesACliff(t *testing.T) {
	w := roamWorld(t)
	w.SetGround(cliff{})
	beetle := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0)
	ls := legs(w, beetle, 300)
	for _, l := range ls {
		if l[0] >= 1010 {
			t.Fatalf("a leg to x %.2f crossed the cliff at 1010", l[0])
		}
	}
	if len(ls) < 100 {
		t.Errorf("only %d legs found", len(ls))
	}
	w.SetGround(slopeGround{})
	beetle.Y, beetle.SpawnY = 100, 100
	beetle.groundKnown = false
	crossed := 0
	for _, l := range legs(w, beetle, 300) {
		if l[0] >= 1010 {
			crossed++
		}
	}
	if crossed == 0 {
		t.Error("no leg up the gentle slope past x 1010: the test proves nothing")
	}
}

// cliff is flat ground at 0 that falls to -10 at x 1010.
type cliff struct{}

func (cliff) Floor(_ uint8, x, y, _ float32) (float32, bool) {
	f := float32(0)
	if x >= 1010 {
		f = -10
	}
	return f, f <= y+2
}

// npcmove's roam 0 stands a creature on its spawn (jib's Hexbones); its own
// radius is its wander's.
func TestNPCMoveStandsACreature(t *testing.T) {
	w := roamWorld(t)
	hex := addNPC(w, 100, "Hexbone", 1005, 1000, creatureModel, 0)
	w.mu.Lock()
	wandered := w.wanders(hex)
	w.mu.Unlock()
	if !wandered {
		t.Fatal("a Hexbone with no row does not wander: the test proves nothing")
	}
	w.SetMoveRules(map[string]database.NPCMoveRule{"hexbone": {Roam: false}, "beetle": {Roam: true, Radius: 5}})
	beetle := addNPC(w, 101, "beetle", 1005, 1010, creatureModel, 0)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wanders(hex) {
		t.Error("a Hexbone told roam 0 wanders")
	}
	for i := 0; i < 50; i++ {
		x, z := w.roamPointLocked(beetle)
		if d := distXZ(x, z, beetle.SpawnX, beetle.SpawnZ); d > 5.001 {
			t.Fatalf("a leg %.2f off the spawn of a creature told radius 5", d)
		}
	}
}
