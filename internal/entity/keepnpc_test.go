package entity

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/session"
)

// The creatures through a restart (keepnpc.go; the user, 2026-11-07: "when we
// restart lets leave mobs and npcs where they were"). Each process here is what
// cmd/gameserver makes: its NPCs loaded from the rows of a real database, its
// paths from npcpaths, so a data step between two processes is SQL against that
// database, and what crosses between them is the JSON a file holds. Every case's
// check runs on a second new process given nothing, its control: it must fail
// there, or the check proves nothing; and for the ones a restart does not carry,
// it must pass there, since those are as a row makes them.

const (
	keepWolf   EntityID = 101 // wildlife on a body of its own: it wanders
	keepGuard  EntityID = 102 // on a player's race's body: it stands
	keepBoss   EntityID = 103 // no animal, a body of its own: it stands
	keepPlain  EntityID = 104 // never touched
	keepDummy  EntityID = 105
	keepOrigin EntityID = 106 // stands out of the world (npcspawnonly)
)

// keepFixture is this file's one database: making one seeds the balance's tables
// row by row, many seconds on a busy machine, and a test needs only its own npcs and
// npcpaths rows. TestMain removes it.
var keepFixture struct {
	once sync.Once
	dir  string
	db   *database.SQLiteDB
	err  error
}

