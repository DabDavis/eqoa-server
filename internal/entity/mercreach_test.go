package entity

import (
	"testing"
	"time"
)

// A merc's melee from its target's side (mercfight.go's mercStandFrom),
// held to Heirloom petition 3229 (Cucumber: "Mercenary bard seems to be able
// to melee hit from very far away"). His bard stood beside him, 6.8 off Arch
// Bishop Lancel's centre (a large body: 3.27 its edge, +3 its allowance), and
// swung from there, inside a large creature's reach but at no player's place.
// Now it walks to the creature's side before it swings or strikes with a melee
// art, closes in again when the creature moves off, and a hit lands only in a
// player's reach (TargetInRange).

const lancelModel uint32 = 0x9C63C836

func lancelRadii(t *testing.T) {
	SetModelRadii(map[uint32]float32{lancelModel: 3.271})
	t.Cleanup(func() { SetModelRadii(nil) })
}

func TestMercStandFrom(t *testing.T) {
	lancelRadii(t)
	big := &NPC{ModelID: int64(lancelModel), Size: 1}
	if r := mercReach(big); r != 7 {
		t.Fatalf("a large body's merc reach %.2f, want 7 (4.5 + 3 - 0.5)", r)
	}
	if s := mercStandFrom(big); s < 4.7 || s > 4.8 {
		t.Errorf("Lancel's stand %.2f, want his edge 3.27 + 1.5", s)
	}
	if c := mercCloseFrom(big); c >= 6.84 {
		t.Errorf("Lancel's close-in distance %.2f: the bard at 6.84 would still swing", c)
	}
	man := &NPC{ModelID: 0x55432169, Size: 1} // a body the server does not know: a man's edge
	if s := mercStandFrom(man); s < 2.8 || s > 3 {
		t.Errorf("a man-sized body's stand %.2f", s)
	}
	if c := mercCloseFrom(man); c != mercReach(man) {
		t.Errorf("a man-sized body's close-in %.2f, want its reach %.2f", c, mercReach(man))
	}
}

// lancelFight is a level 30 bard merc ordered at a large creature dx off its
// owner, the creature on the owner's line, the merc at its owner's side.
func lancelFight(t *testing.T, dx float32) (*mercRig, *mercState, *NPC) {
	lancelRadii(t)
	r := newMercRig(t)
	ms := r.hire(r.kit(classBard, 0, 0), MercAssist)
	boss := r.creature("Arch Bishop Lancel", 32, dx)
	boss.ModelID = int64(lancelModel)
	boss.HP, boss.MaxHP = 9000000, 9000000
	r.run(0, time.Second)
	ms.npc.X, ms.npc.Z = r.p.X-3.6, r.p.Z // behind its owner, the creature ahead
	ms.ordered = boss.ServerID            // /merc attack
	return r, ms, boss
}

func TestMercWalksToABigCreaturesSideBeforeItSwings(t *testing.T) {
	r, ms, boss := lancelFight(t, 3.24)
	m := ms.npc
	if d := distXZ(m.X, m.Z, boss.X, boss.Z); d < 6.8 || d > mercReach(boss) {
		t.Fatalf("the rig's merc %.2f off, want 6.84 as the petition's bard", d)
	}
	hp := boss.HP
	first := float32(-1)
	for d := 1100 * time.Millisecond; d <= 8*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		if boss.HP < hp {
			dd := distXZ(m.X, m.Z, boss.X, boss.Z)
			if first < 0 {
				first = dd
			}
			if dd > mercStandFrom(boss)+0.01 {
				t.Fatalf("a hit landed from %.2f off, its stand %.2f", dd, mercStandFrom(boss))
			}
			hp = boss.HP
		}
	}
	if first < 0 {
		t.Fatal("the merc never hit")
	}
	t.Logf("first hit from %.2f off Lancel's centre (his edge 3.27)", first)
}

func TestMercClosesInWhenItsTargetMovesOff(t *testing.T) {
	r, ms, boss := lancelFight(t, 3.24)
	m := ms.npc
	r.run(1100*time.Millisecond, 6*time.Second)
	if m.RoundStart.IsZero() {
		t.Fatal("no round at the creature's side")
	}
	// it steps off 2.4 (as Lancel did in the petition): inside the old reach of
	// 7, past where the merc now closes in
	oldX := boss.X
	boss.X += 2.4
	if d := distXZ(m.X, m.Z, boss.X, boss.Z); d <= mercCloseFrom(boss) || d > mercReach(boss) {
		t.Fatalf("the creature moved to %.2f off, want between %.2f and %.2f", d, mercCloseFrom(boss), mercReach(boss))
	}
	r.w.SpatialIndex.Update(boss.ServerID, oldX, boss.Z, boss.X, boss.Z)
	hp := boss.HP
	for d := 6100 * time.Millisecond; d <= 12*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		if boss.HP < hp {
			if dd := distXZ(m.X, m.Z, boss.X, boss.Z); dd > mercCloseFrom(boss)+0.01 {
				t.Fatalf("a hit landed from %.2f off after the creature moved off", dd)
			}
			hp = boss.HP
		}
	}
	if d := distXZ(m.X, m.Z, boss.X, boss.Z); d > mercStandFrom(boss)+0.01 {
		t.Errorf("after 6 s the merc stands %.2f off, its stand %.2f", d, mercStandFrom(boss))
	}
}

// A melee art (a bard's Assault, range 5) waits for its target's side; a
// spell of a real range (a Dirge's 30) is cast from where it stands.
func TestMercMeleeArtFromItsTargetsSide(t *testing.T) {
	lancelRadii(t)
	r := newMercRig(t)
	ms := r.hire(r.kit(classBard, 0, 0), MercAssist)
	boss := r.creature("Arch Bishop Lancel", 32, 3.24)
	boss.ModelID = int64(lancelModel)
	m := ms.npc
	m.X, m.Z = r.p.X-3.6, r.p.Z
	art := bookSpell(901204, "Arcanic Assault", 1, 0, 0, 10, "Target, Decrease Hitpoints by 200")
	art.Range = 5
	dirge := bookSpell(700777, "Destructive Dirge", 1, 0, 0, 10, "Target, Decrease Hitpoints by 712")
	if !ms.mercArtAwayLocked(&art, boss) {
		t.Error("the Assault may be struck from 6.84 off Lancel")
	}
	if ms.mercArtAwayLocked(&dirge, boss) {
		t.Error("the Dirge (range 30) held for Lancel's side")
	}
	m.X = boss.X - mercStandFrom(boss)
	if ms.mercArtAwayLocked(&art, boss) {
		t.Error("the Assault held at Lancel's side")
	}
	caster := &mercState{role: roleCaster, npc: m}
	m.X = r.p.X - 3.6
	if caster.mercArtAwayLocked(&art, boss) {
		t.Error("a caster held its range-5 spell for the creature's side")
	}
}
