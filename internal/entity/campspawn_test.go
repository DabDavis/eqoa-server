package entity

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A GM's camps (campspawn.go): their spawns stand whatever their kind, pop one
// of their kinds at random each time they come back, carry the kind through a
// restart that keeps the players, and keep the wander out of the camp. Each
// world here is a new process's over keepnpc_test.go's database (its rows
// 101-106 at (2000..2500, 10, 2000)) with this file's rows and camps added.

const (
	campWolf    EntityID = 201 // a camp's spawn made as a wolf: wildlife, that wanders as its kind
	campMixed   EntityID = 202 // made as a wolf, popping a wolf or a pariah
	campPariah  EntityID = 203 // the pariahs' own row, which the mixed spawn's pariah is made from
	campKeepOff EntityID = 204 // a camp's spawn beside the wolf of row 101
	campLiver   EntityID = 205 // a wolf that spawned in that camp's ground
)

// pariahModel is the pariahs' body.
const pariahModel = 0x5678

// campDB is keepnpc_test.go's database with rows added (id, name, x, z,
// facing, model, level) and the campspawns rows given (spawn -> kinds, all
// in camp 1), none of another test's.
func campDB(t *testing.T, rows []campRow, spawns map[EntityID]string) *database.SQLiteDB {
	t.Helper()
	db := keepNPCDB(t)
	if _, err := db.Exec(`DELETE FROM campspawns`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM campspawns`) })
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, ?, ?, ?, 1.0, ?, 0, '0')`, r.id, r.name, r.x, r.z, r.face, r.model, r.level); err != nil {
			t.Fatal(err)
		}
	}
	for id, kinds := range spawns {
		if _, err := db.Exec(`INSERT INTO campspawns (npcid, camp, kinds) VALUES (?, 1, ?)`, id, kinds); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

type campRow struct {
	id    EntityID
	name  string
	x, z  float64
	face  int
	model int64
	level int
}

// campProcess is keepNPCProcess with the camps read, its kinds' roll seeded.
func campProcess(t *testing.T, db *database.SQLiteDB) *World {
	t.Helper()
	w := keepNPCProcess(t, db)
	w.camps.rnd = rand.New(rand.NewSource(3))
	if _, err := w.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	return w
}

// A camp's wolf stands on its spawn, facing as its row says, for ten minutes
// of the wander, while the hills' wolf (row 101, the control) wanders; the
// GM panel's NPC page says why. Pulled off it, it goes home and turns to its
// row's facing, as a creature that stands does.
func TestACampSpawnStandsWhateverItsKind(t *testing.T) {
	db := campDB(t, []campRow{{campWolf, "wolf", 2600, 2000, 64, creatureModel, 20}}, map[EntityID]string{campWolf: "wolf"})
	w := campProcess(t, db)
	camp, wild := w.NPCs[campWolf], w.NPCs[keepWolf]
	if st, _ := w.NPCStateOf(campWolf); st.Movement != MoveWhyText(whyCampSpawn, "") {
		t.Fatalf("the camp's wolf: %q", st.Movement)
	}
	if st, _ := w.NPCStateOf(keepWolf); st.Movement == MoveWhyText(whyCampSpawn, "") {
		t.Fatalf("the hills' wolf: %q", st.Movement)
	}
	facing := float32(64) / 256 * 2 * math.Pi
	wildMoved := false
	run(w, time.Now(), 10*time.Minute, func(time.Time) {
		if camp.X != 2600 || camp.Z != 2000 || camp.moving() || camp.Facing != facing {
			t.Fatalf("the camp's wolf at (%v, %v) facing %v, moving %v", camp.X, camp.Z, camp.Facing, camp.moving())
		}
		if wild.X != wild.SpawnX || wild.Z != wild.SpawnZ {
			wildMoved = true
		}
	})
	if !wildMoved {
		t.Fatal("the control: the hills' wolf never wandered in ten minutes")
	}

	// a fight took it 20 off: it runs home and faces as its row says
	w.mu.Lock()
	moveTo := func(x, z float32) {
		oldX, oldZ := camp.X, camp.Z
		camp.X, camp.Z, camp.Facing = x, z, 0.5
		w.SpatialIndex.Update(camp.ServerID, oldX, oldZ, x, z)
	}
	moveTo(2620, 2000)
	camp.chase = chaseState{on: true, fromX: 2600, fromY: 10, fromZ: 2000}
	w.goHomeLocked(camp)
	w.mu.Unlock()
	now := time.Now()
	for i := 0; i < 100 && camp.chase.returning; i++ {
		now = now.Add(100 * time.Millisecond)
		w.Tick(now)
	}
	if camp.chase.returning || camp.X != 2600 || camp.Z != 2000 || camp.Facing != facing {
		t.Fatalf("after its fight: at (%v, %v) facing %v, returning %v", camp.X, camp.Z, camp.Facing, camp.chase.returning)
	}
}

// killAndBack kills npc and brings it back as its corpse's and its respawn's
// times (made a millisecond each) let it, in one tick.
func killAndBack(t *testing.T, w *World, id EntityID) {
	t.Helper()
	if !w.KillNPCNow(id) {
		t.Fatalf("NPC %d not killed", id)
	}
	w.Tick(time.Now().Add(time.Second))
	if npc := w.NPCs[id]; npc == nil || npc.Dead {
		t.Fatalf("NPC %d did not come back", id)
	}
}

// The mixed spawn (made as a wolf, popping a wolf or a pariah) comes back as
// either at random: each the kind's own figures (the wolf its own row's, the
// pariah the pariahs' row's: level, body), on its own row's spot and facing,
// alive and whole, the same NPC (its pointer kept, its id). A new kind is out
// of every view for a moment, so every client makes it again. A GM's edit of
// the pariahs' row is in the next pariah. The control: a spawn of one kind
// is always that kind.
func TestACampSpawnPopsOneOfItsKindsEachTimeItComesBack(t *testing.T) {
	db := campDB(t, []campRow{
		{campMixed, "wolf", 2700, 2000, 32, creatureModel, 20},
		{campPariah, "pariah of Xrag", 2800, 2000, 0, pariahModel, 33},
		{campWolf, "wolf", 2650, 2000, 64, creatureModel, 21},
	}, map[EntityID]string{campMixed: "wolf, pariah of Xrag", campWolf: "wolf"})
	w := campProcess(t, db)
	w.SetCorpseTimes(time.Millisecond, time.Millisecond)
	npc, single := w.NPCs[campMixed], w.NPCs[campWolf]
	facing := float32(32) / 256 * 2 * math.Pi
	seen := map[string]int{}
	for i := 0; i < 40; i++ {
		was := npc.Name
		w.mu.Lock()
		npc.HiddenUntil = time.Time{} // the last return's hold run out, as between two deaths
		w.mu.Unlock()
		killAndBack(t, w, campMixed)
		killAndBack(t, w, campWolf)
		if w.NPCs[campMixed] != npc {
			t.Fatal("the spawn is another NPC")
		}
		seen[npc.Name]++
		switch npc.Name {
		case "wolf":
			if npc.Level != 20 || npc.ModelID != creatureModel {
				t.Fatalf("a wolf of level %d, body %x", npc.Level, npc.ModelID)
			}
		case "pariah of Xrag":
			if npc.Level != 33 || npc.ModelID != pariahModel {
				t.Fatalf("a pariah of level %d, body %x", npc.Level, npc.ModelID)
			}
		default:
			t.Fatalf("it came back as %q", npc.Name)
		}
		if npc.X != 2700 || npc.Y != 10 || npc.Z != 2000 || npc.Facing != facing || npc.SpawnX != 2700 || npc.HP != npc.MaxHP || npc.ServerID != campMixed {
			t.Fatalf("%s at (%v, %v, %v) facing %v, spawn x %v, %d of %d hit points, id %d",
				npc.Name, npc.X, npc.Y, npc.Z, npc.Facing, npc.SpawnX, npc.HP, npc.MaxHP, npc.ServerID)
		}
		if hidden := npc.hiddenAt(time.Now()); hidden != (npc.Name != was) {
			t.Fatalf("came back as %s from %s: out of every view %v", npc.Name, was, hidden)
		}
		if single.Name != "wolf" || single.Level != 21 {
			t.Fatalf("the control: the one-kind spawn came back as %s of level %d", single.Name, single.Level)
		}
		if !keepIndexed(w, campMixed, 2700, 2000) {
			t.Fatal("not in the spatial index on its spot")
		}
	}
	if seen["wolf"] < 5 || seen["pariah of Xrag"] < 5 {
		t.Fatalf("in 40 returns: %v", seen)
	}

	// a GM's edit of the pariahs' row (the panel's UpsertNPC): the next pariah has it
	rec, err := database.NewNPCStore(db).GetNPC(int(campPariah))
	if err != nil {
		t.Fatal(err)
	}
	rec.NPCLevel = 40
	if _, err := w.UpsertNPC(rec, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && npc.Name != "pariah of Xrag"; i++ {
		killAndBack(t, w, campMixed)
	}
	if npc.Name != "pariah of Xrag" || npc.Level != 40 {
		t.Fatalf("after the edit: %s of level %d", npc.Name, npc.Level)
	}
}

// The wander keeps out of a camp: the hills' wolf (row 101, at (2000, 2000))
// never ends a leg within campKeepOut of the camp's spawn 40 off it, nor walks
// that near it, in an hour; the control, the same wolf with no camp read,
// does. One that spawned in the camp's ground lives there and wanders as
// before.
func TestTheWanderKeepsOutOfACamp(t *testing.T) {
	rows := []campRow{
		{campKeepOff, "pariah of Xrag", 2040, 2000, 0, pariahModel, 33},
		{campLiver, "wolf", 2060, 2000, 0, creatureModel, 20},
	}
	near := func(w *World) (wolf, liver bool) {
		a, l := w.NPCs[keepWolf], w.NPCs[campLiver]
		run(w, time.Now(), time.Hour, func(time.Time) {
			if distXZ(a.X, a.Z, 2040, 2000) <= campKeepOut {
				wolf = true
			}
			if l.X != l.SpawnX || l.Z != l.SpawnZ {
				liver = true
			}
		})
		return wolf, liver
	}
	db := campDB(t, rows, map[EntityID]string{campKeepOff: "pariah of Xrag"})
	w := campProcess(t, db)
	if wolf, liver := near(w); wolf || !liver {
		t.Fatalf("with the camp: the hills' wolf came within %v of it %v, the wolf living there wandered %v", campKeepOut, wolf, liver)
	}
	// the control: no camp read
	db = campDB(t, rows, nil)
	w = campProcess(t, db)
	if wolf, _ := near(w); !wolf {
		t.Fatalf("the control: with no camp the hills' wolf never came within %v of its spot in an hour", campKeepOut)
	}
}

// A restart that keeps the players keeps a camp's spawn as the kind it stood
// as: the old process's file names it, and the new one makes it so from its
// kinds (NPCsRestored.Kind). The control: a process given nothing has it as
// its row's kind, and a spawn standing as its row's kind is not written.
func TestACampSpawnsKindComesThroughARestart(t *testing.T) {
	rows := []campRow{
		{campMixed, "wolf", 2700, 2000, 32, creatureModel, 20},
		{campPariah, "pariah of Xrag", 2800, 2000, 0, pariahModel, 33},
	}
	db := campDB(t, rows, map[EntityID]string{campMixed: "wolf, pariah of Xrag"})
	old := campProcess(t, db)
	if k := keptOf(old.KeepNPCs(), campMixed); k != nil {
		t.Fatalf("a spawn as its row's kind written: %+v", k)
	}
	old.mu.Lock()
	npc := old.NPCs[campMixed]
	old.makeKindLocked(npc, old.camps.recs[int(campMixed)].rec, old.camps.recs[int(campPariah)], time.Time{})
	old.mu.Unlock()
	kept := keepFile(t, keptOf(old.KeepNPCs(), campMixed))
	if len(kept) != 1 || kept[0].Name != "pariah of Xrag" {
		t.Fatalf("written: %+v", kept)
	}

	w := campProcess(t, db)
	if r := w.RestoreNPCs(kept); r.Kind != 1 || r.Back != 1 || r.NewNPC != 0 {
		t.Fatalf("restored: %s", r)
	}
	if got := w.NPCs[campMixed]; got.Name != "pariah of Xrag" || got.Level != 33 || got.ModelID != pariahModel || got.X != 2700 || got.Z != 2000 {
		t.Fatalf("after the restart: %s of level %d, body %x, at (%v, %v)", got.Name, got.Level, got.ModelID, got.X, got.Z)
	}
	if fresh := campProcess(t, db); fresh.NPCs[campMixed].Name != "wolf" {
		t.Fatalf("the control: a process given nothing has %q", fresh.NPCs[campMixed].Name)
	}
}
