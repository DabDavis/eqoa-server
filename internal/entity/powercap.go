package entity

import (
	"regexp"
	"strconv"
)

// powercap.go — a spell that holds its bearer's maximum power down (jib's
// bug report of 2026-10-27, RNG-4 and RNG-6).
//
// The ranger's Eye line says it in so many words: "Self, Increase WIS by
// 140, Set MaxPower to 0, Increase Power per tic by 20, 45sec" (Raven Eye,
// EQOA-Data's line), and so does the game's own record of it, as
// Allakhazam's import of 2004-04-28 has it (AK 1747, archived): "Increase
// WIS by 140", "Set Maximum Power to 0", "Increase Power per tic by 20", for
// 45 seconds, recast 300 s. A player's note on Sparrow Eye's page (2006)
// gives the line as it was later changed, by the same words: "Level 20 - 500
// Power Max, 74 PoT ... Level 60 - 1500 Power Max, 266 PoT"; the ranger's
// arrow epics are the same trade ("caps power at 1500 and buffs at least 305
// wisdom"). So the clause is a ceiling, kept while the effect lasts: the
// bearer's maximum power is at most N (the lowest of two such effects), its
// power held under it, and the maximum its own again when the effect ends
// (its power where it was left, to regenerate). The words read: "Set
// Maximum Power to N", "Set Max Power to N", "Set MaxPower to N", "MaxPower
// to N" (Sparrow Eye's), "Caps Power at N".
//
// A spell that lasts no time has nothing to keep a ceiling on: its clause
// takes the bearer's power down to N at once. The gates' "Set Maximum Power
// to 0" is such a price, and stays the caster's alone (gates.go).
var (
	rePowerCap   = regexp.MustCompile(`^(?:set\s+)?max(?:imum)?\s*power\s+(?:to|at)\s+(\d+)$`)
	rePowerCapAt = regexp.MustCompile(`^caps?\s+(?:max(?:imum)?\s*)?power\s+at\s+(\d+)$`)
)

// powerCapPart reads a clause that holds the maximum power at a figure.
func powerCapPart(part string) (int, bool) {
	m := rePowerCap.FindStringSubmatch(part)
	if m == nil {
		m = rePowerCapAt.FindStringSubmatch(part)
	}
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// powerCap is the lowest ceiling the player's effects put on its maximum
// power, and whether any does.
func (p *Player) powerCap() (int, bool) {
	top, capped := 0, false
	for _, e := range p.Effects {
		if e.PowerCapped && (!capped || e.PowerCap < top) {
			top, capped = e.PowerCap, true
		}
	}
	return top, capped
}

// lasts is whether something of the spell stays on its target: a duration,
// or ticks (lasting's own test).
func (d *SpellDef) lasts() bool {
	if d.Duration > 0 {
		return true
	}
	for _, e := range d.Effects {
		if e.Kind == EffectHPTick || e.Kind == EffectPowerTick {
			return true
		}
	}
	return false
}

// onlyPowerCaps is whether every effect the line read is a power ceiling
// (and there is one): a gate's line, whose price is its caster's alone.
func (d *SpellDef) onlyPowerCaps() bool {
	for _, e := range d.Effects {
		if e.Kind != EffectPowerCap {
			return false
		}
	}
	return len(d.Effects) > 0
}
