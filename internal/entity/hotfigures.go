package entity

// Heal over time figures (petition 2142, owner-approved
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
//     tellPlayerStatHoTLocked / tellPetStatHoTLocked, its part alone, to the
//     bearer (a pet's, a merc's, a merc's pet's: its owner), the whole of it;
//     and each other player who cast a part of it on that bearer is told its
//     own part over the bearer, as its own (numberHoT's tags, the effect's
//     tickKind), as a "HP per tic" tick is told its caster (petition 2593: a
//     ranger's Forest's Succor on a groupmate or a merc showed it nothing).
//     Whole at full health too (petition 2016's rule): a player's regen tick
//     runs at any health, and a pet's, a merc's or a merc's pet's (owned, so
//     never regenQuiet: npcsets.go) at any health too, its hit points capped.
//
// A meal's or a drink's heal over time (FoodEffectID, DrinkEffectID: a
// grocer's food, a ranger's foraged berries) is told its eater too, its own
// figure at each regen tick (tellPlayerMealHoTLocked) and at the first tick a
// meal gives at once (Consume): Heirloom petition 2688, 2026-11-22, "'foraged
// berries' heal fine ... but the heal numbers dont show up".
//
// Natural regeneration shows nothing: a 50th of the maximum, worn items',
// combat masteries', a workbook pet's row.

// tellNPCHoTLocked tells a heal over time's tick of n on npc (the file's doc).
// Must be called with the lock held.
func (w *World) tellNPCHoTLocked(npc *NPC, ae *ActiveEffect, n int) {
	if n <= 0 || npc.Dead {
		return
	}
	defer w.dpsBySetLocked(ae.Caster)() // a merc's tick: its meter line (dpsmerc.go)
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

// statHoTPart is one caster's part of a bearer's spells' HoT stat (statHoT's
// sum split by caster), told as the first of its effects' tickKind.
type statHoTPart struct {
	caster EntityID
	n      int
	kind   numberKind
}

// statHoTParts is statHoT's sum split by caster, in the order the casters'
// effects first come, without skip's (the one already told the whole).
func statHoTParts(effects []*ActiveEffect, skip EntityID) []statHoTPart {
	var parts []statHoTPart
next:
	for _, ae := range effects {
		if ae == nil || ae.Mods.HoT <= 0 || ae.Food != nil || ae.SpellID == FoodEffectID || ae.SpellID == DrinkEffectID || ae.Caster == skip {
			continue
		}
		for i := range parts {
			if parts[i].caster == ae.Caster {
				parts[i].n += ae.Mods.HoT
				continue next
			}
		}
		parts = append(parts, statHoTPart{caster: ae.Caster, n: ae.Mods.HoT, kind: ae.tickKind(true)})
	}
	return parts
}

// tellStatHoTCastersLocked tells each other player who cast a part of
// bearer's spells' HoT stat its own part over bearer, as its own (the file's
// doc; petition 2593). skip is the one already told the whole (the bearer, a
// pet's owner). Must be called with the lock held.
func (w *World) tellStatHoTCastersLocked(effects []*ActiveEffect, bearer, skip EntityID) {
	for _, part := range statHoTParts(effects, skip) {
		caster := w.Players[part.caster]
		if caster == nil || part.caster == bearer {
			continue // a merc's (no player), or gone
		}
		sess := w.findSessionByEntityIDLocked(caster.ServerID)
		if sess == nil {
			continue
		}
		if id := w.objectIDSeenByLocked(sess, bearer); id != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), part.n, id, part.kind)
		}
	}
}

// tellPlayerStatHoTLocked tells a player its spells' HoT stat at a regen tick
// (the file's doc): from itself on itself, the whole; and each other player
// who cast a part of it, its part. Must be called with the lock held.
func (w *World) tellPlayerStatHoTLocked(p *Player) {
	n, kind := statHoT(p.Effects, p.ServerID)
	if n <= 0 {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendDamageIndication(sess, sess.PlayerObjectID(), n, sess.PlayerObjectID(), kind)
	}
	w.tellStatHoTCastersLocked(p.Effects, p.ServerID, p.ServerID)
}

// mealHoT is what a player's meal and drink add to a regen tick in hit points
// (their Mods.HoT above nothing; statHoT leaves them out).
func mealHoT(effects []*ActiveEffect) (n int) {
	for _, ae := range effects {
		if ae != nil && ae.Mods.HoT > 0 && (ae.SpellID == FoodEffectID || ae.SpellID == DrinkEffectID) {
			n += ae.Mods.HoT
		}
	}
	return n
}

// tellPlayerMealHoTLocked tells a player its meal's and drink's heal over time
// at a regen tick, as its own heal over time's tick (the file's doc). Must be
// called with the lock held.
func (w *World) tellPlayerMealHoTLocked(p *Player) {
	n := mealHoT(p.Effects)
	if n <= 0 {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.dpsWorthLocked(0, func() { w.sendDamageIndication(sess, sess.PlayerObjectID(), n, sess.PlayerObjectID(), numberHoT) }) // food is no healer's (dpsheal.go)
	}
}

// tellPetStatHoTLocked tells a pet's (a merc's, a merc's pet's) owner its
// spells' HoT stat at a regen tick, the whole: from the owner over it; and
// each other player who cast a part of it, its part (the file's doc). Must be
// called with the lock held.
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
	w.tellStatHoTCastersLocked(npc.Effects, npc.ServerID, owner.ServerID)
}
