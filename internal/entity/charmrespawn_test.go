package entity

import (
	"testing"
	"time"
)

// Heirloom petition 2555, Matonxxsk: "charmed a raging rhino on merc enc , RR
// broke free n was killed now RR runs back to where died and stands there".
// A GM's NPC reload remade the merc's charmed rhino from its row (its charm
// flags differed from the row's), wiping its charm under the merc's pet
// state; the merc pulled the now wild rhino to its side as a summoned pet,
// it fought there, and the merc's next charm killed it as a summoned pet goes
// and then took hold of the corpse. Back from that death it still had the
// fight's chase, so it ran to where that fight began and stood there.

// homeTick is one tick of the creatures' fights and their ways home.
func (r *mercRig) homeTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount++
	now := r.at(d)
	r.w.aiSystem(now)
	r.w.homeSystem(now)
}

// A creature killed in a fight comes back without its chase: it stays on its
// spawn point, not sent to where that fight began.
func TestARespawnedCreatureForgetsItsLastChase(t *testing.T) {
	r := newMercRig(t)
	rhino := r.creature("raging rhinoceros", 56, 40)
	rhino.SpawnX, rhino.SpawnY, rhino.SpawnZ = rhino.X, rhino.Y, rhino.Z
	rhino.RespawnTime = 15 * time.Second
	// its fight began 100 off its spawn (beside the merc that held it)
	r.w.mu.Lock()
	rhino.X -= 100
	rhino.chase = chaseState{on: true, fromX: rhino.X, fromY: rhino.Y, fromZ: rhino.Z}
	rhino.HP = 0
	r.w.killNPC(rhino)
	r.w.respawnNPCLocked(rhino)
	r.w.mu.Unlock()
	if rhino.chase.on || rhino.chase.returning {
		t.Errorf("back from its death: chase on %v, returning %v", rhino.chase.on, rhino.chase.returning)
	}
	for d := time.Duration(0); d <= 8*time.Second; d += 100 * time.Millisecond {
		r.homeTick(d)
	}
	if got := distXZ(rhino.X, rhino.Z, rhino.SpawnX, rhino.SpawnZ); got > 0.5 {
		t.Fatalf("8 s after its respawn it is %.1f off its spawn point (its last fight began 100 off)", got)
	}
}

// The control: a live creature whose fight ends still goes back to where it
// began, and stands there.
func TestAFightOverStillGoesHome(t *testing.T) {
	r := newMercRig(t)
	rhino := r.creature("raging rhinoceros", 56, 40)
	rhino.SpawnX, rhino.SpawnY, rhino.SpawnZ = rhino.X, rhino.Y, rhino.Z
	r.w.mu.Lock()
	fromX := rhino.X
	rhino.chase = chaseState{on: true, fromX: fromX, fromY: rhino.Y, fromZ: rhino.Z}
	rhino.X += 20
	r.w.wakeNPCLocked(rhino)
	r.w.mu.Unlock()
	for d := time.Duration(0); d <= 8*time.Second; d += 100 * time.Millisecond {
		r.homeTick(d)
	}
	if got := distXZ(rhino.X, rhino.Z, fromX, rhino.chase.fromZ); got > 0.5 || rhino.chase.returning {
		t.Fatalf("its fight over: %.1f off where it began, returning %v", got, rhino.chase.returning)
	}
}

// The merc's charmed pet, its charm wiped from under it (as the reload did),
// charmed again: taken alive, never killed and then charmed dead.
func TestAMercsCharmNeverTakesTheDead(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = func(int) int { return 99 } // never resisted
	r := newMercRig(t)
	r.p.Level = 49
	ms := r.hire(r.kit(classEnchanter, 0, 0, entrancing("Entrancing Enticement", 30, "Target, Charm ; Increase 25% based on CHA , 10min")), MercAssist)
	crab := r.creature("Desert Crab", 20, 10)
	r.charmTarget(crab)
	if _, why := r.w.MercCharm(r.sess, 0, r.at(0)); why != "" {
		t.Fatalf("/merc charm: %q", why)
	}
	r.run(0, 8*time.Second)
	if ms.pet == nil || ms.pet.npc != crab || crab.Charm == nil || crab.Dead {
		t.Fatal("the control: the crab was not charmed alive")
	}
	// what the reload's remake left: the creature its row's again, the merc's
	// pet state still on it
	r.w.mu.Lock()
	own := crab.Charm.npcType
	crab.Charm, crab.Owner, crab.mercPet, crab.NPCType = nil, 0, nil, own
	ms.ready = map[int]time.Time{}
	ms.busy = time.Time{}
	r.w.mu.Unlock()
	if _, why := r.w.MercCharm(r.sess, 0, r.at(9*time.Second)); why != "" {
		t.Fatalf("/merc charm again: %q", why)
	}
	r.run(9*time.Second, 17*time.Second)
	if crab.Dead {
		t.Fatal("the merc killed its own pet as it charmed it again")
	}
	if crab.Charm == nil || crab.Owner != r.p.ServerID || ms.pet == nil || ms.pet.npc != crab {
		t.Fatalf("charmed again: charm %v, owner %d, merc's pet %v", crab.Charm != nil, crab.Owner, ms.pet != nil)
	}
}

// A GM's reload leaves a charmed creature whose row did not change alone: its
// charm flags are not a change of its row's.
func TestACharmedCreatureIsItsRowsSame(t *testing.T) {
	row := &NPC{ServerID: 104905, Name: "raging rhinoceros", NPCType: 0x0100, Level: 56, MaxHP: 1000, ModelID: 7, Size: 1}
	charmed := *row
	charmed.Charm = &charmHold{npcType: row.NPCType}
	charmed.NPCType = charmNPCType
	if !npcSameRecord(&charmed, row) {
		t.Error("a charmed creature of an unchanged row counts as changed")
	}
	// the control: a row that did change is still one
	changed := *row
	changed.Level = 57
	if npcSameRecord(&charmed, &changed) {
		t.Error("a changed row counts as the same")
	}
}
