package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// A ranged weapon's shot costs power (petition 1581, the GM Jibrish: "Range
// attack abilities, such as alchemist throw or ranger firing their bow (NOT
// ABILITIES!) should take exactly 10% of the players maximum mana"): every
// auto-attack shot of a ranged weapon (CLIENT's Range1-4, GetSwing's kinds
// 8-11: a bow, a throwing weapon), at a creature, a training dummy or a foe,
// costs DefaultRangedPowerPct of its shooter's most power (world.
// ranged_power_pct; 0 none), rounded, as a thrown potion's (potion.go,
// world.throw_power_pct, the same 10% since this petition). A shot is paid as
// it is loosed in reach (a round's hit, Range4's two arrows one shot); short
// of the power it is refused, told in red (ShotNoPower), nothing landed and
// nothing taken, and the fight is let go, as a throw's. A spell or a combat
// art that wants a bow (cast.go wantsRanged) is a cast: its own power, none of
// this. Until then a shot cost none.
const DefaultRangedPowerPct = 10

// ShotNoPower is the refusal of a shot its shooter has not the power for (ours,
// as ThrowNoPower).
const ShotNoPower = "Not enough power to shoot."

// rangedPower is the World's ranged shot's cost (SetRangedPowerPct; set false:
// DefaultRangedPowerPct).
type rangedPower struct {
	pct int
	set bool
}

// SetRangedPowerPct sets a ranged shot's power, a percent of the shooter's
// most (world.ranged_power_pct; 0 none, below 0 the default).
func (w *World) SetRangedPowerPct(pct int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rangedPower = rangedPower{pct: min(pct, 100), set: pct >= 0}
}

// rangedShot is whether p's swing is a ranged weapon's shot (no worn potion:
// that is a throw, paid by throwPowerLocked).
func rangedShot(p *Player) bool {
	return p.Swing.Kind >= swingRange1 && p.Swing.Kind <= swingRange4 && p.Swing.Potion == 0
}

// shotPowerCost is what a ranged shot costs p now.
func (w *World) shotPowerCost(p *Player) int {
	pct := DefaultRangedPowerPct
	if w.rangedPower.set {
		pct = w.rangedPower.pct
	}
	if pct <= 0 || p.MaxPower <= 0 {
		return 0
	}
	return (p.MaxPower*pct + 50) / 100
}

// shotPowerLocked takes a ranged shot's power from p and reports true (any
// other swing: true, nothing taken), or tells p it has not enough (sess nil:
// untold) and reports false, nothing taken. Must be called with the lock held.
func (w *World) shotPowerLocked(sess *session.Session, p *Player) bool {
	if !rangedShot(p) {
		return true
	}
	cost := w.shotPowerCost(p)
	if cost <= 0 {
		return true
	}
	if p.Power < cost {
		log.Printf("Combat: %s has %d power, a shot costs %d: refused", p.Name, p.Power, cost)
		if sess != nil {
			w.sendColoredLocked(sess, ShotNoPower, castRefusalRGB)
		}
		return false
	}
	p.Power -= cost
	p.StatsDirty = true
	return true
}
