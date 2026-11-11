package entity

import (
	"math"
	"testing"
)

// A bow's shot (the user, 2026-11-06: "at level 35 bow damage should double,
// because 2 arrows are shot for one hit", "tmac should hit 2000 on level 50
// training dummy with bow"), on the ranged roll since 2026-11-11 (the beta
// sheet's: DEX the top, WIS the bottom): Tmac as the Heirloom snapshot has him
// (a level 60 ranger, STR 106, DEX 453, WIS 260, the heirloom bow's 759) rolls
// 503-512 an arrow on a level 50 training dummy, and shoots it for 2,000 on
// average at Range4 (two arrows), 1,000 at Range2 (one arrow). The controls:
// the same player's melee swing (Attack4) and a thrown potion's roll stay the
// melee roll (STR and DEX, 455 here).
func TestABowShotIsItsArrows(t *testing.T) {
	p := &Player{Class: classRanger, Level: 60, Str: 106, Dex: 453, Wis: 260}
	p.Gear.WeaponDamage, p.Gear.WeaponCond, p.Gear.WeaponSlot = 759, 1, 18
	npc := &NPC{Level: 50, Dummy: TrainingDummy}
	floor, low, high := playerRangedRoll(p, npc)
	if floor != 86 || low != 503 || high != 512 {
		t.Fatalf("the ranged roll %d (floor), %d-%d, want 86, 503-512", floor, low, high)
	}
	melee := playerMeleeHit(p, npc, func(int) int { return 0 })
	if melee != 455 {
		t.Fatalf("the melee roll %d, want 455", melee)
	}
	// every roll of the arrow once, each as likely: the shot's mean
	var two, one float64
	n := high - low + 1
	for i := range n {
		roll := func(int) int { return i }
		p.Swing.Kind = swingRange4
		two += float64(playerShotHit(p, npc, roll))
		p.Swing.Kind = 9
		one += float64(playerShotHit(p, npc, roll))
	}
	if m := two / float64(n); math.Abs(m-2000) > 5 {
		t.Errorf("Range4's mean shot %.1f, want 2000", m)
	}
	if m := one / float64(n); math.Abs(m-1000) > 3 {
		t.Errorf("Range2's mean shot %.1f, want 1000", m)
	}
	first := func(int) int { return 0 }
	for _, c := range []struct {
		kind, potion, want int
	}{{swingRange4, 0, 2 * int(math.Round(503*rangedShare))}, {swingRange1, 0, int(math.Round(503 * rangedShare))},
		{5, 0, melee}, {swingRange4, 50106, melee}} {
		p.Swing.Kind, p.Swing.Potion = c.kind, c.potion
		if got := playerShotHit(p, npc, first); got != c.want {
			t.Errorf("swing kind %d (potion %d): %d, want %d", c.kind, c.potion, got, c.want)
		}
	}
}

// The beta sheet's ranged rule (the owner, 2026-11-11): DEX raises a shot's
// top, as STR does a melee hit's, and WIS its bottom, as DEX does a melee
// hit's, by the same figures; STR does nothing to a shot and WIS nothing to a
// melee hit.
func TestARangedRollGoesByDEXAndWIS(t *testing.T) {
	base := func() *Player {
		p := &Player{Class: classRanger, Level: 40, Str: 100, Dex: 100, Wis: 100}
		p.Gear.WeaponDamage, p.Gear.WeaponCond, p.Gear.WeaponSlot = 300, 1, 18
		return p
	}
	npc := &NPC{Level: 40}
	_, low0, high0 := playerRangedRoll(base(), npc)
	_, mlow0, mhigh0 := playerMeleeRoll(base(), npc)

	p := base()
	p.Dex += 100
	if _, low, high := playerRangedRoll(p, npc); high-high0 != int(math.Round(100*offHighSTR)) || low != low0 {
		t.Errorf("DEX +100: the shot %d-%d from %d-%d, want its top up %v alone", low, high, low0, high0, 100*offHighSTR)
	}
	p = base()
	p.Wis += 100
	if _, low, high := playerRangedRoll(p, npc); low <= low0 || high != high0 {
		t.Errorf("WIS +100: the shot %d-%d from %d-%d, want its bottom up alone", low, high, low0, high0)
	}
	if _, low, high := playerMeleeRoll(p, npc); low != mlow0 || high != mhigh0 {
		t.Errorf("WIS +100: the melee roll %d-%d, want it as it was, %d-%d", low, high, mlow0, mhigh0)
	}
	p = base()
	p.Str += 100
	if _, low, high := playerRangedRoll(p, npc); low != low0 || high != high0 {
		t.Errorf("STR +100: the shot %d-%d, want it as it was, %d-%d", low, high, low0, high0)
	}
	if _, _, high := playerMeleeRoll(p, npc); high <= mhigh0 {
		t.Errorf("STR +100: the melee top %d, want it above %d", high, mhigh0)
	}
}
