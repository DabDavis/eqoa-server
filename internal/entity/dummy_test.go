package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The training dummies held to what Sandstorm's did (2026-10-21): the
// round's swing and hit times, the hit with its heal, a hit taken off
// nothing, one player at a time, and the fight over when the player stops.
// What a hit is, is defence.go's (defence_test.go).

// newDummy is the caster's fixture with its creature made a dummy of the
// level and kind given, the player engaged with it.
func newDummy(t *testing.T, name string, level int) *caster {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.Dummy = name, level, DummyKindOf(name)
	c.npc.HP, c.npc.MaxHP = 4000, 4000
	c.p.AttackTarget, c.p.AttackObjectID = c.npc.ServerID, c.npcO
	return c
}

func (c *caster) dummyTick(d time.Duration) [][]byte {
	c.w.mu.Lock()
	c.w.aiSystem(c.at(d))
	c.w.mu.Unlock()
	return c.sent()
}

// 0x00DB as the client reads it: attacker, zigzag amount, defender.
func damageMsg(t *testing.T, from uint32, amount int, to uint32) []byte {
	t.Helper()
	b := []byte{0xDB, 0x00, byte(from), byte(from >> 8), byte(from >> 16), byte(from >> 24)}
	z := uint64(amount) << 1
	if amount < 0 {
		z = uint64(-amount)<<1 - 1
	}
	for z >= 0x80 {
		b = append(b, byte(z)|0x80)
		z >>= 7
	}
	b = append(b, byte(z), byte(to), byte(to>>8), byte(to>>16), byte(to>>24))
	return b
}

func TestTrainingDummyFightsInRounds(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv35", 35)
	c.p.AC = 600 // enough armour for the Lv35's floor, 14, every hit
	c.w.mu.Lock()
	killed := c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	hit := c.sent()
	if killed || c.npc.Dead || c.npc.HP != 4000 || c.p.TotalXP != 0 {
		t.Fatalf("the hit killed %v, dead %v, HP %d, XP %d: a dummy takes nothing", killed, c.npc.Dead, c.npc.HP, c.p.TotalXP)
	}
	if len(hit) != 1 || opOfMsg(hit[0]) != protocol.OpcodeDamageIndication {
		t.Fatalf("the hit sent %x, want its 0x00DB", hit)
	}
	if c.npc.DummyTarget != c.p.ServerID || c.npc.Threats != nil || c.npc.CurrentEnemy != 0 {
		t.Fatalf("target %d, threats %v, enemy %d: the dummy fights the hitter and holds no aggro", c.npc.DummyTarget, c.npc.Threats, c.npc.CurrentEnemy)
	}

	// The round: the swing at once, Attack4 over its 6 s
	start := c.dummyTick(0)
	want := unhex(t, "b100982e3101d0a12414007017000014ea9bda") // 0x00B1 dummy -> player, 0 more, 6000 ms, Attack4
	if len(start) != 1 || !bytes.Equal(start[0], want) {
		t.Fatalf("round start % X, want % X", start, want)
	}
	if got := c.dummyTick(499 * time.Millisecond); len(got) != 0 {
		t.Fatalf("a hit before 0.5 s: %x", got)
	}
	// each hit 14 off and a 300 heal after it, within the most
	first := c.dummyTick(500 * time.Millisecond)
	if len(first) != 2 || !bytes.Equal(first[0], damageMsg(t, c.npcO, -14, c.me)) || !bytes.Equal(first[1], damageMsg(t, c.npcO, 300, c.me)) {
		t.Fatalf("first hit %x", first)
	}
	if c.p.HP != 536 {
		t.Errorf("HP %d after 500 -14 +300, want the most, 536", c.p.HP)
	}
	if got := c.dummyTick(2400 * time.Millisecond); len(got) != 6 {
		t.Fatalf("hits at 1, 1.7 and 2.4 s: %d messages", len(got))
	}
	if got := c.dummyTick(5999 * time.Millisecond); len(got) != 0 {
		t.Fatalf("more than four hits a round: %x", got)
	}
	if got := c.dummyTick(6 * time.Second); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
		t.Fatalf("the second round's swing: %x", got)
	}
	// The player lets go: the dummy fights on, as Sandstorm's did, until
	// the player walks out of its reach.
	c.p.disengage()
	if got := c.dummyTick(8400 * time.Millisecond); len(got) != 8 {
		t.Errorf("the round's four hits after a letting go: %d messages", len(got))
	}
	if got := c.dummyTick(12 * time.Second); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
		t.Errorf("no third round for a player still beside it: %x", got)
	}
	c.p.X += 20
	if got := c.dummyTick(12500 * time.Millisecond); len(got) != 0 || c.npc.DummyTarget != 0 {
		t.Errorf("hits on a player 20 away: %x, target %d", got, c.npc.DummyTarget)
	}
}

