package handlers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// Heroic raids' triggers (gmtriggers.go's instance, the owner 2026-11-28: "make the
// trigger engine run inside a dungeon instance"): a trigger of the dungeons' world
// runs in one party's copy of a dungeon at a time, its spawns in that copy alone, its
// death rows fired by that copy's kills, its lines told to that copy's players. Two
// players alone (Anna, Cora) each open their own copy of a test dungeon; Bert stays
// outside.

type heroicTrigRig struct {
	*portalRig
	e   *TriggerEngine
	db  *database.SQLiteDB
	now time.Time
}

func newHeroicTrigRig(t *testing.T) *heroicTrigRig {
	t.Helper()
	r := newPortalRig(t)
	block := `[{"key":"hr","name":"Test Raid","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	// the raid's boss template, spawn-only, outside the dungeon's box (no instance's
	// opening copies it: only the trigger's spawn does)
	r.gw.NPCs[800] = &entity.NPC{ServerID: 800, Name: "Test Kraken", HP: 500, MaxHP: 500, Level: 65,
		X: 2000, Y: 50, Z: 2000, SpawnX: 2000, SpawnY: 50, SpawnZ: 2000, World: 14, SpawnOnly: true}
	// a creature of the dungeon (in its box): each copy's own
	r.gw.NPCs[801] = &entity.NPC{ServerID: 801, Name: "Test Eel", HP: 100, MaxHP: 100, Level: 60,
		X: 1050, Y: 50, Z: 1050, SpawnX: 1050, SpawnY: 50, SpawnZ: 1050, World: 14, SpawnOnly: true}
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	for _, s := range []*session.Session{r.anna, r.cora, r.bert} {
		s.SetServerWorlds([]uint8{14})
	}
	db, err := database.NewSQLite(t.TempDir() + "/triggers.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := &heroicTrigRig{portalRig: r, db: db, now: time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)}
	h.e = NewTriggerEngine(db, r.gw, r.sm, nil)
	h.e.now = func() time.Time { return h.now }
	h.e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(h.e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(h.e, nil) })
	return h
}

func (h *heroicTrigRig) setTrigger(slot int, name string, world int, x, y, z float32, steps string) {
	h.t.Helper()
	if _, err := h.db.Exec(`INSERT OR REPLACE INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps)
		VALUES (?, ?, 1, 'near', 300, ?, ?, ?, ?, ?)`, slot, name, world, x, y, z, steps); err != nil {
		h.t.Fatal(err)
	}
	if err := h.e.Reload(); err != nil {
		h.t.Fatal(err)
	}
}

// enter takes s's player alone into its own copy of the test raid.
func (h *heroicTrigRig) enter(s *session.Session, p *entity.Player) uint32 {
	h.t.Helper()
	dungeonCommand(h.gw, s, "enter hr")
	h.travel()
	if p.World != 14 || p.Instance == 0 {
		h.t.Fatalf("%s not in: world %d instance %d", p.Name, p.World, p.Instance)
	}
	return p.Instance
}

// copies is the living temporary copies of name, by instance.
func (h *heroicTrigRig) copies(name string) map[uint32][]*entity.NPC {
	out := map[uint32][]*entity.NPC{}
	for _, n := range h.gw.NPCs {
		if n.Temporary && !n.Dead && n.Name == name {
			out[n.Instance] = append(out[n.Instance], n)
		}
	}
	return out
}

const krakenSteps = `[{"do":"awaitnear","start":true,"radius":300},{"do":"lock","lock":"test kraken"},
	{"do":"spawn","npc":"Test Kraken","hostile":true,"minutes":60,"world":14,"x":1000,"y":50,"z":1000},
	{"do":"awaitdead","npc":"Test Kraken"},{"do":"broadcast","text":"The kraken sinks, %p."}]`

// An awaitnear start fires once in each party's copy: a run an instance, each one's
// Kraken standing in that copy alone (its instance's), none outside any instance, and
// a second look starts nothing more (each copy's lock held there). The control: a
// trigger of the open world (Bert's) runs once, in no instance, as ever.
func TestHeroicTriggerRunsInEachInstance(t *testing.T) {
	h := newHeroicTrigRig(t)
	a := h.enter(h.anna, h.pa)
	c := h.enter(h.cora, h.pc)
	if a == c {
		t.Fatalf("Anna and Cora share instance %d", a)
	}
	h.setTrigger(901, "Test Kraken Rises", 14, 1000, 50, 1000, krakenSteps)
	h.e.StartNear()
	got := h.copies("Test Kraken")
	if len(got[a]) != 1 || len(got[c]) != 1 || len(got[0]) != 0 || len(got) != 2 {
		t.Fatalf("krakens by instance: %v (want one in %d and one in %d)", counts(got), a, c)
	}
	h.e.StartNear()
	if got := h.copies("Test Kraken"); len(got[a]) != 1 || len(got[c]) != 1 {
		t.Fatalf("a second look started more: %v", counts(got))
	}
	runs := h.e.Runs()
	if len(runs) != 2 {
		t.Fatalf("%d runs, want 2", len(runs))
	}
	// the open world's control
	h.setTrigger(902, "Open Kraken", 0, 20000, 50, 20000, `[{"do":"awaitnear","start":true,"radius":300},
		{"do":"spawn","npc":"Test Kraken","minutes":60,"world":0,"x":20000,"y":50,"z":20000}]`)
	h.e.StartNear()
	if got := h.copies("Test Kraken"); len(got[0]) != 1 {
		t.Fatalf("the open world's kraken: %v", counts(got))
	}
}

// Anna's Kraken dead, her run goes on and its line is told to her alone: Cora, in her
// own copy and within the trigger's reach of its place, hears nothing, and her run
// still waits on her Kraken.
func TestHeroicTriggerDeathEndsItsOwnRun(t *testing.T) {
	h := newHeroicTrigRig(t)
	a := h.enter(h.anna, h.pa)
	c := h.enter(h.cora, h.pc)
	h.setTrigger(901, "Test Kraken Rises", 14, 1000, 50, 1000, krakenSteps)
	h.e.StartNear()
	got := h.copies("Test Kraken")
	if len(got[a]) != 1 || len(got[c]) != 1 {
		t.Fatalf("krakens: %v", counts(got))
	}
	h.drain()
	got[a][0].Dead = true
	h.now = h.now.Add(2 * time.Second)
	h.e.Tick()
	if !anyHas(yellow(t, h.anna), "The kraken sinks, Anna.") {
		t.Fatalf("Anna not told: %q", yellow(t, h.anna))
	}
	if l := yellow(t, h.cora); len(l) != 0 {
		t.Fatalf("Cora, in her own copy, heard: %q", l)
	}
	if got := h.copies("Test Kraken"); len(got[c]) != 1 {
		t.Fatalf("Cora's kraken: %v", counts(got))
	}
	// Anna's run done (and gone), Cora's still waiting on her kraken
	if runs := h.e.Runs(); len(runs) != 1 || runs[0].Step != 5 || runs[0].Spawns != 1 {
		t.Fatalf("runs: %+v", runs)
	}
}

// A death row fires in the dead creature's copy: an eel killed in Cora's copy brings
// its trigger's spawn there, beside it, in her instance alone; one killed in Anna's
// brings it in Anna's. An awaitkill of one copy counts that copy's kills alone.
func TestHeroicTriggerDeathRowsFireInTheirInstance(t *testing.T) {
	h := newHeroicTrigRig(t)
	a := h.enter(h.anna, h.pa)
	c := h.enter(h.cora, h.pc)
	h.setTrigger(903, "Eel Calls Kraken", -1, 0, 0, 0, `[{"do":"lock","lock":"eel call"},
		{"do":"spawn","npc":"Test Kraken","hostile":true,"minutes":30}]`)
	if _, err := h.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('test eel', 'Eel Calls Kraken', 100)`); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Reload(); err != nil {
		t.Fatal(err)
	}
	eels := h.copies("Test Eel")
	if len(eels[a]) != 1 || len(eels[c]) != 1 {
		t.Fatalf("eels: %v", counts(eels))
	}
	h.e.Killed(entity.KillCredit{NPCID: eels[c][0].ServerID, NPCName: "Test Eel"})
	got := h.copies("Test Kraken")
	if len(got[c]) != 1 || len(got[a]) != 0 || len(got[0]) != 0 {
		t.Fatalf("after Cora's eel: %v", counts(got))
	}
	h.e.Killed(entity.KillCredit{NPCID: eels[a][0].ServerID, NPCName: "Test Eel"})
	if got := h.copies("Test Kraken"); len(got[a]) != 1 || len(got[c]) != 1 {
		t.Fatalf("after Anna's eel (each copy's own lock): %v", counts(got))
	}
}

