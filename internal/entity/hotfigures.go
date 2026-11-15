package entity

// Heal over time figures (petition 2142, Lonesnecro, owner-approved
// 2026-11-15: "does soul mastery have a HoT component to it? can we get the
// heal tick to be visable?"). A buff's heal over time shows each tick as a
// green 0x00DB, whole (what it was worth, at full health too: healLocked's
// rule, petition 2016), one figure a tick a bearer, to whom other heals'
// figures go:
//
//   - a "HP per tic" effect (EffectHPTick) on a player: runTickLocked's
//     healLocked, to its caster (numberHoT) and to its bearer;
//   - one on a pet, a merc or a merc's pet (petEffectsLocked,
//     mercEffectsLocked): tellNPCHoTLocked, to its caster as its own over the
//     NPC (numberHoT; the pet's own buff on itself 0x87, figuresPet), and to
//     the NPC's owner when another cast it, from the caster (untagged) or,
//     the caster gone or no player (a merc's), from the owner itself
//     (tagged as the merc's for its own merc's, as runTickLocked);
//   - a spell's HoT stat (EffectStat StatHoT, the workbook's "hot": Mods.HoT
//     above nothing) at the six-second regen (regenSystem, npcRegenSystem):
//     tellStatHoTLocked, its part alone, to the bearer (a pet's: its owner).
//
// Natural regeneration shows nothing: a 50th of the maximum, worn items',
// combat masteries', a meal's or a drink's (FoodEffectID, DrinkEffectID), a
// workbook pet's row.

// tellNPCHoTLocked tells a heal over time's tick of n on npc (the file's doc).
// Must be called with the lock held.
func (w *World) tellNPCHoTLocked(npc *NPC, ae *ActiveEffect, n int) {
	if n <= 0 || npc.Dead {
		return
	}
	caster := w.Players[ae.Caster]
	if caster != nil {
		w.tellAmountLocked(caster, w.findSessionByEntityIDLocked(caster.ServerID), npc.ServerID, n, ae.tickKind(true))
	}
	owner := w.Players[npc.Owner]
	if owner == nil || owner == caster {
		return
	}
	sess := w.findSessionByEntityIDLocked(owner.ServerID)
	if sess == nil {
		return
	}
	id := w.objectIDForEntityLocked(sess, npc.ServerID)
	if id == 0 {
		return
	}
	src, kind := sess.PlayerObjectID(), numberKind(0)
	switch {
	case caster != nil:
		if src = w.objectIDSeenByLocked(sess, caster.ServerID); src == 0 {
			return
		}
	case ae.figures == figuresMerc && w.mercHiredByLocked(ae.Caster, owner.ServerID):
		kind = ae.tickKind(true)
	}
	w.sendDamageIndication(sess, src, n, id, kind)
}

// statHoT is what effects' HoT stats (a spell's, Mods.HoT above nothing) add
// to a regen tick, without a meal's or a drink's, and the kind it is told to
// owner as: its own when every one of them is owner's cast (the first's
// figures), else another's (0).
func statHoT(effects []*ActiveEffect, owner EntityID) (n int, kind numberKind) {
	own, first := true, true
	for _, ae := range effects {
		if ae == nil || ae.Mods.HoT <= 0 || ae.Food != nil || ae.SpellID == FoodEffectID || ae.SpellID == DrinkEffectID {
			continue
		}
		n += ae.Mods.HoT
		if ae.Caster != owner {
			own = false
		}
		if first {
			kind, first = ae.tickKind(true), false
		}
	}
	if !own {
		kind = 0
	}
	return n, kind
}

// tellPlayerStatHoTLocked tells a player its spells' HoT stat at a regen tick
// (the file's doc): from itself on itself. Must be called with the lock held.
func (w *World) tellPlayerStatHoTLocked(p *Player) {
	n, kind := statHoT(p.Effects, p.ServerID)
	if n <= 0 {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendDamageIndication(sess, sess.PlayerObjectID(), n, sess.PlayerObjectID(), kind)
	}
}

// tellPetStatHoTLocked tells a pet's (a merc's) owner its spells' HoT stat at
// a regen tick: from the owner over it. Must be called with the lock held.
func (w *World) tellPetStatHoTLocked(npc *NPC) {
	owner := w.Players[npc.Owner]
	if owner == nil || npc.Dead || npc.HP <= 0 {
		return
	}
	n, kind := statHoT(npc.Effects, owner.ServerID) // a merc's too: its body's Mods are its effects' (syncMercBody)
	if n <= 0 {
		return
	}
	if sess := w.findSessionByEntityIDLocked(owner.ServerID); sess != nil {
		if id := w.objectIDForEntityLocked(sess, npc.ServerID); id != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), n, id, kind)
		}
	}
}
