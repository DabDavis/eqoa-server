package entity

import (
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A creature's own row (npcwander) makes it wander where its name's row, or
// its kind, stands it (petitions 1734-1736, the GM Jibrish: "Spread out the
// wolfwere's here, make them roam"): a non-wildlife name, a name npcmove
// stands, a players' race's body. The controls: the same names, no row,
// stand; and a row never walks what never wanders (a pet, a dummy, a path's).
func TestARowsOwnWanderIsAheadOfItsNameAndKind(t *testing.T) {
	w := roamWorld(t)
	w.SetMoveRules(map[string]database.NPCMoveRule{"wolfwere": {Roam: false}})
	walk := []*NPC{
		addNPC(w, 101, "wolfwere", 1005, 1000, creatureModel, 0),    // its name's row stands it
		addNPC(w, 102, "treant", 1010, 1000, creatureModel, 0),      // not wildlife
		addNPC(w, 103, "Dawn tribesman", 1015, 1000, humanModel, 0), // a players' race's body
	}
	stand := []*NPC{
		addNPC(w, 201, "wolfwere", 995, 1000, creatureModel, 0),
		addNPC(w, 202, "treant", 990, 1000, creatureModel, 0),
		addNPC(w, 203, "Dawn tribesman", 985, 1000, humanModel, 0),
		addNPC(w, 204, "Training Dummy Warrior", 980, 1000, creatureModel, 0),
		addNPC(w, 205, "orc pawn", 975, 1000, creatureModel, 0x80), // the NPC bit
	}
	w.SetRowWander(map[EntityID]float32{101: 0, 102: 0, 103: 30, 204: 0, 205: 0})
	run(w, time.Now(), 3*time.Minute, nil)
	for _, npc := range walk {
		if npc.X == npc.SpawnX && npc.Z == npc.SpawnZ {
			t.Errorf("%d %q with its own row never moved", npc.ServerID, npc.Name)
		}
	}
	for _, npc := range stand {
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ {
			t.Errorf("%d %q moved to (%.1f, %.1f): no row, or one that never wanders", npc.ServerID, npc.Name, npc.X, npc.Z)
		}
	}
	w.mu.Lock()
	if r := w.roamRadiusLocked(walk[2]); r != 30 {
		t.Errorf("a row's radius 30 read as %v", r)
	}
	if r := w.roamRadiusLocked(walk[0]); r != DefaultRoamRadius {
		t.Errorf("a row's radius 0 read as %v, want the server's %v", r, DefaultRoamRadius)
	}
	w.mu.Unlock()
}

// The table, read: its rows by npcs id, a negative radius as -1 (it stands).
func TestNPCWanderRowsAreRead(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO npcwander (npcid, radius, source) VALUES (7, 45, 'disperse')`,
		`INSERT INTO npcwander (npcid, radius) VALUES (9, -3)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWorld()
	n, err := w.LoadRowWander(db)
	if err != nil || n != 2 {
		t.Fatalf("LoadRowWander = %d, %v", n, err)
	}
	if w.rowWander[7] != 45 || w.rowWander[9] != -1 {
		t.Errorf("rows read %v", w.rowWander)
	}
	var _ *sql.DB
}

// A far roamer (the owner, 2026-11-12: "skeletal giant and others are not
// roaming correctly, they only move a small distance"): given a radius of 280,
// four times the server's, its legs are drawn four times Sandstorm's and it
// crosses its ground, going past 150 from its spawn within an hour; the
// control, the same creature at the server's radius, walks Sandstorm's legs
// (median 14-25) and never past 70.
func TestAFarRoamerCrossesItsGround(t *testing.T) {
	far := func(radius float32) (legMed, reach float64) {
		w := roamWorld(t)
		w.SetMoveRules(map[string]database.NPCMoveRule{"skeletal giant": {Roam: true, Radius: radius}})
		npc := addNPC(w, 100, "skeletal giant", 1000, 1000, creatureModel, 0)
		var legs []float64
		var fx, fz float32 = npc.X, npc.Z
		was := false
		run(w, time.Now(), time.Hour, func(time.Time) {
			if d := float64(distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ)); d > reach {
				reach = d
			}
			if was && !npc.moving() {
				legs = append(legs, float64(distXZ(fx, fz, npc.X, npc.Z)))
			}
			if !was && npc.moving() {
				fx, fz = npc.X-npc.VelX*0.1, npc.Z-npc.VelZ*0.1
			}
			was = npc.moving()
		})
		if len(legs) < 20 {
			t.Fatalf("radius %v: %d legs in an hour", radius, len(legs))
		}
		sort.Float64s(legs)
		return legs[len(legs)/2], reach
	}
	med, reach := far(280)
	if reach < 150 || reach > 280.01 || med < 40 {
		t.Errorf("a far roamer (280): legs median %.1f, went %.1f from its spawn; want long legs and past 150, within 280", med, reach)
	}
	med, reach = far(0) // the server's radius: the control
	if reach > float64(DefaultRoamRadius)+0.01 || med < 14 || med > 25 {
		t.Errorf("at the server's radius: legs median %.1f, went %.1f; want Sandstorm's legs within %v", med, reach, DefaultRoamRadius)
	}
}

// A row of its own with a negative radius stands its creature (petitions
// 2088-2089, the GM Jibcrotwo: "He does not roam"), though its name wanders
// (crocodiles: wildlife, npcmove's roam); the control, the same name with no
// row, wanders.
func TestARowsOwnNoRoamStandsItsCreature(t *testing.T) {
	w := roamWorld(t)
	w.SetMoveRules(map[string]database.NPCMoveRule{"crocodile": {Roam: true}})
	still := addNPC(w, 301, "crocodile", 1000, 1000, creatureModel, 0)
	walks := addNPC(w, 302, "crocodile", 1010, 1000, creatureModel, 0)
	w.SetRowWander(map[EntityID]float32{301: -1})
	run(w, time.Now(), 3*time.Minute, nil)
	if still.X != still.SpawnX || still.Z != still.SpawnZ {
		t.Errorf("the row's own no-roam moved to (%.1f, %.1f)", still.X, still.Z)
	}
	if walks.X == walks.SpawnX && walks.Z == walks.SpawnZ {
		t.Errorf("the control, no row, never moved")
	}
	w.mu.Lock()
	ok, why, _ := w.movementLocked(still)
	w.mu.Unlock()
	if ok || why != whyRowOwnStands {
		t.Errorf("movement %v, %v; want it stands by its own row", ok, why)
	}
}
