package entity

import (
	"math"
	"testing"
	"time"
)

// #phcheck's reading (phcheck.go): a /ph binding's named is its kind of the
// lowest odds (several when they tie; none at even odds), npcrare's rare is a
// named at its chance, both on one spot combine; each spot's state (up, as the
// named, dead with its return) and the named's creatures up. Read-only: the
// world is as it was. Over campDB's database with these rows.

const (
	phSpotA    EntityID = 231 // a bandit spot: bandit 60, lookout 37, Bandit Lord 3
	phSpotB    EntityID = 232 // the same binding, and npcrare's Bandit Lord at 10%
	phEven     EntityID = 233 // even odds: no named
	phGriffon  EntityID = 234 // griffon 94, three Elders 2 each
	phRareOnly EntityID = 235 // npcrare alone: Bandit Lord at 20%
	phLord     EntityID = 236 // the Bandit Lord's own row (it stands: up, not on a spot)
	phLookout  EntityID = 237 // the lookouts' row
	phElderA   EntityID = 238
	phElderB   EntityID = 239
	phElderC   EntityID = 240
)

func phWorld(t *testing.T) *World {
	t.Helper()
	db := campDB(t, []campRow{
		{phSpotA, "bandit", 3000, 3000, 0, creatureModel, 20},
		{phSpotB, "bandit", 3050, 3000, 0, creatureModel, 20},
		{phEven, "wolf", 3100, 3000, 0, creatureModel, 20},
		{phGriffon, "griffon", 3150, 3000, 0, creatureModel, 30},
		{phRareOnly, "bandit", 3200, 3000, 0, creatureModel, 20},
		{phLord, "Bandit Lord", 3250, 3000, 0, creatureModel, 25},
		{phLookout, "lookout", 3300, 3000, 0, creatureModel, 20},
		{phElderA, "Elder A", 3350, 3000, 0, creatureModel, 30},
		{phElderB, "Elder B", 3400, 3000, 0, creatureModel, 30},
		{phElderC, "Elder C", 3450, 3000, 0, creatureModel, 30},
	}, map[EntityID]string{
		phSpotA:   "bandit, lookout, Bandit Lord",
		phSpotB:   "bandit, lookout, Bandit Lord",
		phEven:    "wolf, lookout",
		phGriffon: "griffon, Elder A, Elder B, Elder C",
	})
	for q, args := range map[string][]any{
		`UPDATE campspawns SET camp = 0, weights = '60, 37, 3' WHERE npcid IN (?, ?)`: {phSpotA, phSpotB},
		`UPDATE campspawns SET camp = 0, weights = '94, 2, 2, 2' WHERE npcid = ?`:     {phGriffon},
		`UPDATE campspawns SET camp = 0 WHERE npcid = ?`:                              {phEven},
	} {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DELETE FROM npcrare`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM npcrare`) })
	for id, chance := range map[EntityID]float64{phSpotB: 10, phRareOnly: 20} {
		if _, err := db.Exec(`INSERT INTO npcrare (npc_row, rare, chance) VALUES (?, 'bandit lord', ?)`, id, chance); err != nil {
			t.Fatal(err)
		}
	}
	w := campProcess(t, db)
	if n, err := w.LoadRareSpawns(db); err != nil || n != 2 {
		t.Fatalf("rules read: %d, %v", n, err)
	}
	return w
}

func phGroupOf(t *testing.T, gs []PHGroup, key string) PHGroup {
	t.Helper()
	for _, g := range gs {
		if g.Key == key {
			return g
		}
	}
	t.Fatalf("no group %q in %+v", key, gs)
	return PHGroup{}
}

func TestPHGroupsReadTheNamedsTheirSpotsAndChances(t *testing.T) {
	w := phWorld(t)
	now := time.Now()
	gs := w.PHGroups(now)
	if len(gs) != 4 { // Bandit Lord, the three Elders; the wolf/lookout binding has none
		var names []string
		for _, g := range gs {
			names = append(names, g.Named)
		}
		t.Fatalf("groups %q", names)
	}
	lord := phGroupOf(t, gs, "bandit lord")
	if lord.Named != "Bandit Lord" || !lord.OneAtATime || len(lord.Spots) != 3 {
		t.Fatalf("Bandit Lord: %+v", lord)
	}
	want := map[EntityID]float64{phSpotA: 3, phSpotB: 10 + 90*0.03, phRareOnly: 20}
	for _, s := range lord.Spots {
		if math.Abs(s.Chance-want[s.ID]) > 1e-9 {
			t.Errorf("spot %d: chance %v, want %v", s.ID, s.Chance, want[s.ID])
		}
		if s.Dead || s.AsNamed || s.Own != "bandit" {
			t.Errorf("spot %+v", s)
		}
	}
	if s := lord.Spots[0]; s.Odds != "bandit 60%, lookout 37%, Bandit Lord 3%" {
		t.Errorf("spot A's odds %q", s.Odds)
	}
	if s := lord.Spots[1]; s.Odds != "rare Bandit Lord 10%, else bandit 60%, lookout 37%, Bandit Lord 3%" {
		t.Errorf("spot B's odds %q", s.Odds)
	}
	if len(lord.Up) != 1 || lord.Up[0].ID != phLord || lord.Up[0].OnSpot {
		t.Fatalf("Bandit Lord up: %+v", lord.Up)
	}
	for _, k := range []string{"elder a", "elder b", "elder c"} {
		g := phGroupOf(t, gs, k)
		if len(g.Spots) != 1 || g.Spots[0].ID != phGriffon || g.Spots[0].Chance != 2 || g.OneAtATime {
			t.Fatalf("%s: %+v", k, g)
		}
	}

	// a death: the spot dead, its corpse lying, its return its decay and respawn after
	w.SetCorpseTimes(time.Minute, 5*time.Minute)
	if !w.KillNPCNow(phSpotA) || !w.KillNPCNow(phLord) {
		t.Fatal("not killed")
	}
	before := w.NPCs[phSpotA].Name
	lord = phGroupOf(t, w.PHGroups(now), "bandit lord")
	if len(lord.Up) != 0 {
		t.Fatalf("the Lord dead, up: %+v", lord.Up)
	}
	a := lord.Spots[0]
	if !a.Dead || !a.Corpse || a.Back.IsZero() {
		t.Fatalf("dead spot %+v", a)
	}
	if left := a.Back.Sub(w.NPCs[phSpotA].DeathTime); left < 5*time.Minute {
		t.Fatalf("back %v after the death", left)
	}
	// read-only: nothing came back, nothing changed kind
	if n := w.NPCs[phSpotA]; !n.Dead || n.Name != before || !w.NPCs[phLord].Dead {
		t.Fatal("the reading changed the world")
	}
}
