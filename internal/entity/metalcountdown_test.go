package entity

import (
	"fmt"
	"testing"
	"time"
)

// Petition 1623 (the GM Jibrish): from his rising Metal Moogie is held for MetalCountdown seconds, the
// challenger told "Fight starts in 5", "4", "3", "2", "1" a second apart on the world's tick, then
// "FIGHT!", at which he goes for the challenger. During the count no one hurts him: the challenger's
// swing is not engaged (told to wait), her hit, nuke, area and pet land nothing, Bert is refused as
// before; he holds no hate, notices no one and hits no one. After FIGHT the duel is as it was: he is
// after Anna and hits her, and her swing engages and lands.
func TestMetalMoogieCountsDownToTheFight(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1029)
	b, sb := r.player("Bert", 1004, 1026)
	a.HP, a.MaxHP = 2_000_000, 2_000_000
	_, ok, c := r.rise(sa, 0)
	if !ok || c == nil {
		t.Fatal("no Metal Moogie")
	}
	if !c.duelHeld() || c.CurrentEnemy != 0 || len(c.Threats) != 0 {
		t.Fatalf("at his rising: held %v, enemy %d, threats %v; want held, no one", c.duelHeld(), c.CurrentEnemy, c.Threats)
	}
	oa, ob := r.see(sa, c.ServerID), r.see(sb, c.ServerID)
	drain(sa)
	drain(sb)

	want := map[time.Duration]string{
		0:               fmt.Sprintf(MetalLineCount, 5),
		1 * time.Second: "4",
		2 * time.Second: "3",
		3 * time.Second: "2",
		4 * time.Second: "1",
		5 * time.Second: MetalLineFight,
	}
	swing := Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}
	nuke := CastSpell{SpellID: 9301, Name: "Test Bolt", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	storm := CastSpell{SpellID: 9303, Name: "Test Storm", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 100")}
	storm.Area.Centre = AreaOnTarget
	petA := &NPC{ServerID: 9002, Name: "Anna's pet", Owner: a.ServerID, World: 0}
	var got []string
	for d := time.Duration(0); d < 5*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		lines := linesOf(drain(sa))
		got = append(got, lines...)
		if w, ok := want[d]; ok {
			if len(lines) != 1 || lines[0] != w {
				t.Errorf("at %v Anna told %q, want %q", d, lines, w)
			}
		} else if len(lines) != 0 {
			t.Errorf("at %v Anna told %q, want nothing", d, lines)
		}
		if l := linesOf(drain(sb)); len(l) != 0 {
			t.Errorf("at %v Bert told %q", d, l)
		}
		if d != 2500*time.Millisecond {
			continue
		}
		// mid-count: no one may hurt him, Anna included
		if r.w.EngageAttack(sa, oa, swing, r.at(d)) || a.AttackTarget != 0 {
			t.Error("Anna engaged him before FIGHT")
		}
		if l := linesOf(drain(sa)); len(l) != 1 || l[0] != MetalLineWait {
			t.Errorf("Anna's early swing told %q, want %q", l, MetalLineWait)
		}
		if r.w.EngageAttack(sb, ob, swing, r.at(d)) {
			t.Error("Bert engaged him")
		}
		if l := linesOf(drain(sb)); len(l) != 1 || l[0] != fmt.Sprintf(MetalLineNotYour, "Anna") {
			t.Errorf("Bert told %q", l)
		}
		r.w.mu.Lock()
		hp := c.HP
		r.w.playerHitLocked(sa, a, c, oa)
		r.w.landOnNPCLocked(&castState{spell: nuke, caster: a.ServerID, targets: []EntityID{c.ServerID}}, a, sa, c, r.at(d))
		r.w.spellHitHateLocked(a, sa, c, 500, 0, numberSpell, nil)
		if ids, why := r.w.castTargetsLocked(sa, a, &nuke, oa); len(ids) != 0 || why != MetalLineWait {
			t.Errorf("Anna's nuke before FIGHT: %v %q", ids, why)
		}
		csA := &castState{spell: storm, caster: a.ServerID}
		if near := r.w.areaCreaturesLocked(a, csA, c.X, c.Y, c.Z, 50, r.at(d)); len(near) != 0 {
			t.Errorf("Anna's area takes %v before FIGHT", near)
		}
		if petMayFight(petA, c) {
			t.Error("Anna's pet may fight him before FIGHT")
		}
		r.w.addThreatLocked(c, a.ServerID, 100000)
		if c.HP != hp || len(c.Threats) != 0 || c.CurrentEnemy != 0 {
			t.Errorf("before FIGHT: HP %d -> %d, threats %v, enemy %d", hp, c.HP, c.Threats, c.CurrentEnemy)
		}
		if r.w.threatensLocked(c, a) || r.w.threatensLocked(c, b) {
			t.Error("he notices someone before FIGHT")
		}
		r.w.mu.Unlock()
		drain(sa)
	}
	if a.HP != a.MaxHP {
		t.Errorf("he hurt Anna during the count: %d/%d", a.HP, a.MaxHP)
	}
	if len(got) != 5 {
		t.Errorf("the count: %q", got)
	}

	// FIGHT: he goes for Anna, and the duel is as it was
	r.tick(5 * time.Second)
	if l := linesOf(drain(sa)); len(l) != 1 || l[0] != MetalLineFight {
		t.Errorf("at 5 s Anna told %q, want %q", l, MetalLineFight)
	}
	if c.duelHeld() || c.CurrentEnemy != a.ServerID {
		t.Fatalf("after FIGHT: held %v, enemy %d, want Anna %d", c.duelHeld(), c.CurrentEnemy, a.ServerID)
	}
	r.w.mu.Lock()
	if !r.w.threatensLocked(c, a) || r.w.threatensLocked(c, b) {
		t.Error("after FIGHT he should notice Anna alone")
	}
	r.w.mu.Unlock()
	if !r.w.EngageAttack(sa, oa, swing, r.at(5*time.Second)) || a.AttackTarget != c.ServerID {
		t.Error("Anna could not engage him after FIGHT")
	}
	if r.w.EngageAttack(sb, ob, swing, r.at(5*time.Second)) {
		t.Error("Bert engaged him after FIGHT")
	}
	r.w.mu.Lock()
	hp := c.HP
	r.w.playerHitLocked(sa, a, c, oa)
	if c.HP >= hp {
		t.Errorf("Anna's hit after FIGHT took nothing: %d -> %d", hp, c.HP)
	}
	r.w.mu.Unlock()
	for d := 5 * time.Second; d <= 15*time.Second && a.HP == a.MaxHP; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if a.HP == a.MaxHP {
		t.Error("he never hit Anna in 10 s after FIGHT")
	}
	if l := linesOf(drain(sa)); hasLine(l, MetalLineFight) || hasLine(l, "1") {
		t.Errorf("the count went on after FIGHT: %q", l)
	}
}

