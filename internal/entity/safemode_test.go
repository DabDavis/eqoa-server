package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Petition 1424: /testinvis, a player's safe mode. Safe, a bear
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
// (a GM: "If they willfully engage in combat that still works as normal,
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

// The owner, 2026-11-26: Garrak, a sub-GM in Safe Mode (#aggro off), melees
// the dreadnaught and never took its Dread Curse or its damage over time. A
// GM in Safe Mode on the creature's hate list (meleeing it, not its target)
// is taken down to 700 and ticked as anyone; so is one it fights; a GM in
// Safe Mode standing by, no part of the fight, is left alone. The DoT ticks
// on after the landing. The control: the bystander out of Safe Mode is taken.
func TestGMSafeModeInTheFightTakesTheSetHPAndItsDoT(t *testing.T) {
	r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 700, Radius: 30, Every: 30,
		Name: "Dread Curse", Tick: 650})
	garrak, gsess := r.player(1002, "Garrak", 5000, 1005, 1005)
	by, bsess := r.player(1003, "Bystander", 5000, 1005, 995)
	for _, s := range []*session.Session{r.sess, gsess, bsess} {
		if !r.w.SetAggroIgnored(s, true) {
			t.Fatal("not in the world")
		}
	}
	r.boss.Threats[garrak.ServerID] = 50 // his melee's hate; the boss fights r.p
	r.at(0)
	if garrak.HP != 50 || countDoT(garrak.Effects) != 1 {
		t.Fatalf("Garrak, on its hate list in Safe Mode: HP %d (want 50), %d DoT (want 1)", garrak.HP, countDoT(garrak.Effects))
	}
	if r.p.HP != 50 || countDoT(r.p.Effects) != 1 {
		t.Fatalf("the one it fights, in Safe Mode: HP %d (want 50), %d DoT (want 1)", r.p.HP, countDoT(r.p.Effects))
	}
	if by.HP != 5000 || countDoT(by.Effects) != 0 {
		t.Fatalf("a GM in Safe Mode standing by: HP %d, %d DoT; want untouched", by.HP, countDoT(by.Effects))
	}
	garrak.HP = 5000
	r.w.mu.Lock()
	for s := 1; s <= 6; s++ {
		r.w.effectSystem(r.now.Add(time.Duration(s) * time.Second))
	}
	r.w.mu.Unlock()
	if garrak.HP >= 5000 {
		t.Fatalf("its damage over time did not tick on Garrak after the landing: HP %d", garrak.HP)
	}
	r.w.SetAggroIgnored(bsess, false)
	r.at(30 * time.Second)
	if by.HP != 50 {
		t.Fatalf("the control: the bystander out of Safe Mode, HP %d, want 50", by.HP)
	}
}

// The same rule for a templated creature's area spell (a breath, an area
// nuke): a GM in Safe Mode is left out of it while no part of its fight, and
// taken once on its hate list or its target.
func TestGMSafeModeAreaTakesOnlyThoseInTheFight(t *testing.T) {
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
		t.Fatal("the control: a player near, not in Safe Mode, left out of the area")
	}
	f.w.SetAggroIgnored(f.sess, true)
	if taken() {
		t.Error("a GM in Safe Mode, no part of the fight, taken by the area")
	}
	boss.Threats = map[EntityID]int64{f.p.ServerID: 10}
	if !taken() {
		t.Error("a GM in Safe Mode on the creature's hate list left out of its area")
	}
	boss.Threats, boss.CurrentEnemy = nil, f.p.ServerID
	if !taken() {
		t.Error("a GM in Safe Mode the creature fights left out of its area")
	}
}

// Safe Mode still keeps creatures off on sight, while in a fight with
// another: a GM in Safe Mode on the dreadnaught's hate list walks past a bear
// that attacks on sight, and a self-heal draws no green bear. Out of Safe
// Mode the bear comes.
func TestGMSafeModeInAFightStillDrawsNoneOnSight(t *testing.T) {
	f := newAggroFix(t, 20)
	boss := f.npc(2002, "dreadnaught", 55, creationBody, 1020, 1000)
	boss.CurrentEnemy = f.p.ServerID
	boss.Threats = map[EntityID]int64{f.p.ServerID: 10}
	bear := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
	f.w.SetAggroIgnored(f.sess, true)
	f.look()
	if f.attacks(bear) {
		t.Fatal("a bear attacked a GM in Safe Mode on sight")
	}
	f.p.HP = 500
	f.selfHeal(50)
	if f.attacks(bear) {
		t.Fatal("a self-heal in Safe Mode drew the bear")
	}
	f.w.SetAggroIgnored(f.sess, false)
	f.look()
	if !f.attacks(bear) {
		t.Error("the control: the bear did not attack once Safe Mode was off")
	}
}
