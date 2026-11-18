package entity

import "testing"

// Ticket 2380 (Waldorf): Collonridge's instanced basement Decaying Flesh (L64)
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

// Petition 2378 (Waldorf): a familiar (Healbot) shows the ally face a pet
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
