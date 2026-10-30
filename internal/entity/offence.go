package entity

import "math"

// A player's melee hit on an NPC, as Sandstorm's lands: measured 2026-10-22
// on Muniel's Tea Garden's training dummies and a beach Gharial, 1,595
// swings by three characters of three archetypes, the weapon, its
// condition, STR, DEX and the target moved one at a time
// (claude-notes/offence-sandstorm-20261022.md):
//
//	D     = the weapon's DMG x what its condition is worth (x1.215 two-handed)
//	high  = 0.8973*D + 0.196*STR + the archetype's offset - its share of the target's defence
//	low   = its share of (0.1657 + 0.001135*DEX)*D + its offset - the same defence, at most high
//	floor = 0.104*DMG*the floor's condition worth (x1.05 two-handed) + 0.02*STR, whole
//	hit   = max(floor, a whole number from low to high, each as likely)
//
// STR raises the top, DEX the bottom (a narrower roll: the bottom rose 70
// for 129 DEX, the top 10). A broken weapon rolls a fifth of what it does
// in repair, its floor a third. The target's defence grows with its level,
// a training dummy's as measured. A caster's roll sits 96 below a tank's
// and loses 1.39 times as much to a higher target; a melee class's, 13
// above and 2.04 times (one bard, his rapiers 1.6% of their hit points).
// What a priest does was not measured: between the melee's and the
// caster's. Every attacker measured was level 49-51.

// offenceArch is an archetype's place on the tank's lines: the high and low
// ends' offsets (against a level 20 training dummy), the low end's share of
// the roll, and how many times the tank's the target's defence takes.
type offenceArch struct {
	High, Low, LowShare, Defence float64
}

var offenceArchs = [...]offenceArch{
	archTank:   {High: 18.95, Low: 16.46, LowShare: 1.002, Defence: 1.02},
	archMelee:  {High: 32.28, Low: -90.0, LowShare: 2.495, Defence: 2.041},
	archPriest: {High: -22.5, Low: -97.4, LowShare: 2.07, Defence: 1.71},
	archCaster: {High: -77.35, Low: -104.7, LowShare: 1.636, Defence: 1.385},
}

// The lines themselves, the tank's (fitted on its weapons in repair: an
// axe, a blade and a staff, at four dummies).
var (
	offHighD    = 0.8973   // the top's share of D
	offHighSTR  = 0.196    // the top's per point of STR
	offLowD     = 0.1657   // the bottom's share of D at no DEX...
	offLowDEX   = 0.001135 // ...and a point of DEX's
	offTwoHand  = 1.215    // a two-hander's D
	offFloorD   = 0.104    // the floor's share of DMG
	offFloorSTR = 0.02     // the floor's per point of STR
	offFloorTwo = 1.05     // a two-hander's floor
)

// rollWorth and floorWorth are what a weapon's condition (its hit points
// over their most, 0 to 1) is worth to the roll and to the floor: a broken
// one a fifth and a third, one at 92% all but whole.
func rollWorth(cond float64) float64  { return 1 - 0.805*math.Pow(1-cond, 2) }
func floorWorth(cond float64) float64 { return 1 - 0.657*math.Pow(1-cond, 2.3) }

// targetDefence is what an NPC takes off a tank's roll beyond a level 20
// training dummy: the measured dummies' own, else a creature's line in its
// level (through the Lv20, Lv30 and Lv45 dummies; the Lv35 Gharial on it).
func targetDefence(npc *NPC) float64 {
	if npc.Dummy == TrainingDummy {
		switch npc.Level {
		case 20:
			return 0
		case 30:
			return 32.65
		case 45:
			return 105.85
		case 55:
			return 245.85 // the Lv55 is tougher than its level
		}
	}
	l := float64(npc.Level)
	return 3.26*(l-20) + 0.0652*(l-20)*(l-30)
}

// playerMeleeRoll is the player's roll on the NPC: its floor and its ends.
func playerMeleeRoll(p *Player, npc *NPC) (floor, low, high int) {
	g := &p.Gear
	cond := g.WeaponCond
	d := float64(g.WeaponDamage) * rollWorth(cond)
	fl := float64(g.WeaponDamage) * floorWorth(cond)
	if g.WeaponTwoHanded {
		d *= offTwoHand
		fl *= offFloorTwo
	}
	a := offenceArchs[archetypeOf(p.Class)]
	str, dex := float64(p.StatNow(StatStr)), float64(p.StatNow(StatDex))
	def := a.Defence * targetDefence(npc)
	high = int(math.Round(offHighD*d + offHighSTR*str + a.High - def))
	low = min(int(math.Round(a.LowShare*(offLowD+offLowDEX*dex)*d+a.Low-def)), high)
	floor = int(math.Floor(offFloorD*fl + offFloorSTR*str))
	return max(floor, 1), low, high
}

// playerMeleeHit is one of the player's hits on the NPC, intn the random
// source (rand.Intn), with what the player's effects add to every hit
// (Frenzy's "Increase Damage per Hit by 500": vampire.go).
func playerMeleeHit(p *Player, npc *NPC, intn func(int) int) int {
	floor, low, high := playerMeleeRoll(p, npc)
	hit := low + intn(high-low+1)
	return npc.meleeTaken(max(hit, floor) + max(p.Mods.Melee, 0)) // as its class template takes it (classtemplate.go)
}
