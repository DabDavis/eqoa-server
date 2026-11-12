package entity

import (
	"math"
	"regexp"
	"strconv"
)

// hpcap.go — three clauses a line may hold for a buff that trades its
// bearer's strength for speed: the GM Jibrish's petition 1725, the Tester Pro
// ("When the player uses it, it gives them a run speed buff that is x2 SoW
// speed. However, also make it reduce their max HP to 1, and set all of their
// stats to 1 while the buff is up"; tools/tester_pro_20261112.py writes the
// item and its line, "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP
// to 1, Set All Stats to 1, 30min").
//
//   - "Set Maximum HP to N" ("Set Max HP to N", "Caps HP at N"; EffectHPCap):
//     a ceiling as the Eye line's on power is (powercap.go): while it lasts the
//     bearer's maximum hit points are at most N (the lowest of two such, never
//     below 1), its hit points held under it (effectsChangedLocked); at its end
//     the maximum is its own again, the hit points where they were left, to
//     regenerate. Lasting no time, it takes the hit points down to N at once.
//   - "Set All Stats to N" (EffectStatPin): each of STR, STA, AGI, DEX, WIS,
//     INT and CHA is N while it lasts, whatever the bearer's own, its gear,
//     its masteries or another effect would make it (StatNow), so the hit
//     points and power its stats give follow (MaxHPNow, MaxPowerNow); at its
//     end every stat is as it was. Unlike "Set STR to 500" (vampire.go), which
//     is the difference from the stat as it lands, it holds whatever changes
//     under it. Two such: the lowest.
//   - "Run Speed Nx Spirit of the Wolf" ("Movement Speed 2x SoW"; EffectSpeed
//     with SowPct): the movement buff's run speed (cast.go's speedBuff, a
//     movement rate of 100) times N while it lasts, past the rate's cap of 100
//     (SpeedNow, which the movement check judges a report by: movecheck.go);
//     the fastest such counts, and a lesser speed's removal ("removing lesser
//     speed boosts", cureMatches) leaves it on.

var (
	reHPCap   = regexp.MustCompile(`^(?:set\s+)?max(?:imum)?\s*(?:hp|hit\s*points|hitpoints|health)\s+(?:to|at)\s+(\d+)$`)
	reHPCapAt = regexp.MustCompile(`^caps?\s+(?:max(?:imum)?\s*)?(?:hp|hit\s*points|hitpoints|health)\s+at\s+(\d+)$`)
	reStatPin = regexp.MustCompile(`^set\s+all\s+(?:of\s+(?:the\s+|its\s+|their\s+)?)?stats\s+to\s+(\d+)$`)
	// an item's use that takes nothing of it (SpellDef.Unlimited)
	reUnlimited = regexp.MustCompile(`^(?:unlimited|infinite)\s+(?:uses|charges)$|^never\s+spent$`)
	reSowSpeed  = regexp.MustCompile(`^(?:increase\s+)?(?:run|movement)\s+speed\s+(?:to\s+)?(\d+(?:\.\d+)?)\s*x\s+(?:sow|spirit\s+of\s+the\s+wolf)$`)
)

// SowTimesMost is the most times the movement buff's run speed a line may ask.
const SowTimesMost = 5

// hpCapPart reads a clause that holds the maximum hit points at a figure.
func hpCapPart(part string) (int, bool) {
	m := reHPCap.FindStringSubmatch(part)
	if m == nil {
		m = reHPCapAt.FindStringSubmatch(part)
	}
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// statPinPart reads a clause that holds every stat at a figure.
func statPinPart(part string) (int, bool) {
	m := reStatPin.FindStringSubmatch(part)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// sowSpeedPart reads a clause that gives a run speed as a multiple of the
// movement buff's: its percent of it (2x is 200), above 100 and at most
// SowTimesMost times.
func sowSpeedPart(part string) (int, bool) {
	m := reSowSpeed.FindStringSubmatch(part)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v <= 1 {
		return 0, false
	}
	return int(math.Round(min(v, SowTimesMost) * 100)), true
}

// hpCap is the lowest ceiling the player's effects put on its maximum hit
// points, and whether any does.
func (p *Player) hpCap() (int, bool) {
	top, capped := 0, false
	for _, e := range p.Effects {
		if e.Rules.HPCapped && (!capped || e.Rules.HPCap < top) {
			top, capped = e.Rules.HPCap, true
		}
	}
	return max(top, 1), capped
}

// statPin is what the player's effects hold each stat at (the lowest), and
// whether any does.
func (p *Player) statPin() (int, bool) {
	n, pinned := 0, false
	for _, e := range p.Effects {
		if e.Rules.StatPinned && (!pinned || e.Rules.StatPin < n) {
			n, pinned = e.Rules.StatPin, true
		}
	}
	return n, pinned
}

// sowFactor is what the player's fastest speed of its own makes of the
// movement buff's run speed: 1 with none.
func (p *Player) sowFactor() float32 {
	pct := 100
	for _, e := range p.Effects {
		if e.Speed {
			pct = max(pct, e.Rules.SowPct)
		}
	}
	return float32(pct) / 100
}
