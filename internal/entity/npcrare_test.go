package entity

import (
	"math/rand"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Rare spawns on placeholders (npcrare.go): two placeholders of one rare come
// back as it at their chance, one at a time, on their own spots, and as
// themselves otherwise; the rare's own row stays out of the world; a restart
// that keeps the players keeps a placeholder as its rare. Over keepnpc_test.go's
// database (campDB) with these rows added.

const (
	rarePH1   EntityID = 221 // a placeholder (a myrmidon)
	rarePH2   EntityID = 222 // another of the same rare
	rareOrig  EntityID = 223 // the rare's own row, out of the world
	rarePlain EntityID = 224 // a myrmidon with no rule: the control
)

const rareModel = 0xDA72BCD5

// rareDB is campDB with the placeholders, the rare's row and the rules at chance.
func rareDB(t *testing.T, chance float64) *database.SQLiteDB {
	t.Helper()
	db := campDB(t, []campRow{
		{rarePH1, "welahi-nai myrmidon", 2700, 2000, 32, creatureModel, 60},
		{rarePH2, "welahi-nai myrmidon", 2750, 2000, 32, creatureModel, 59},
		{rareOrig, "Lord Koreshaa", 2800, 2000, 0, rareModel, 61},
		{rarePlain, "welahi-nai myrmidon", 2650, 2000, 0, creatureModel, 60},
	}, nil)
	if _, err := db.Exec(`DELETE FROM npcrare`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM npcrare`) })
	for _, id := range []EntityID{rarePH1, rarePH2} {
		if _, err := db.Exec(`INSERT INTO npcrare (npc_row, rare, chance) VALUES (?, 'lord koreshaa', ?)`, id, chance); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// rareProcess is campProcess with the rare's row out of the world (as its
// npcspawnonly row "#223" makes it) and the rules read, the roll seeded.
func rareProcess(t *testing.T, db *database.SQLiteDB) *World {
	t.Helper()
	w := campProcess(t, db)
	w.mu.Lock()
	w.NPCs[rareOrig].SpawnOnly = true
	w.mu.Unlock()
	if n, err := w.LoadRareSpawns(db); err != nil || n != 2 {
		t.Fatalf("rules read: %d, %v", n, err)
	}
	return w
}

func TestAPlaceholderComesBackAsItsRareOneAtATime(t *testing.T) {
	db := rareDB(t, 40)
	w := rareProcess(t, db)
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	ph1, ph2, plain := w.NPCs[rarePH1], w.NPCs[rarePH2], w.NPCs[rarePlain]
	seen := map[string]int{}
	for i := 0; i < 60; i++ {
		for _, id := range []EntityID{rarePH1, rarePH2, rarePlain} {
			w.mu.Lock()
			w.NPCs[id].HiddenUntil = time.Time{}
			w.mu.Unlock()
			killAndBack(t, w, id)
		}
		if w.NPCs[rarePH1] != ph1 || w.NPCs[rarePH2] != ph2 {
			t.Fatal("a placeholder is another NPC")
		}
		rares := 0
		for _, n := range []*NPC{ph1, ph2} {
			seen[n.Name]++
			switch n.Name {
			case "Lord Koreshaa":
				rares++
				if n.Level != 61 || n.ModelID != rareModel || n.SpawnOnly || n.hiddenAt(time.Now().Add(NPCRefreshHold)) {
					t.Fatalf("the rare: level %d, body %x, out of the world %v", n.Level, n.ModelID, n.SpawnOnly)
				}
			case "welahi-nai myrmidon":
				if n.ModelID != creatureModel {
					t.Fatalf("a myrmidon of body %x", n.ModelID)
				}
			default:
				t.Fatalf("a placeholder came back as %q", n.Name)
			}
		}
		if rares > 1 {
			t.Fatalf("return %d: both placeholders stand as the rare", i)
		}
		if ph1.X != 2700 || ph1.Z != 2000 || ph2.X != 2750 || ph1.ServerID != rarePH1 || ph2.ServerID != rarePH2 {
			t.Fatalf("off their spots: (%v, %v) id %d, (%v, %v) id %d", ph1.X, ph1.Z, ph1.ServerID, ph2.X, ph2.Z, ph2.ServerID)
		}
		if plain.Name != "welahi-nai myrmidon" {
			t.Fatalf("the control: a myrmidon with no rule came back as %q", plain.Name)
		}
		if orig := w.NPCs[rareOrig]; !orig.SpawnOnly || orig.X != 2800 {
			t.Fatalf("the rare's own row: out of the world %v at %v", orig.SpawnOnly, orig.X)
		}
	}
	if seen["Lord Koreshaa"] < 5 || seen["welahi-nai myrmidon"] < 30 {
		t.Fatalf("in 120 returns: %v", seen)
	}

	// a miss: a placeholder that stood as its rare comes back as itself
	w.mu.Lock()
	w.makeKindLocked(ph1, w.rares[rarePH1].own.rec, w.rares[rarePH1].rare, time.Time{})
	w.mu.Unlock()
	if ph1.Name != "Lord Koreshaa" {
		t.Fatal("setup: not the rare")
	}
	rareRand = func(*World) float64 { return 40 } // the chance is 40: a miss
	t.Cleanup(func() { rareRand = func(w *World) float64 { return w.campRand().Float64() * 100 } })
	killAndBack(t, w, rarePH1)
	if ph1.Name != "welahi-nai myrmidon" || ph1.ModelID != creatureModel || ph1.Level != 60 {
		t.Fatalf("a miss: %s of level %d", ph1.Name, ph1.Level)
	}
}

func TestAPlaceholderAsItsRareComesThroughARestart(t *testing.T) {
	db := rareDB(t, 10)
	old := rareProcess(t, db)
	if k := keptOf(old.KeepNPCs(), rarePH1); k != nil {
		t.Fatalf("a placeholder as itself written: %+v", k)
	}
	old.mu.Lock()
	r := old.rares[rarePH1]
	old.makeKindLocked(old.NPCs[rarePH1], r.own.rec, r.rare, time.Time{})
	old.mu.Unlock()
	kept := keepFile(t, keptOf(old.KeepNPCs(), rarePH1))
	if len(kept) != 1 || kept[0].Name != "Lord Koreshaa" {
		t.Fatalf("written: %+v", kept)
	}
	w := rareProcess(t, db)
	w.camps.rnd = rand.New(rand.NewSource(1))
	if res := w.RestoreNPCs(kept); res.Kind != 1 || res.Back != 1 {
		t.Fatalf("restored: %s", res)
	}
	if got := w.NPCs[rarePH1]; got.Name != "Lord Koreshaa" || got.Level != 61 || got.X != 2700 {
		t.Fatalf("after the restart: %s of level %d at %v", got.Name, got.Level, got.X)
	}
}
