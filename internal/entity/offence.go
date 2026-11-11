package entity

import (
	"math"
	"time"
)

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
	return targetDefenceAt(npc, npc.Level)
}

// targetDefenceAt is targetDefence for a creature counted as of that level
// (an OFF mod's, workbookmore.go); a dummy keeps its measured defence.
func targetDefenceAt(npc *NPC, level int) float64 {
	if npc.Dummy == TrainingDummy {
		switch npc.Level {
		case 20, 30, 45, 55:
			return targetDefence(npc)
		}
	}
	l := float64(level)
	return 3.26*(l-20) + 0.0652*(l-20)*(l-30)
}

// A monk's bare hands (the user, 2026-11-01: "fix the monks' bare hands"):
// a monk with no weapon in its hands and none in the ranged slot
// (Gear.BareHands) rolls on a hand-to-hand DMG by its level, a one-hander's
// in repair, where a bare hand rolled a weapon's DMG of 0 (its STR alone). Its
// punches want bare hands (the workbook's required equipment None,
// workbookmore.go), so until then a monk fought with a weapon or with
// nothing. Ours: no record of the live game's hand-to-hand damage exists,
// and Sandstorm's unarmed characters do not swing at all. It is the line of
// the server's own items: the median DMG of the one-handed weapons a monk may
// wear (wear 12 and 14, attack types 1-6, a class mask with the monk's bit or
// none, no GM's test item) at each levelreq 1-60 (eqoaclassic's of
// 2026-11-01: 283 weapons over 58 levels) lies on 10.64 a level less 4
// (least absolute deviations; each ten levels' median residual within 3 of
// it; least squares on the medians 10.34 L + 5.6, on every weapon 10.95 L -
// 6.1): 7 at level 1, 49 at 5, 102 at 10, 209 at 20, 315 at 30, 422 at 40,
// 528 at 50, 634 at 60. The levels' own medians at 58-60 run above it (640,
// 690, 700: the epics).
//
// Then a tenth less (petition 1455, the monk Drunkenfist, 2026-11-10: "Monks
// barefist damage should be reduced by 10% to match live. Bare hand damage is
// doing 450 dmg a hit vs 430 a hit with force of mind"): handToHandShare of the
// line, 6 at level 1, 44 at 5, 92 at 10, 188 at 20, 284 at 30, 379 at 40, 475
// at 50, 571 at 60.
var (
	handToHandPerLevel = 10.64
	handToHandAt0      = -4.0
	handToHandShare    = 0.9
)

// The monk's class id, and the ranger's (balancekit.go).
const (
	classMonk   = 4
	classRanger = 1
)

// handToHandDMG is a monk's bare hands' DMG at a level.
func handToHandDMG(level int) int {
	return max(int(math.Round(handToHandShare*(handToHandPerLevel*float64(level)+handToHandAt0))), 1)
}

// meleeWeapon is what the player's melee rolls on: its weapon's DMG, its
// condition and whether it takes both hands, or a monk's bare hands'
// (handToHandDMG). Any other class's bare hands roll a weapon's 0.
func (p *Player) meleeWeapon() (dmg int, cond float64, twoHanded bool) {
	g := &p.Gear
	if g.BareHands && p.Class == classMonk {
		return handToHandDMG(p.Level), 1, false
	}
	return g.WeaponDamage, g.WeaponCond, g.WeaponTwoHanded
}

// playerMeleeRoll is the player's roll on the NPC: its floor and its ends.
func playerMeleeRoll(p *Player, npc *NPC) (floor, low, high int) {
	return playerRollOn(p, npc, p.StatNow(StatStr), p.StatNow(StatDex))
}

// playerRangedRoll is the player's roll on the NPC for one arrow of a ranged
// swing (attack.go's playerShotHit): the melee roll's lines, the same
// coefficients, with DEX where the melee's top has STR (the floor's too) and
// WIS where its bottom has DEX. The beta's stat sheet (betaformula.webp,
// adopted by the owner 2026-11-11): "For ranged attacks Dexterity factors in
// to the maximum damage potential", "Wisdom factors in to how much of your
// maximum damage potential you hit for each shot/throw in ranged combat". Until
// then a shot rolled the melee's, STR and DEX. Ours: no record of the live
// game's ranged roll exists.
func playerRangedRoll(p *Player, npc *NPC) (floor, low, high int) {
	return playerRollOn(p, npc, p.StatNow(StatDex), p.StatNow(StatWis))
}

// playerRollOn is a roll of the player's on the NPC with top the stat that
// raises its top and floor (STR in melee), bottom the one that raises its
// bottom (DEX in melee).
func playerRollOn(p *Player, npc *NPC, top, bottom int) (floor, low, high int) {
	dmg, cond, two := p.meleeWeapon()
	d := float64(dmg) * rollWorth(cond)
	fl := float64(dmg) * floorWorth(cond)
	if two {
		d *= offTwoHand
		fl *= offFloorTwo
	}
	a := offenceArchs[archetypeOf(p.Class)]
	str, dex := float64(top), float64(bottom)
	def := a.Defence * targetDefenceAt(npc, npc.Level-p.Mods.OffMod) // an OFF mod's levels (workbookmore.go)
	def += npc.creatureDefence(p.Level)                              // its effects' AC and AGI (pet.go)
	high = int(math.Round(offHighD*d + offHighSTR*str + a.High - def))
	low = min(int(math.Round(a.LowShare*(offLowD+offLowDEX*dex)*d+a.Low-def)), high)
	floor = int(math.Floor(offFloorD*fl + offFloorSTR*str))
	return max(floor, 1), low, high
}

// playerMeleeHit is one of the player's hits on the NPC, intn the random
// source (rand.Intn), with what the player's effects add to every hit
// (Frenzy's "Increase Damage per Hit by 500": vampire.go).
func playerMeleeHit(p *Player, npc *NPC, intn func(int) int) int {
	return playerHitOf(p, npc, intn, playerMeleeRoll)
}

// playerRangedHit is playerMeleeHit on the ranged roll: one arrow's, before
// rangedShare (attack.go).
func playerRangedHit(p *Player, npc *NPC, intn func(int) int) int {
	return playerHitOf(p, npc, intn, playerRangedRoll)
}

// playerHitOf is one hit of the player's on the NPC on that roll.
func playerHitOf(p *Player, npc *NPC, intn func(int) int, roll func(*Player, *NPC) (int, int, int)) int {
	floor, low, high := roll(p, npc)
	hit := low + intn(high-low+1)
	// as its class template takes it (classtemplate.go), and the creature's
	// melee resist debuff of the weapon's kind (workbookmore.go)
	n := meleeDebuffed(p, npc, max(hit, floor)+max(p.Mods.Melee, 0), time.Now())
	return npc.meleeTaken(npc.kindTakenOf(meleeKindOf(p.heldType()), n))
}
