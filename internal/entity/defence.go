package entity

import "math"

// An NPC's melee hit on a player, as Sandstorm's lands: measured
// 2026-10-21/22 on Muniel's Tea Garden's training dummies, 2,660 hits on
// four characters of three archetypes, their AC moved by taking armour off
// and their AGI by forms, songs and a talisman, one at a time
// (claude-notes/defence-sandstorm-20261022.md):
//
//	hit = max(floor, a whole number from top-width to top, each as likely)
//	top = the NPC's top - AC*its AC slope - AGI*the archetype's AGI slope
//	      + the archetype's offset
//
// AC and AGI are the stat channel's (worn and from spells: StatNow). The
// AC slope is the NPC's (0.1256 against the Lv45 dummy, 0.1137 against the
// Lv55); the AGI slope and the offset are the defender's archetype's, the
// same against every NPC. DEX counts for nothing (Anthem of Triumph's +126
// changed no hit), nor does the NPC's own AGI (Symphonic Blades' -35 on the
// dummy changed none), and a form (the lion and rat forms) is only the
// stats it gives. The hits a round lands are always all of them: AGI
// dodges none.

// archetype is what a class defends as, by its hit-point factor
// (hpFactor): tank 24, melee 16, priest 13, caster 10.
type archetype uint8

const (
	archTank archetype = iota
	archMelee
	archPriest
	archCaster
)

func archetypeOf(class int) archetype {
	switch hpFactor(class) {
	case 24:
		return archTank
	case 16:
		return archMelee
	case 13:
		return archPriest
	}
	return archCaster
}

// agiSlope is what a point of AGI takes off the top, by archetype: a shadow
// knight's talisman (+23) and Shriek of Horror (+45), a bard's rat forms
// (+45 to +120) and a necromancer's lion forms (+35 to +75). A priest's was
// not measured (no priest of a dummy's levels): between the melee's and the
// caster's.
var agiSlope = [...]float64{archTank: 0.305, archMelee: 0.227, archPriest: 0.18, archCaster: 0.133}

// archOffset is what an archetype takes above a tank of the same AC and
// AGI. A priest's is between the melee's and the caster's, unmeasured.
var archOffset = [...]float64{archTank: 0, archMelee: 15.1, archPriest: 23.4, archCaster: 31.7}

// meleeRoll is an NPC's melee: the least a hit is, the top on a tank of no
// AC and no AGI, how far below the top a hit reaches, and what a point of
// AC takes off the top.
type meleeRoll struct {
	Floor   int
	Top     float64
	Width   int
	ACSlope float64
}

// hitOn is one hit of the roll on the player, intn the random source
// (rand.Intn): never less than one.
func (r meleeRoll) hitOn(p *Player, intn func(int) int) int {
	a := archetypeOf(p.Class)
	top := int(math.Round(r.Top - r.ACSlope*float64(p.StatNow(StatAC)) -
		agiSlope[a]*float64(p.StatNow(StatAgi)) + archOffset[a]))
	hit := top - r.Width + intn(r.Width+1)
	return max(hit, r.Floor, 1)
}

// creatureRoll is the roll of a creature of the level: the floor half the
// level (the beach's Lv34 Desert Crab 17, Lv35 Gharial 18); the top, width
// and AC slope on the line through the two dummies measured whole, Lv45 and
// Lv55 (the top as level^1.52, the width level^0.81, the slope falling
// 0.00119 a level). A creature's roll above its floor was seen only twice
// (a crab's 19 and 45 on an enchanter, which the line allows), so the
// creatures' line is the dummies' until creatures are measured.
func creatureRoll(level int) meleeRoll {
	l := float64(max(level, 1))
	return meleeRoll{
		Floor:   (level + 1) / 2,
		Top:     182.6 * math.Pow(l/45, 1.52),
		Width:   int(math.Round(138 * math.Pow(l/45, 0.81))),
		ACSlope: min(max(0.1256-0.00119*(l-45), 0.08), 0.2),
	}
}

// dummyRolls is each dummy's roll by its level, measured. The Lv45 and Lv55
// Training Dummies are whole (the joint fit); the Lv50 and the Regular
// Dummies' tops and widths are fitted with the archetype terms held (12, 13
// and 5 hits above their floors); the Lv20-35 Training Dummies hit their
// floors on every character measured, the lowest-defended an enchanter of
// 167 AC (one 18 from the Lv35), so their tops are the Lv35's (62) scaled
// down as creatures' are, under the most those floors allow. A width or a
// slope of 0 is the creature line's at the level; a dummy level not here
// (the Lv60 Training Dummy, the Lv70 Boss Dummy) rolls as a creature of it.
var dummyRolls = map[DummyKind]map[int]meleeRoll{
	TrainingDummy: {
		20: {Floor: 8, Top: 26.4},
		25: {Floor: 10, Top: 37.2},
		30: {Floor: 12, Top: 49},
		35: {Floor: 14, Top: 62},
		45: {Floor: 24, Top: 182.6, Width: 138, ACSlope: 0.1256},
		50: {Floor: 21, Top: 134.1, Width: 119},
		55: {Floor: 30, Top: 247.8, Width: 162, ACSlope: 0.1137},
	},
	RegularDummy: {
		55: {Floor: 30, Top: 224.1, Width: 137, ACSlope: 0.1137},
		60: {Floor: 33, Top: 269},
	},
}

// npcMeleeRoll is the NPC's roll: a dummy's measured one, else a
// creature's of its level.
func npcMeleeRoll(npc *NPC) meleeRoll {
	c := creatureRoll(npc.Level)
	r, ok := dummyRolls[npc.Dummy][npc.Level]
	if !ok {
		return c
	}
	if r.Width == 0 {
		r.Width = c.Width
	}
	if r.ACSlope == 0 {
		r.ACSlope = c.ACSlope
	}
	return r
}
