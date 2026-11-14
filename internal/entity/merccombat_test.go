package entity

import (
	"testing"
	"time"
)

// Run the normal mercenary system with a controlled combat state and clock.
func (r *mercRig) mercTick(d time.Duration) {
	r.t.Helper()
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.mercSystem(r.at(d))
}

func TestMercAutomaticCombatWait(t *testing.T) {
	for _, stance := range []MercStance{0, MercAssist, MercAggressive} {
		t.Run(MercStanceNames[stance], func(t *testing.T) {
			r := newMercRig(t)
			nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
			ms := r.hire(r.kit(13, 0, 0, nuke), stance)
			crab := r.creature("Crab", 28, 10)
			r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
			for _, d := range []time.Duration{0, time.Second - time.Nanosecond} {
				r.mercTick(d)
				if len(ms.ready) != 0 || ms.npc.Power != ms.npc.MaxPower {
					t.Fatalf("automatic cast began at %v", d)
				}
			}
			r.mercTick(time.Second)
			if ms.ready[nuke.SpellID].IsZero() {
				t.Fatal("automatic cast did not begin after one second")
			}
		})
	}
}

func TestMercCombatWaitSurvivesTargetChanges(t *testing.T) {
	r := newMercRig(t)
	nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
	ms := r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	first := r.creature("First crab", 28, 10)
	second := r.creature("Second crab", 28, 12)
	r.p.AttackTarget, r.p.AutoAttack = first.ServerID, true
	r.mercTick(0)
	deadline := ms.combatReady
	r.p.AttackTarget = second.ServerID
	r.mercTick(500 * time.Millisecond)
	r.mercTick(time.Second)
	if !ms.combatReady.Equal(deadline) || ms.ready[nuke.SpellID].IsZero() || ms.target != second.ServerID {
		t.Fatal("switching targets during the opening wait restarted it")
	}

	// Another mob after the first one dies, with the player still attacking.
	second.Dead = true
	r.p.AttackTarget = first.ServerID
	r.mercTick(10 * time.Second)
	if !ms.combatReady.Equal(deadline) || !ms.ready[nuke.SpellID].After(r.at(10*time.Second)) {
		t.Fatal("a new mob in continuous combat incurred another wait")
	}

	// The merc has no target, but a distant living mob still has the player
	// on its hate list while targeting someone else.
	first.Dead = true
	r.p.AutoAttack = false
	r.p.disengage()
	distant := r.creature("Distant crab", 28, petLeash+100)
	distant.Threats = map[EntityID]int64{r.p.ServerID: 10}
	r.mercTick(20 * time.Second)
	if ms.target != 0 || !ms.combatReady.Equal(deadline) {
		t.Fatal("a gap in merc targets reset the wait during the player's fight")
	}
	third := r.creature("Third crab", 28, 10)
	r.p.AttackTarget, r.p.AutoAttack = third.ServerID, true
	r.mercTick(21 * time.Second)
	if !ms.ready[nuke.SpellID].After(r.at(21 * time.Second)) {
		t.Fatal("rejoining the ongoing fight incurred another wait")
	}

	// Once every fight ends, the next pull gets a fresh second.
	third.Dead, distant.Dead = true, true
	r.p.AutoAttack = false
	r.p.disengage()
	r.mercTick(30 * time.Second)
	if !ms.combatReady.IsZero() {
		t.Fatal("combat wait stayed armed after combat ended")
	}
	fourth := r.creature("Fourth crab", 28, 10)
	r.p.AttackTarget, r.p.AutoAttack = fourth.ServerID, true
	previousCast := ms.ready[nuke.SpellID]
	r.mercTick(31 * time.Second)
	r.mercTick(32*time.Second - time.Nanosecond)
	if !ms.ready[nuke.SpellID].Equal(previousCast) {
		t.Fatal("a new fight skipped the opening wait")
	}
	r.mercTick(32 * time.Second)
	if !ms.ready[nuke.SpellID].After(previousCast) {
		t.Fatal("a new fight did not begin after its opening wait")
	}
}

func TestMercCombatWaitSurvivesStanceChanges(t *testing.T) {
	for _, stance := range []MercStance{MercAssist, MercAggressive} {
		t.Run(MercStanceNames[stance], func(t *testing.T) {
			r := newMercRig(t)
			nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
			ms := r.hire(r.kit(13, 0, 0, nuke), MercPassive)
			crab := r.creature("Crab", 28, 10)
			r.w.updateAggro(crab, r.p.ServerID, 10)
			r.mercTick(0)
			r.w.SetMercStance(r.sess, 0, stance)
			r.mercTick(500 * time.Millisecond)
			if len(ms.ready) != 0 {
				t.Fatal("changing stance bypassed the opening wait")
			}
			r.mercTick(time.Second)
			if ms.ready[nuke.SpellID].IsZero() {
				t.Fatal("changing stance restarted the opening wait")
			}
			r.w.SetMercStance(r.sess, 0, MercPassive)
			r.mercTick(10 * time.Second)
			if ms.target != 0 {
				t.Fatal("a passive merc acquired a target automatically")
			}
			r.w.SetMercStance(r.sess, 0, stance)
			r.mercTick(11 * time.Second)
			if !ms.ready[nuke.SpellID].After(r.at(11 * time.Second)) {
				t.Fatal("changing stance in ongoing combat incurred another wait")
			}
		})
	}
}

