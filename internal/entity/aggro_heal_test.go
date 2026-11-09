package entity

import "testing"

// Petition 1225: a heal is hate on its healer from the creatures fighting the one it
// healed, half of what it restored, shared; what went past the most is none, and a
// creature fighting someone else, or no one, hears nothing.
func TestAHealIsHateOnItsHealer(t *testing.T) {
	r := newProcRig(t)
	if r.w.SpatialIndex == nil {
		t.Skip("no spatial index in the rig")
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.SpatialIndex.Insert(r.npc.ServerID, r.npc.X, r.npc.Z)
	r.npc.Threats = map[EntityID]int64{r.friend.ServerID: 1000}
	r.npc.CurrentEnemy = r.friend.ServerID
	r.friend.HP = r.friend.MaxHPNow() - 300
	r.w.healLocked(r.p, r.sess, r.friend, 500, numberHeal)
	if got := r.npc.Threats[r.p.ServerID]; got != 150 {
		t.Errorf("a heal restoring 300: hate %d, want 150", got)
	}
	if r.npc.CurrentEnemy != r.friend.ServerID {
		t.Errorf("the creature turned to %d", r.npc.CurrentEnemy)
	}
	r.w.healLocked(r.p, r.sess, r.friend, 500, numberHeal) // at the most: nothing restored
	if got := r.npc.Threats[r.p.ServerID]; got != 150 {
		t.Errorf("a heal at the most: hate %d, want 150 still", got)
	}
	// a creature fighting someone else
	r.npc.Threats = map[EntityID]int64{EntityID(77777): 10}
	r.npc.CurrentEnemy = 77777
	r.friend.HP = r.friend.MaxHPNow() - 300
	r.w.healLocked(r.p, r.sess, r.friend, 500, numberHeal)
	if got := r.npc.Threats[r.p.ServerID]; got != 0 {
		t.Errorf("a creature fighting another: hate %d, want 0", got)
	}
}

// The user, 2026-11-07: a worn item's heal is its wearer's hate, as a spell's is its
// caster's: Worldbreaker's 25 on a wearer short of it is 12 on the creature fighting it.
func TestAWornHealIsItsWearersHate(t *testing.T) {
	r := newProcRig(t)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.SpatialIndex.Insert(r.npc.ServerID, r.npc.X, r.npc.Z)
	s, ok := WornShieldOf(0, "worldbreaker chestguard")
	if !ok {
		t.Fatal("no Worldbreaker")
	}
	r.friend.Gear.Shields = []WornShield{s}
	r.npc.Threats = map[EntityID]int64{r.friend.ServerID: 1000}
	r.npc.CurrentEnemy = r.friend.ServerID
	r.friend.HP = r.friend.MaxHPNow() - 100
	r.w.wornShieldsLocked(r.fsess, r.friend, r.npc)
	if got := r.npc.Threats[r.friend.ServerID]; got != 1012 {
		t.Errorf("hate %d, want 1012", got)
	}
}
