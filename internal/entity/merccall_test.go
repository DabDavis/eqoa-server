package entity

import (
	"testing"
	"time"
)

// GM ticket 1959 (merccall.go): a merc and its pet take on only what their
// owner called them to; a player's own pet is unchanged.

// choose runs the merc's and its pet's choice once, from none, and gives
// their targets.
func chooseOnce(r *mercRig, ms *mercState) (merc, pet EntityID) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ms.target = 0
	r.w.mercChooseTargetLocked(ms)
	if ms.pet != nil {
		ms.pet.target = 0
		r.w.mercPetChooseTargetLocked(ms, ms.pet)
		pet = ms.pet.target
	}
	return ms.target, pet
}

func bigCrab(r *mercRig, dx float32) *NPC {
	c := r.creature("crab", 28, dx)
	c.HP, c.MaxHP = 1_000_000, 1_000_000
	return c
}

// A crab fighting a group mate, a group mate's merc, or the owner himself (it
// aggroed on him: he is on its hate list) is left alone by the merc and its
// pet; the owner's auto-attack on it, or a hit (a harmful spell, a shot, a
// swing: updateAggro), calls them both onto it.
func TestMercWaitsForItsOwnersCall(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
	bs, b := r.player("Bea", 0, 0)
	g := r.group(b)
	k := r.kit(0, 0, 0)
	k.Name = "Kel"
	if _, why := r.w.HireMerc(bs, k, MercAssist, r.at(10*time.Second)); why != "" {
		t.Fatalf("Bea's hire: %s", why)
	}
	r.w.GroupAccept(bs, bs.PlayerObjectID())
	other := r.w.merc1(b.ServerID)
	if other == nil || !groupHas(g, other.npc.ServerID) {
		t.Fatalf("the group %v: Bea's merc in it", g.Members)
	}
	for i, victim := range []EntityID{other.npc.ServerID, b.ServerID, r.p.ServerID} {
		crab := bigCrab(r, float32(6+2*i))
		r.w.mu.Lock()
		r.w.addThreatLocked(crab, victim, 10) // it aggroed on the victim
		r.w.mu.Unlock()
		if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
			t.Errorf("a crab on %d, uncalled: the merc's target %d, its pet's %d", victim, m, p)
		}
		r.w.mu.Lock()
		crab.CurrentEnemy, crab.Threats = 0, nil
		r.w.mu.Unlock()
	}

	// the owner's auto-attack
	a := bigCrab(r, 6)
	r.w.mu.Lock()
	r.w.addThreatLocked(a, b.ServerID, 10)
	r.p.AttackTarget, r.p.AutoAttack = a.ServerID, true
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != a.ServerID || p != a.ServerID {
		t.Errorf("the owner's auto-attack: the merc's target %d, its pet's %d, want %d", m, p, a.ServerID)
	}
	// it holds once he stops swinging, while the crab still fights
	r.w.mu.Lock()
	r.p.AttackTarget, r.p.AutoAttack = 0, false
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != a.ServerID || p != a.ServerID {
		t.Errorf("after his swing: the merc's target %d, its pet's %d, want %d", m, p, a.ServerID)
	}
	r.w.mu.Lock()
	r.w.killNPC(a)
	r.w.mu.Unlock()
	if r.w.mercCalls[r.p.ServerID][a.ServerID] {
		t.Error("a dead crab's call is kept")
	}

	// his hit (a harmful spell's landing is updateAggro too)
	c := bigCrab(r, 8)
	r.w.mu.Lock()
	r.w.addThreatLocked(c, b.ServerID, 10)
	r.w.updateAggro(c, r.p.ServerID, 5)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != c.ServerID || p != c.ServerID {
		t.Errorf("the owner's hit: the merc's target %d, its pet's %d, want %d", m, p, c.ServerID)
	}
}

// An assist merc (the default) leaves a group mate's fight alone: Bea's
// auto-attack and hit call nothing of the owner's (GM ticket 2025 is the
// aggressive stance's: merccallgroup_test.go).
func TestAnAssistMercLeavesItsGroupsFights(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	_, b := r.player("Bea", 0, 0)
	r.group(b)
	crab := bigCrab(r, 6)
	r.w.mu.Lock()
	b.AttackTarget, b.AutoAttack = crab.ServerID, true
	r.w.updateAggro(crab, b.ServerID, 10) // Bea's own hit calls nothing of the owner's
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Errorf("an assist merc took on Bea's crab: merc %d, pet %d", m, p)
	}
}

