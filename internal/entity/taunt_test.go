package entity

import (
	"testing"
	"time"
)

// Taunts put their caster at the top of a creature's hate (taunt.go; the owner, 2026-11-11, in a
// Cazic-Thule raid). Each rule beside its control. The lines are EQOA-Data's; the creatures, the
// players and their damage are made up.

// goad is the warrior's Goad as its line reads (954 + 16% of STR: 970 at the rig's STR 100).
func goad() CastSpell {
	s := spellOf(2775, 1, "Goad", "Target, Increase Hate by 954 ; Increase 16% based on STR")
	s.CastTime, s.Recast = 0, 5*time.Second
	return s
}

// goadHate is Goad's hate at the rig's STR 100, and the hate of 1 any harmful spell's landing gives
// (cast.go landOnNPCLocked)
const goadHate = 954 + 16 + 1

// tauntRig is the area rig's caster (the taunter, 1001) beside a raid boss that a damage dealer (1002)
// has hit for 100,000.
func tauntRig(t *testing.T) (*areaRig, *NPC, EntityID) {
	t.Helper()
	r := newAreaRig(t)
	boss := r.creature(2001, "Cazic-Thule", 1010, 1000)
	boss.HP, boss.MaxHP = 5_000_000, 5_000_000
	const dps EntityID = 1002
	r.player(dps, "Tmac", 1005, 1005)
	r.w.mu.Lock()
	r.w.updateAggro(boss, dps, 100_000)
	r.w.mu.Unlock()
	if boss.CurrentEnemy != dps {
		t.Fatalf("the damage dealer is not the boss's enemy: %d", boss.CurrentEnemy)
	}
	return r, boss, dps
}

// A taunt after big damage makes the taunter the enemy, at the top threat and its hate; a second
// taunt, already on top, adds its hate alone and keeps it; damage past top + hate takes it back.
func TestATauntTakesTheTop(t *testing.T) {
	r, boss, dps := tauntRig(t)
	me := r.p.ServerID
	r.cast(t, goad(), boss.ServerID)
	if boss.CurrentEnemy != me {
		t.Fatalf("after Goad the boss fights %d, want the taunter %d", boss.CurrentEnemy, me)
	}
	if got, want := boss.Threats[me], int64(100_000+goadHate); got != want {
		t.Errorf("the taunter's threat is %d, want the top (100000) and Goad's %d: %d", got, goadHate, want)
	}

	r.p.artReady = time.Time{} // its 0.5 s over (TestATauntLandsAtOnceThenHoldsHalfASecond)
	r.cast(t, goad(), boss.ServerID)
	if got, want := boss.Threats[me], int64(100_000+2*goadHate); got != want || boss.CurrentEnemy != me {
		t.Errorf("a second Goad on top: threat %d (want %d), enemy %d", got, want, boss.CurrentEnemy)
	}

	r.w.mu.Lock()
	r.w.updateAggro(boss, dps, int(boss.Threats[me]-boss.Threats[dps])-1) // one short of the taunter
	held := boss.CurrentEnemy
	r.w.updateAggro(boss, dps, 2)
	took := boss.CurrentEnemy
	r.w.mu.Unlock()
	if held != me || took != dps {
		t.Errorf("damage one short of the taunter: enemy %d (want %d); past it: %d (want %d)", held, me, took, dps)
	}
}

