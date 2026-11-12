package entity

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1363 (Battaryhog: "if my mag or enc sets still to long the pets
// despawn, and the charmed ones attack"): a keep restart carries the pet. The
// charmed crab is written beside its master (through JSON, as the file), and
// in the new process the crab its row made, whole on its spawn, is its
// master's again where it stood, with its share of hit points, under its charm
// for the time it had left, and goes free at that time, not before; a summoned
// Fire Elemental is made again by its spell with its share and stance, and its
// owner told after the dump. The controls: with nothing restored the crab is
// its own on its spawn (what attacked Battaryhog), and a pet that died is not
// carried.
func TestAPetComesThroughARestart(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	carry := func(t *testing.T, r *petRig, at time.Duration) KeptPet {
		t.Helper()
		kp, ok := r.w.KeepPlayer(r.sess, r.at(at))
		b, err := json.Marshal(kp)
		var back KeptPlayer
		if !ok || err != nil || json.Unmarshal(b, &back) != nil || back.Pet == nil {
			t.Fatalf("kept %v (%v): %s", ok, err, b)
		}
		return *back.Pet
	}

	// charmed: a 4 minute charm and 150 CHA's 37.5 s, landed at 6.4 s
	timed := entrancing("Entrancing Captivation", 39, "Target, Charm ; Increase 25% based on CHA , 4min")
	a, _, _ := charmRig(t)
	if began, _ := castCharm(t, a, timed, crabID, 0); !began || a.w.petOfLocked(a.p) != a.enemy {
		t.Fatal("the crab was not charmed")
	}
	a.enemy.HP = 1020
	a.enemy.X += 2
	k := carry(t, a, time.Minute)
	left := 4*time.Minute + 37500*time.Millisecond + 6400*time.Millisecond - time.Minute
	if k.NPC != 3200 || k.Charm != "Entrancing Captivation" || time.Duration(k.LeftMS)*time.Millisecond != left || k.HPPct != 0.5 {
		t.Errorf("kept %+v, want the crab, its charm and %v left at half", k, left)
	}

	b, _, _ := charmRig(t) // the new process: the crab as its row made it
	if b.enemy.Owner != 0 || b.enemy.HP != b.enemy.MaxHP {
		t.Fatalf("the control: the crab in the new process is %d's at %d HP", b.enemy.Owner, b.enemy.HP)
	}
	if !b.w.RestorePet(b.sess, k, nil, b.at(0)) || b.w.petOfLocked(b.p) != b.enemy || b.enemy.Charm == nil ||
		b.enemy.HP != 1020 || b.enemy.X != a.enemy.X || b.enemy.NPCType != charmNPCType {
		t.Fatalf("the crab after the restart: pet %v, owner %d, HP %d, x %.1f (want %.1f)", b.w.petOfLocked(b.p), b.enemy.Owner,
			b.enemy.HP, b.enemy.X, a.enemy.X)
	}
	b.sent()
	b.tick(left - time.Second)
	if b.enemy.Charm == nil || b.enemy.Owner != b.p.ServerID {
		t.Fatal("the charm broke before its time")
	}
	if set := opIndex(b.sent(), protocol.OpcodeSetPet); set < 0 {
		t.Error("its master was not told its pet (0x007E)")
	}
	b.tick(left + time.Second)
	if b.enemy.Charm != nil || b.enemy.Owner != 0 {
		t.Errorf("the charm outlived its time: owner %d", b.enemy.Owner)
	}

	// summoned: made again by its spell, half its hit points, aggressive
	s := newPetRig(t)
	pet, _ := s.summon(t, fireElemental(), 0)
	pet.HP = pet.MaxHP / 2
	s.w.pets[s.p.ServerID].stance = petAggressive
	ks := carry(t, s, 20*time.Second)
	if ks.Spell != 717 || ks.NPC != 0 {
		t.Errorf("kept %+v, want spell 717", ks)
	}
	s2 := newPetRig(t)
	spell := fireElemental()
	if !s2.w.RestorePet(s2.sess, ks, &spell, s2.at(0)) {
		t.Fatal("the Fire Elemental did not come back")
	}
	ps := s2.w.petStateLocked(s2.p)
	if ps == nil || ps.npc.Name != pet.Name || ps.npc.HP != ps.npc.MaxHP/2 || ps.stance != petAggressive {
		t.Fatalf("the Fire Elemental after the restart: %+v", ps)
	}
	if s2.w.RestorePet(s2.sess, ks, &spell, s2.at(time.Second)) {
		t.Error("a second pet given back over the first")
	}

	// the control: a dead pet is not carried
	pet.Dead = true
	if kp, _ := s.w.KeepPlayer(s.sess, s.at(30*time.Second)); kp.Pet != nil {
		t.Errorf("a dead pet carried: %+v", kp.Pet)
	}
}

// The owner, 2026-11-12 ("pets are disappearing"): what is on a pet comes
// through with it. A summoned pet's buff on itself and its buff on its owner
// that lasts while it is out (a logout's save leaves that out: buffsave.go) are
// written beside it and put back with its time left, the owner's still the
// pet's (it goes with the pet). The control: a pet given back from a file of
// before (no effects) is bare and its owner without the buff.
func TestAPetsBuffsComeThroughARestart(t *testing.T) {
	s := newPetRig(t)
	pet, _ := s.summon(t, fireElemental(), 0)
	now := s.at(10 * time.Second)
	s.w.mu.Lock()
	pet.Effects = append(pet.Effects, &ActiveEffect{SpellID: 4001, Name: "Pet Haste", Until: now.Add(5 * time.Minute)})
	s.p.Effects = append(s.p.Effects, &ActiveEffect{SpellID: 4002, Name: "Familiar Blessing", Until: now.Add(time.Hour), petBound: true},
		&ActiveEffect{SpellID: 4003, Name: "Its own", Until: now.Add(time.Hour)})
	s.w.mu.Unlock()
	kp, ok := s.w.KeepPlayer(s.sess, now)
	b, err := json.Marshal(kp)
	var back KeptPlayer
	if !ok || err != nil || json.Unmarshal(b, &back) != nil || back.Pet == nil {
		t.Fatalf("kept %v (%v): %s", ok, err, b)
	}
	k := *back.Pet
	if len(k.Effects) != 1 || k.Effects[0].SpellID != 4001 || len(k.OwnerBuffs) != 1 || k.OwnerBuffs[0].SpellID != 4002 {
		t.Fatalf("kept effects %+v, owner's %+v", k.Effects, k.OwnerBuffs)
	}
	spell := fireElemental()

	// the control: a file of before
	c := newPetRig(t)
	bare := k
	bare.Effects, bare.OwnerBuffs = nil, nil
	if !c.w.RestorePet(c.sess, bare, &spell, c.at(0)) || len(c.w.petStateLocked(c.p).npc.Effects) != 0 || c.p.hasEffect(4002) {
		t.Fatal("control: a pet from a file of before came back with buffs")
	}

	s2 := newPetRig(t)
	then := s2.at(0)
	if !s2.w.RestorePet(s2.sess, k, &spell, then) {
		t.Fatal("the pet did not come back")
	}
	ps := s2.w.petStateLocked(s2.p)
	if len(ps.npc.Effects) != 1 || ps.npc.Effects[0].SpellID != 4001 || ps.npc.Effects[0].Until.Sub(then) != 5*time.Minute {
		t.Fatalf("on the pet: %+v", ps.npc.Effects)
	}
	var blessing *ActiveEffect
	for _, e := range s2.p.Effects {
		if e.SpellID == 4002 {
			blessing = e
		}
	}
	if blessing == nil || !blessing.petBound || blessing.Until.Sub(then) != time.Hour || s2.p.hasEffect(4003) {
		t.Fatalf("on its owner: %+v", s2.p.Effects)
	}
}
