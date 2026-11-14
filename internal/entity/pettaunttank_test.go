package entity

import "testing"

// Live petition 5055 (Innoruuk, a level 17 shadow knight): a magician's level 16 earth pet's taunt
// puts it 320 over the top (20 a level); a shadow knight's Taunt of ~120 hate (x1.05) put him a
// mere 142 over the pet (its raise to the top at 105% too), which its melee passed long before its next taunt. Now a player's taunt
// is at least the margin a pet of his level gets (pettaunttank.go): 340 at 17, 357 at his 105%, 373 in all.
func TestAShadowKnightsTauntTakesTheCreatureFromAnEarthPet(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return 0 }
	t.Cleanup(func() { meleeIntn = old })
	const sk EntityID = 8888
	for _, c := range []struct {
		level int
		lead  int64
	}{{0, 142}, {17, 373}} {
		r := newTagRig(t)
		r.pet.Level, r.ps.earth = 16, true
		r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
		r.npc.Threats = map[EntityID]int64{sk: 600}
		r.npc.CurrentEnemy = sk
		r.w.mu.Lock()
		r.w.petTopHateLocked(r.pet, r.npc) // the pet's taunt: 600 + 320
		r.w.tauntHateLocked(r.npc, sk, 120, playerTauntHatePct(3), c.level)
		r.w.mu.Unlock()
		lead := r.npc.Threats[sk] - r.npc.Threats[r.pet.ServerID]
		if r.npc.CurrentEnemy != sk || lead != c.lead {
			t.Errorf("level %d: the knight %d, the pet %d (lead %d, want %d), enemy %d", c.level, r.npc.Threats[sk],
				r.npc.Threats[r.pet.ServerID], lead, c.lead, r.npc.CurrentEnemy)
		}
	}
	if f := playerTauntFloor(954, 17, 1000); f != 954 {
		t.Errorf("a taunt of more hate than the margin: %d, want its own 954", f)
	}
	if f := playerTauntFloor(120, 17, 10_000); f != 1000 {
		t.Errorf("over a top of 10000: %d, want 10%%, 1000", f)
	}
}

// A pet does not taunt a creature off a player tank of its owner's group (pettaunttank.go): it
// leaves the tank on top. It still taunts one on its own owner, and one on a groupmate who is no
// tank, and one on a tank of no group of its owner's.
func TestAPetLeavesAGroupedTanksCreature(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return 0 }
	t.Cleanup(func() { meleeIntn = old })
	for _, c := range []struct {
		name       string
		on         func(r *tagRig) EntityID
		class      int
		group      uint32
		wantTaunts bool
	}{
		{"a grouped shadow knight", func(r *tagRig) EntityID { return r.friend.ServerID }, 3, 7, false},
		{"a grouped warrior", func(r *tagRig) EntityID { return r.friend.ServerID }, 0, 7, false},
		{"a grouped shaman", func(r *tagRig) EntityID { return r.friend.ServerID }, 8, 7, true},
		{"a knight of another group", func(r *tagRig) EntityID { return r.friend.ServerID }, 3, 9, true},
		{"its own owner", func(r *tagRig) EntityID { return r.p.ServerID }, 3, 7, true},
	} {
		r := newTagRig(t)
		r.p.GroupID, r.friend.GroupID, r.friend.Class = 7, c.group, c.class
		on := c.on(r)
		r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
		r.npc.Threats = map[EntityID]int64{on: 1000}
		r.npc.CurrentEnemy = on
		r.w.mu.Lock()
		r.w.petTauntLocked(r.ps, r.npc, r.t0)
		r.w.mu.Unlock()
		taunted := r.npc.CurrentEnemy == r.pet.ServerID
		if taunted != c.wantTaunts {
			t.Errorf("%s: the pet taunted %v (threat %d, enemy %d), want %v", c.name, taunted,
				r.npc.Threats[r.pet.ServerID], r.npc.CurrentEnemy, c.wantTaunts)
		}
	}
}
