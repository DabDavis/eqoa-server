package entity

import (
	"testing"
	"time"
)

// A pet that yields in a duel is out of that duel, not its owner's pets after
// it (the owner, 2026-11-18, on Heirloom: "pets if killed in a duel cant be
// resummoned": a player's Construct yielded to a GM and the two he summoned
// after stood out of the duel until he forfeited).

// newYieldDuelists is the duelists with Alpha a level 45 magician, the Fire
// Elemental's bodies in the world for its summoning (pet_test.go).
func newYieldDuelists(t *testing.T) *duelists {
	t.Helper()
	d := newDuelists(t)
	d.a.Class, d.a.Level, d.a.Power, d.a.MaxPower = 10, 45, 50000, 50000
	for i := 0; i < 2; i++ {
		n := &NPC{ServerID: EntityID(3000 + i), Name: "Fire Elemental", Level: 39, HP: 2340, MaxHP: 2340, ModelID: 0x8A416241,
			Size: 1, NPCType: 0x80, X: 20000, Z: 20000}
		d.w.NPCs[n.ServerID] = n
	}
	return d
}

// summonFire has Alpha cast the Fire Elemental at `at` and land it.
func (d *duelists) summonFire(t *testing.T, at time.Duration) *NPC {
	t.Helper()
	spell := fireElemental()
	if !d.w.StartCast(d.sa, spell, 0xFFFFFFFF, d.at(at)) {
		t.Fatal("the summon did not begin")
	}
	d.w.mu.Lock()
	d.w.castSystem(d.at(at + spell.CastTime + castLand))
	pet := d.w.petOfLocked(d.a)
	d.w.mu.Unlock()
	if pet == nil {
		t.Fatal("no pet")
	}
	return pet
}

// petTicks runs the pet and duel systems from a to b.
func (d *duelists) petTicks(a, b time.Duration) {
	for x := a; x < b; x += 100 * time.Millisecond {
		d.w.mu.Lock()
		d.w.petSystem(d.at(x))
		d.w.mu.Unlock()
		d.tick(x)
	}
}

// yield leaves Alpha's pet at a hit point, out of the duel.
func (d *duelists) yield(t *testing.T, duel *Duel, pet *NPC, at time.Duration) {
	t.Helper()
	d.w.mu.Lock()
	d.w.duelHurtPetLocked(duel, d.b, pet, 10_000_000, d.at(at), numberMelee)
	out := duel.petOut[d.a.ServerID]
	d.w.mu.Unlock()
	if pet.HP != 1 || !out {
		t.Fatalf("the yield: HP %d, out %v", pet.HP, out)
	}
}

// The pet yields; its owner summons another in the same duel: it stays, it is
// the duel's (the opponent's foe, shown to it in sub 7), it joins by its
// stance and lands its hits, and an attack command at the opponent is taken.
func TestDuelPetResummonedAfterAYield(t *testing.T) {
	d := newYieldDuelists(t)
	duel := d.start(t)
	pet := d.summonFire(t, time.Second)
	d.yield(t, duel, pet, 20*time.Second)

	pet2 := d.summonFire(t, 30*time.Second)
	if pet2 == pet {
		t.Fatal("the summon kept the old pet")
	}
	d.w.mu.Lock()
	in := d.w.duelFoeLocked(d.b, pet2.ServerID) && !duel.petOut[d.a.ServerID]
	d.w.mu.Unlock()
	if !in {
		t.Fatalf("the new pet is out of the duel: out %v", duel.petOut[d.a.ServerID])
	}
	// a defensive pet joins once the opponent struck its owner
	d.w.mu.Lock()
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 10, d.at(40*time.Second), numberMelee)
	d.w.mu.Unlock()
	before := d.b.HP
	d.petTicks(40*time.Second, 50*time.Second)
	d.w.mu.Lock()
	still := d.w.petOfLocked(d.a) == pet2 && !pet2.Dead && d.w.NPCs[pet2.ServerID] != nil
	joined := duel.pets[d.a.ServerID]
	d.w.mu.Unlock()
	if !still {
		t.Fatal("the new pet went")
	}
	if !joined || d.b.HP >= before {
		t.Fatalf("the new pet did not fight: joined %v, the opponent's HP %d of %d", joined, d.b.HP, before)
	}

	// an attack command at the opponent is the duel's
	d.w.mu.Lock()
	d.w.petStandsDownLocked(d.w.pets[d.a.ServerID])
	d.w.mu.Unlock()
	d.w.PetCommand(d.sa, petCmdAttack, d.aSeesB, d.at(51*time.Second))
	if !duel.pets[d.a.ServerID] || duel.petTarget[d.a.ServerID] != d.b.ServerID {
		t.Fatalf("the attack command: in %v, target %d", duel.pets[d.a.ServerID], duel.petTarget[d.a.ServerID])
	}

	// it can yield in its turn
	d.yield(t, duel, pet2, 52*time.Second)
}