// keepNPCDB is the database with the rows above and no others, at (2000, 10, 2000)
// and round it, and no paths.
func keepNPCDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	f := &keepFixture
	f.once.Do(func() {
		if f.dir, f.err = os.MkdirTemp("", "keepnpc"); f.err == nil {
			f.db, f.err = database.NewSQLite(filepath.Join(f.dir, "keep.db"))
		}
	})
	if f.err != nil {
		t.Fatal(f.err)
	}
	db := f.db
	for _, q := range []string{`DELETE FROM npcs`, `DELETE FROM npcpaths`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []struct {
		id    EntityID
		name  string
		x, z  float64
		face  int
		model int64
		level int
	}{
		{keepWolf, "wolf", 2000, 2000, 64, creatureModel, 20},
		{keepGuard, "Guard Keeper", 2100, 2000, 32, humanModel, 35},
		{keepBoss, "Lord Keeper", 2200, 2000, 16, creatureModel, 50},
		{keepPlain, "a plain stander", 2300, 2000, 0, creatureModel, 5},
		{keepDummy, "Training Dummy Lv50", 2400, 2000, 0, humanModel, 50},
		{keepOrigin, "the original", 2500, 2000, 0, creatureModel, 40},
	} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, ?, ?, ?, 1.0, ?, 0, '0')`, r.id, r.name, r.x, r.z, r.face, r.model, r.level); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// keepNPCProcess is a new process's world over db, as cmd/gameserver loads one,
// with a player near every row.
func keepNPCProcess(t *testing.T, db *database.SQLiteDB) *World {
	t.Helper()
	w := NewWorld()
	w.SessionManager = session.NewManager()
	w.roamRnd = rand.New(rand.NewSource(7))
	w.SetPlayerModels(map[uint32]bool{humanModel: true})
	w.LoadNPCs(database.NewNPCStore(db))
	recs, _, err := database.NewNPCPathStore(db).All()
	if err != nil {
		t.Fatal(err)
	}
	w.LoadNPCPaths(recs)
	w.mu.Lock()
	if npc := w.NPCs[keepOrigin]; npc != nil {
		npc.SpawnOnly = true
	}
	w.mu.Unlock()
	p := &Player{ServerID: 1, Name: "Viewer", X: 2250, Y: 10, Z: 2000, Level: 60, HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	return w
}

// keepFile is what a file carries of kept: through JSON and back.
func keepFile(t *testing.T, kept []KeptNPC) []KeptNPC {
	t.Helper()
	b, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptNPC
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// keptOf is kept's entry for id alone: the wolf wanders in any process that ticks.
func keptOf(kept []KeptNPC, id EntityID) []KeptNPC {
	for i := range kept {
		if EntityID(kept[i].ID) == id {
			return kept[i : i+1]
		}
	}
	return nil
}

// moveNPC moves npc as a step does: its place, the spatial index.
func moveNPC(w *World, npc *NPC, x, y, z, facing float32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	oldX, oldZ := npc.X, npc.Z
	npc.X, npc.Y, npc.Z, npc.Facing = x, y, z, facing
	w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, x, z)
}

// standsAt is an error unless npc stands at the place, indexed there and not on
// its spawn, not moving.
func standsAt(w *World, npc *NPC, x, y, z, facing float32) error {
	if npc.X != x || npc.Y != y || npc.Z != z || npc.Facing != facing {
		return fmt.Errorf("at (%v, %v, %v) facing %v, want (%v, %v, %v) facing %v", npc.X, npc.Y, npc.Z, npc.Facing, x, y, z, facing)
	}
	if npc.moving() {
		return fmt.Errorf("moving at %v, %v, %v", npc.VelX, npc.VelY, npc.VelZ)
	}
	if !keepIndexed(w, npc.ServerID, x, z) {
		return fmt.Errorf("not in the spatial index at (%v, %v)", x, z)
	}
	if (npc.SpawnX != x || npc.SpawnZ != z) && keepIndexed(w, npc.ServerID, npc.SpawnX, npc.SpawnZ) {
		return fmt.Errorf("still in the spatial index at its spawn")
	}
	return nil
}

func keepIndexed(w *World, id EntityID, x, z float32) bool {
	for _, found := range w.SpatialIndex.QueryRadius(x, z, 0.5) {
		if found == id {
			return true
		}
	}
	return false
}

// keepCase is a creature's state in the old process, a data step, and what the
// new process must make of it.
type keepCase struct {
	name string
	id   EntityID
	old  func(t *testing.T, w *World, npc *NPC, now time.Time)
	step string // the data step's SQL, in the swap's hold
	// asARow: nothing carried shows in it, so its check holds with nothing given
	// back too; otherwise the check must fail there
	asARow bool
	want   NPCsRestored
	check  func(w *World, npc *NPC, now time.Time) error
}

func TestTheCreaturesComeThroughARestartAsTheyWere(t *testing.T) {
	const (
		x, y, z, f = float32(2030), float32(11), float32(1988), float32(1.25)
	)
	spawnedAt := func(npc *NPC, x, y, z, f float32) error {
		if npc.SpawnX != x || npc.SpawnY != y || npc.SpawnZ != z || npc.SpawnFacing != f {
			return fmt.Errorf("its spawn is (%v, %v, %v) facing %v", npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing)
		}
		return nil
	}
	hurt := func(w *World, npc *NPC, hp int) {
		w.mu.Lock()
		npc.HP = hp
		w.wakeNPCLocked(npc)
		w.mu.Unlock()
	}
	// comesBack: dead now, still dead at due less a second, and at due on its
	// spawn, whole, indexed there
	comesBack := func(w *World, npc *NPC, now, due time.Time) error {
		if !npc.Dead || npc.HP != 0 {
			return fmt.Errorf("dead %v, %d hit points", npc.Dead, npc.HP)
		}
		w.Tick(now)
		w.Tick(due.Add(-time.Second))
		if !npc.Dead {
			return fmt.Errorf("back a second before its time")
		}
		w.Tick(due)
		if npc.Dead || npc.HP != npc.MaxHP || npc.X != npc.SpawnX || npc.Z != npc.SpawnZ {
			return fmt.Errorf("at its time: dead %v, %d of %d hit points, at (%v, %v)", npc.Dead, npc.HP, npc.MaxHP, npc.X, npc.Z)
		}
		if !keepIndexed(w, npc.ServerID, npc.SpawnX, npc.SpawnZ) {
			return fmt.Errorf("not in the spatial index on its spawn")
		}
		return nil
	}
	fight := func(w *World, npc *NPC, fromX, fromY, fromZ float32) {
		w.mu.Lock()
		npc.chase = chaseState{on: true, fromX: fromX, fromY: fromY, fromZ: fromZ}
		npc.CurrentEnemy = 1
		npc.Threats = map[EntityID]int64{1: 500}
		w.wakeNPCLocked(npc)
		w.mu.Unlock()
	}
	// runsHome: a creature on its way back from (x, z) to home: at most a step a
	// tick, no fight, standing there at the end
	runsHome := func(w *World, npc *NPC, now time.Time, homeX, homeY, homeZ float32) error {
		if npc.CurrentEnemy != 0 || npc.Threats != nil {
			return fmt.Errorf("still fighting: enemy %d, threats %v", npc.CurrentEnemy, npc.Threats)
		}
		if !npc.chase.returning {
			return fmt.Errorf("not on its way home")
		}
		px, pz := npc.X, npc.Z
		for i := 0; i < 400 && npc.chase.returning; i++ {
			now = now.Add(100 * time.Millisecond)
			w.Tick(now)
			if d := distXZ(px, pz, npc.X, npc.Z); d > NPCRunSpeed*0.1+0.01 {
				return fmt.Errorf("went %v in a tick", d)
			}
			px, pz = npc.X, npc.Z
		}
		if npc.chase.returning || npc.moving() || distXZ(npc.X, npc.Z, homeX, homeZ) > 0.01 || npc.Y != homeY {
			return fmt.Errorf("did not get home: at (%v, %v, %v), returning %v, moving %v", npc.X, npc.Y, npc.Z, npc.chase.returning, npc.moving())
		}
		return nil
	}

	cases := []keepCase{
		{name: "moved", id: keepWolf, want: NPCsRestored{Back: 1, Placed: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) { moveNPC(w, npc, x, y, z, f) },
			check: func(w *World, npc *NPC, now time.Time) error {
				return standsAt(w, npc, x, y, z, f)
			}},
		{name: "hurt on its spawn", id: keepBoss, want: NPCsRestored{Back: 1, Hurt: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) { hurt(w, npc, npc.MaxHP*2/5) },
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != npc.MaxHP*2/5 {
					return fmt.Errorf("%d of %d hit points", npc.HP, npc.MaxHP)
				}
				if err := standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing); err != nil {
					return err
				}
				// its regeneration runs on: past its two calm ticks it heals, the sets knowing it
				was := npc.HP
				for i := 0; i < 8; i++ {
					w.TickCount += uint64(RegenInterval/(100*time.Millisecond)) - 1
					w.Tick(now.Add(time.Duration(i) * RegenInterval))
				}
				if npc.HP <= was {
					return fmt.Errorf("does not heal: %d of %d", npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "moved and hurt", id: keepGuard, want: NPCsRestored{Back: 1, Hurt: 1, Placed: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, npc.SpawnX+0.25, npc.SpawnY, npc.SpawnZ, f)
				hurt(w, npc, 7)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != 7 || npc.X != npc.SpawnX+0.25 || npc.Facing != f {
					return fmt.Errorf("%d hit points at x %v facing %v", npc.HP, npc.X, npc.Facing)
				}
				return nil
			}},
		{name: "dead where it fell, its corpse lying", id: keepBoss, want: NPCsRestored{Back: 1, Dead: 1, Placed: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
				if !w.KillNPCNow(npc.ServerID) {
					t.Fatal("no kill")
				}
				npc.DeathTime = now.Add(-time.Minute)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if err := standsAt(w, npc, x, y, z, f); err != nil {
					return err
				}
				died := now.Add(-time.Minute)
				if !npc.DeathTime.Equal(died) || !npc.CorpseGone.IsZero() {
					return fmt.Errorf("died %v, corpse gone %v", npc.DeathTime, npc.CorpseGone)
				}
				if !npc.corpseLies(now, w.corpseDecay) {
					return fmt.Errorf("no corpse lies")
				}
				return comesBack(w, npc, now, died.Add(DefaultCorpseDecay+DefaultRespawnAfter))
			}},
		{name: "dead, its corpse gone, its respawn's time left", id: keepBoss, want: NPCsRestored{Back: 1, Dead: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				w.KillNPCNow(npc.ServerID)
				npc.DeathTime, npc.CorpseGone = now.Add(-time.Minute), now.Add(-20*time.Second)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.corpseLies(now, w.corpseDecay) {
					return fmt.Errorf("its corpse lies again")
				}
				return comesBack(w, npc, now, now.Add(DefaultRespawnAfter-20*time.Second))
			}},
		{name: "dead past its time: back at the first tick", id: keepBoss, want: NPCsRestored{Back: 1, Dead: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				w.KillNPCNow(npc.ServerID)
				npc.DeathTime = now.Add(-(DefaultCorpseDecay + DefaultRespawnAfter + time.Second))
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if !npc.Dead {
					return fmt.Errorf("alive before its first tick")
				}
				w.Tick(now)
				if npc.Dead || npc.HP != npc.MaxHP {
					return fmt.Errorf("after its first tick: dead %v, %d of %d", npc.Dead, npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "a GM's hold stays: it does not come back", id: keepBoss, want: NPCsRestored{Back: 1, Dead: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				w.HoldNPC(npc.ServerID, true)
				npc.DeathTime = now.Add(-time.Hour)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				w.Tick(now)
				if !npc.Dead || !npc.GMHeld {
					return fmt.Errorf("dead %v, held %v", npc.Dead, npc.GMHeld)
				}
				w.HoldNPC(npc.ServerID, false)
				if npc.Dead {
					return fmt.Errorf("Unkill Spawner did not bring it back")
				}
				return nil
			}},
		{name: "the guards' claim on the kill stays", id: keepBoss, want: NPCsRestored{Back: 1, Dead: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				npc.guardHit = true
				w.KillNPCNow(npc.ServerID)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if !npc.Dead || !w.GuardsKill(npc.ServerID) {
					return fmt.Errorf("dead %v, the guards' kill %v", npc.Dead, w.GuardsKill(npc.ServerID))
				}
				return nil
			}},

		{name: "on its way home after a fight", id: keepBoss, want: NPCsRestored{Back: 1, Hurt: 1, Placed: 1, Fight: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, npc.SpawnX+40, npc.SpawnY, npc.SpawnZ, f)
				hurt(w, npc, npc.MaxHP/2)
				w.mu.Lock()
				npc.chase = chaseState{on: true, fromX: npc.SpawnX, fromY: npc.SpawnY, fromZ: npc.SpawnZ}
				w.goHomeLocked(npc)
				w.mu.Unlock()
				if !npc.chase.returning {
					t.Fatal("not on its way home")
				}
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != npc.MaxHP/2 || npc.X != npc.SpawnX+40 {
					return fmt.Errorf("%d of %d hit points at x %v", npc.HP, npc.MaxHP, npc.X)
				}
				return runsHome(w, npc, now, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
			}},
		{name: "in a fight: it ends and it goes home", id: keepBoss, want: NPCsRestored{Back: 1, Hurt: 1, Placed: 1, Fight: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, npc.SpawnX+25, npc.SpawnY, npc.SpawnZ+5, f)
				hurt(w, npc, npc.MaxHP*2/5)
				fight(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != npc.MaxHP*2/5 {
					return fmt.Errorf("%d of %d hit points", npc.HP, npc.MaxHP)
				}
				return runsHome(w, npc, now, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
			}},
		{name: "in a fight where it began: it stands, the fight over", id: keepGuard, want: NPCsRestored{Back: 1, Hurt: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				hurt(w, npc, 11)
				fight(w, npc, npc.X, npc.Y, npc.Z)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != 11 || npc.chase.on || npc.chase.returning || npc.CurrentEnemy != 0 || npc.moving() {
					return fmt.Errorf("%d hit points, chase %+v, enemy %d", npc.HP, npc.chase, npc.CurrentEnemy)
				}
				return standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing)
			}},
		{name: "left away from its spawn with nothing to bring it back: it walks", id: keepBoss,
			want: NPCsRestored{Back: 1, Placed: 1, ToSpawn: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, npc.SpawnX+30, npc.SpawnY, npc.SpawnZ, f)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.X != npc.SpawnX+30 || !w.paths.home[npc.ServerID] {
					return fmt.Errorf("at x %v, walking to its spawn %v", npc.X, w.paths.home[npc.ServerID])
				}
				for i := 1; i < 200; i++ {
					w.Tick(now.Add(time.Duration(i) * 100 * time.Millisecond))
				}
				if w.paths.home[npc.ServerID] || npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.Facing != npc.SpawnFacing || npc.moving() {
					return fmt.Errorf("after 20 s: at (%v, %v) facing %v, walking %v", npc.X, npc.Z, npc.Facing, w.paths.home[npc.ServerID])
				}
				return nil
			}},

		{name: "let go by a charm away from its spawn: free, and it walks back", id: keepBoss,
			want: NPCsRestored{Back: 1, Hurt: 1, Placed: 1, ToSpawn: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				spell := captivation()
				w.mu.Lock()
				w.charmLocked(w.Players[1], nil, npc, &spell, now)
				npc.X, npc.Z = npc.SpawnX+50, npc.SpawnZ
				w.SpatialIndex.Update(npc.ServerID, npc.SpawnX, npc.SpawnZ, npc.X, npc.Z)
				npc.HP = npc.MaxHP / 2
				w.releaseCharmLocked(w.pets[1], nil, "", false)
				w.mu.Unlock()
				if npc.Owner != 0 || npc.Charm != nil {
					t.Fatalf("not let go: owner %d, charm %v", npc.Owner, npc.Charm)
				}
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.X != npc.SpawnX+50 || npc.HP != npc.MaxHP/2 || npc.Owner != 0 || npc.Charm != nil {
					return fmt.Errorf("at x %v, %d of %d, owner %d", npc.X, npc.HP, npc.MaxHP, npc.Owner)
				}
				for i := 1; i < 200; i++ {
					w.Tick(now.Add(time.Duration(i) * 100 * time.Millisecond))
				}
				if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.moving() {
					return fmt.Errorf("after 20 s: at (%v, %v)", npc.X, npc.Z)
				}
				return nil
			}},

		{name: "a row deleted: its entry unused", id: keepBoss, step: `DELETE FROM npcs WHERE id = 103`,
			asARow: true, want: NPCsRestored{RowGone: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
				w.KillNPCNow(npc.ServerID)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc != nil {
					return fmt.Errorf("still in the world: %+v", npc)
				}
				if n := len(w.SpatialIndex.QueryRadius(x, z, 1)); n != 0 {
					return fmt.Errorf("%d in the index at its old place", n)
				}
				return nil
			}},
		{name: "a row's spawn moved: the new place, its hit points kept", id: keepBoss,
			step: `UPDATE npcs SET x_coord = 2600, y_coord = 20, z_coord = 2700, facing = 128 WHERE id = 103`,
			want: NPCsRestored{Back: 1, Hurt: 1, RowMoved: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
				hurt(w, npc, npc.MaxHP/4)
				fight(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if err := spawnedAt(npc, 2600, 20, 2700, math.Pi); err != nil {
					return err
				}
				if err := standsAt(w, npc, 2600, 20, 2700, math.Pi); err != nil {
					return err
				}
				if npc.chase.returning || npc.chase.on || keepIndexed(w, npc.ServerID, x, z) {
					return fmt.Errorf("chase %+v, or in the index at the kept place", npc.chase)
				}
				if npc.HP != npc.MaxHP/4 {
					return fmt.Errorf("%d of %d hit points", npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "a row's facing changed: its new facing", id: keepBoss, step: `UPDATE npcs SET facing = 128 WHERE id = 103`,
			asARow: true, want: NPCsRestored{Back: 1, RowMoved: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, npc.SpawnX+0.25, npc.SpawnY, npc.SpawnZ, f)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				return standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, math.Pi)
			}},
		{name: "a row's world changed: in its new world", id: keepBoss, step: `UPDATE npcs SET world = 1 WHERE id = 103`,
			asARow: true, want: NPCsRestored{Back: 1, RowMoved: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.World != 1 {
					return fmt.Errorf("world %d", npc.World)
				}
				return standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing)
			}},
		{name: "a row moved while it is dead: dead still, back on the new place", id: keepBoss,
			step: `UPDATE npcs SET x_coord = 2600, z_coord = 2700 WHERE id = 103`,
			want: NPCsRestored{Back: 1, Dead: 1, RowMoved: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
				w.KillNPCNow(npc.ServerID)
				npc.DeathTime = now.Add(-time.Minute)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if err := spawnedAt(npc, 2600, 10, 2700, npc.SpawnFacing); err != nil {
					return err
				}
				if npc.X != 2600 || npc.Z != 2700 || !keepIndexed(w, npc.ServerID, 2600, 2700) {
					return fmt.Errorf("its corpse at (%v, %v)", npc.X, npc.Z)
				}
				return comesBack(w, npc, now, now.Add(DefaultCorpseDecay+DefaultRespawnAfter-time.Minute))
			}},
		{name: "another creature on the id: as its row makes it", id: keepBoss,
			step:   `UPDATE npcs SET npc_name = 'a stranger', x_coord = 2600, z_coord = 2700 WHERE id = 103`,
			asARow: true, want: NPCsRestored{NewNPC: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				moveNPC(w, npc, x, y, z, f)
				w.KillNPCNow(npc.ServerID)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.Dead || npc.HP != npc.MaxHP || npc.Name != "a stranger" {
					return fmt.Errorf("%q dead %v, %d of %d", npc.Name, npc.Dead, npc.HP, npc.MaxHP)
				}
				return standsAt(w, npc, 2600, 10, 2700, npc.SpawnFacing)
			}},
		{name: "a name changed on its spawn: the same creature, its name the row's", id: keepBoss,
			step: `UPDATE npcs SET npc_name = 'Lord Keepir' WHERE id = 103`,
			want: NPCsRestored{Back: 1, Dead: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				w.KillNPCNow(npc.ServerID)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if !npc.Dead || npc.Name != "Lord Keepir" {
					return fmt.Errorf("%q dead %v", npc.Name, npc.Dead)
				}
				return nil
			}},
		{name: "a level changed: the same share of its hit points", id: keepBoss,
			step: `UPDATE npcs SET npc_level = 60 WHERE id = 103`,
			want: NPCsRestored{Back: 1, Hurt: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				hurt(w, npc, npc.MaxHP*2/5)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				want := NPCMaxHP(60)
				if npc.Level != 60 || npc.MaxHP != want {
					return fmt.Errorf("level %d, %d hit points at most, want 60 and %d", npc.Level, npc.MaxHP, want)
				}
				if d := npc.HP - want*2/5; d < -1 || d > 1 {
					return fmt.Errorf("%d of %d hit points, want 2/5", npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "a row made a dummy by a data step: not used", id: keepBoss,
			step:   `UPDATE npcs SET npc_name = 'Training Dummy Lv50' WHERE id = 103`,
			asARow: true, want: NPCsRestored{NotKept: 1},
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				w.KillNPCNow(npc.ServerID)
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.Dead {
					return fmt.Errorf("a dead dummy")
				}
				return nil
			}},

		{name: "a charmed creature: free, whole, on its spawn", id: keepBoss, asARow: true,
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				rowType, rowFaction := npc.NPCType, npc.Faction
				spell := captivation()
				w.mu.Lock()
				w.charmLocked(w.Players[1], nil, npc, &spell, now)
				w.mu.Unlock()
				if npc.Owner != 1 || npc.Charm == nil || npc.NPCType == rowType && rowType != charmNPCType {
					t.Fatalf("not charmed: owner %d charm %v", npc.Owner, npc.Charm)
				}
				_ = rowFaction
				moveNPC(w, npc, x, y, z, f)
				npc.World = 3
				npc.HP = 5
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.Owner != 0 || npc.Charm != nil || npc.Dead || npc.HP != npc.MaxHP || npc.World != 0 || npc.MaxPower != 0 {
					return fmt.Errorf("owner %d charm %v dead %v, %d of %d, world %d", npc.Owner, npc.Charm, npc.Dead, npc.HP, npc.MaxHP, npc.World)
				}
				if npc.NPCType != 0 || w.pets[1] != nil {
					return fmt.Errorf("npc flags %#x, the player's pet %v", npc.NPCType, w.pets[1])
				}
				if err := standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing); err != nil {
					return err
				}
				w.Tick(now) // and nothing in the systems minds
				return nil
			}},
		{name: "a charmed creature dead as a pet: back, as a restart always had it", id: keepBoss, asARow: true,
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				spell := captivation()
				w.mu.Lock()
				w.charmLocked(w.Players[1], nil, npc, &spell, now)
				npc.HP = 0
				w.killNPC(npc)
				w.mu.Unlock()
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.Dead || npc.Owner != 0 || npc.Charm != nil || npc.HP != npc.MaxHP {
					return fmt.Errorf("dead %v owner %d charm %v, %d of %d", npc.Dead, npc.Owner, npc.Charm, npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "a training dummy", id: keepDummy, asARow: true,
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {
				npc.DummyTarget = 1
				npc.Facing = f
			},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.DummyTarget != 0 || npc.Facing != npc.SpawnFacing {
					return fmt.Errorf("fights %d, facing %v", npc.DummyTarget, npc.Facing)
				}
				return nil
			}},
		{name: "a spawn-only original", id: keepOrigin, asARow: true,
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) { hurt(w, npc, 1) },
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.HP != npc.MaxHP {
					return fmt.Errorf("%d of %d hit points", npc.HP, npc.MaxHP)
				}
				return nil
			}},
		{name: "untouched: not written, as its row makes it", id: keepPlain, asARow: true,
			old: func(t *testing.T, w *World, npc *NPC, now time.Time) {},
			check: func(w *World, npc *NPC, now time.Time) error {
				if npc.Dead || npc.HP != npc.MaxHP {
					return fmt.Errorf("dead %v, %d of %d", npc.Dead, npc.HP, npc.MaxHP)
				}
				return standsAt(w, npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing)
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := keepNPCDB(t)
			old := keepNPCProcess(t, db)
			now := time.Now()
			c.old(t, old, old.NPCs[c.id], now)
			kept := keepFile(t, old.KeepNPCs())
			written := false
			for _, k := range kept {
				written = written || EntityID(k.ID) == c.id
			}
			if !c.asARow && !written || c.want == (NPCsRestored{}) && written {
				t.Fatalf("written: %v (%+v)", written, kept)
			}
			if c.step != "" {
				if _, err := db.Exec(c.step); err != nil {
					t.Fatal(err)
				}
			}

			w := keepNPCProcess(t, db)
			if r := w.RestoreNPCs(kept); r != c.want {
				t.Errorf("restored %+v, want %+v", r, c.want)
			}
			if err := c.check(w, w.NPCs[c.id], now); err != nil {
				t.Errorf("after the restart: %v", err)
			}
			w.Tick(now.Add(time.Hour)) // no system minds it, the sets checked (main_test.go)

			// the control: a new process given nothing, as every restart was
			control := keepNPCProcess(t, db)
			err := c.check(control, control.NPCs[c.id], now)
			if c.asARow && err != nil {
				t.Errorf("control: a row makes it otherwise: %v", err)
			}
			if !c.asARow && err == nil {
				t.Error("control: its check passes with nothing given back, so it proves nothing")
			}
		})
	}
}

// Only what differs from its row is written: the section stays small however many
// NPCs the world holds, and none of those that are not carried.
func TestOnlyWhatDiffersFromItsRowIsWritten(t *testing.T) {
	w := NewWorld()
	w.SetPlayerModels(map[uint32]bool{humanModel: true})
	for i := 0; i < 1000; i++ {
		addNPC(w, EntityID(1000+i), "beetle", 2000+float32(i%40), 2000+float32(i/40), creatureModel, 0)
	}
	if got := w.KeepNPCs(); len(got) != 0 {
		t.Fatalf("%d written of a world on its rows: %+v", len(got), got[0])
	}
	moveNPC(w, w.NPCs[1000], 2001.5, 0, 2000, 0)
	w.NPCs[1001].HP = 1
	w.KillNPCNow(1002)
	w.NPCs[1003].Facing = 2 // a turn alone: it stands otherwise than its row
	w.NPCs[1004].Temporary = true
	w.NPCs[1004].HP = 1
	w.NPCs[1005].Owner = 1
	w.NPCs[1005].HP = 1
	got := map[uint32]KeptNPC{}
	for _, k := range w.KeepNPCs() {
		got[k.ID] = k
	}
	if len(got) != 4 {
		t.Fatalf("written %v, want 1000 to 1003", got)
	}
	for _, id := range []uint32{1000, 1001, 1002, 1003} {
		if _, ok := got[id]; !ok {
			t.Errorf("%d not written", id)
		}
	}
	if k := got[1001]; k.Place != nil || k.HP != 1 || k.MaxHP != w.NPCs[1001].MaxHP {
		t.Errorf("the hurt one: %+v", k)
	}
	if k := got[1002]; !k.Dead || k.Died.IsZero() || !k.Gone.IsZero() || k.Place != nil {
		t.Errorf("the dead one: %+v", k)
	}
	if b, err := json.Marshal(got[1003]); err != nil || len(b) > 120 {
		t.Errorf("one turned, as JSON: %d bytes, %v: %s", len(b), err, b)
	}
}

// What no row made is not written and does not come back: a GM's spawn, a quest's
// copy, a pet, a genie. The control is the row beside them, moved the same way.
func TestATemporaryOneIsGoneAndNoRowIsTouched(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	id, _, ok := old.SpawnNPCCopy("Lord Keeper", 2210, 10, 2005, 1, 0)
	if !ok {
		t.Fatal("no spawn")
	}
	spawned := old.NPCs[id]
	spawned.HP = 3
	moveNPC(old, spawned, 2215, 10, 2005, 2)
	pet := &NPC{ServerID: id + 1, Name: "a skeleton", Temporary: true, Owner: 1, Level: 20, HP: 3, MaxHP: 50, X: 2216, Z: 2005, SpawnX: 2216, SpawnZ: 2005}
	old.mu.Lock()
	old.putNPCLocked(pet)
	old.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	old.mu.Unlock()
	boss := old.NPCs[keepBoss]
	moveNPC(old, boss, boss.SpawnX+0.25, boss.SpawnY, boss.SpawnZ, 2)

	kept := keepFile(t, old.KeepNPCs())
	if len(kept) != 1 || EntityID(kept[0].ID) != keepBoss {
		t.Fatalf("written %+v, want the row alone", kept)
	}
	w := keepNPCProcess(t, db)
	if r := w.RestoreNPCs(kept); r != (NPCsRestored{Back: 1, Placed: 1}) {
		t.Fatalf("restored %+v", r)
	}
	if w.NPCs[id] != nil || w.NPCs[id+1] != nil {
		t.Fatalf("back: %+v %+v", w.NPCs[id], w.NPCs[id+1])
	}
	// and were one on the id (a new process's spawn), the file's word is not for it
	w.mu.Lock()
	w.NPCs[keepBoss].Temporary = true
	w.mu.Unlock()
	if r := w.RestoreNPCs(kept); r != (NPCsRestored{NotKept: 1}) {
		t.Fatalf("over a temporary one: %+v", r)
	}
}

// A creature that wanders takes its next leg from where it stands: first a rest,
// then steps of a walk's length from there, never its spawn's. A creature on a leg
// when the restart came stands until then, and no leg's state comes along.
func TestAWandererTakesItsNextLegFromWhereItStands(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	wolf := old.NPCs[keepWolf]
	old.Players[1].X, old.Players[1].Z = wolf.X, wolf.Z // the viewer beside it

	now := time.Now()
	walked := false
	now = pathRun(old, now, time.Minute, func(time.Time) {
		walked = walked || wolf.roam.walking
	})
	for !wolf.roam.walking { // mid-leg, moving
		now = pathRun(old, now, 100*time.Millisecond, nil)
	}
	now = pathRun(old, now, 500*time.Millisecond, nil)
	if !walked || !wolf.moving() || wolf.X == wolf.SpawnX {
		t.Fatalf("no walk to carry: at (%v, %v), moving %v", wolf.X, wolf.Z, wolf.moving())
	}
	x, y, z, f := wolf.X, wolf.Y, wolf.Z, wolf.Facing

	kept := keepFile(t, old.KeepNPCs())
	w := keepNPCProcess(t, db)
	w.Players[1].X, w.Players[1].Z = x, z
	if r := w.RestoreNPCs(kept); r != (NPCsRestored{Back: 1, Placed: 1}) {
		t.Fatalf("restored %+v of %+v", r, kept)
	}
	wolf = w.NPCs[keepWolf]
	if err := standsAt(w, wolf, x, y, z, f); err != nil {
		t.Fatal(err)
	}
	if wolf.roam.walking || !wolf.roam.restUntil.IsZero() {
		t.Fatalf("a leg's state came along: %+v", wolf.roam)
	}

	// stands, then walks from there at a walk: no snap at any step
	px, pz := wolf.X, wolf.Z
	moved := false
	pathRun(w, now, 2*time.Minute, func(time.Time) {
		if d := distXZ(px, pz, wolf.X, wolf.Z); d > NPCWalkSpeed*0.5+0.01 {
			t.Fatalf("went %v in a tick, from (%v, %v) to (%v, %v)", d, px, pz, wolf.X, wolf.Z)
		}
		moved = moved || wolf.X != x
		if distXZ(wolf.X, wolf.Z, wolf.SpawnX, wolf.SpawnZ) > w.roamRadiusLocked(wolf)+0.01 {
			t.Fatalf("past its radius at (%v, %v)", wolf.X, wolf.Z)
		}
		px, pz = wolf.X, wolf.Z
	})
	if !moved {
		t.Fatal("never walked on")
	}
}

// keepPathRow is a path of the square (path_test.go) round a row, in npcpaths.
func keepPathRow(t *testing.T, db *database.SQLiteDB, id EntityID, mode string) {
	t.Helper()
	points := `[{"x":2180,"y":10,"z":1980},{"x":2220,"y":10,"z":1980},{"x":2220,"y":10,"z":2020},{"x":2180,"y":10,"z":2020}]`
	if _, err := db.Exec(`INSERT INTO npcpaths (npcid, world, mode, pause, speed, points) VALUES (?, 0, ?, 0, 'walk', ?)`, id, mode, points); err != nil {
		t.Fatal(err)
	}
}

// A path walker carries on along the leg it was on, with no snap and no start over,
// and one held at the end of a path walked once stays there (a fresh pathState
// would walk it back to the first point). The control: given nothing it starts on
// its spawn; and given its place alone (no path state) the held one walks away.
func TestAPathWalkerCarriesOnFromWhereItStood(t *testing.T) {
	t.Run("a loop, mid-leg", func(t *testing.T) {
		db := keepNPCDB(t)
		keepPathRow(t, db, keepBoss, "loop")
		old := keepNPCProcess(t, db)
		npc := old.NPCs[keepBoss]
		p := old.paths.byNPC[keepBoss]
		now := pathRun(old, time.Now(), 20*time.Second, nil) // to the path, and along it
		for onPath(p, npc.X, npc.Z) > 0.01 || !npc.moving() {
			now = pathRun(old, now, 100*time.Millisecond, nil)
		}
		next, x, z := npc.path.next, npc.X, npc.Z

		kept := keptOf(keepFile(t, old.KeepNPCs()), keepBoss)
		w := keepNPCProcess(t, db)
		if r := w.RestoreNPCs(kept); r != (NPCsRestored{Back: 1, Placed: 1, Path: 1}) {
			t.Fatalf("restored %+v", r)
		}
		npc = w.NPCs[keepBoss]
		if npc.X != x || npc.Z != z || npc.moving() {
			t.Fatalf("at (%v, %v) moving %v, want (%v, %v) standing", npc.X, npc.Z, npc.moving(), x, z)
		}
		var seen []int
		px, pz := npc.X, npc.Z
		pathRun(w, now, 40*time.Second, func(at time.Time) {
			visits(p, npc, &seen)(at)
			if d := distXZ(px, pz, npc.X, npc.Z); d > NPCWalkSpeed*0.5+0.01 {
				t.Fatalf("went %v in a tick", d)
			}
			if d := onPath(p, npc.X, npc.Z); d > 0.01 {
				t.Fatalf("%v off its path at (%v, %v)", d, npc.X, npc.Z)
			}
			px, pz = npc.X, npc.Z
		})
		if len(seen) < 3 || seen[0] != next || seen[1] != (next+1)%4 {
			t.Fatalf("it reached %v from the leg to %d", seen, next)
		}
	})

	t.Run("walked once, held at its end", func(t *testing.T) {
		db := keepNPCDB(t)
		keepPathRow(t, db, keepBoss, "once")
		old := keepNPCProcess(t, db)
		npc := old.NPCs[keepBoss]
		now := pathRun(old, time.Now(), 2*time.Minute, nil)
		if !npc.path.held || npc.X != 2180 || npc.Z != 2020 {
			t.Fatalf("not at its end: at (%v, %v), %+v", npc.X, npc.Z, npc.path)
		}
		kept := keptOf(keepFile(t, old.KeepNPCs()), keepBoss)

		w := keepNPCProcess(t, db)
		if r := w.RestoreNPCs(kept); r != (NPCsRestored{Back: 1, Placed: 1, Path: 1}) {
			t.Fatalf("restored %+v", r)
		}
		npc = w.NPCs[keepBoss]
		pathRun(w, now, time.Minute, func(time.Time) {
			if npc.X != 2180 || npc.Z != 2020 {
				t.Fatalf("left its end: at (%v, %v)", npc.X, npc.Z)
			}
		})
		if !npc.path.held {
			t.Fatalf("not held: %+v", npc.path)
		}

		// the control: its place given back and no path state walks it off again
		c := keepNPCProcess(t, db)
		for i := range kept {
			kept[i].PathDir = 0
		}
		c.RestoreNPCs(kept)
		left := false
		pathRun(c, now, time.Minute, func(time.Time) {
			left = left || c.NPCs[keepBoss].X != 2180 || c.NPCs[keepBoss].Z != 2020
		})
		if !left {
			t.Fatal("control: a fresh path state holds it too, so this proves nothing")
		}
	})

	t.Run("its path deleted by a data step: it walks home", func(t *testing.T) {
		db := keepNPCDB(t)
		keepPathRow(t, db, keepBoss, "loop")
		old := keepNPCProcess(t, db)
		now := pathRun(old, time.Now(), 30*time.Second, nil)
		kept := keptOf(keepFile(t, old.KeepNPCs()), keepBoss)
		if _, err := db.Exec(`DELETE FROM npcpaths`); err != nil {
			t.Fatal(err)
		}
		w := keepNPCProcess(t, db)
		if r := w.RestoreNPCs(kept); r.ToSpawn != 1 || r.Path != 0 {
			t.Fatalf("restored %+v", r)
		}
		npc := w.NPCs[keepBoss]
		pathRun(w, now, 30*time.Second, nil)
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.Facing != npc.SpawnFacing || npc.moving() || w.paths.home[keepBoss] {
			t.Fatalf("at (%v, %v) facing %v, walking %v", npc.X, npc.Z, npc.Facing, w.paths.home[keepBoss])
		}
	})

	t.Run("walking home with its path taken away: on it goes", func(t *testing.T) {
		db := keepNPCDB(t)
		keepPathRow(t, db, keepBoss, "loop")
		old := keepNPCProcess(t, db)
		now := pathRun(old, time.Now(), 30*time.Second, nil)
		old.ClearNPCPath(keepBoss)
		now = pathRun(old, now, time.Second, nil)
		if !old.paths.home[keepBoss] {
			t.Fatal("not walking home")
		}
		kept := keptOf(keepFile(t, old.KeepNPCs()), keepBoss)
		if _, err := db.Exec(`DELETE FROM npcpaths`); err != nil {
			t.Fatal(err)
		}
		w := keepNPCProcess(t, db)
		if r := w.RestoreNPCs(kept); r.ToSpawn != 1 || !w.paths.home[keepBoss] {
			t.Fatalf("restored %+v", r)
		}
		npc := w.NPCs[keepBoss]
		pathRun(w, now, 30*time.Second, nil)
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || w.paths.home[keepBoss] {
			t.Fatalf("at (%v, %v), walking %v", npc.X, npc.Z, w.paths.home[keepBoss])
		}
	})
}

// A wanderer within its radius is left to wander; one past it, where no leg ends
// (a charm's end, up to a zone from home), walks back.
func TestAWandererPastItsReachWalksBack(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	wolf := old.NPCs[keepWolf]
	moveNPC(old, wolf, wolf.SpawnX+40, wolf.SpawnY, wolf.SpawnZ, 1)
	got := old.KeepNPCs()
	if len(got) != 1 || got[0].ToSpawn {
		t.Fatalf("within its radius: %+v", got)
	}
	moveNPC(old, wolf, wolf.SpawnX+DefaultRoamRadius+20, wolf.SpawnY, wolf.SpawnZ, 1)
	got = keepFile(t, old.KeepNPCs())
	if len(got) != 1 || !got[0].ToSpawn {
		t.Fatalf("past its radius: %+v", got)
	}
	w := keepNPCProcess(t, db)
	if r := w.RestoreNPCs(got); r.ToSpawn != 1 {
		t.Fatalf("restored %+v", r)
	}
	wolf = w.NPCs[keepWolf]
	pathRun(w, time.Now(), 30*time.Second, nil)
	if d := distXZ(wolf.X, wolf.Z, wolf.SpawnX, wolf.SpawnZ); d > DefaultRoamRadius {
		t.Fatalf("still %v from its spawn", d)
	}
}

// A swimmer stands in the water still treading, and nothing reads the world's files
// to find the surface again.
func TestASwimmerStaysAtTheSurface(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	npc := old.NPCs[keepWolf]
	moveNPC(old, npc, npc.SpawnX+5, 8.5, npc.SpawnZ, 1)
	npc.swimming, npc.surface = true, 10
	kept := keepFile(t, old.KeepNPCs())
	w := keepNPCProcess(t, db)
	w.RestoreNPCs(kept)
	npc = w.NPCs[keepWolf]
	if !npc.swimming || npc.surface != 10 || npc.Loco != LocoTread || npc.Y != 8.5 {
		t.Fatalf("swimming %v at %v, loco %d, y %v", npc.swimming, npc.surface, npc.Loco, npc.Y)
	}
}

// A place that is no number loses that creature its keeping, not everyone their
// restart: JSON would refuse the whole file.
func TestAPlaceThatIsNoNumberDoesNotFailTheFile(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	moveNPC(old, old.NPCs[keepBoss], float32(math.NaN()), 10, 2000, 0)
	old.NPCs[keepGuard].chase = chaseState{on: true, fromX: float32(math.Inf(1))}
	moveNPC(old, old.NPCs[keepPlain], 2301, 10, 2000, 0)
	got := keepFile(t, old.KeepNPCs())
	if len(got) != 1 || EntityID(got[0].ID) != keepPlain {
		t.Fatalf("written %+v", got)
	}
}

// A client's slots through a restart (RestoreView): each kept creature's channel,
// made from the restored creature, is byte for byte what the client last held: it
// sees it where it stands, hurt or a corpse, at once. The control: the same slots
// with nothing given back show a creature that has gone home whole, the slide this
// stops. Under the same id, on the same slot.
func TestAClientsSlotsShowTheCreaturesAsTheyWere(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	addr := &net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 6301}
	sess := old.SessionManager.CreateSession(addr, 31, 31, 31, 2, true)
	sess.SetAccountID(7)
	sess.SetInGame(true)
	sess.SetCharacterInWorld(true)
	sess.SetECSEntity(old.Players[1])

	moved, hurt, dead := old.NPCs[keepBoss], old.NPCs[keepGuard], old.NPCs[keepWolf]
	moveNPC(old, moved, 2230, 11, 2010, 1.25)
	moved.VelX, moved.VelZ, moved.Loco = 3, 4, LocoWalk // walking as the restart comes
	hurt.HP = hurt.MaxHP / 3
	moveNPC(old, dead, 2010, 10, 2010, 2)
	old.KillNPCNow(keepWolf)
	dead.CorpseGone = time.Time{}
	rig := &keepRig{w: old, sm: old.SessionManager, sess: sess}
	ids := []EntityID{keepBoss, keepGuard, keepWolf, keepPlain}
	rig.see(t, sess, ids...)
	// what KeepRestart does first: the client told they stand, its channel saying so
	if n := old.StandNPCs(); n != 1 || moved.moving() || moved.Loco != LocoStand {
		t.Fatalf("%d stood; velocity %v %v, loco %d", n, moved.VelX, moved.VelZ, moved.Loco)
	}
	vs := sess.VisibilityState().(*VisibilityState)
	held := make([][]byte, len(ids))
	objects := make([]uint32, len(ids))
	keptConn := sess.Keep().Conn
	for i := range ids {
		held[i] = keptConn.Objects[i+1].Data
		objects[i] = vs.ChannelObjectID[i]
	}
	kv := old.KeepView(sess)
	b, err := json.Marshal(struct {
		S session.Kept
		V KeptView
		N []KeptNPC
	}{sess.Keep(), kv, old.KeepNPCs()})
	if err != nil {
		t.Fatal(err)
	}

	next := func(restore bool) (*World, *session.Session) {
		var back struct {
			S session.Kept
			V KeptView
			N []KeptNPC
		}
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		w := keepNPCProcess(t, db)
		s, err := w.SessionManager.Resume(back.S)
		if err != nil {
			t.Fatal(err)
		}
		s.SetECSEntity(w.Players[1])
		if restore {
			w.RestoreNPCs(back.N)
		}
		if kept, gone := w.RestoreView(s, back.V); kept != len(ids) || gone != 0 {
			t.Fatalf("slots kept %d, gone %d", kept, gone)
		}
		return w, s
	}

	// one visibility round's serialization, as the first tick makes it
	w, s := next(true)
	vs2 := s.VisibilityState().(*VisibilityState)
	for i, id := range ids {
		if vs2.ChannelEntity[i] != id || vs2.ChannelObjectID[i] != objects[i] {
			t.Fatalf("slot %d: entity %d id %d, want %d id %d", i, vs2.ChannelEntity[i], vs2.ChannelObjectID[i], id, objects[i])
		}
	}
	w.mu.Lock()
	for i, id := range ids {
		got := w.serializeEntityForChannel(s, id, objects[i])
		if string(got) != string(held[i]) {
			t.Errorf("slot %d (%s) differs from what the client holds:\n%x\n%x", i, w.NPCs[id].Name, got, held[i])
		}
	}
	w.mu.Unlock()
	w.Tick(time.Now()) // the corpse stays on its slot through a visibility round
	if vs2.ChannelEntity[2] != keepWolf {
		t.Errorf("the corpse left its slot: %v", vs2.ChannelEntity)
	}

	c, cs := next(false)
	c.mu.Lock()
	for i, id := range ids[:3] {
		if string(c.serializeEntityForChannel(cs, id, objects[i])) == string(held[i]) {
			t.Errorf("control: slot %d is the same with nothing given back", i)
		}
	}
	c.mu.Unlock()
}

// keepCostWorld is 61,136 NPCs (eqoaclassic's count on 2026-11-07), n of them away
// from their spawns, on the rows' own spread of names.
func keepCostWorld(n int) *World {
	w := NewWorld()
	w.SetPlayerModels(map[uint32]bool{humanModel: true})
	r := rand.New(rand.NewSource(1))
	names := []string{"a gnoll pup", "Guard Keeper", "wolf", "Deathfist reaver", "moss snake", "a very ancient Tae Ew crusader"}
	for i := 0; i < 61136; i++ {
		x, z := 1000+r.Float32()*40000, 1000+r.Float32()*40000
		npc := addNPC(w, EntityID(1000+i), names[i%len(names)], x, z, creatureModel, 0)
		npc.Y, npc.SpawnY = 54.125, 54.125
		npc.Facing, npc.SpawnFacing = r.Float32()*6, 1.5
		npc.Facing = npc.SpawnFacing
		if i < n {
			npc.X, npc.Z = x+r.Float32()*20, z+r.Float32()*20
			npc.Facing = r.Float32() * 6
			w.SpatialIndex.Update(npc.ServerID, x, z, npc.X, npc.Z)
		}
	}
	return w
}

// BenchmarkKeepNPCs is the hold's cost of the section, with every wanderer of a long
// uptime off its spawn: the walk over the world, the JSON, and the restore.
func BenchmarkKeepNPCs(b *testing.B) {
	for _, moved := range []int{0, 5000, 30000, 61136} {
		w := keepCostWorld(moved)
		kept := w.KeepNPCs()
		bytes, _ := json.Marshal(kept)
		b.Run(fmt.Sprintf("written/%d", moved), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = w.KeepNPCs()
			}
		})
		b.Run(fmt.Sprintf("marshal/%d", moved), func(b *testing.B) {
			b.SetBytes(int64(len(bytes)))
			for i := 0; i < b.N; i++ {
				if _, err := json.Marshal(kept); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("unmarshal/%d", moved), func(b *testing.B) {
			b.SetBytes(int64(len(bytes)))
			for i := 0; i < b.N; i++ {
				var back []KeptNPC
				if err := json.Unmarshal(bytes, &back); err != nil {
					b.Fatal(err)
				}
			}
		})
		fresh := keepCostWorld(0)
		b.Run(fmt.Sprintf("restore/%d", moved), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				fresh.RestoreNPCs(kept)
			}
		})
	}
}

// realKeepWorld is a World as cmd/gameserver loads one from db, as far as what a
// creature's keeping asks of it, on the baked ground when dir names one.
func realKeepWorld(t *testing.T, db *database.SQLiteDB, dir string) *World {
	t.Helper()
	store := database.NewNPCStore(db)
	w := NewWorld()
	models, err := store.PlayerModels()
	if err != nil {
		t.Fatal(err)
	}
	w.SetPlayerModels(models)
	w.LoadNPCs(store)
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadMoveRules(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadUndeadTags(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	recs, _, err := database.NewNPCPathStore(db).All()
	if err != nil {
		t.Fatal(err)
	}
	w.LoadNPCPaths(recs)
	if dir != "" {
		g, err := ground.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { g.Close() })
		w.SetGround(g)
		if env, err := ground.OpenEnv(dir); err == nil {
			w.SetEnv(env)
		}
	}
	return w
}

// What the section costs on a real world (EQOA_REAL_DB, a server's database, copied;
// EQOA_GROUND_DIR, cmd/groundbake's, optional), at the worst a keep restart comes to:
// every creature of the kind that wanders off its spawn, as they are after a long
// uptime that only such restarts interrupt (no walk takes one home), with some hurt
// and some dead. It says what each step takes (-v); the numbers are in
// claude-notes/session-keep-restart-20261106.md. Skipped without the database: it is
// not in this repository.
func TestRealKeepNPCsCost(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "eqoa.db")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := os.Getenv("EQOA_GROUND_DIR")
	w := realKeepWorld(t, db, dir)
	r := rand.New(rand.NewSource(1))
	wanderers, hurt, dead := 0, 0, 0
	w.mu.Lock()
	for _, npc := range w.NPCs {
		if !w.wanders(npc) {
			continue
		}
		wanderers++
		rr := w.roamRadiusLocked(npc) * 0.7
		oldX, oldZ := npc.X, npc.Z
		npc.X += (r.Float32() - 0.5) * rr
		npc.Z += (r.Float32() - 0.5) * rr
		npc.Y += r.Float32()
		npc.Facing = r.Float32() * 6.28
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		switch wanderers % 100 {
		case 0:
			npc.HP = npc.MaxHP / 2
			hurt++
		case 1:
			npc.HP = 0
			w.killNPC(npc)
			dead++
		}
	}
	w.mu.Unlock()

	file := filepath.Join(t.TempDir(), "keep.json")
	lap := time.Now()
	since := func() time.Duration { d := time.Since(lap); lap = time.Now(); return d.Round(100 * time.Microsecond) }
	kept := w.KeepNPCs()
	walk := since()
	out, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	marshal := since()
	if err := os.WriteFile(file, out, 0o600); err != nil {
		t.Fatal(err)
	}
	write := since()
	in, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	read := since()
	var back []KeptNPC
	if err := json.Unmarshal(in, &back); err != nil {
		t.Fatal(err)
	}
	unmarshal := since()
	w2 := realKeepWorld(t, db, dir)
	since()
	got := w2.RestoreNPCs(back)
	restore := since()
	t.Logf("%d NPCs, %d of the kind that wanders (%d hurt, %d dead): %d written, %d bytes, %d a creature; old process: walk %v, marshal %v, write %v; new process: read %v, unmarshal %v, restore %v; %s",
		len(w.NPCs), wanderers, hurt, dead, len(kept), len(out), len(out)/max(len(kept), 1), walk, marshal, write, read, unmarshal, restore, got)
	if got.Back != len(kept) || got.Dead != dead {
		t.Fatalf("%+v of %d written", got, len(kept))
	}
}
