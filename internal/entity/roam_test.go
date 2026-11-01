package entity

import (
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// Creatures wander round their spawn at Sandstorm's walking pace, resting
// between walks; townsfolk, merchants and dummies stand (roam.go; F3
// 20261025-044257).

const (
	creatureModel = 0x1234     // a body of its own
	humanModel    = 0x70D898C6 // a player's race (charactermodel)
)

// roamWorld is a world with a player at (1000, 1000) and a seeded wander.
func roamWorld(t *testing.T) *World {
	t.Helper()
	w := NewWorld()
	w.roamRnd = rand.New(rand.NewSource(7))
	w.SetPlayerModels(map[uint32]bool{humanModel: true})
	p := &Player{ServerID: 1, Name: "Viewer", X: 1000, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	return w
}

// addNPC places one at (x, z), its spawn, with the world's settings.
func addNPC(w *World, id EntityID, name string, x, z float32, model int64, npcType int) *NPC {
	npc := &NPC{ServerID: id, Name: name, X: x, Z: z, SpawnX: x, SpawnZ: z, ModelID: model, NPCType: npcType,
		Level: 5, HP: 50, MaxHP: 50, Dummy: DummyKindOf(name)}
	w.applyNPCSettingsLocked(npc)
	w.NPCs[id] = npc
	w.SpatialIndex.Insert(id, x, z)
	return npc
}

// run ticks the wander every 100 ms from start for d, calling each after
// each tick.
func run(w *World, start time.Time, d time.Duration, each func(now time.Time)) time.Time {
	now := start
	for end := start.Add(d); now.Before(end); now = now.Add(100 * time.Millisecond) {
		w.TickCount++
		w.roamSystem(now)
		if each != nil {
			each(now)
		}
	}
	return now
}

func TestACreatureWandersWithinItsRadiusAtAWalkAndRests(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	var walked, stood, rests int
	var resting time.Time
	var legFromX, legFromZ float32
	var legs []float64
	px, pz := npc.X, npc.Z
	run(w, time.Now(), 30*time.Minute, func(now time.Time) {
		step := math.Hypot(float64(npc.X-px), float64(npc.Z-pz))
		px, pz = npc.X, npc.Z
		if d := math.Hypot(float64(npc.X-npc.SpawnX), float64(npc.Z-npc.SpawnZ)); d > float64(DefaultRoamRadius)+0.01 {
			t.Fatalf("%.1f from its spawn, past its radius %v", d, DefaultRoamRadius)
		}
		if step > float64(NPCWalkSpeed)*0.1+1e-3 {
			t.Fatalf("moved %.2f in a tick: faster than a walk", step)
		}
		switch {
		case npc.moving():
			walked++
			if npc.Loco != LocoWalk {
				t.Fatalf("walking with animation %d", npc.Loco)
			}
			if v := math.Hypot(float64(npc.VelX), float64(npc.VelZ)); math.Abs(v-float64(NPCWalkSpeed)) > 1e-3 {
				t.Fatalf("walking at %.2f, want %v", v, NPCWalkSpeed)
			}
			if !resting.IsZero() {
				// Sandstorm's rest after a walk (the first, before any, is
				// shorter), and never a short pause
				rest, slack := now.Sub(resting), 200*time.Millisecond
				if walked > 1 && (rest < roamRestMin-slack || rest > roamRestMax+slack) {
					t.Fatalf("stood %v between walks, want Sandstorm's rest of %v to %v", rest, roamRestMin, roamRestMax)
				}
				resting = time.Time{}
			}
		default:
			if walked > 0 {
				stood++
			}
			if npc.Loco != LocoStand {
				t.Fatalf("standing with animation %d", npc.Loco)
			}
			if resting.IsZero() {
				if walked > 0 {
					leg := distXZ(legFromX, legFromZ, npc.X, npc.Z)
					if leg < roamMinStep-0.01 || leg > roamLegs[len(roamLegs)-1]+0.01 {
						t.Fatalf("walked a leg of %.2f, want %v to %v", leg, roamMinStep, roamLegs[len(roamLegs)-1])
					}
					legs = append(legs, float64(leg))
				}
				resting = now
				rests++
				legFromX, legFromZ = npc.X, npc.Z
			}
		}
	})
	if len(legs) < 100 {
		t.Fatalf("in thirty minutes it walked %d legs", len(legs))
	}
	// Sandstorm's legs: median 19, 90% under 40.4 (roam.go); on its feet
	// about half the time
	sort.Float64s(legs)
	med, p90 := legs[len(legs)/2], legs[len(legs)*9/10]
	if med < 14 || med > 25 || p90 < 30 || p90 > 50 {
		t.Errorf("legs median %.1f, 90%% under %.1f: want Sandstorm's 19 and 40", med, p90)
	}
	if f := float64(walked) / float64(walked+stood); f < 0.38 || f > 0.62 {
		t.Errorf("on its feet %.0f%% of the time, want about half, as Sandstorm's", f*100)
	}
}

// Sandstorm's leg lengths are drawn as measured: roamLeg at u is the table
// at its points and straight between them.
func TestALegIsSandstormsLength(t *testing.T) {
	for i, want := range roamLegs {
		if got := roamLeg(float64(i) / float64(len(roamLegs)-1)); math.Abs(float64(got-want)) > 1e-4 {
			t.Errorf("roamLeg(%d/20) = %v, want %v", i, got, want)
		}
	}
	if got, want := roamLeg(0.525), (roamLegs[10]+roamLegs[11])/2; math.Abs(float64(got-want)) > 1e-4 {
		t.Errorf("roamLeg(0.525) = %v, want %v (halfway)", got, want)
	}
	if roamLeg(-1) != roamLegs[0] || roamLeg(2) != roamLegs[len(roamLegs)-1] {
		t.Errorf("roamLeg out of 0..1 is not clamped")
	}
}

// A creature's wander keeps out of a town: a chain of ten townsfolk and the
// ground within 40 of them (towns.go), its legs neither ending there nor
// crossing it. The control: five townsfolk are not a town, and the same
// creature walks right up to them.
func TestTheWanderKeepsOutOfTowns(t *testing.T) {
	closest := func(folk int) float32 {
		w := roamWorld(t)
		for i := 0; i < folk; i++ {
			addNPC(w, EntityID(200+i), "Guard", float32(1060+20*i), 1000, humanModel, 0)
		}
		npc := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0)
		near := float32(1e9)
		run(w, time.Now(), 20*time.Minute, func(time.Time) {
			for i := 0; i < folk; i++ {
				near = min(near, distXZ(npc.X, npc.Z, float32(1060+20*i), 1000))
			}
		})
		return near
	}
	if d := closest(10); d <= townMargin {
		t.Errorf("came within %.1f of a town's people, its margin %v", d, townMargin)
	}
	if d := closest(5); d > townMargin {
		t.Errorf("five townsfolk kept it %.1f off: the control", d)
	}
}

// Townsfolk (a player's race), a merchant or anyone talked to (0x80), a
// dummy and a body-less NPC stand, however near a player.
func TestTownsfolkMerchantsAndDummiesStand(t *testing.T) {
	w := roamWorld(t)
	still := []*NPC{
		addNPC(w, 101, "Guard Dahvel", 1005, 1000, humanModel, 0),
		addNPC(w, 102, "Spirit Merchant", 1006, 1000, creatureModel, 0x81),
		addNPC(w, 103, "Training Dummy Warrior", 1007, 1000, creatureModel, 0),
		addNPC(w, 104, "Ice Statue", 1008, 1000, 0, 0),
	}
	moved := addNPC(w, 105, "beetle", 1009, 1000, creatureModel, 0)
	run(w, time.Now(), 2*time.Minute, nil)
	for _, npc := range still {
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.moving() {
			t.Errorf("%q wandered to (%.1f, %.1f)", npc.Name, npc.X, npc.Z)
		}
	}
	if moved.X == moved.SpawnX && moved.Z == moved.SpawnZ {
		t.Fatal("the creature beside them never moved: the control")
	}
}

// A creature stops the moment it is fought or rooted, or no player is
// within range, and none far from every player moves at all.
func TestAFightARootOrNoOneNearStopsTheWalk(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	far := addNPC(w, 200, "beetle", 5000, 5000, creatureModel, 0)
	now := time.Now()
	walking := func() {
		for i := 0; !npc.moving(); i++ {
			if i > 600 {
				t.Fatal("never set off in a minute")
			}
			now = run(w, now, 100*time.Millisecond, nil)
		}
	}

	walking()
	npc.CurrentEnemy = 1
	now = run(w, now, 100*time.Millisecond, nil)
	if npc.moving() {
		t.Fatal("walks on while fought")
	}
	x, z := npc.X, npc.Z
	now = run(w, now, 30*time.Second, nil)
	if npc.X != x || npc.Z != z {
		t.Fatal("wandered while fighting")
	}
	npc.CurrentEnemy = 0

	walking()
	npc.Effects = []*ActiveEffect{{Root: true, Until: now.Add(time.Hour)}}
	now = run(w, now, 100*time.Millisecond, nil)
	if npc.moving() {
		t.Fatal("walks on rooted")
	}
	npc.Effects = nil

	walking()
	p := w.Players[1]
	w.SpatialIndex.Update(p.ServerID, p.X, p.Z, 20000, 20000)
	p.X, p.Z = 20000, 20000
	run(w, now, 2*time.Second, nil)
	if npc.moving() {
		t.Fatal("walks on with no one near")
	}
	if far.X != far.SpawnX || far.Z != far.SpawnZ {
		t.Fatal("a creature far from every player moved")
	}
}

// The walk on the state channel: the velocity (bytes 40-42), the walk (58)
// and the facing toward where it goes (15: CLIENT's atan2(x, z) × 128/pi).
func TestTheWalkOnTheChannel(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 1000, 1000, creatureModel, 0)
	w.stepNPCToward(npc, 1010, 1000, NPCWalkSpeed, 0.1) // along +x
	buf := SerializeNPCUpdate(1, npc)
	if buf[40] != velocityToByte(NPCWalkSpeed) || buf[41] != 0 || buf[42] != 0 || buf[58] != LocoWalk {
		t.Fatalf("walking +x: velocity % x, animation %d", buf[40:43], buf[58])
	}
	if buf[15] != 64 {
		t.Fatalf("walking +x faces byte %d, want 64 (pi/2)", buf[15])
	}
	w.stepNPCToward(npc, npc.X, 990, NPCWalkSpeed, 0.1) // along -z
	buf = SerializeNPCUpdate(1, npc)
	if int8(buf[42]) != int8(velocityToByte(-NPCWalkSpeed)) || buf[15] != 128 {
		t.Fatalf("walking -z: velocity z %d, facing %d (want pi: 128)", int8(buf[42]), buf[15])
	}
	if !w.stepNPCToward(npc, npc.X, npc.Z-0.1, NPCWalkSpeed, 0.1) {
		t.Fatal("within a step of its point, not arrived")
	}
	buf = SerializeNPCUpdate(1, npc)
	if buf[40] != 0 || buf[42] != 0 || buf[58] != LocoStand {
		t.Fatalf("arrived: velocity % x, animation %d", buf[40:43], buf[58])
	}
	if got := w.SpatialIndex.QueryRadius(npc.X, npc.Z, 0.5); len(got) != 1 || got[0] != npc.ServerID {
		t.Fatalf("the spatial index has %v where it stands", got)
	}
}

// A creature that respawns starts on its spawn point, standing.
func TestARespawnStandsOnItsSpawn(t *testing.T) {
	w := roamWorld(t)
	npc := addNPC(w, 100, "beetle", 1005, 1000, creatureModel, 0)
	now := run(w, time.Now(), time.Minute, nil)
	for !npc.moving() {
		now = run(w, now, 100*time.Millisecond, nil)
	}
	w.killNPC(npc)
	npc.DeathTime = now
	w.corpseSystem(now.Add(DefaultCorpseDecay + DefaultRespawnAfter))
	if npc.Dead || npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.moving() || npc.roam.walking {
		t.Fatalf("respawned at (%.1f, %.1f), moving %v, walking %v", npc.X, npc.Z, npc.moving(), npc.roam.walking)
	}
}
