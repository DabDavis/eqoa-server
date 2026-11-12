package entity

import (
	"testing"
	"time"
)

// Petition 1656 (Waldorf, magician 60, multiboxing petters): a pet's hate falls to its master when the pet
// dies or goes (pethate.go). The controls: a dead master gets none (the creature goes home as before), nor
// one far off or in another world, nor a merc's hirer; a merc's pet's goes to its merc.

// petHateRig is the pet rig with the owner's pet on the crab's list and the crab fighting it; the owner
// has never touched the crab.
func petHateRig(t *testing.T) (*petRig, *NPC) {
	t.Helper()
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.mu.Lock()
	r.w.updateAggro(r.enemy, pet.ServerID, 40)
	r.w.mu.Unlock()
	if r.enemy.CurrentEnemy != pet.ServerID {
		t.Fatalf("the crab fights %d, want the pet %d", r.enemy.CurrentEnemy, pet.ServerID)
	}
	if _, on := r.enemy.Threats[r.p.ServerID]; on {
		t.Fatal("the owner is on the crab's list before the pet goes")
	}
	return r, pet
}

// Killed by the creature, the pet's hate is its owner's, and the creature comes on and hits the owner
// rather than running home.
func TestAKilledPetsHateFallsToItsOwner(t *testing.T) {
	r, pet := petHateRig(t)
	pet.HP = 30
	r.enemy.Level = 60
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	r.w.pets[r.p.ServerID].stance = petPassive
	ms := 10000
	for ; ms < 20000 && !pet.Dead; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if !pet.Dead {
		t.Fatalf("the crab never killed the pet: %d HP", pet.HP)
	}
	if r.enemy.CurrentEnemy != r.p.ServerID || r.enemy.Threats[r.p.ServerID] < 40 {
		t.Fatalf("the pet dead: the crab fights %d, the owner's hate %d (want the owner, at least the pet's 40)",
			r.enemy.CurrentEnemy, r.enemy.Threats[r.p.ServerID])
	}
	hp := r.p.HP
	for end := ms + 15000; ms < end && r.p.HP == hp; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if r.p.HP == hp || r.enemy.CurrentEnemy != r.p.ServerID {
		t.Errorf("the crab never hit the owner (HP %d), fights %d", r.p.HP, r.enemy.CurrentEnemy)
	}
}

// Dismissed in a fight, the pet's hate is its owner's.
func TestADismissedPetsHateFallsToItsOwner(t *testing.T) {
	r, pet := petHateRig(t)
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(10*time.Second))
	if !pet.Dead {
		t.Fatal("the pet was not dismissed")
	}
	if r.enemy.CurrentEnemy != r.p.ServerID || r.enemy.Threats[r.p.ServerID] != 40 {
		t.Errorf("dismissed: the crab fights %d, the owner's hate %d: want the owner, 40", r.enemy.CurrentEnemy, r.enemy.Threats[r.p.ServerID])
	}
	if _, on := r.enemy.Threats[pet.ServerID]; on {
		t.Error("the dismissed pet is still on the list")
	}
}

// The owner dead, its pet goes with it and the creature has no one: it gives the fight up, as before.
func TestADeadOwnersPetLeavesNoHate(t *testing.T) {
	r, pet := petHateRig(t)
	r.p.HP = 0
	r.p.DiedAt = r.at(10 * time.Second)
	r.tick(10 * time.Second)
	if !pet.Dead {
		t.Fatal("the dead owner's pet lives on")
	}
	r.tick(10100 * time.Millisecond)
	if r.enemy.CurrentEnemy != 0 || len(r.enemy.Threats) != 0 {
		t.Errorf("the owner dead: the crab fights %d, its list %v: want none", r.enemy.CurrentEnemy, r.enemy.Threats)
	}
}

