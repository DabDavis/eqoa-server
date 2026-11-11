package entity

import (
	"testing"
	"time"
)

// A merc's own pet (mercpet.go, the owner 2026-11-11: "let mercenaries cast
// their pets"): a magician merc summons the best summoning of its book as a
// player casts it (its cast time, its power), the pet of its merc's level
// beside it, the merc's and not its hirer's class pet (no 0x007E; the hirer's
// own pet slot empty); it fights what the merc fights and its kill is the
// hirer's; killed, it is summoned again once the spell's recast is over (not
// before); it goes with its merc. The control: a warrior merc, no summoning in
// its book, summons nothing.
func TestMercSummonsItsPet(t *testing.T) {
	r := newMercRig(t)
	// the body the magician's pets wear
	r.w.mu.Lock()
	r.w.putNPCLocked(&NPC{ServerID: 4900, Name: "Fire Elemental", Level: 39, HP: 2340, MaxHP: 2340, ModelID: 0x8A416241, Size: 1,
		NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	r.w.mu.Unlock()
	low := bookSpell(632, "Earth Elementaling", 0, 3*time.Second, 5*time.Second, 20, "Summons an earth elementaling to aid you")
	low.Level = 1
	fire := bookSpell(717, "Fire Elemental", 0, 8*time.Second, 45*time.Second, 300, "Summons a Fire Elemental to aid you")
	fire.Level = 28
	ms := r.hire(r.kit(10, 0, 0, low, fire), MercAssist)
	r.sent()

	r.run(0, 7*time.Second)
	if ms.pet != nil {
		t.Fatal("a pet before the 8 s cast was over")
	}
	r.run(7100*time.Millisecond, 10*time.Second)
	ps := ms.pet
	if ps == nil {
		t.Fatal("no pet after the summoning's cast")
	}
	pet := ps.npc
	if pet.Name != "Fire Elemental" || pet.Level != 28 || pet.Owner != r.p.ServerID || !pet.isMercPet() || pet.Dead {
		t.Fatalf("pet %q level %d owner %d merc's %v dead %v; want the merc's Fire Elemental of 28", pet.Name, pet.Level, pet.Owner,
			pet.isMercPet(), pet.Dead)
	}
	if ms.npc.Power != ms.npc.MaxPower-300 && ms.npc.Power >= ms.npc.MaxPower {
		t.Errorf("the merc's power %d/%d: the summoning paid nothing", ms.npc.Power, ms.npc.MaxPower)
	}
	if r.w.pets[r.p.ServerID] != nil || r.sent()[0x007E] != 0 {
		t.Error("the merc's pet took its hirer's class pet slot")
	}
	if d := distXZ(pet.X, pet.Z, ms.npc.X, ms.npc.Z); d > 2*petFollowStart {
		t.Errorf("the pet %.1f off its merc", d)
	}

	// it fights what its merc fights; the kill is the hirer's
	crab := r.creature("crab", 28, 6)
	crab.HP, crab.MaxHP = 150, 150
	r.w.mu.Lock()
	r.p.AttackTarget = crab.ServerID
	r.p.AutoAttack = true
	r.w.mu.Unlock()
	xp := r.p.TotalXP + r.p.XPInLevel
	sawFight := false
	for d := 10100 * time.Millisecond; d <= 40*time.Second && !crab.Dead; d += 100 * time.Millisecond {
		r.tick(d)
		if ps.target == crab.ServerID {
			sawFight = true
		}
	}
	if !sawFight {
		t.Fatal("the pet never took on its merc's target")
	}
	if !crab.Dead {
		t.Fatal("the crab still stands after 30 s of the merc and its pet")
	}
	if r.p.TotalXP+r.p.XPInLevel <= xp {
		t.Error("the kill gave the hirer nothing")
	}

	// killed, summoned again once its recast is over
	r.w.mu.Lock()
	r.p.AutoAttack = false
	ms.npc.Power = ms.npc.MaxPower
	pet.HP = 0
	r.w.killNPC(pet)
	cast := ms.ready[717]
	r.w.mu.Unlock()
	r.tick(40100 * time.Millisecond)
	if ms.pet != nil {
		t.Fatal("the dead pet is still the merc's")
	}
	at := 40200 * time.Millisecond
	for ; r.at(at).Before(cast) && at < 120*time.Second; at += 100 * time.Millisecond {
		r.tick(at)
		if ms.pet != nil {
			t.Fatalf("a new pet at %v, before the summoning's lockout ended at %v", at, cast.Sub(r.t0))
		}
	}
	r.run(at, at+12*time.Second)
	if ms.pet == nil || ms.pet.npc == pet || ms.pet.npc.Dead {
		t.Fatal("no new pet after the recast")
	}
	again := ms.pet.npc

	// it goes with its merc
	if _, ok := r.w.DismissMerc(r.sess, r.at(at+13*time.Second)); !ok {
		t.Fatal("no merc to dismiss")
	}
	if !again.Dead || ms.pet != nil {
		t.Errorf("after the dismissal the pet is dead %v, the merc's %v", again.Dead, ms.pet != nil)
	}

	// the control: a warrior's book has no summoning
	w := r.hire(r.kit(0, 0, 0), MercAssist)
	r.run(at+14*time.Second, at+30*time.Second)
	if w.pet != nil {
		t.Error("a warrior merc summoned a pet")
	}
}
