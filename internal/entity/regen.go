package entity

import (
	"slices"
	"time"
)

// Health and power over time, the live game's (eqoa.fandom.com's Formulas,
// "Formula for HoT and PoT", and its "Health over Time (HoT)" page): one HoT
// (PoT) a 50 maximum hit points (power), truncated; worn items' on top, the
// highest in full, the second at SecondItemShare, every further one 1; and
// every point is 1 hit point (power) a tick, a tick every six seconds.
//
// Sandstorm agrees (pcaps/ps2_main_capture.pcap, its stat channel): a level
// 51 bard's power rose every 6.0-6.2 s for over two minutes, by 119, 127 and
// 131 at maxima of 3735, 4149 and 4347, each a 50 of the maximum plus the
// same 45 (its items'); its hit points rose 89 a tick once its songs ended
// (4201 / 50 = 84, plus 5); and both kept rising while it was hit between
// ticks, so a fight does not stop them. Sandstorm leaves the stat channel's
// PoT and HoT (bytes 48 and 52) 0; they are filled here.
//
// A spell's healing over time is not counted here: a "per tic" line is its
// own effect (EffectHPTick, EffectPowerTick) on its own six-second tick
// (cast.go's effectTick), for its duration. No spell line in spelleffect
// names HoT or PoT as a stat (2026-10-25).

// RegenInterval is how often HoT and PoT tick.
const RegenInterval = 6 * time.Second

// SecondItemShare is the percent of the second best HoT (PoT) item that
// counts: the Formulas page's 40 ("a second 25 PoT item gives 10"). Its HoT
// page says 50 (four 20 HoT items: 20 + 10 + 1 + 1). CLIENT carries a third
// rule, the best in full and every other at a tenth (VIClient::GearStat,
// 0x00BCA060), which nothing calls.
const SecondItemShare = 40

// ItemCredit is what worn items' HoT (PoT) add: the highest in full, the
// second at SecondItemShare percent (truncated), every further one 1.
func ItemCredit(values []int) int {
	v := slices.Clone(values)
	slices.SortFunc(v, func(a, b int) int { return b - a })
	n := 0
	for i, x := range v {
		switch i {
		case 0:
			n += x
		case 1:
			n += x * SecondItemShare / 100
		default:
			n++
		}
	}
	return n
}

// HoTBase and PoTBase are a 50 of the maxima, at least 1: the HoT page says
// every character has an innate rate, and a 50 alone gives a new character
// (16 to 34 hit points at level 1) none; HoTNow and PoTNow add the worn
// items' (ItemCredit) and the effects' (food and drink, consumable.go).
func (p *Player) HoTBase() int { return max(1, p.MaxHPNow()/50) }
func (p *Player) PoTBase() int { return max(1, p.MaxPowerNow()/50) }
func (p *Player) HoTNow() int {
	return p.HoTBase() + ItemCredit(p.Gear.HoTItems) + p.Mods.HoT + p.CMEffects[cmSlotHoT]
}
func (p *Player) PoTNow() int {
	return p.PoTBase() + ItemCredit(p.Gear.PoTItems) + p.Mods.PoT + p.CMEffects[cmSlotPoT]
}

// regenTick gives the player its HoT in hit points and its PoT in power,
// each within its maximum: what a tick does. A HoT below nothing is a price
// over time (the Lich form's -196 a tick, the workbook's): it takes its hit
// points at full health too (the user, 2026-11-01, "yes"), where until then
// the tick ran only below full health and a full necromancer paid nothing.
// One a spell's effect makes (Mods.HoT below nothing: Lich) can kill (the
// owner, 2026-11-10, for the GM Jibrish's ruling of 2026-10-29 in jib's PR #1:
// "You could definitely kill yourself with lich form, pacts and so on");
// until then it stopped at 1, as anything else (a worn item's) still does. It
// reports whether the tick took the last hit point (regenSystem: the death).
func (p *Player) regenTick() (died bool) {
	switch top, hot := p.MaxHPNow(), p.HoTNow(); {
	case hot < 0:
		least := 1
		if p.Mods.HoT < 0 {
			least = 0 // a spell's price over time
		}
		if p.HP > least {
			p.HP = max(p.HP+hot, least)
			died = p.HP <= 0
		}
	case p.HP < top:
		p.HP = min(p.HP+hot, top)
	}
	if top := p.MaxPowerNow(); p.Power < top && !died {
		p.Power = min(p.Power+p.PoTNow(), top)
	}
	return died
}