// The owner's heal on a group mate a crab fights calls his merc and its pet
// onto that crab; the same heal on a mate no one fights calls nothing.
func TestAnOwnersHealCallsHisMercs(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	_, b := r.player("Bea", 0, 0)
	r.group(b)
	heal := mercHealSpell()
	land := func() {
		r.w.mu.Lock()
		hurtTo(b, 50)
		cs := &castState{spell: heal, caster: r.p.ServerID, targets: []EntityID{b.ServerID}}
		r.w.landOnPlayerLocked(cs, r.p, r.sess, b, r.at(11*time.Second))
		r.w.mu.Unlock()
	}
	land()
	if len(r.w.mercCalls[r.p.ServerID]) != 0 {
		t.Fatalf("a heal on a mate no one fights called %v", r.w.mercCalls[r.p.ServerID])
	}
	x := bigCrab(r, 6)
	y := bigCrab(r, -6) // fighting no one of theirs
	r.w.mu.Lock()
	r.w.addThreatLocked(x, b.ServerID, 10)
	r.w.addThreatLocked(y, 99999, 10)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != 0 {
		t.Fatalf("before the heal: merc %d, pet %d", m, p)
	}
	land()
	if m, p := chooseOnce(r, ms); m != x.ServerID || p != x.ServerID {
		t.Errorf("after the owner's heal on Bea: the merc's target %d, its pet's %d, want X %d", m, p, x.ServerID)
	}
	if r.w.mercCalls[r.p.ServerID][y.ServerID] {
		t.Error("the heal called a crab fighting no one healed")
	}
}

// Self-defence: a crab fighting the merc itself is fought back by the merc
// and its pet; one fighting the pet alone by the pet, not the merc.
func TestAMercFightsBackWhatHitsIt(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	crab := bigCrab(r, 6)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, ms.npc.ServerID, 10)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != crab.ServerID || p != crab.ServerID {
		t.Errorf("a crab on the merc: the merc's target %d, its pet's %d, want %d", m, p, crab.ServerID)
	}
	r.w.mu.Lock()
	crab.CurrentEnemy, crab.Threats = 0, nil
	r.w.addThreatLocked(crab, ms.pet.npc.ServerID, 10)
	r.w.mu.Unlock()
	if m, p := chooseOnce(r, ms); m != 0 || p != crab.ServerID {
		t.Errorf("a crab on the pet: the merc's target %d (want 0), its pet's %d (want %d)", m, p, crab.ServerID)
	}
}

// A healer merc's heals wait for its owner's call while a creature fights
// him: none while a crab beats on its owner untouched, then its heal once he
// hits it back; out of a fight it tops him up as before.
func TestAMercsHealsWaitForTheCall(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0, mercHealSpell()), MercAssist)
	r.run(0, time.Second)
	hurtTo(r.p, 50)
	crab := bigCrab(r, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 10)
	r.w.mu.Unlock()
	if c := r.firstMercCast(time.Second, 3*time.Second); c != nil {
		t.Fatalf("a heal before the owner's call: %q on %d", c.spell.Name, c.target)
	}
	r.w.mu.Lock()
	r.w.updateAggro(crab, r.p.ServerID, 5) // he hits back
	r.w.mu.Unlock()
	hurtTo(r.p, 50)
	if c := r.firstMercCast(3*time.Second, 5*time.Second); c == nil || c.target != r.p.ServerID {
		t.Fatalf("the owner's fight called: the merc's cast %+v, want its heal on him", c)
	}
	_ = ms

	// out of a fight: its top-up as before
	r2 := newMercRig(t)
	r2.hire(r2.kit(9, 0, 0, mercHealSpell()), MercAssist)
	r2.run(0, time.Second)
	hurtTo(r2.p, 50)
	if c := r2.firstMercCast(time.Second, 3*time.Second); c == nil || c.target != r2.p.ServerID {
		t.Fatalf("out of a fight: the merc's cast %+v, want its heal on its owner", c)
	}
}

// A player's own pet is unchanged: on defensive it takes on a crab that
// aggroed on its owner, uncalled; his merc beside it does not.
func TestAPlayersOwnPetStillDefends(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	pet := r.creature("Golem", 28, 2)
	pet.Owner = r.p.ServerID
	r.w.mu.Lock()
	if r.w.pets == nil {
		r.w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: r.p, npc: pet, stance: petDefensive}
	r.w.pets[r.p.ServerID] = ps
	r.w.mu.Unlock()
	crab := bigCrab(r, 6)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 10)
	r.w.petChooseTargetLocked(ps, r.at(time.Second))
	r.w.mu.Unlock()
	if ps.target != crab.ServerID {
		t.Errorf("the player's pet left the crab on its owner (target %d)", ps.target)
	}
	if m, _ := chooseOnce(r, ms); m != 0 {
		t.Errorf("the merc took on the uncalled crab %d", m)
	}
}