// A Regular Dummy hits as a creature does and heals nothing.
func TestRegularDummyNoHeal(t *testing.T) {
	c := newDummy(t, "Regular Dummy Lv60", 60)
	c.w.mu.Lock()
	c.w.dummyEngagedLocked(c.npc, c.p.ServerID, 1, c.at(0))
	c.w.mu.Unlock()
	c.dummyTick(0)
	got := c.dummyTick(500 * time.Millisecond)
	if len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeDamageIndication || c.p.HP >= 500 {
		t.Errorf("a Regular Dummy's hit: %x, HP %d", got, c.p.HP)
	}
}

// One player at a time: a second player's hit does not take the dummy, and
// walking out of its reach ends the fight.
func TestDummyFightsOnePlayer(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv20", 20)
	other := &Player{ServerID: 1002, Name: "Other", HP: 100, MaxHP: 100, X: 100, Z: 100}
	c.w.Players[other.ServerID] = other
	c.w.mu.Lock()
	c.w.dummyEngagedLocked(c.npc, c.p.ServerID, 1, c.at(0))
	c.w.dummyEngagedLocked(c.npc, other.ServerID, 1, c.at(0))
	c.w.mu.Unlock()
	if c.npc.DummyTarget != c.p.ServerID {
		t.Fatalf("the dummy went to %d", c.npc.DummyTarget)
	}
	c.p.X += 20
	if got := c.dummyTick(0); len(got) != 0 || c.npc.DummyTarget != 0 {
		t.Errorf("fought a player 20 away: %x, target %d", got, c.npc.DummyTarget)
	}
}

// A spell on a dummy is told and takes nothing, and the dummy fights the caster.
func TestDummySpellHit(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv45", 45)
	c.w.mu.Lock()
	killed := c.w.spellHitLocked(c.p, c.sess, c.npc, 5000, numberSpell)
	c.w.mu.Unlock()
	if killed || c.npc.HP != 4000 || c.npc.DummyTarget != c.p.ServerID {
		t.Errorf("killed %v, HP %d, target %d", killed, c.npc.HP, c.npc.DummyTarget)
	}
	if got := c.sent(); len(got) != 1 || !bytes.Equal(got[0], damageMsg(t, c.me, -5000, c.npcO)) {
		t.Errorf("the spell's figure: %x", got)
	}
}

// Petition 1484 (the magician Lifeispain): "Sparing/Training dummy's in FP
// won't attack a pet, even if the caster is well out of striking distance.
// They just stand there. They should attack whichever has the most hate".
// A pet's hits on a dummy make the dummy's hate the pet's: with its owner 25
// off the dummy fights the pet, a Training Dummy's heal after each hit.
func TestADummyFightsThePetThatHitsIt(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	dummy := r.enemy
	dummy.Name, dummy.Dummy, dummy.Level = "Training Dummy Lv35", TrainingDummy, 35
	dummy.X = r.p.X + 25 // the owner well out of its reach
	r.w.SpatialIndex.Update(dummy.ServerID, 8640, 5421.5, dummy.X, dummy.Z)
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(10*time.Second))
	r.sent()
	swings, low := 0, pet.MaxHP
	pet.HP = pet.MaxHP - 100 // room to see the heal
	for ms := 10100; ms < 40000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
		low = min(low, pet.HP)
		for _, m := range r.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat && len(m) >= 6 &&
				uint32(m[2])|uint32(m[3])<<8|uint32(m[4])<<16|uint32(m[5])<<24 == 0x0133E8A0 {
				swings++
			}
		}
	}
	if dummy.DummyTarget != pet.ServerID || swings == 0 {
		t.Fatalf("the dummy fights %d (the pet is %d) and swung %d times: want it on the pet", dummy.DummyTarget, pet.ServerID, swings)
	}
	if dummy.HP != dummy.MaxHP || dummy.CurrentEnemy != 0 || dummy.Threats != nil {
		t.Errorf("the dummy took something or holds aggro: HP %d/%d, enemy %d, threats %v", dummy.HP, dummy.MaxHP, dummy.CurrentEnemy, dummy.Threats)
	}
	if pet.Dead || pet.HP != pet.MaxHP {
		t.Errorf("a Training Dummy's hits on a pet are healed after: pet HP %d/%d, dead %v", pet.HP, pet.MaxHP, pet.Dead)
	}
	if r.p.HP != r.p.MaxHP {
		t.Errorf("the owner, 25 off, was hit: HP %d", r.p.HP)
	}
	// the pet called back out of its reach: the dummy's fight ends
	r.w.PetCommand(r.sess, byte(petPassive), 0, r.at(40*time.Second))
	for ms := 40100; ms < 50000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if dummy.DummyTarget != 0 || len(dummy.dummyHate) != 0 {
		t.Errorf("the pet gone, the dummy still fights %d (hate %v)", dummy.DummyTarget, dummy.dummyHate)
	}
}

