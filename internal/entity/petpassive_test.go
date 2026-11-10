package entity

import (
	"strings"
	"testing"
	"time"
)

// Petition 1483 (the magician Lifeispain): "Pet Passive doesn't stop the pet
// from fighting. It should stop fighting and run back to the owner and stand
// there until it's told to fight". A pet in a fight made passive lets its
// target go at once, lands no more hits, walks back to its owner and stands;
// an attack command sends it out again; the other stances leave a fight on.
func TestAPassivePetStopsFightingAndComesBack(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	r.enemy.X = r.p.X + 25 // a walk off, within its leash
	r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
	r.w.SpatialIndex.Update(r.enemy.ServerID, 8640, 5421.5, r.enemy.X, r.enemy.Z)
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(10*time.Second))
	ms := 10100
	for ; ms < 30000 && r.enemy.HP == r.enemy.MaxHP; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if r.enemy.HP == r.enemy.MaxHP || distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z) > petReach {
		t.Fatalf("the pet never fought: crab %d HP, pet %.1f off it", r.enemy.HP, distXZ(pet.X, pet.Z, r.enemy.X, r.enemy.Z))
	}
	// the other stances leave its fight on
	for _, st := range []petStance{petDefensive, petNeutral, petAggressive} {
		r.w.PetCommand(r.sess, byte(st), 0, r.at(time.Duration(ms)*time.Millisecond))
		if ps.target != r.enemy.ServerID {
			t.Fatalf("stance %d ended the pet's fight", st)
		}
	}
	r.sent()
	r.w.PetCommand(r.sess, byte(petPassive), 0, r.at(time.Duration(ms)*time.Millisecond))
	if msgs := r.sent(); len(msgs) != 1 || chatText(msgs[0]) != petStanceWords[petPassive] {
		t.Fatalf("passive's answer: %x", msgs)
	}
	if ps.target != 0 || !pet.RoundStart.IsZero() {
		t.Fatalf("made passive, the pet keeps its fight: target %d, round %v", ps.target, pet.RoundStart)
	}
	r.enemy.CurrentEnemy, r.enemy.Threats = 0, nil // the crab stands: only the pet's own doing is looked at
	hp := r.enemy.HP
	end := ms + 15000
	for ms += 100; ms < end; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
		r.enemy.CurrentEnemy, r.enemy.Threats = 0, nil
	}
	if r.enemy.HP != hp {
		t.Errorf("the passive pet went on hitting: crab %d -> %d HP", hp, r.enemy.HP)
	}
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d > petFollowStart || pet.Loco != LocoStand || ps.target != 0 {
		t.Errorf("the passive pet is %.1f from its owner, loco %d, target %d: want standing beside it", d, pet.Loco, ps.target)
	}
	// told to fight, it does
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(time.Duration(ms)*time.Millisecond))
	if msgs := r.sent(); len(msgs) == 0 || !strings.Contains(chatText(msgs[len(msgs)-1]), "attacks Desert Crab") {
		t.Fatalf("the attack's answer: %x", msgs)
	}
	for end = ms + 15000; ms < end && r.enemy.HP == hp; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if r.enemy.HP == hp || ps.stance != petPassive {
		t.Errorf("a passive pet told to attack did not: crab %d HP, stance %d", r.enemy.HP, ps.stance)
	}
}

// A passive pet leaves its owner's duel too, until told to attack.
func TestAPassivePetLeavesTheDuel(t *testing.T) {
	r := newPetRig(t)
	r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	foe := &Player{ServerID: 1002, Name: "Rival", Level: 45, HP: 900, MaxHP: 900, X: r.p.X + 5, Y: r.p.Y, Z: r.p.Z, World: r.p.World}
	r.w.Players[foe.ServerID] = foe
	d := &Duel{ID: 7, A: r.p.ServerID, B: foe.ServerID, who: [2]*Player{r.p, foe}, Phase: duelOn, World: r.p.World,
		pets: map[EntityID]bool{r.p.ServerID: true}, petOut: map[EntityID]bool{}, petTarget: map[EntityID]EntityID{r.p.ServerID: foe.ServerID},
		struck: map[EntityID]bool{}, petStruck: map[EntityID]bool{}}
	r.w.duels.of = map[EntityID]*Duel{r.p.ServerID: d, foe.ServerID: d}
	r.w.PetCommand(r.sess, byte(petPassive), 0, r.at(10*time.Second))
	if d.pets[r.p.ServerID] || ps.target != 0 {
		t.Errorf("a passive pet stays in the duel: pets %v, target %d", d.pets, ps.target)
	}
}
