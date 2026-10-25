package entity

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The wander over a real server's NPCs (EQOA_REAL_DB names its database; a
// copy is opened, the file is never written): a player stands in each of the
// largest towns, and half an hour of wandering round them leaves no creature
// in a town's ground that does not live there, every one within its radius.
// Skipped without a database: none is in this repository.
func TestTheWanderKeepsOutOfTheTownsOfARealWorld(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database to wander over")
	}
	w := realWorld(t)
	w.roamRnd = rand.New(rand.NewSource(1))

	w.mu.Lock()
	w.buildTownsLocked()
	// a player at the middle of each of the eight largest towns' grid cells
	type cell struct {
		k townKey
		n int
	}
	var cells []cell
	for k, people := range w.towns {
		cells = append(cells, cell{k, len(people)})
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].n > cells[j].n })
	for i := 0; i < 8 && i < len(cells); i++ {
		k := cells[i].k
		p := &Player{ServerID: EntityID(900000 + i), Name: "Watcher", HP: 100, MaxHP: 100,
			X: (float32(k.cx) + 0.5) * townCell, Z: (float32(k.cz) + 0.5) * townCell, World: k.world}
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	}
	t.Logf("%d townsfolk in towns, %d NPCs", w.townFolk, len(w.NPCs))
	w.mu.Unlock()

	now := time.Unix(1_800_000_000, 0)
	entered, outside, moved := map[EntityID]bool{}, map[EntityID]bool{}, map[EntityID]bool{}
	for tick := 0; tick < 18000; tick++ {
		w.mu.Lock()
		w.TickCount++
		w.roamSystem(now)
		if tick%10 == 0 {
			for _, npc := range w.roamers {
				if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ {
					moved[npc.ServerID] = true
				}
				if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) > npc.RoamRadius+0.01 {
					outside[npc.ServerID] = true
				}
				if w.inTownLocked(npc.World, npc.X, npc.Z) && !w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ) {
					entered[npc.ServerID] = true
				}
			}
		}
		w.mu.Unlock()
		now = now.Add(100 * time.Millisecond)
	}
	t.Logf("%d creatures wandered round the players", len(moved))
	if len(moved) == 0 {
		t.Fatal("no creature wandered")
	}
	if len(entered) != 0 || len(outside) != 0 {
		t.Errorf("%d creatures went into a town and %d past their radius", len(entered), len(outside))
	}
}

// realWorld is a World of the NPCs of a copy of the database EQOA_REAL_DB
// names (the file itself is never opened for writing).
func realWorld(t *testing.T) *World {
	t.Helper()
	src := os.Getenv("EQOA_REAL_DB")
	path := filepath.Join(t.TempDir(), "eqoa.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewNPCStore(db)
	w := NewWorld()
	models, err := store.PlayerModels()
	if err != nil {
		t.Fatal(err)
	}
	w.SetPlayerModels(models)
	w.LoadNPCs(store)
	return w
}
