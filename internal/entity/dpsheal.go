package entity

import "github.com/eqoa/server/internal/session"

// dpsheal.go — the damage meter's healing and power (Heirloom petition 3030, a
// player: "DPS meter should have options to include healing per second, and if
// possible power provided per second"; the owner: build both). Layout 2 of
// OpcodeDPSMeterData (dpsmeter.go) gives each line, beside its damage, what its
// heals restored and the power it gave; the client divides each by the fight's
// length, as it does the damage.
//
// Healing is what a heal RESTORED, not what it was worth: a heal's figure is
// told whole, at full health too (healLocked, petition 2016), but the meter
// counts no overhealing. It is counted where the figure is, as the damage is: a
// heal figure (a positive 0x00DB) of a kind (numbertags.go: a spell's heal, a
// heal over time's tick, a proc's heal; each its pet's or its merc's too) is the
// healing of the player it is told to, on its group's fight, its pet's and merc's
// share kept apart in it. Each path that heals tells its figure inside
// dpsWorthLocked with the hit points the heal filled, so the figures told count
// that much at most: healLocked (a spell's heal, direct or area, a heal over
// time's tick on a player, a tap's or a drain's take, a proc's heal), a heal on a
// pet or a merc (pet.go's, a heal over time's tick on one: tellNPCHoTLocked's
// callers, a drain's take for the caster's pet), a mark's and a worn item's heal
// at a hit, the pets' own heals (on themselves, their owners, their side), a
// familiar's, and a spell's HoT stat at the regen tick (its part of what the tick
// restored; a meal's and a drink's count nothing: food is no healer's). A path
// that tells a heal figure outside one counts it whole. A merc's heal on a player
// other than its owner is told to no one of its side as the merc's, so healLocked
// counts it itself, for the merc's owner.
//
// Healing and power count only on a fight under way (dpsAddLocked): a top-up
// after the fight, or a long buff's ticks between pulls, start none.
//
// Power is the power a player (its pet's, its merc's too) gave ANOTHER, a
// player, a pet or a merc, at what it filled (none past the most): a spell's
// power (an enchanter's Power Boon, a bard's Power Ballad), a power over time's
// tick (Clarity, Power Dance) on a player, a pet or a merc, a drain's or a tap's
// power for the group (a necromancer's power tap shared, Destroy Contraption's),
// a weapon's proc's power on the wielder's group, and a spell's PoT stat (an
// enchanter's group PoT buff) at the regen tick, its part of what the tick
// restored. Power one gives oneself (a tap's for its caster alone, its own
// regeneration) is not counted. No figure is told of power, so each path counts
// it itself (dpsPowerLocked).

// dpsWorthLocked runs tell, a heal's figures, with what they count on the
// meter held to filled in all: the hit points the heal restored (the figures
// are what it was worth). Must be called with the lock held.
func (w *World) dpsWorthLocked(filled int, tell func()) {
	was, on := w.dpsWorth, w.dpsWorthOn
	w.dpsWorth, w.dpsWorthOn = max(filled, 0), true
	tell()
	w.dpsWorth, w.dpsWorthOn = was, on
}

// dpsHealFigureLocked counts a heal figure of n of kind told to sess's player
// (sendDamageIndication) as that player's healing, at most what is left of the
// heal's worth (dpsWorthLocked). Must be called with the lock held.
func (w *World) dpsHealFigureLocked(sess *session.Session, n int, kind numberKind) {
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if w.dpsWorthOn {
		n = min(n, w.dpsWorth)
		w.dpsWorth -= max(n, 0)
	}
	w.dpsAddByLocked(p, dpsHeal, n, kind, w.dpsFigureMercLocked(p, kind), dpsMeterNow())
}

// dpsMakerLocked is the player id's figures count for: a player itself; a pet,
// a charmed creature or a merc (an NPC with an owner) its owner, as its
// helper's share; else none. Must be called with the lock held.
func (w *World) dpsMakerLocked(id EntityID) (maker *Player, helper bool) {
	if p := w.Players[id]; p != nil {
		return p, false
	}
	if n := w.NPCs[id]; n != nil && n.Owner != 0 {
		if o := w.Players[n.Owner]; o != nil {
			return o, true
		}
	}
	return nil, false
}

// dpsMercBodyLocked is whether caster is a merc's body (a Player that is no
// player in the world: merc.go), and its owner. Must be called with the lock
// held.
func (w *World) dpsMercBodyLocked(caster *Player) (owner *Player, ok bool) {
	if caster == nil || w.Players[caster.ServerID] == caster {
		return nil, false
	}
	if n := w.NPCs[caster.ServerID]; n.isMerc() && n.merc.owner != nil {
		return n.merc.owner, true
	}
	return nil, false
}

// dpsPowerLocked counts gained, the power giver (a player, or a pet's or a
// merc's id) gave receiver, as the power of the player it counts for
// (dpsMakerLocked): power one gives oneself counts nothing. Must be called with
// the lock held.
func (w *World) dpsPowerLocked(giver, receiver EntityID, gained int) {
	if gained <= 0 || giver == 0 || giver == receiver {
		return
	}
	if maker, helper := w.dpsMakerLocked(giver); maker != nil {
		if merc := w.dpsMercOfLocked(maker, giver); merc != 0 {
			w.dpsAddByLocked(maker, dpsPower, gained, numberByMerc, merc, dpsMeterNow())
		} else {
			w.dpsAddLocked(maker, dpsPower, gained, helper, dpsMeterNow())
		}
	}
}

// dpsPowerByLocked is dpsPowerLocked for a caster that may be a merc's body
// (its id is its merc's). Must be called with the lock held.
func (w *World) dpsPowerByLocked(caster *Player, receiver EntityID, gained int) {
	if caster != nil {
		w.dpsPowerLocked(caster.ServerID, receiver, gained)
	}
}

// dpsStatPoTLocked counts a regen tick's gained power on bearer as the power of
// those whose spells' PoT stat gave it (each effect's caster but the bearer's
// own, a meal's and a drink's none): each its part (its effects' PoT), as far
// as gained goes, in the order the casters' effects come. Must be called with
// the lock held.
func (w *World) dpsStatPoTLocked(effects []*ActiveEffect, bearer EntityID, gained int) {
	for _, ae := range effects {
		if gained <= 0 {
			return
		}
		if ae == nil || ae.Mods.PoT <= 0 || ae.Food != nil || ae.SpellID == FoodEffectID || ae.SpellID == DrinkEffectID || ae.Caster == bearer {
			continue
		}
		n := min(ae.Mods.PoT, gained)
		gained -= n
		w.dpsPowerLocked(ae.Caster, bearer, n)
	}
}