// A run whose copy has closed ends: nothing of it is left to run.
func TestHeroicTriggerEndsWithItsInstance(t *testing.T) {
	h := newHeroicTrigRig(t)
	a := h.enter(h.anna, h.pa)
	h.setTrigger(901, "Test Kraken Rises", 14, 1000, 50, 1000, krakenSteps)
	h.e.StartNear()
	if len(h.e.Runs()) != 1 {
		t.Fatalf("runs: %+v", h.e.Runs())
	}
	// Anna out of the world, her copy closed (no one in it, its time up)
	dungeonCommand(h.gw, h.anna, "leave")
	h.travel()
	if h.pa.World == 14 {
		t.Fatal("Anna still in")
	}
	msg := h.gw.DungeonReset(h.anna, "hr")
	if h.gw.InstanceOpen(a) {
		t.Fatalf("instance %d still open (%q)", a, msg)
	}
	h.now = h.now.Add(2 * time.Second)
	h.e.Tick()
	if runs := h.e.Runs(); len(runs) != 0 {
		t.Fatalf("runs after its instance closed: %+v", runs)
	}
}

func counts(m map[uint32][]*entity.NPC) map[uint32]int {
	out := map[uint32]int{}
	for k, v := range m {
		out[k] = len(v)
	}
	return out
}