// The control: a hit whose damage outweighs its hate (Hero's Blow) adds its hate alone, as before, so
// the damage dealer keeps the boss; and isTaunt's reading of the records.
func TestAHitWithHateIsNoTaunt(t *testing.T) {
	r, boss, dps := tauntRig(t)
	blow := spellOf(700787, 1, "Hero's Blow", "Target, Increase Hate by 292 ; Increase 53% based on STR, Decrease Hitpoints by 514 ; Increase 50% based on STR")
	blow.CastTime = 0
	r.cast(t, blow, boss.ServerID)
	if boss.CurrentEnemy != dps || boss.Threats[r.p.ServerID] >= 100_000 {
		t.Errorf("after Hero's Blow the boss fights %d (want the damage dealer %d), the striker's threat %d",
			boss.CurrentEnemy, dps, boss.Threats[r.p.ServerID])
	}
	for _, c := range []struct {
		name, line string
		taunt      bool
	}{
		{"Taunt", "Target, Increase Hate by 100 ; Increase 10% based on STR", true},
		{"Pillar of Might", "Target, Increase AC by 500 to Self, Increase Hate by 795, 12sec", true},
		{"Goading Touch", "Target, Drain HP by 20 give self, Increase Hate by 954 ; Increase 16% based on STR", true},
		{"Goading Shout", "Target, Increase Hate by 191 ; Increase 16% based on STR, Increase Hate by 954 ; Increase 16% based on STR, AOE", true},
		{"Hero's Blow", "Target, Increase Hate by 292 ; Increase 53% based on STR, Decrease Hitpoints by 514 ; Increase 50% based on STR", false},
		{"Tongue Lash", "Target, Decrease Hitpoints by 800 ; Increase 200% based on DEX, Increase Hate by 400", false},
		{"Evade", "Target, Decrease Hate by 445", false},
		{"Fire Storm", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36", false},
	} {
		s := spellOf(1, 1, c.name, c.line)
		if got := isTaunt(&s); got != c.taunt {
			t.Errorf("isTaunt(%s) = %v, want %v", c.name, got, c.taunt)
		}
	}
	self := spellOf(1, 0, "Rage", "Self, Increase Hate by 100")
	if isTaunt(&self) {
		t.Error("a self spell is a taunt")
	}
}

// An AoE shout puts the taunter on top of each of the three creatures it reaches, whoever was on each.
func TestAShoutTakesTheTopOfEach(t *testing.T) {
	r := newAreaRig(t)
	me := r.p.ServerID
	r.player(1002, "Tmac", 1005, 1005)
	r.player(1003, "Kenny", 1005, 995)
	var npcs []*NPC
	for i, x := range []float32{1004, 1006, 1003} {
		n := r.creature(EntityID(2001+i), "Deathfist Pawn", x, 1002)
		n.HP, n.MaxHP = 500_000, 500_000
		npcs = append(npcs, n)
	}
	r.w.mu.Lock()
	r.w.updateAggro(npcs[0], 1002, 50_000)
	r.w.updateAggro(npcs[1], 1003, 80_000)
	r.w.updateAggro(npcs[2], 1002, 30_000)
	r.w.updateAggro(npcs[2], 1003, 40_000)
	r.w.mu.Unlock()
	shout := spellOf(701079, 1, "Goading Shout", "Target, Increase Hate by 191 ; Increase 16% based on STR, Increase Hate by 954 ; Increase 16% based on STR, AOE")
	shout.CastTime = 0
	r.cast(t, shout, npcs[0].ServerID)
	for i, n := range npcs {
		var top int64
		for id, v := range n.Threats {
			if id != me && v > top {
				top = v
			}
		}
		if n.CurrentEnemy != me || n.Threats[me] <= top {
			t.Errorf("creature %d after the shout: enemy %d, taunter's threat %d against the top %d", i, n.CurrentEnemy, n.Threats[me], top)
		}
	}
}

// A resisted taunt changes nothing but the hate of 1 any resisted spell gives; the control, the same
// taunt not resisted, takes the top.
func TestAResistedTauntChangesNothing(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	wb := func() CastSpell {
		s := goad()
		s.Def = SpellDef{Harmful: true, Target: TargetSingle, Workbook: true,
			Effects: []SpellEffect{{Kind: EffectHate, Amount: 954, AmountMax: 954, Resist: StatAR, ID: "083C"}}}
		return s
	}
	for _, resisted := range []bool{true, false} {
		r, boss, dps := tauntRig(t)
		boss.Level = 65 // above the caster: a resist chance to roll under
		if resisted {
			procIntn = dice(0)
		} else {
			procIntn = dice(99)
		}
		r.cast(t, wb(), boss.ServerID)
		me := r.p.ServerID
		switch {
		case resisted && (boss.CurrentEnemy != dps || boss.Threats[me] != 1):
			t.Errorf("a resisted taunt: enemy %d (want %d), the taunter's threat %d (want 1)", boss.CurrentEnemy, dps, boss.Threats[me])
		case !resisted && (boss.CurrentEnemy != me || boss.Threats[me] != 100_000+954+1):
			t.Errorf("the taunt not resisted: enemy %d (want %d), threat %d (want %d)", boss.CurrentEnemy, me, boss.Threats[me], 100_000+954+1)
		}
	}
}

// A dummy's hate is its own (dummy.go): a taunt on one keeps no threat list, as before; a preferred
// foe (prefer.go) stays its creature's enemy through a taunt; an offensive proc's hate is added as a
// hit's.
func TestTauntsLeaveDummiesPreferredFoesAndProcs(t *testing.T) {
	r := newAreaRig(t)
	d := r.creature(2001, "Sparring Dummy", 1010, 1000)
	d.Dummy = RegularDummy
	r.cast(t, goad(), d.ServerID)
	if len(d.Threats) != 0 {
		t.Errorf("a taunt on a dummy left threats %v", d.Threats)
	}

	r, boss, dps := tauntRig(t)
	boss.tmpl = &ClassTemplate{prefer: []string{"tmac"}}
	r.cast(t, goad(), boss.ServerID)
	if boss.CurrentEnemy != dps {
		t.Errorf("a boss that fights Tmac first fights %d after a taunt, want %d", boss.CurrentEnemy, dps)
	}

	r, boss, dps = tauntRig(t)
	proc := goad()
	proc.figures = figuresProc
	cs := &castState{spell: proc, caster: r.p.ServerID}
	r.w.mu.Lock()
	r.w.landOnNPCLocked(cs, r.p, r.sess, boss, r.t0)
	r.w.mu.Unlock()
	if boss.CurrentEnemy != dps || boss.Threats[r.p.ServerID] >= 100_000 {
		t.Errorf("a proc's hate took the boss: enemy %d, threat %d", boss.CurrentEnemy, boss.Threats[r.p.ServerID])
	}
}

// A taunt lands at once and holds the next ability 0.5 s (board #575, petition 1434): its hate is on
// the creature the first tick after its cast, an art 0.4 s after it is not answered, one 0.5 s after it
// goes; the control, an art that is no taunt, still holds the next art the arts' 1.5 s, and a spell
// that is no taunt still lands castLand after its cast time.
func TestATauntLandsAtOnceThenHoldsHalfASecond(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	c.w.balance = SeedBalance()
	taunt := artOf(803, "Goad")
	taunt.Def = ParseSpellText("Target, Increase Hate by 954")
	if !isTaunt(&taunt) || !isArt(&taunt) {
		t.Fatal("Goad is not read as a taunt and an art")
	}
	if !c.w.StartCast(c.sess, taunt, c.npcO, c.at(0)) {
		t.Fatal("the taunt was not begun")
	}
	c.tick(simTick)
	if c.npc.Threats[c.p.ServerID] < 954 || c.npc.CurrentEnemy != c.p.ServerID {
		t.Fatalf("a tick after the taunt the creature's hate is %v, enemy %d", c.npc.Threats, c.npc.CurrentEnemy)
	}
	kick := artOf(801, "Kick")
	if c.w.StartCast(c.sess, kick, c.npcO, c.at(400*time.Millisecond)) {
		t.Error("an art 0.4 s after a taunt began")
	}
	if !c.w.StartCast(c.sess, kick, c.npcO, c.at(tauntLockout)) {
		t.Error("an art 0.5 s after a taunt was held")
	}
	if c.w.StartCast(c.sess, artOf(802, "Bash"), c.npcO, c.at(tauntLockout+time.Second)) {
		t.Error("an art 1 s after an art that is no taunt began")
	}

	c = newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(0)) {
		t.Fatal("the strike was not begun")
	}
	c.tick(strike().CastTime + castLand - simTick)
	if c.npc.HP != c.npc.MaxHP {
		t.Error("a spell that is no taunt landed before castLand")
	}
}

