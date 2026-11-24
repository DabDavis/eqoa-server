package entity

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A placeholder (a GM's /ph: a campspawns row of camp 0, with weights): it
// pops its kinds at their odds, moves as the kind it is (no camp's stand, no
// keep-out), carries its kind through a keep restart, and once unbound is its
// own row's kind again at its next return.

// phDB is campDB with spawn's row a placeholder (camp 0) of kinds at weights.
func phDB(t *testing.T, rows []campRow, spawn EntityID, kinds, weights string) *database.SQLiteDB {
	t.Helper()
	db := campDB(t, rows, nil)
	if _, err := db.Exec(`INSERT INTO campspawns (npcid, camp, kinds, weights) VALUES (?, 0, ?, ?)`, spawn, kinds, weights); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPickKindByWeights(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	cs := &campSpawn{kinds: []int{1, 2, 3}, weights: []int{70, 20, 10}}
	n := [3]int{}
	const draws = 100000
	for i := 0; i < draws; i++ {
		n[cs.pickKind(rnd)]++
	}
	for i, want := range []float64{0.70, 0.20, 0.10} {
		if got := float64(n[i]) / draws; math.Abs(got-want) > 0.01 {
			t.Fatalf("kind %d drawn %.3f of the time, want %.2f (%v)", i, got, want, n)
		}
	}
	// even odds with no weights, or weights that do not fit
	for _, w := range [][]int{nil, {50}} {
		cs := &campSpawn{kinds: []int{1, 2}, weights: w}
		m := [2]int{}
		for i := 0; i < 10000; i++ {
			m[cs.pickKind(rnd)]++
		}
		if m[0] < 4700 || m[1] < 4700 {
			t.Fatalf("even odds (weights %v): %v", w, m)
		}
	}
}

// The near wolf of a placeholder row popping wolf 80, pariah 20 comes back a
// pariah about one time in five over 400 returns (seeded), each the kind's own
// figures on its spot; it is no camp's static spawn (it moves as its kind),
// keeps no wander out, and is no Teleporter camp.
func TestAPlaceholderPopsItsKindsAtTheirOdds(t *testing.T) {
	db := phDB(t, []campRow{
		{campMixed, "wolf", 2700, 2000, 32, creatureModel, 20},
		{campPariah, "pariah of Xrag", 2800, 2000, 0, pariahModel, 33},
	}, campMixed, "wolf, pariah of Xrag", "80, 20")
	w := campProcess(t, db)
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	if st, _ := w.NPCStateOf(campMixed); st.Movement == MoveWhyText(whyCampSpawn, "") {
		t.Fatalf("a placeholder stands as a camp's: %q", st.Movement)
	}
	if len(w.camps.areas[0]) != 0 || len(w.GMCampSpots()) != 0 {
		t.Fatalf("a placeholder has a camp's ground: %+v, spots %+v", w.camps.areas, w.GMCampSpots())
	}
	npc := w.NPCs[campMixed]
	seen := map[string]int{}
	const returns = 400
	for i := 0; i < returns; i++ {
		w.mu.Lock()
		npc.HiddenUntil = time.Time{}
		w.mu.Unlock()
		killAndBack(t, w, campMixed)
		seen[npc.Name]++
		switch npc.Name {
		case "wolf":
			if npc.Level != 20 || npc.ModelID != creatureModel {
				t.Fatalf("a wolf of level %d", npc.Level)
			}
		case "pariah of Xrag":
			if npc.Level != 33 || npc.ModelID != pariahModel {
				t.Fatalf("a pariah of level %d", npc.Level)
			}
		default:
			t.Fatalf("came back as %q", npc.Name)
		}
		if npc.SpawnX != 2700 || npc.SpawnZ != 2000 {
			t.Fatalf("off its spot: %v, %v", npc.SpawnX, npc.SpawnZ)
		}
	}
	if p := float64(seen["pariah of Xrag"]) / returns; p < 0.14 || p > 0.26 {
		t.Fatalf("pariahs %.2f of %d returns at 20%%: %v", p, returns, seen)
	}
}

func TestLegacyPlaceholderRetainsItsWeightedAlternate(t *testing.T) {
	rows := []campRow{
		{campMixed, "wolf", 2700, 2000, 32, creatureModel, 20},
		{campPariah, "pariah of Xrag", 2800, 2000, 0, pariahModel, 33},
	}
	db := phDB(t, rows, campMixed, "wolf, pariah of Xrag", "75, 25")
	campProcess(t, db) // existing rule, first load with the donor still available
	if _, err := db.Exec("DELETE FROM npcs WHERE id = ?", campPariah); err != nil {
		t.Fatal(err)
	}
	// Reusing the retired numeric ID for an unrelated mob must not supply the rare's body.
	if _, err := db.Exec("INSERT INTO npcs (id, npc_name, npc_level) VALUES (?, 'badger', 5)", campPariah); err != nil {
		t.Fatal(err)
	}
	w := campProcess(t, db)
	cs := w.camps.spawns[campMixed]
	if len(cs.weights) != 2 || cs.weights[0] != 75 || cs.weights[1] != 25 {
		t.Fatalf("weights after reload: %v", cs.weights)
	}
	seen := map[string]bool{}
	for range 100 {
		w.KillNPCNow(campMixed)
		w.RespawnNPCNow(campMixed)
		n := w.NPCs[campMixed]
		seen[n.Name] = true
		if n.Name == "pariah of Xrag" && (n.ModelID != pariahModel || n.Level != 33) {
			t.Fatalf("saved rare: model %x, level %d", n.ModelID, n.Level)
		}
	}
	if len(seen) != 2 || !seen["wolf"] || !seen["pariah of Xrag"] {
		t.Fatalf("kinds after cold start: %v", seen)
	}
	// A matching source becomes authoritative again when one is restored.
	if _, err := db.Exec("UPDATE npcs SET npc_name = 'pariah of Xrag', npc_level = 41 WHERE id = ?", campPariah); err != nil {
		t.Fatal(err)
	}
	w = campProcess(t, db)
	for range 100 {
		w.KillNPCNow(campMixed)
		w.RespawnNPCNow(campMixed)
		if n := w.NPCs[campMixed]; n.Name == "pariah of Xrag" {
			if n.Level != 41 {
				t.Fatalf("restored rare has level %d", n.Level)
			}
			return
		}
	}
	t.Fatal("restored rare never rolled")
}

// A placeholder standing as its other kind comes through a keep restart as
// it; unbound (its row deleted, as /ph clear does, and the camps read again),
// it stands as that kind until it dies and then is its own row's kind. The
// control: still bound, its return rolls its kinds.
func TestAPlaceholdersKindThroughARestartAndItsClear(t *testing.T) {
	rows := []campRow{
		{campMixed, "wolf", 2700, 2000, 32, creatureModel, 20},
		{campPariah, "pariah of Xrag", 2800, 2000, 0, pariahModel, 33},
	}
	db := phDB(t, rows, campMixed, "pariah of Xrag", "")
	old := campProcess(t, db)
	old.SetCorpseTimes(time.Millisecond, time.Millisecond)
	killAndBack(t, old, campMixed)
	if n := old.NPCs[campMixed]; n.Name != "pariah of Xrag" {
		t.Fatalf("bound to a pariah alone, it came back as %q", n.Name)
	}
	kept := keepFile(t, keptOf(old.KeepNPCs(), campMixed))
	if len(kept) != 1 || kept[0].Name != "pariah of Xrag" {
		t.Fatalf("written: %+v", kept)
	}
	w := campProcess(t, db)
	if r := w.RestoreNPCs(kept); r.Kind != 1 {
		t.Fatalf("restored: %s", r)
	}
	if n := w.NPCs[campMixed]; n.Name != "pariah of Xrag" || n.X != 2700 {
		t.Fatalf("after the restart: %q at %v", n.Name, n.X)
	}

	// /ph clear: the row gone, read again
	if _, err := db.Exec(`DELETE FROM campspawns WHERE npcid = ?`, campMixed); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	if n := w.NPCs[campMixed]; n.Name != "pariah of Xrag" {
		t.Fatalf("cleared, it changed before its death: %q", n.Name)
	}
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	killAndBack(t, w, campMixed)
	if n := w.NPCs[campMixed]; n.Name != "wolf" || n.Level != 20 {
		t.Fatalf("cleared, it came back as %q of level %d", n.Name, n.Level)
	}
	killAndBack(t, w, campMixed)
	if n := w.NPCs[campMixed]; n.Name != "wolf" {
		t.Fatalf("its second return: %q", n.Name)
	}
}

// An old campspawns table (no weights) is given the column, its rows even
// odds; a weights list that does not fit its kinds is read as even odds.
func TestCampSpawnsWeightsColumn(t *testing.T) {
	if w := database.CampKindWeights("90, 10", 2); len(w) != 2 || w[0] != 90 || w[1] != 10 {
		t.Fatalf("%v", w)
	}
	for _, s := range []string{"", "90", "90, x", "90, 0", "1,2,3"} {
		if w := database.CampKindWeights(s, 2); w != nil {
			t.Fatalf("%q read as %v", s, w)
		}
	}
	if s := database.CampWeightsText([]int{75, 25}); s != "75, 25" {
		t.Fatal(s)
	}
}
