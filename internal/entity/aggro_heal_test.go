package entity

import "testing"

// Petitions 1225, 1962 and 1967 (the GM Jibwiz): a heal that put back at least 1 point is
// hate on its healer of its WHOLE figure, as that much damage would be, on every creature
// fighting the one it healed, each whole; a heal on a full target is a fifth of it; a
// creature fighting someone else, or no one, hears nothing.
func TestAHealIsHateOnItsHealer(t *testing.T) {
	r := newProcRig(t)
	if r.w.SpatialIndex == nil {
		t.Skip("no spatial index in the rig")
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.SpatialIndex.Insert(r.npc.ServerID, r.npc.X, r.npc.Z)
	r.npc.Threats = map[EntityID]int64{r.friend.ServerID: 100000}
	r.npc.CurrentEnemy = r.friend.ServerID
	r.friend.HP = r.friend.MaxHPNow() - 1 // 1 short: the whole 1000 is hate
	r.w.healLocked(r.p, r.sess, r.friend, 1000, numberHeal)
	if got := r.npc.Threats[r.p.ServerID]; got != 1000 {
		t.Errorf("a 1000 heal restoring 1: hate %d, want 1000", got)
	}
	if r.npc.CurrentEnemy != r.friend.ServerID {
		t.Errorf("the creature turned to %d", r.npc.CurrentEnemy)
	}
	if r.friend.HP != r.friend.MaxHPNow() {
		t.Errorf("HP %d, want the most", r.friend.HP)
	}
	r.w.healLocked(r.p, r.sess, r.friend, 1000, numberHeal) // at the most: a fifth
	if got := r.npc.Threats[r.p.ServerID]; got != 1200 {
		t.Errorf("a 1000 heal at the most: hate %d, want 1200", got)
	}
	// a second creature on the same target: each hears the whole of it
	if r.w.NPCs == nil {
		t.Skip("no NPC map")
	}
	two := &NPC{ServerID: r.npc.ServerID + 7001, Name: "a second", World: r.npc.World, X: r.npc.X + 2, Z: r.npc.Z,
		HP: 100, MaxHP: 100, Level: r.npc.Level, Threats: map[EntityID]int64{r.friend.ServerID: 100000},
		CurrentEnemy: r.friend.ServerID}
	r.w.NPCs[two.ServerID] = two
	r.w.SpatialIndex.Insert(two.ServerID, two.X, two.Z)
	r.friend.HP = r.friend.MaxHPNow() - 500
	r.w.healLocked(r.p, r.sess, r.friend, 1000, numberHeal)
	if a, b := r.npc.Threats[r.p.ServerID], two.Threats[r.p.ServerID]; a != 2200 || b != 1000 {
		t.Errorf("two creatures: hate %d and %d, want 2200 and 1000", a, b)
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
// caster's: Worldbreaker's 25 on a wearer short of it is 25 on the creature fighting it (1967).
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
	if got := r.npc.Threats[r.friend.ServerID]; got != 1025 {
		t.Errorf("hate %d, want 1025", got)
	}
}