func TestMercCombatWaitCancelsAnAbandonedPull(t *testing.T) {
	r := newMercRig(t)
	nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
	ms := r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	crab := r.creature("Crab", 28, 10)
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.mercTick(0)
	r.p.AutoAttack = false
	r.p.disengage()
	r.mercTick(500 * time.Millisecond)
	r.mercTick(2 * time.Second)
	if ms.target != 0 || !ms.combatReady.IsZero() || len(ms.ready) != 0 {
		t.Fatal("the merc kept the abandoned pull through the wait")
	}
}

func TestMercCombatWaitSurvivesCatchingUpToItsOwner(t *testing.T) {
	r := newMercRig(t)
	nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
	ms := r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	crab := r.creature("Crab", 28, 10)
	crab.Threats = map[EntityID]int64{r.p.ServerID: 10}
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.mercTick(0)
	r.mercTick(time.Second)
	deadline := ms.combatReady
	ms.npc.X = r.p.X + petLost + 100
	r.mercTick(10 * time.Second)
	if !ms.combatReady.Equal(deadline) || !ms.ready[nuke.SpellID].After(r.at(10*time.Second)) {
		t.Fatal("catching up to its owner restarted the wait during combat")
	}
}

func TestMercCombatWaitHoldsMeleeAndTaunts(t *testing.T) {
	r := newMercRig(t)
	taunt := bookSpell(951, "Taunt", 1, 0, 0, 0, "Target, Increase Hate by 500")
	ms := r.hire(r.kit(0, 0, 0, taunt), MercAssist)
	crab := r.creature("Crab", 28, 1)
	crab.CurrentEnemy = r.p.ServerID
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.mercTick(0)
	r.mercTick(time.Second - time.Nanosecond)
	if !ms.npc.RoundStart.IsZero() || len(ms.ready) != 0 || crab.CurrentEnemy != r.p.ServerID {
		t.Fatal("the tank swung or taunted during the opening wait")
	}
	r.mercTick(time.Second)
	if ms.ready[taunt.SpellID].IsZero() {
		t.Fatal("the tank did not taunt after the opening wait")
	}
}

func TestMercCombatWaitHoldsAutomaticHealsInEveryStance(t *testing.T) {
	for _, stance := range []MercStance{MercPassive, MercAssist, MercAggressive} {
		t.Run(MercStanceNames[stance], func(t *testing.T) {
			r := newMercRig(t)
			heal := bookSpell(952, "Heal", 1, 0, 0, 40, "Target, Increase Hitpoints by 300")
			ms := r.hire(r.kit(9, 0, 0, heal), stance)
			crab := r.creature("Crab", 28, 10)
			r.w.updateAggro(crab, r.p.ServerID, 10)
			r.p.HP = r.p.MaxHPNow() / 4
			r.mercTick(0)
			r.mercTick(time.Second - time.Nanosecond)
			if len(ms.ready) != 0 {
				t.Fatal("the healer entered combat during the opening wait")
			}
			r.mercTick(time.Second)
			if ms.ready[heal.SpellID].IsZero() {
				t.Fatal("the healer did not act after the opening wait")
			}
		})
	}
}

func TestMercCombatWaitAllowsExplicitAttack(t *testing.T) {
	for _, stance := range []MercStance{MercPassive, MercAssist, MercAggressive} {
		t.Run(MercStanceNames[stance], func(t *testing.T) {
			r := newMercRig(t)
			nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 300")
			ms := r.hire(r.kit(13, 0, 0, nuke), stance)
			crab := r.creature("Crab", 28, 10)
			r.w.mu.Lock()
			r.w.visibilitySystem()
			r.p.TargetObjectID = r.w.objectIDForEntityLocked(r.sess, crab.ServerID)
			r.w.mu.Unlock()
			if _, refusal := r.w.MercAttack(r.sess, 0); refusal != "" {
				t.Fatal(refusal)
			}
			r.mercTick(0)
			if ms.ready[nuke.SpellID].IsZero() {
				t.Fatal("an explicit attack incurred the automatic entry wait")
			}
		})
	}
}

func TestMercCombatWaitHoldsItsPet(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	crab := r.creature("Crab", 28, 10)
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.mercTick(11 * time.Second)
	r.mercTick(12*time.Second - time.Nanosecond)
	if ms.pet.target != 0 || !ms.pet.npc.RoundStart.IsZero() {
		t.Fatal("the merc's pet entered combat during the opening wait")
	}
	r.mercTick(12 * time.Second)
	if ms.pet.target != crab.ServerID {
		t.Fatal("the merc's pet did not join after the opening wait")
	}
}