// A challenger who flees during the count ends the duel as any flight does; one brought to nothing
// during it (another creature's hit) loses it as a duel's loser does: left at 1 HP, he gone.
func TestMetalMoogieCountdownEnds(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1020)
	if _, ok, c := r.rise(sa, 0); !ok || c == nil {
		t.Fatal("no Metal Moogie")
	}
	r.tick(0)
	a.Z += MetalArenaRadius + 20
	r.tick(time.Second)
	if r.w.metal != nil {
		t.Error("the duel stood after Anna fled during the count")
	}
	if l := linesOf(drain(sa)); !hasLine(l, MetalLineFled) || hasLine(l, "4") {
		t.Errorf("Anna told %q at her flight", l)
	}

	r2 := newMetalRig(t)
	a2, sa2 := r2.player("Anna", 1000, 1020)
	if _, ok, c := r2.rise(sa2, 0); !ok || c == nil {
		t.Fatal("no Metal Moogie")
	}
	r2.tick(0)
	r2.w.mu.Lock()
	a2.HP = 0
	lost := r2.w.challengeDefeatLocked(sa2, a2, r2.at(time.Second))
	r2.w.mu.Unlock()
	if !lost || a2.HP != 1 || r2.w.metal != nil {
		t.Errorf("Anna brought down during the count: lost %v, HP %d, duel %v", lost, a2.HP, r2.w.metal)
	}
}