func anyHas(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// A death row matches by name, so an eel of the open world and one of a heroic copy
// fire both of its triggers: each runs where it belongs and the other is refused
// (misplacedRun). The open world's eel brings the open world's kraken alone (none in
// world 14, where one in no instance would stand in every copy); the copy's eel brings
// the heroic kraken in its copy alone (none in world 0).
func TestHeroicTriggerDeathRowsRunWhereTheyBelong(t *testing.T) {
	h := newHeroicTrigRig(t)
	c := h.enter(h.cora, h.pc)
	h.gw.NPCs[802] = &entity.NPC{ServerID: 802, Name: "Test Eel", HP: 100, MaxHP: 100, Level: 10,
		X: 20000, Y: 50, Z: 20000, SpawnX: 20000, SpawnY: 50, SpawnZ: 20000, World: 0}
	// the open world's: its spawn put in world 0 by its step
	h.setTrigger(904, "Open Eel Calls", -1, 0, 0, 0, `[{"do":"spawn","npc":"Test Kraken","minutes":30,"world":0,"x":20005,"y":50,"z":20005}]`)
	// the heroic copy's: its spawn put in world 14 by its step
	h.setTrigger(905, "Heroic Eel Calls", -1, 0, 0, 0, `[{"do":"spawn","npc":"Test Kraken","minutes":30,"world":14,"x":1050,"y":50,"z":1050}]`)
	for _, trig := range []string{"Open Eel Calls", "Heroic Eel Calls"} {
		if _, err := h.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('test eel', ?, 100)`, trig); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.e.Reload(); err != nil {
		t.Fatal(err)
	}
	h.e.Killed(entity.KillCredit{NPCID: 802, NPCName: "Test Eel"})
	got := h.copies("Test Kraken")
	if len(got[0]) != 1 || got[0][0].World != 0 {
		t.Fatalf("after the open world's eel: %v (want one kraken, in world 0)", counts(got))
	}
	for _, n := range got[0] {
		if n.World == 14 {
			t.Fatal("a kraken in world 14 outside any instance")
		}
	}
	eels := h.copies("Test Eel")
	h.e.Killed(entity.KillCredit{NPCID: eels[c][0].ServerID, NPCName: "Test Eel"})
	got = h.copies("Test Kraken")
	if len(got[c]) != 1 || got[c][0].World != 14 {
		t.Fatalf("after Cora's eel: %v (want one kraken in her copy)", counts(got))
	}
	if n := 0; true {
		for _, k := range got[0] {
			if k.World == 0 {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("world 0 has %d krakens after Cora's eel, want still 1", n)
		}
	}
	// a fixed heroic trigger fired by no instance (a GM's panel) is refused
	h.setTrigger(906, "Heroic Fixed", 14, 1000, 50, 1000, `[{"do":"spawn","npc":"Test Kraken","minutes":30}]`)
	if _, err := h.e.Fire(906, TriggerFirer{Name: "the panel"}); !errors.Is(err, ErrTriggerElsewhere) {
		t.Fatalf("a heroic trigger fired outside any instance: %v", err)
	}
}

// A fixed trigger of the open world is not fired by its creature's heroic copy's
// death (an Archon's portal down opening in world 1 for a kill in world 14), nor a
// fixed heroic trigger by an open world death; the open world's own kill still
// fires its own.
func TestFixedTriggersIgnoreOtherWorldsDeaths(t *testing.T) {
	h := newHeroicTrigRig(t)
	c := h.enter(h.cora, h.pc)
	h.gw.NPCs[803] = &entity.NPC{ServerID: 803, Name: "Test Eel", HP: 100, MaxHP: 100, Level: 10,
		X: 20000, Y: 50, Z: 20000, SpawnX: 20000, SpawnY: 50, SpawnZ: 20000, World: 0}
	h.setTrigger(907, "Open Portal", 0, 20010, 50, 20010, `[{"do":"spawn","npc":"Test Kraken","minutes":30}]`)
	h.setTrigger(908, "Heroic Fixed Portal", 14, 1050, 50, 1050, `[{"do":"spawn","npc":"Test Kraken","minutes":30}]`)
	for _, trig := range []string{"Open Portal", "Heroic Fixed Portal"} {
		if _, err := h.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('test eel', ?, 100)`, trig); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.e.Reload(); err != nil {
		t.Fatal(err)
	}
	eels := h.copies("Test Eel")
	h.e.Killed(entity.KillCredit{NPCID: eels[c][0].ServerID, NPCName: "Test Eel"})
	if got := h.copies("Test Kraken"); len(got[0])+len(got[c]) != 1 || len(got[c]) != 1 {
		t.Fatalf("after Cora's eel: %v (want one kraken, in her copy)", counts(got))
	}
	h.e.Killed(entity.KillCredit{NPCID: 803, NPCName: "Test Eel"})
	got := h.copies("Test Kraken")
	n0 := 0
	for _, k := range got[0] {
		if k.World == 0 {
			n0++
		} else {
			t.Fatalf("a kraken in world %d outside any instance", k.World)
		}
	}
	if n0 != 1 || len(got[c]) != 1 {
		t.Fatalf("after the open world's eel: %v (want one in world 0, still one in Cora's copy)", counts(got))
	}
}

// A heroic raid's end as tools/heroic_raids2_20261128.py writes it: a party that wipes
// (its copy's place empty past the clock's empty time) has its run end, so the boss
// rises again for it; a party that kills him keeps its run (a 4-hour clock with no empty
// time, and a wait as long), so he does not rise again in that copy, until it is over.
func TestHeroicRaidWipeStartsAgainKillHolds(t *testing.T) {
	h := newHeroicTrigRig(t)
	a := h.enter(h.anna, h.pa)
	c := h.enter(h.cora, h.pc)
	h.setTrigger(909, "Test Heroic Kraken", 14, 1000, 50, 1000, `[{"do":"awaitnear","start":true,"radius":300},
		{"do":"lock","lock":"heroic kraken"},{"do":"clock","seconds":3600,"empty":600},
		{"do":"spawn","npc":"Test Kraken","hostile":true,"minutes":60,"world":14,"x":1000,"y":50,"z":1000},
		{"do":"awaitdead","npc":"Test Kraken"},{"do":"broadcast","text":"The kraken sinks."},
		{"do":"clock","seconds":14400},{"do":"wait","seconds":14400},{"do":"end"},{"do":"despawn"}]`)
	h.e.StartNear()
	got := h.copies("Test Kraken")
	if len(got[a]) != 1 || len(got[c]) != 1 {
		t.Fatalf("krakens: %v", counts(got))
	}
	step := func(d time.Duration) {
		for i := 0; i < 4; i++ {
			h.now = h.now.Add(d / 4)
			h.e.Tick()
		}
	}
	got[a][0].Dead = true       // Anna's party kills hers
	h.pc.X, h.pc.Z = 5000, 5000 // Cora's party is gone from the place (a wipe)
	step(11 * time.Minute)
	if got := h.copies("Test Kraken"); len(got[c]) != 0 {
		t.Fatalf("Cora's kraken after her wipe: %v (want taken away)", counts(got))
	}
	if runs := h.e.Runs(); len(runs) != 1 {
		t.Fatalf("runs after the wipe and the kill: %+v (want Anna's alone)", runs)
	}
	h.pc.X, h.pc.Z = 1000, 1150
	h.e.StartNear()
	got = h.copies("Test Kraken")
	if len(got[c]) != 1 {
		t.Fatalf("Cora back: %v (want her kraken again)", counts(got))
	}
	if len(got[a]) != 0 {
		t.Fatalf("Anna's kraken rose again after her kill: %v", counts(got))
	}
	step(3*time.Hour + 50*time.Minute)
	h.e.StartNear()
	if got := h.copies("Test Kraken"); len(got[a]) != 0 {
		t.Fatalf("Anna's kraken rose again within her copy's rest: %v", counts(got))
	}
	step(20 * time.Minute)
	if runs := h.e.Runs(); len(runs) != 1 || runs[0].Spawns != 1 {
		t.Fatalf("after Anna's rest: %+v (want Cora's new run alone: the look after her first rest started it)", runs)
	}
}
