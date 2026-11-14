package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// setsWorld is a world of n quiet NPCs put in it the server's way, and a
// player among them.
func setsWorld(t *testing.T, n int) (*World, *Player) {
	t.Helper()
	w := NewWorld()
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := 0; i < n; i++ {
		npc := &NPC{ServerID: EntityID(10000 + i), Name: fmt.Sprintf("rat %d", i), HP: 10, MaxHP: 10,
			X: 1000 + float32(i%50), Z: 1000 + float32(i/50), Level: 1}
		w.putNPCLocked(npc)
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	}
	p := &Player{ServerID: 1, Name: "Watcher", HP: 100, MaxHP: 100, X: 1000, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	return w, p
}

// regenTicks runs the NPCs' regeneration k times, on its clock.
func regenTicks(w *World, k int) {
	for i := 0; i < k; i++ {
		w.TickCount = uint64(i) * uint64(RegenInterval/(100*time.Millisecond))
		w.npcRegenSystem(time.Now())
	}
}

// The sets hold the NPCs with work and no others: a world of quiet NPCs
// leaves the fights' set after one look and the regeneration's once the calm
// ticks are counted; a hit puts its NPC in both, its death in the corpses',
// and its respawn takes it out of that one again.
func TestNPCSetsHoldOnlyTheNPCsWithWork(t *testing.T) {
	w, p := setsWorld(t, 500)
	w.SetCorpseLinger(-1) // the creature's own corpse time, no linger (corpselinger.go)
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()

	if len(w.sets.ai.m) != 500 || len(w.sets.regen.m) != 500 {
		t.Fatalf("new NPCs: %d in the fights' set and %d in the regeneration's, want all 500 to be looked at once",
			len(w.sets.ai.m), len(w.sets.regen.m))
	}
	w.aiSystem(now)
	if len(w.sets.ai.m) != 0 {
		t.Fatalf("after one look %d quiet NPCs are still in the fights' set", len(w.sets.ai.m))
	}
	regenTicks(w, npcCalmTicks+2) // the calm ticks counted, then one look at them whole and calm
	if len(w.sets.regen.m) != 0 {
		t.Fatalf("%d whole, calm NPCs still in the regeneration's set", len(w.sets.regen.m))
	}
	for _, npc := range w.NPCs {
		if npc.calmTicks <= npcCalmTicks {
			t.Fatalf("%s's calm ticks are %d: the set left before its count was done", npc.Name, npc.calmTicks)
		}
	}

	rat := w.NPCs[10007]
	w.addThreatLocked(rat, p.ServerID, 3)
	if w.sets.ai.m[rat.ServerID] == nil || w.sets.regen.m[rat.ServerID] == nil {
		t.Fatal("a hit left its NPC out of the fights' or the regeneration's set")
	}
	w.aiSystem(now)
	if w.sets.ai.m[rat.ServerID] == nil || len(w.sets.ai.m) != 1 {
		t.Fatalf("fighting: the fights' set holds %d (the rat in it: %v)", len(w.sets.ai.m), w.sets.ai.m[rat.ServerID] != nil)
	}

	rat.HP = 0
	w.killNPC(rat)
	if w.sets.corpse.m[rat.ServerID] == nil || len(w.sets.corpse.m) != 1 {
		t.Fatalf("a death: the corpses' set holds %d", len(w.sets.corpse.m))
	}
	w.aiSystem(now) // its fight put away (a corpse's)
	w.aiSystem(now)
	if len(w.sets.ai.m) != 0 {
		t.Fatalf("a corpse with its fight put away is still in the fights' set")
	}
	w.corpseSystem(now.Add(DefaultCorpseDecay + time.Second)) // decayed and, with no respawn time, back
	if rat.Dead {
		t.Fatal("the rat did not come back")
	}
	w.corpseSystem(now.Add(DefaultCorpseDecay + 2*time.Second))
	if len(w.sets.corpse.m) != 0 {
		t.Fatalf("a living NPC is still in the corpses' set")
	}
}

// An NPC that leaves the world leaves every set; one made again in place
// under its id (the GM panel's remake) is the same member, and the map's own
// NPC is the one the sets give.
func TestNPCSetsFollowTheMap(t *testing.T) {
	w, p := setsWorld(t, 3)
	w.mu.Lock()
	defer w.mu.Unlock()
	a, b := w.NPCs[10000], w.NPCs[10001]
	w.addThreatLocked(a, p.ServerID, 1)
	w.removeNPCLocked(a)
	if w.sets.ai.m[a.ServerID] != nil || w.sets.regen.m[a.ServerID] != nil {
		t.Fatal("an NPC taken out of the world is still in a set")
	}
	fresh := *b
	fresh.CurrentEnemy = p.ServerID
	w.putNPCLocked(&fresh) // a new NPC under b's id
	if got := w.setMember(w.sets.ai.m, b.ServerID, b); got != &fresh {
		t.Fatalf("the fights' set gives %p for b's id, want the map's %p", got, &fresh)
	}
}

// A test (or a server's code) that fills the map by hand is caught up with:
// the sets are filled again from the map at the systems' next turn.
func TestNPCSetsFillAgainWhenTheMapChangedByHand(t *testing.T) {
	w := NewWorld()
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := &NPC{ServerID: 20000, Name: "bear", HP: 10, MaxHP: 10, CurrentEnemy: 4242}
	w.NPCs[npc.ServerID] = npc // by hand: no putNPCLocked
	w.aiSystem(time.Now())
	if npc.CurrentEnemy != 0 {
		t.Fatal("an NPC put in the map by hand was never looked at: its gone enemy is still its")
	}
}

// With npcSetsCheck on (this package's tests), a change of an NPC's state
// made without its hook is caught at the system's next turn.
func TestNPCSetsCheckCatchesAChangeWithoutItsHook(t *testing.T) {
	if npcSetsCheckLog {
		t.Skip("EQOA_CHECK_NPC_SETS=log mends an NPC it finds; it does not panic")
	}
	w, _ := setsWorld(t, 4)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.aiSystem(time.Now()) // all quiet, out of the set
	w.NPCs[10002].CurrentEnemy = 77
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), "fight set") {
			t.Fatalf("a fight begun by hand: %v, want the check's panic", r)
		}
	}()
	w.aiSystem(time.Now())
}

