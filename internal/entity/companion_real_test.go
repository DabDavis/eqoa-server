package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// realCompanionWorld is a server's world over a real database (EQOA_REAL_DB,
// copied), loaded as cmd/gameserver loads it for what the pairs need: the
// NPCs, the paths, the camps, the movement rules, the spawn-only names, and
// the ground when EQOA_GROUND_DIR names it.
func realCompanionWorld(t *testing.T) *World {
	t.Helper()
	db := simRealDB(t)
	store := database.NewNPCStore(db)
	w := NewWorld()
	if models, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(models)
	}
	w.LoadNPCs(store)
	if dir := os.Getenv("EQOA_GROUND_DIR"); dir != "" {
		if g, err := ground.Open(dir); err == nil {
			w.SetGround(g)
		}
		if e, err := ground.OpenEnv(dir); err == nil {
			w.SetEnv(e)
		}
	}
	if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
		w.LoadNPCPaths(recs)
	}
	if _, err := w.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadUndeadTags(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadMoveRules(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	return w
}

// TestRealCompanionsReport lists, over a real database, every NPC named as a
// pet that has no owner, and every pair whose owner and pet do not move
// alike (one wanders, the other stands, not walking with it), into
// EQOA_COMPANION_REPORT.
func TestRealCompanionsReport(t *testing.T) {
	out := os.Getenv("EQOA_COMPANION_REPORT")
	if out == "" {
		t.Skip("EQOA_COMPANION_REPORT names the report's file")
	}
	w := realCompanionWorld(t)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.linkCompanionsLocked()
	f, err := os.Create(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var ids []EntityID
	for id, n := range w.NPCs {
		if companionFighter(n) && companionName(n.Name) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	paired, apart := 0, 0
	for _, id := range ids {
		pet := w.NPCs[id]
		pw, pwhy, _ := w.movementLocked(pet)
		o, ok := w.companionOf[id]
		if !ok {
			fmt.Fprintf(f, "ORPHAN\t%d\t%s\tw%d\t%.0f,%.0f\t%v %s\n", id, pet.Name, pet.World, pet.SpawnX, pet.SpawnZ, pw, MoveWhyText(pwhy, ""))
			continue
		}
		paired++
		own := w.NPCs[o]
		ow, owhy, word := w.movementLocked(own)
		d := distXZ(pet.SpawnX, pet.SpawnZ, own.SpawnX, own.SpawnZ)
		tag := "PAIR"
		if ow != pw && pwhy != whyCompanion { // a pet that walks with its owner moves as it does
			tag = "APART"
			apart++
		}
		fmt.Fprintf(f, "%s\t%d\t%s\tw%d\t%.1f\t%d\t%s\towner %v %s\tpet %v %s\n", tag, id, pet.Name, pet.World, d, o, own.Name,
			ow, MoveWhyText(owhy, word), pw, MoveWhyText(pwhy, ""))
	}
	t.Logf("%d named as pets, %d paired, %d of them moving apart", len(ids), paired, apart)
}
