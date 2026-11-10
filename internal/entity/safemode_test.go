package entity

import "testing"

// Petition 1424 (the GM Jibrish): /testinvis, a player's safe mode. Safe, a bear
// 10 off that attacks on sight leaves the player alone, and with the mode off
// again it comes; the GMs' #aggro flag is no part of it.
func TestSafeModeDrawsNoCreatureOnSight(t *testing.T) {
	f := newAggroFix(t, 20)
	bear := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	if !f.w.SetSafeMode(f.sess, true) {
		t.Fatal("not in the world")
	}
	if on, ok := f.w.SafeMode(f.sess); !on || !ok {
		t.Fatalf("SafeMode %v %v, want on", on, ok)
	}
	if ign, _, _ := f.w.AggroIgnored(f.sess); ign {
		t.Error("safe mode set the GMs' #aggro flag")
	}
	f.look()
	if f.attacks(bear) {
		t.Fatal("a bear attacked a player in safe mode on sight")
	}
	f.w.SetSafeMode(f.sess, false)
	f.look()
	if !f.attacks(bear) {
		t.Error("the bear did not attack once safe mode was off")
	}
}

// A heal of its own draws no free creature on a player in safe mode; the
// control, the same heal with the mode off, draws the green bear (as in
// TestAGreenCreatureComesWhenYouHealYourself).
func TestSafeModeHealDrawsNone(t *testing.T) {
	f := newAggroFix(t, 30)
	green := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	f.w.SetSafeMode(f.sess, true)
	f.p.HP = 500
	f.selfHeal(50)
	if f.attacks(green) {
		t.Fatal("a self-heal in safe mode drew the green bear")
	}
	f.w.SetSafeMode(f.sess, false)
	f.p.HP = 500
	f.selfHeal(50)
	if !f.attacks(green) {
		t.Error("the control: a self-heal with the mode off did not draw the green bear")
	}
}

// A creature's area spell leaves out a player in safe mode who is no part of
// its fight, and takes one on its hate list, or the one it fights, as anyone
// (Jibrish: "If they willfully engage in combat that still works as normal,
// and they can be killed"). The control: the player with the mode off is taken.
func TestSafeModeAreaTakesOnlyThoseInTheFight(t *testing.T) {
	f := newAggroFix(t, 60)
	boss := f.npc(2001, "Lady Vox", 60, bearBody, 1005, 1000)
	ally := &Player{ServerID: 1002, CharServerID: 78, Name: "Ally", Level: 60, HP: 1000, MaxHP: 1000, X: 1003, Y: 10, Z: 1000}
	f.w.Players[ally.ServerID] = ally
	f.w.SpatialIndex.Insert(ally.ServerID, ally.X, ally.Z)
	spell := &CastSpell{SpellID: 905000, Name: "Frost Breath", Area: SpellArea{Centre: AreaOnCaster, Radius: 30}}
	taken := func() bool {
		f.w.mu.Lock()
		defer f.w.mu.Unlock()
		for _, id := range f.w.templateAreaLocked(boss, spell, ally.ServerID) {
			if id == f.p.ServerID {
				return true
			}
		}
		return false
	}
	if !taken() {
		t.Fatal("the control: a player near, not in safe mode, left out of the area")
	}
	f.w.SetSafeMode(f.sess, true)
	if taken() {
		t.Error("a player in safe mode, no part of the fight, taken by the area")
	}
	boss.Threats = map[EntityID]int64{f.p.ServerID: 10}
	if !taken() {
		t.Error("a player in safe mode on the creature's hate list left out of its area")
	}
	boss.Threats, boss.CurrentEnemy = nil, f.p.ServerID
	if !taken() {
		t.Error("a player in safe mode the creature fights left out of its area")
	}
}
