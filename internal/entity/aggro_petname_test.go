package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Ticket 2380: Collonridge's instanced basement Decaying Flesh (L64)
// stood neutral: the seed's npcaggro row for a player's pet's name ("Decaying
// Flesh ", aggressive 0, "a player's pet recorded beside its owner") held it.
// In an instanced dungeon that row is not the creature's; in the open world it
// still keeps a recorded pet of the name peaceful, and a GM's own peaceful row
// (not a pet's name) still holds in a dungeon.
func TestDungeonCreatureWithAPetsNameIsHostile(t *testing.T) {
	dungeonWorlds[14].Store(true)
	t.Cleanup(func() { dungeonWorlds[14].Store(false) })
	f := newAggroFix(t, 60)
	inside := f.npc(3001, "Decaying Flesh", 64, bearBody, 1010, 1000)
	inside.World = 14
	outside := f.npc(3002, "Decaying Flesh", 20, bearBody, 1200, 1000)
	warden := f.npc(3003, "Dungeon Warden", 60, bearBody, 1300, 1000)
	warden.World = 14
	f.w.SetAggroRules(map[string]AggroRule{
		"Decaying Flesh ": {Aggressive: false, PetName: true},
		"Dungeon Warden":  {Aggressive: false},
	})
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if !f.w.hostileLocked(inside) {
		t.Error("the dungeon's Decaying Flesh is not hostile")
	}
	if f.w.hostileLocked(outside) {
		t.Error("the open world's recorded pet of the name became hostile")
	}
	if f.w.hostileLocked(warden) {
		t.Error("a GM's peaceful row (not a pet's name) no longer holds in a dungeon")
	}
}

// Two spellings of one name, both peaceful: a pet's name only when both are
// (a GM's peaceful spelling wins over the seed's).
func TestPetNameNeedsEveryPeacefulSpelling(t *testing.T) {
	dungeonWorlds[14].Store(true)
	t.Cleanup(func() { dungeonWorlds[14].Store(false) })
	f := newAggroFix(t, 60)
	n := f.npc(3011, "Howling Death", 64, bearBody, 1010, 1000)
	n.World = 14
	for i := 0; i < 20; i++ { // map order varies: every order must agree
		f.w.SetAggroRules(map[string]AggroRule{
			"Howling Death ": {Aggressive: false, PetName: true},
			"Howling Death":  {Aggressive: false},
		})
		f.w.mu.Lock()
		h := f.w.hostileLocked(n)
		f.w.mu.Unlock()
		if h {
			t.Fatal("a GM's peaceful spelling was lost to the seed's pet row")
		}
	}
}

// Petition 2378: a familiar (Healbot) shows the ally face a pet
// does, not a creature's; the control: a creature of no faction beside it.
func TestAFamiliarShowsTheAllyFace(t *testing.T) {
	f := newAggroFix(t, 60)
	fam := f.npc(3021, "Healbot", 60, bearBody, 1005, 1000)
	fam.familiar = &familiarState{owner: f.p, npc: fam}
	other := f.npc(3022, "Rock", 60, bearBody, 1300, 1000)
	other.NPCType = 0x80
	if got := f.face(fam); got != FaceAlly {
		t.Errorf("the familiar's face is %d, want the ally's %d", got, FaceAlly)
	}
	if got := f.face(other); got == FaceAlly {
		t.Error("the control, another NPC, showed the ally face")
	}
}

// Heirloom petition 2508 (2026-11-20): Grundle's cave's Earth Elementalkin stood peaceful under the seed's row for
// a magician's pet's name. A GM's npcfaction place row naming it there (no faction) makes it a creature, its nature
// deciding; the control: one of the name outside the place stays the recorded pet the seed's row keeps peaceful.
func TestAPlaceRowMakesAPetNamedCreatureACreature(t *testing.T) {
	f := newAggroFix(t, 60)
	cave := f.npc(3031, "Earth Elementalkin", 40, bearBody, 1010, 1000)
	away := f.npc(3032, "Earth Elementalkin", 40, bearBody, 1600, 1000)
	cave.SpawnX, cave.SpawnZ, away.SpawnX, away.SpawnZ = cave.X, cave.Z, away.X, away.Z
	f.w.SetAggroRules(map[string]AggroRule{"Earth Elementalkin": {Aggressive: false, PetName: true}})
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "Earth Elementalkin", World: int(cave.World), X: 1000, Z: 1000, Radius: 200}})
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if !f.w.hostileLocked(cave) {
		t.Error("the cave's Earth Elementalkin, a place row's, is not hostile")
	}
	if f.w.hostileLocked(away) {
		t.Error("an Earth Elementalkin outside the place became hostile")
	}
}