// The guards' set is every NPC with a helping social rule, made again when
// the rules change.
func TestGuardSetIsTheSocialNPCs(t *testing.T) {
	w := NewWorld()
	w.SetSocialRules(map[string]SocialRule{"Guard Bob": {Social: true, Faction: "Guards of Freeport"}})
	w.SetPlayerModels(map[uint32]bool{0x70D898C6: true})
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, name := range []string{"Guard Bob", "rat", "guard bob", "Guard Bob"} {
		npc := &NPC{ServerID: EntityID(30000 + i), Name: name, HP: 10, MaxHP: 10}
		w.putNPCLocked(npc)
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	}
	w.TickCount = 0
	w.guardWatchSystem(time.Now())
	if len(w.sets.guard.m) != 3 || w.sets.guard.m[30001] != nil {
		t.Fatalf("the guards' set holds %d (the rat: %v), want the three Guard Bobs", len(w.sets.guard.m), w.sets.guard.m[30001] != nil)
	}
	w.mu.Unlock()
	w.SetSocialRules(map[string]SocialRule{"rat": {Social: true}})
	w.mu.Lock()
	w.guardWatchSystem(time.Now())
	if len(w.sets.guard.m) != 1 || w.sets.guard.m[30001] == nil {
		t.Fatalf("new rules: the guards' set holds %d, want the rat alone", len(w.sets.guard.m))
	}
}

// A set that held many and holds few has its map made again: a Go map keeps
// its room, and a range walks it all (the start's whole world in the fights'
// set, every NPC looked at once).
func TestNPCSetsGiveBackTheirRoom(t *testing.T) {
	w, p := setsWorld(t, 2000)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sets.ai.peak != 2000 {
		t.Fatalf("the fights' set's most is %d, want 2000", w.sets.ai.peak)
	}
	w.addThreatLocked(w.NPCs[10003], p.ServerID, 1)
	w.aiSystem(time.Now())
	if len(w.sets.ai.m) != 1 || w.sets.ai.peak != 1 {
		t.Fatalf("one NPC fighting of 2000 looked at: %d in the set, its most %d; want its map made again for 1",
			len(w.sets.ai.m), w.sets.ai.peak)
	}
}
