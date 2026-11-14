package entity

import "testing"

// The owner, 2026-11-14: "pet taunts may not be working strongly enough". Beside a caster
// with 10,000 threat, a pet's taunt (its hit on no dice: 0 damage) puts the pet a margin over
// the top: 11,000 (10%), and a level 1 pet over a low top its 20 a level; the caster's next
// heal of 500 hate leaves the creature on the pet (a top+1 taunt lost it to any hate). A
// player tank's taunt (Goad's 954) takes the creature back from the pet.
func TestAPetsTauntTakesAndHoldsTheCreature(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return 0 }
	t.Cleanup(func() { meleeIntn = old })
	const caster, tank EntityID = 7777, 8888
	r := newTagRig(t)
	r.npc.HP, r.npc.MaxHP = 1_000_000, 1_000_000
	r.npc.Threats = map[EntityID]int64{caster: 10_000}
	r.npc.CurrentEnemy = caster
	r.ps.enchanter = false
	r.w.mu.Lock()
	r.w.petTopHateLocked(r.pet, r.npc)
	r.w.mu.Unlock()
	if got := r.npc.Threats[r.pet.ServerID]; got != 11_000 || r.npc.CurrentEnemy != r.pet.ServerID {
		t.Fatalf("the pet's taunt: threat %d, enemy %d: want 11000 and the pet", got, r.npc.CurrentEnemy)
	}
	r.w.mu.Lock()
	r.npc.Threats[caster] += 500 // the caster's heal
	if e := r.w.pickEnemyLocked(r.npc); e != r.pet.ServerID {
		t.Errorf("the caster's heal took the creature from the pet (%d)", e)
	}
	r.w.tauntHateLocked(r.npc, tank, 954, playerTauntHatePct(0))
	r.w.mu.Unlock()
	if r.npc.Threats[tank] <= r.npc.Threats[r.pet.ServerID] || r.npc.CurrentEnemy != tank {
		t.Errorf("a tank's taunt did not take it back: tank %d, pet %d, enemy %d", r.npc.Threats[tank], r.npc.Threats[r.pet.ServerID], r.npc.CurrentEnemy)
	}

	if m := petTauntMargin(&NPC{Level: 1}, 50); m != 20 {
		t.Errorf("a level 1 pet's margin over 50: %d, want 20", m)
	}
}
