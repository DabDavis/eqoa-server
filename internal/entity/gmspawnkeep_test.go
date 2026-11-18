package entity

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Rows of the owner's case (2026-11-18): Tunare and her two pets, spawn-only
// originals standing as eqoaclassic's 162665, 162670 and 162671 stand off each
// other, paired by nothing but trigger 141's resummons; and a creature of no one
// 9 off her, the control for every pair.
const (
	gmTunare   EntityID = 130
	gmWrath    EntityID = 131
	gmGrace    EntityID = 132
	gmStranger EntityID = 133
)

// gmSpawnRows adds the rows to keepNPCDB's database.
func gmSpawnRows(t *testing.T, db *database.SQLiteDB) {
	t.Helper()
	for _, r := range []struct {
		id   EntityID
		name string
		x, z float64
	}{
		{gmTunare, "Tunare", 3000, 3000},
		{gmWrath, "Wrath of the Mother", 2995.29, 2994.02},
		{gmGrace, "Grace of the Mother", 3007.61, 3005.21},
		{gmStranger, "a stranger", 3009, 3000},
	} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, ?, 0, ?, 1.0, 60, 0, '0')`, r.id, r.name, r.x, r.z, creatureModel); err != nil {
			t.Fatal(err)
		}
	}
}

// gmSpawnProcess is keepNPCProcess's world with the three originals spawn-only
// (npcspawnonly) and trigger 141's resummons read (handlers' Reload), or not.
func gmSpawnProcess(t *testing.T, db *database.SQLiteDB, triggers bool) *World {
	t.Helper()
	w := keepNPCProcess(t, db)
	w.mu.Lock()
	for _, id := range []EntityID{gmTunare, gmWrath, gmGrace} {
		w.NPCs[id].SpawnOnly = true
	}
	w.mu.Unlock()
	if triggers {
		w.SetTriggerCompanions(map[string]string{"Wrath of the Mother": "Tunare", "Grace of the Mother": "Tunare"})
	}
	return w
}

func gmNear(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// The owner, 2026-11-18: a GM's copy of Tunare came without Wrath and Grace of the
// Mother. #spawn (SpawnGMCopy) now brings a copy of each, standing off hers as
// the originals stand off the original, turned with her facing, paired with her
// alone: a hit on her brings both, her death takes both, #despawn takes all
// three. The controls: with no trigger naming them Tunare brings no one (the
// pair is the trigger's resummon, nothing else), a copy a trigger or a quest
// makes (SpawnNPCCopy) brings no one, and the stranger 9 off is no one's.
func TestAGMSpawnBringsItsCompanions(t *testing.T) {
	db := keepNPCDB(t)
	gmSpawnRows(t, db)

	// the control: no trigger's resummon, no companions
	if _, _, pets, ok := gmSpawnProcess(t, db, false).SpawnGMCopy("Tunare", 2250, 10, 2010, 0, 0); !ok || len(pets) != 0 {
		t.Fatalf("no trigger: %v %v, want Tunare alone", pets, ok)
	}

	w := gmSpawnProcess(t, db, true)
	// a trigger's spawn (SpawnNPCCopy) brings no one: trigger 141 spawns its own
	before := len(w.NPCs)
	if _, _, ok := w.SpawnNPCCopy("Tunare", 2250, 10, 1980, 0, 0); !ok || len(w.NPCs) != before+1 {
		t.Fatalf("a trigger's spawn made %d NPCs", len(w.NPCs)-before)
	}

	// facing a quarter turn from the original's: the offsets turn with her
	id, spelled, pets, ok := w.SpawnGMCopy("Tunare", 2250, 10, 2010, math.Pi/2, 0)
	if !ok || spelled != "Tunare" || len(pets) != 2 {
		t.Fatalf("#spawn Tunare: %d %q %v %v, want her two pets", id, spelled, pets, ok)
	}
	wrath, grace := w.NPCs[pets[0]], w.NPCs[pets[1]]
	if wrath.Name != "Wrath of the Mother" || grace.Name != "Grace of the Mother" || wrath.CopyOf != gmWrath ||
		!wrath.GMSpawn || !wrath.Temporary || wrath.SpawnOnly || !w.NPCs[id].GMSpawn {
		t.Fatalf("the pets: %+v / %+v", wrath, grace)
	}
	// Wrath stands (-4.71, -5.98) off the original; turned a quarter: (-5.98, 4.71)
	if !gmNear(wrath.X, 2250-5.98) || !gmNear(wrath.Z, 2010+4.71) || !gmNear(grace.X, 2250+5.21) || !gmNear(grace.Z, 2010-7.61) {
		t.Fatalf("placed at (%.2f, %.2f) and (%.2f, %.2f)", wrath.X, wrath.Z, grace.X, grace.Z)
	}
	if !gmNear(wrath.Facing, math.Pi/2) {
		t.Fatalf("Wrath faces %.2f, want turned with her", wrath.Facing)
	}
	for _, p := range pets {
		if m, ok := w.CompanionMaster(p); !ok || m != id {
			t.Fatalf("pet %d's master %d %v, want the copy %d", p, m, ok, id)
		}
	}
	if _, ok := w.CompanionMaster(gmStranger); ok {
		t.Fatal("the control: the stranger is someone's")
	}
	if _, ok := w.CompanionMaster(gmWrath); ok {
		t.Fatal("the spawn-only original is paired")
	}

	// a hit on her brings both; the stranger stays out of it
	p := w.Players[1]
	w.mu.Lock()
	w.updateAggro(w.NPCs[id], p.ServerID, 50)
	w.mu.Unlock()
	if wrath.Threats[p.ServerID] == 0 || grace.Threats[p.ServerID] == 0 || len(w.NPCs[gmStranger].Threats) != 0 {
		t.Fatalf("hit: Wrath %v, Grace %v, stranger %v", wrath.Threats, grace.Threats, w.NPCs[gmStranger].Threats)
	}
	// her death takes both
	w.mu.Lock()
	w.killNPC(w.NPCs[id])
	dead := wrath.Dead && grace.Dead && !w.NPCs[gmStranger].Dead
	w.mu.Unlock()
	if !dead {
		t.Fatal("her death did not take her pets")
	}

	// #despawn of another takes its pets with it
	id2, _, pets2, _ := w.SpawnGMCopy("Tunare", 2260, 10, 2010, 0, 0)
	if _, ok := w.DespawnNPC(id2); !ok {
		t.Fatal("no despawn")
	}
	for _, p := range append(pets2, id2) {
		if w.NPCs[p] != nil {
			t.Fatalf("%d outlived #despawn", p)
		}
	}
}

// keepGMSpawnsFile is kept through JSON, as the file holds it.
func keepGMSpawnsFile(t *testing.T, kept []KeptGMSpawn) []KeptGMSpawn {
	t.Helper()
	b, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptGMSpawn
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// The owner, 2026-11-18: "if a gm spawns a mob the rolling restart shouldnt
// remove that spawn". A GM's living spawn and the companions it brought come
// through a keep restart: under their ids, where they stood, at their hit points,
// one that was fighting running back to where the fight began, its /size, and the
// pets paired with her again (a hit brings them). A GM's dead spawn's corpse is
// keepcopy.go's, as before. Not carried here: a trigger's or a quest's living copy
// (its run's or quest's), a GM's spawn a player charmed. The control: a new
// process given nothing has none of them.
func TestAGMSpawnComesThroughARestart(t *testing.T) {
	db := keepNPCDB(t)
	gmSpawnRows(t, db)
	old := gmSpawnProcess(t, db, true)
	id, _, pets, ok := old.SpawnGMCopy("Tunare", 2250, 10, 2010, 0, 0)
	if !ok || len(pets) != 2 {
		t.Fatalf("#spawn Tunare: %v %v", pets, ok)
	}
	lone, _, _, _ := old.SpawnGMCopy("Lord Keeper", 2270, 10, 2010, 1, 0)
	event, _, _ := old.SpawnNPCCopy("Lord Keeper", 2280, 10, 2010, 1, 0) // a trigger's: its run's
	charmed, _, _, _ := old.SpawnGMCopy("wolf", 2290, 10, 2010, 1, 0)
	deadOne, _, _, _ := old.SpawnGMCopy("wolf", 2295, 10, 2010, 1, 0)
	p := old.Players[1]
	old.mu.Lock()
	boss := old.NPCs[id]
	boss.HP = boss.MaxHP / 2
	boss.Size, boss.gmScaleBase = 2.5, 1
	old.updateAggro(boss, p.ServerID, 50) // a fight with a player: not carried, run home from
	moved := old.NPCs[pets[1]]
	moved.X, moved.Z = 2262, 2004
	old.NPCs[charmed].Owner = p.ServerID
	old.mu.Unlock()
	killCopy(old, old.NPCs[deadOne], time.Now().Add(-5*time.Second))

	kept := keepGMSpawnsFile(t, old.KeepGMSpawns(time.Now()))
	got := map[EntityID]KeptGMSpawn{}
	for _, k := range kept {
		got[EntityID(k.ID)] = k
	}
	if len(kept) != 4 || got[id].Row != uint32(gmTunare) || got[pets[0]].CompanionOf != uint32(id) || got[lone].ID == 0 {
		t.Fatalf("written %+v, want Tunare, her pets and the lone Lord Keeper", kept)
	}
	if _, ok := got[event]; ok {
		t.Fatal("a trigger's spawn was written as a GM's")
	}
	copies := old.KeepCopies()
	if len(copies) != 1 || EntityID(copies[0].ID) != deadOne {
		t.Fatalf("the dead GM's spawn's corpse: %+v", copies)
	}

	// the control: a new process given nothing
	c := gmSpawnProcess(t, db, true)
	for _, x := range []EntityID{id, pets[0], pets[1], lone} {
		if c.NPCs[x] != nil {
			t.Fatalf("the control has %d", x)
		}
	}

	n := gmSpawnProcess(t, db, true)
	n.RestoreCopies(keepCopiesFile(t, copies))
	ids, lost := n.RestoreGMSpawns(kept)
	if lost != 0 || len(ids) != 4 {
		t.Fatalf("restored %v, %d lost", ids, lost)
	}
	nb := n.NPCs[id]
	if nb == nil || ids[id] != id || nb.Name != "Tunare" || !nb.GMSpawn || !nb.Temporary || nb.Dead {
		t.Fatalf("Tunare back: %+v", nb)
	}
	if nb.HP != boss.HP || nb.MaxHP != boss.MaxHP || nb.Size != 2.5 || nb.gmScaleBase != 1 {
		t.Fatalf("Tunare at %d/%d size %v, want %d/%d at 2.5", nb.HP, nb.MaxHP, nb.Size, boss.HP, boss.MaxHP)
	}
	if len(nb.Threats) != 0 || !nb.chase.returning && (nb.X != nb.SpawnX || nb.Z != nb.SpawnZ) {
		t.Fatalf("her fight carried: threats %v, chase %+v", nb.Threats, nb.chase)
	}
	if nm := n.NPCs[pets[1]]; nm == nil || nm.X != 2262 || nm.Z != 2004 || nm.SpawnX != moved.SpawnX {
		t.Fatalf("Grace where she stood: %+v", nm)
	}
	for _, pid := range pets {
		if m, ok := n.CompanionMaster(pid); !ok || m != id {
			t.Fatalf("pet %d's master after the restart: %d %v", pid, m, ok)
		}
	}
	if n.NPCs[charmed] != nil {
		t.Fatal("a charmed GM's spawn came back")
	}
	if d := n.NPCs[deadOne]; d == nil || !d.Dead {
		t.Fatal("the dead GM's spawn's corpse did not come back")
	}
	// fights again as one
	n.mu.Lock()
	n.updateAggro(nb, p.ServerID, 50)
	n.mu.Unlock()
	if n.NPCs[pets[0]].Threats[1] == 0 {
		t.Fatal("after the restart a hit on her does not bring Wrath")
	}

	// through a second restart too (its file from the restored world)
	again := keepGMSpawnsFile(t, n.KeepGMSpawns(time.Now()))
	m := gmSpawnProcess(t, db, true)
	if ids, lost := m.RestoreGMSpawns(again); len(ids) != 4 || lost != 0 {
		t.Fatalf("a second restart: %v, %d lost", ids, lost)
	}
	if mm, ok := m.CompanionMaster(pets[0]); !ok || mm != id {
		t.Fatal("the pair lost at the second restart")
	}
}

// /place's rows (handlers' gmnpcrow.go): a row of Tunare's name and rows of her
// pets' names standing as the originals do pair by trigger 141's resummons, as
// any row's companions, at every start; the spawn-only originals do not. The
// control: no trigger, no pair.
func TestPlacedRowsPairByTheTriggersResummon(t *testing.T) {
	db := keepNPCDB(t)
	gmSpawnRows(t, db)
	w := gmSpawnProcess(t, db, true)
	spots := w.CompanionSpots(gmTunare, 2600, 10, 2600, 0, 0)
	if len(spots) != 2 || spots[0].Row != gmWrath || !spots[0].SpawnOnly || !gmNear(spots[0].X, 2600-4.71) {
		t.Fatalf("spots %+v", spots)
	}
	for i, r := range []struct {
		id   EntityID
		name string
		x, z float32
	}{{140, "Tunare", 2600, 2600}, {141, spots[0].Name, spots[0].X, spots[0].Z}, {142, spots[1].Name, spots[1].X, spots[1].Z}} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, ?, 0, ?, 1.0, 60, 0, '0')`, r.id, r.name, r.x, r.z, creatureModel); err != nil {
			t.Fatal(i, err)
		}
	}
	for _, triggers := range []bool{true, false} {
		w := gmSpawnProcess(t, db, triggers)
		m1, ok1 := w.CompanionMaster(141)
		m2, ok2 := w.CompanionMaster(142)
		if triggers && (!ok1 || !ok2 || m1 != 140 || m2 != 140) {
			t.Fatalf("placed rows: %d %v, %d %v, want both 140's", m1, ok1, m2, ok2)
		}
		if !triggers && (ok1 || ok2) {
			t.Fatal("the control: paired with no trigger")
		}
	}
}