// Several boxes' pets on one creature, their owners never touching it: each pet that falls puts its own
// owner on the list with its hate; the creature fights the highest, then the next as each owner goes.
// An owner far off (past npcGiveUp) or in another world gets none.
func TestSeveralOwnersPetsHateFallsToEachOwner(t *testing.T) {
	r := newPetRig(t)
	crab := r.enemy
	w := r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	owners := map[EntityID]*Player{}
	pets := map[EntityID]*NPC{}
	for i, hate := range []int{30, 70, 50, 90, 20} {
		o := &Player{ServerID: EntityID(1600 + i), Name: "Box", Level: 60, HP: 900, MaxHP: 900,
			X: crab.X + 10, Y: crab.Y, Z: crab.Z, World: crab.World}
		switch i {
		case 3:
			o.X = crab.X + 400 // far off
		case 4:
			o.World = crab.World + 1 // another world
		}
		w.Players[o.ServerID] = o
		pet := &NPC{ServerID: EntityID(4000 + i), Name: "Pet", Level: 55, HP: 1000, MaxHP: 1000, Owner: o.ServerID,
			X: crab.X + 2, Y: crab.Y, Z: crab.Z, World: crab.World}
		w.NPCs[pet.ServerID] = pet
		owners[o.ServerID], pets[o.ServerID] = o, pet
		w.addThreatLocked(crab, pet.ServerID, hate)
	}
	die := func(owner EntityID) {
		pet := pets[owner]
		pet.HP = 0
		w.killNPC(pet)
		w.dropAggroLocked(pet.ServerID)
	}
	if crab.CurrentEnemy != pets[1603].ServerID {
		t.Fatalf("the crab fights %d, want the far owner's pet (90)", crab.CurrentEnemy)
	}
	die(1603) // its owner far off: none to it, the next pet
	if _, on := crab.Threats[1603]; on || crab.CurrentEnemy != pets[1601].ServerID {
		t.Fatalf("the far owner's pet dead: list %v, fights %d: want no far owner, the 70 pet", crab.Threats, crab.CurrentEnemy)
	}
	die(1601) // 70 to its owner: the owner the highest
	if crab.Threats[1601] != 70 || crab.CurrentEnemy != 1601 {
		t.Fatalf("pet of 1601 dead: list %v, fights %d", crab.Threats, crab.CurrentEnemy)
	}
	die(1602)
	die(1600)
	die(1604) // another world: none to it
	want := map[EntityID]int64{1600: 30, 1601: 70, 1602: 50}
	if len(crab.Threats) != len(want) {
		t.Fatalf("every pet dead: list %v, want %v", crab.Threats, want)
	}
	for id, h := range want {
		if crab.Threats[id] != h {
			t.Errorf("owner %d's hate %d, want %d", id, crab.Threats[id], h)
		}
	}
	if crab.CurrentEnemy != 1601 {
		t.Errorf("the crab fights %d, want 1601 (70)", crab.CurrentEnemy)
	}
	// the owners go down one by one: the creature turns to the next, not home
	for _, next := range []EntityID{1602, 1600, 0} {
		o := owners[crab.CurrentEnemy]
		o.HP = 0
		w.dropAggroLocked(o.ServerID)
		if crab.CurrentEnemy != next {
			t.Fatalf("owner %d dead: the crab fights %d, want %d", o.ServerID, crab.CurrentEnemy, next)
		}
	}
}

// An owner already on the list keeps its own hate and adds its pet's.
func TestAPetsHateAddsToItsOwners(t *testing.T) {
	r, pet := petHateRig(t)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.addThreatLocked(r.enemy, r.p.ServerID, 15)
	r.w.dropAggroLocked(pet.ServerID)
	if r.enemy.Threats[r.p.ServerID] != 55 || r.enemy.CurrentEnemy != r.p.ServerID {
		t.Errorf("owner's hate %d, fights %d: want 15+40, the owner", r.enemy.Threats[r.p.ServerID], r.enemy.CurrentEnemy)
	}
}

// A merc is the group's and fights on its own: its hate does not pass to its hirer. A merc's pet's passes
// to its merc.
func TestMercHateStaysAndMercPetsFallsToTheMerc(t *testing.T) {
	r := newPetRig(t)
	crab, w := r.enemy, r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	merc := &NPC{ServerID: 4100, Name: "Merc", Level: 50, HP: 900, MaxHP: 900, Owner: r.p.ServerID,
		X: crab.X + 3, Y: crab.Y, Z: crab.Z, World: crab.World}
	ms := &mercState{owner: r.p, npc: merc}
	merc.merc = ms
	mpet := &NPC{ServerID: 4101, Name: "Merc pet", Level: 40, HP: 500, MaxHP: 500, Owner: r.p.ServerID, mercPet: ms,
		X: crab.X + 2, Y: crab.Y, Z: crab.Z, World: crab.World}
	w.NPCs[merc.ServerID], w.NPCs[mpet.ServerID] = merc, mpet
	w.addThreatLocked(crab, mpet.ServerID, 60)
	w.dropAggroLocked(mpet.ServerID)
	if crab.Threats[merc.ServerID] != 60 || crab.CurrentEnemy != merc.ServerID {
		t.Fatalf("merc pet gone: list %v, fights %d: want the merc with 60", crab.Threats, crab.CurrentEnemy)
	}
	merc.HP = 0
	w.killNPC(merc)
	w.dropAggroLocked(merc.ServerID)
	if _, on := crab.Threats[r.p.ServerID]; on || crab.CurrentEnemy != 0 {
		t.Errorf("merc dead: list %v, fights %d: want its hirer not on it", crab.Threats, crab.CurrentEnemy)
	}
}