// The dummy fights whichever in its reach has its most hate: the pet while its
// hits outweigh its owner's, the owner once its own do, and whoever leaves its
// reach is let go. A Regular Dummy heals nothing and its hits take the pet's
// hit points; a Sparring Dummy stops at the pet's last hit point.
func TestADummyFightsItsMostHate(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	dummy := r.enemy
	dummy.Name, dummy.Dummy, dummy.Level = "Regular Dummy Lv60", RegularDummy, 60
	dummy.X = r.p.X + 5
	r.w.SpatialIndex.Update(dummy.ServerID, 8640, 5421.5, dummy.X, dummy.Z)
	pet.X, pet.Z = dummy.X+2, dummy.Z
	foe := func(from, to time.Duration) EntityID {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		for d := from; d <= to; d += 100 * time.Millisecond {
			r.w.aiSystem(r.at(d))
		}
		return dummy.DummyTarget
	}
	r.w.mu.Lock()
	r.w.dummyEngagedLocked(dummy, r.p.ServerID, 100, r.at(0))
	r.w.dummyEngagedLocked(dummy, pet.ServerID, 300, r.at(0))
	r.w.mu.Unlock()
	// its round's hits come at 0.5, 1, 1.7 and 2.4 s
	hp := pet.HP
	if got := foe(0, 600*time.Millisecond); got != pet.ServerID || pet.HP >= hp || r.p.HP != r.p.MaxHP {
		t.Fatalf("300 of the pet's against 100 of its owner's: the dummy fights %d, pet HP %d -> %d, owner %d", got, hp, pet.HP, r.p.HP)
	}
	r.w.mu.Lock()
	r.w.dummyEngagedLocked(dummy, r.p.ServerID, 500, r.at(700*time.Millisecond))
	r.w.mu.Unlock()
	hp, mine := pet.HP, r.p.HP
	if got := foe(700*time.Millisecond, 1100*time.Millisecond); got != r.p.ServerID || r.p.HP >= mine || pet.HP != hp {
		t.Fatalf("600 of the owner's against the pet's 300: the dummy fights %d, owner HP %d -> %d, pet %d -> %d", got, mine, r.p.HP, hp, pet.HP)
	}
	// a taunt puts the pet on top
	r.w.mu.Lock()
	r.w.petTopHateLocked(pet, dummy)
	r.w.mu.Unlock()
	if got := foe(1200*time.Millisecond, 1800*time.Millisecond); got != pet.ServerID || pet.HP >= hp {
		t.Fatalf("after the pet's taunt the dummy fights %d, pet HP %d -> %d", got, hp, pet.HP)
	}
	// the pet out of its reach: its owner again; the owner out too: no one
	pet.X += 30
	if got := foe(1900*time.Millisecond, 1900*time.Millisecond); got != r.p.ServerID {
		t.Fatalf("the pet 30 off, the dummy fights %d", got)
	}
	r.p.X -= 30
	if got := foe(2*time.Second, 2*time.Second); got != 0 || dummy.dummyHate != nil {
		t.Fatalf("everyone gone, the dummy fights %d (hate %v)", got, dummy.dummyHate)
	}
	// a Regular Dummy may kill a pet
	r.p.X += 30
	pet.X, pet.HP = dummy.X+2, 1
	r.w.mu.Lock()
	r.w.dummyEngagedLocked(dummy, pet.ServerID, 50, r.at(20*time.Second))
	r.w.mu.Unlock()
	for d := 20 * time.Second; d < 30*time.Second && !pet.Dead; d += 100 * time.Millisecond {
		foe(d, d)
	}
	if got := foe(30*time.Second, 30*time.Second); !pet.Dead || got != 0 {
		t.Errorf("a Regular Dummy's hit on a pet of 1 HP: dead %v, the dummy fights %d", pet.Dead, dummy.DummyTarget)
	}
}

func TestASparringDummyStopsShortOfKillingAPet(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	dummy := r.enemy
	dummy.Name, dummy.Dummy, dummy.Level, dummy.DummyHits = "Sparring Dummy x2", SparringDummy, 60, 2
	pet.X, pet.Z, pet.HP = dummy.X+2, dummy.Z, 5
	r.w.mu.Lock()
	r.w.dummyEngagedLocked(dummy, pet.ServerID, 50, r.at(0))
	r.w.mu.Unlock()
	r.sent()
	for d := time.Duration(0); d < 10*time.Second && dummy.DummyTarget != 0; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		r.w.aiSystem(r.at(d))
		r.w.mu.Unlock()
	}
	if pet.Dead || pet.HP != 1 || dummy.DummyTarget != 0 {
		t.Fatalf("a sparring dummy on a pet of 5 HP: dead %v, HP %d, still fighting %d", pet.Dead, pet.HP, dummy.DummyTarget)
	}
	said := false
	for _, m := range r.sent() {
		said = said || bytes.Contains([]byte(chatText(m)), []byte("That will do"))
	}
	if !said {
		t.Error("the owner was not told the sparring dummy let its pet be")
	}
}