// A tank's taunt gives 5% more threat than another player's (petition 1935): the raise to the top and
// its hate each at 105%; a wizard's (the rig's) as before (TestATauntTakesTheTop).
func TestATanksTauntGivesFivePercentMore(t *testing.T) {
	for _, c := range []struct {
		class int
		want  int64
	}{
		{13, 100_000 + goadHate},            // a wizard's: the top and Goad's
		{0, 105_000 + (970*105+50)/100 + 1}, // a warrior's: 105% of the raise and of Goad's
		{3, 105_000 + (970*105+50)/100 + 1}, // a shadow knight's
		{2, 105_000 + (970*105+50)/100 + 1}, // a paladin's
	} {
		r, boss, _ := tauntRig(t)
		r.p.Class = c.class
		r.cast(t, goad(), boss.ServerID)
		if got := boss.Threats[r.p.ServerID]; got != c.want || boss.CurrentEnemy != r.p.ServerID {
			t.Errorf("class %d's Goad: threat %d (want %d), enemy %d", c.class, got, c.want, boss.CurrentEnemy)
		}
	}
	if pctOf(-970, 105) != -1019 || pctOf(970, 70) != 679 || pctOf(5, 100) != 5 {
		t.Errorf("pctOf: %d %d %d", pctOf(-970, 105), pctOf(970, 70), pctOf(5, 100))
	}
}