// The pet yields and the duel ends: the pet the owner summons after stays,
// with nothing of the duel on it, and a later duel has it.
func TestDuelPetResummonedAfterTheDuel(t *testing.T) {
	d := newYieldDuelists(t)
	duel := d.start(t)
	pet := d.summonFire(t, time.Second)
	d.yield(t, duel, pet, 20*time.Second)
	d.w.mu.Lock()
	d.w.endDuelLocked(duel, d.a.ServerID, duelLeft, d.at(21*time.Second))
	d.w.mu.Unlock()

	pet2 := d.summonFire(t, 30*time.Second)
	d.petTicks(40*time.Second, 50*time.Second)
	d.w.mu.Lock()
	still := d.w.petOfLocked(d.a) == pet2 && !pet2.Dead && pet2.HP == pet2.MaxHP
	d.w.mu.Unlock()
	if !still {
		t.Fatalf("the new pet: %v, HP %d/%d", d.w.petOfLocked(d.a) == pet2, pet2.HP, pet2.MaxHP)
	}

	d.t0 = d.at(time.Minute)
	duel2 := d.start(t)
	d.w.mu.Lock()
	in := d.w.duelFoeLocked(d.b, pet2.ServerID) && !duel2.petOut[d.a.ServerID]
	d.w.mu.Unlock()
	if !in {
		t.Fatal("the next duel has the pet out")
	}
}

// The control: a pet killed out of any duel; the next one stays.
func TestPetResummonedAfterItsDeath(t *testing.T) {
	d := newYieldDuelists(t)
	pet := d.summonFire(t, time.Second)
	d.w.mu.Lock()
	pet.HP = 0
	d.w.killNPC(pet)
	d.w.mu.Unlock()
	d.petTicks(20*time.Second, 21*time.Second)
	d.w.mu.Lock()
	gone := d.w.petOfLocked(d.a) == nil
	d.w.mu.Unlock()
	if !gone {
		t.Fatal("the dead pet is still the pet")
	}
	pet2 := d.summonFire(t, 30*time.Second)
	d.petTicks(40*time.Second, 50*time.Second)
	d.w.mu.Lock()
	still := d.w.petOfLocked(d.a) == pet2 && !pet2.Dead
	d.w.mu.Unlock()
	if !still {
		t.Fatal("the new pet went")
	}
}

// A merc is no duel's (merc.go): it cannot be fought in one, so it never
// yields; its owner's pet yielding and being summoned again leaves it so,
// and the new pet is the duel's beside it.
func TestDuelMercBesideAResummonedPet(t *testing.T) {
	d := newYieldDuelists(t)
	merc := &NPC{ServerID: 4100, Name: "Hireling", Level: 40, HP: 3000, MaxHP: 3000, Owner: d.a.ServerID, X: 102, Y: 10, Z: 102}
	merc.merc = &mercState{owner: d.a, npc: merc}
	d.w.NPCs[merc.ServerID] = merc
	duel := d.start(t)
	d.w.mu.Lock()
	foe := d.w.duelFoeLocked(d.b, merc.ServerID)
	d.w.mu.Unlock()
	if foe {
		t.Fatal("a merc is the opponent's foe")
	}
	pet := d.summonFire(t, time.Second)
	d.yield(t, duel, pet, 20*time.Second)
	pet2 := d.summonFire(t, 30*time.Second)
	d.w.mu.Lock()
	mercFoe, petFoe := d.w.duelFoeLocked(d.b, merc.ServerID), d.w.duelFoeLocked(d.b, pet2.ServerID)
	d.w.mu.Unlock()
	if mercFoe || !petFoe || merc.Dead || merc.HP != 3000 {
		t.Fatalf("merc foe %v (HP %d), new pet foe %v", mercFoe, merc.HP, petFoe)
	}
}
