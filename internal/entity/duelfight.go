package entity

import (
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A duel's spells and pets (duel.go): what cast.go and pet.go hand over.

// duelSpellTargetLocked is castTargetsLocked's duel half: a harmful spell at
// p's duel opponent, or at the opponent's pet, is cast at it (a charm never:
// the pet is its owner's). ok false when target is not a duel's.
func (w *World) duelSpellTargetLocked(p *Player, spell *CastSpell, target Entity) (ids []EntityID, refusal string, ok bool) {
	d := &spell.Def
	if !d.Harmful || target == nil || !w.duelFoeLocked(p, target.GetEntityID()) {
		return nil, "", false
	}
	if d.Charms() {
		return nil, castFriendly, true
	}
	x, y, z := target.GetPosition()
	extra := float32(0)
	if npc, isNPC := target.(*NPC); isNPC {
		extra = npcReachExtra(npc)
	}
	if outOfRange(p, x, y, z, spell.Range+extra) {
		return nil, castOutOfRange, true
	}
	return []EntityID{target.GetEntityID()}, "", true
}

// duelLandLocked is landLocked's duel half for a player target: a harmful
// spell on the caster's duel opponent lands as on a creature, each hit at
// DuelDamageShare (duelHurtLocked): its damage (a life tap's take is the
// halved damage), its power taken, and what lasts (a damage over time
// halved at each tick, a stat taken, a hold or a slow) put on the opponent
// as the duel's (duelFoe), off it at the duel's end. A harmful spell on a
// player whose duel is over (a cast that outlived it) lands on nothing. It
// reports whether the spell was a harmful one, handled here.
func (w *World) duelLandLocked(cs *castState, caster *Player, casterSess *session.Session, t *Player, now time.Time) bool {
	if !cs.spell.Def.Harmful {
		return false
	}
	d := w.duelOnLocked(caster.ServerID)
	if d == nil || d.other(caster.ServerID) != t.ServerID || t.IsDead() {
		log.Printf("Duel: %s's %q on %s lands on nothing: no duel between them", caster.Name, cs.spell.Name, t.Name)
		return true
	}
	caster.LastCombatTime = now
	for _, e := range cs.spell.Def.Effects {
		switch e.Kind {
		case EffectHP:
			if n := spellAmount(e, caster); n < 0 {
				if e.Drain != DrainNone {
					w.drainLocked(cs, caster, casterSess, e, duelHalf(-n), now)
				}
				if w.duelHurtLocked(d, caster, caster.ServerID, t, -n, now) {
					return true // the duel is over: nothing more of it lands
				}
			}
		case EffectPower:
			if n := spellAmount(e, caster); n < 0 {
				take := min(duelHalf(-n), t.Power)
				t.Power -= take
				t.StatsDirty = true
				if e.Drain != DrainNone {
					w.drainLocked(cs, caster, casterSess, e, take, now)
				}
			}
		}
	}
	if ae := lasting(cs, caster, now); ae != nil {
		ae.duelFoe = caster.ServerID
		// what lasts of a harmful spell is its harm: a heal over time or a
		// stat raised in it is not the opponent's to have
		ae.HPTick = min(ae.HPTick, 0)
		ae.PowerTick = min(ae.PowerTick, 0)
		w.addEffectLocked(t, ae, now)
	}
	d.struck[t.ServerID] = true
	log.Printf("Duel %d: %s's %q lands on %s (%d/%d HP)", d.ID, caster.Name, cs.spell.Name, t.Name, t.HP, t.MaxHPNow())
	return true
}

// duelLandPetLocked is landLocked's duel half for a pet: a harmful spell on
// the opponent's pet lands its damage, halved (duelHurtPetLocked); what would
// last on it does not (a pet's effects are its owner's spells). It reports
// whether the spell was the duel's, handled here.
func (w *World) duelLandPetLocked(cs *castState, caster *Player, pet *NPC, now time.Time) bool {
	if !cs.spell.Def.Harmful {
		return false
	}
	d := w.duelOnLocked(caster.ServerID)
	if d == nil || pet.Owner != d.other(caster.ServerID) || d.petOut[pet.Owner] {
		log.Printf("Duel: %s's %q on %s lands on nothing: no duel with its owner", caster.Name, cs.spell.Name, pet.Name)
		return true
	}
	caster.LastCombatTime = now
	for _, e := range cs.spell.Def.Effects {
		if e.Kind != EffectHP {
			continue
		}
		if n := spellAmount(e, caster); n < 0 {
			w.duelHurtPetLocked(d, caster, pet, -n, now)
			if d.petOut[pet.Owner] {
				return true
			}
		}
	}
	return true
}

// duelTickLocked is a damage over time's tick on t (cast.go's
// tickEffectLocked): the duel's, from its opponent, halved; anything else's
// (a duel since ended) nothing.
func (w *World) duelTickLocked(t *Player, ae *ActiveEffect, now time.Time) {
	if ae.duelFoe == 0 || ae.HPTick >= 0 {
		return
	}
	d := w.duelOnLocked(t.ServerID)
	caster := w.Players[ae.duelFoe]
	if d == nil || caster == nil || d.other(t.ServerID) != ae.duelFoe {
		return
	}
	w.duelHurtLocked(d, caster, caster.ServerID, t, -ae.HPTick, now)
}

// duelPetLocked is petSystem's duel half: a duelist's pet fights the
// opponent (or the opponent's pet) by its stance as it would a creature
// (pet.go): aggressive once its owner fights, defensive once the opponent
// struck its owner or it, neutral once the opponent struck it, passive only
// when told (0x0041 8: duelPetCommandLocked); every hit duel damage. A pet
// left at a hit point is out. It reports whether the pet fought the duel this
// tick (its own fights and following wait).
func (w *World) duelPetLocked(ps *petState, dt float32, now time.Time) bool {
	owner, pet := ps.owner, ps.npc
	d := w.duelOnLocked(owner.ServerID)
	if d == nil || d.petOut[owner.ServerID] || pet.Charm != nil {
		return false
	}
	foe := d.other(owner.ServerID)
	if !d.pets[owner.ServerID] && !now.Before(ps.heldUntil) {
		join := false
		switch ps.stance {
		case petAggressive:
			join = d.struck[owner.ServerID] || (owner.Attacking() && w.duelFoeLocked(owner, owner.AttackTarget))
		case petDefensive:
			join = d.struck[owner.ServerID]
		case petNeutral:
			join = d.petStruck[owner.ServerID]
		}
		if join {
			d.pets[owner.ServerID] = true
			if owner.AttackTarget != 0 && w.duelFoeLocked(owner, owner.AttackTarget) {
				d.petTarget[owner.ServerID] = owner.AttackTarget
			} else {
				d.petTarget[owner.ServerID] = foe
			}
			log.Printf("Duel %d: %s's pet %q joins the duel", d.ID, owner.Name, pet.Name)
		}
	}
	if !d.pets[owner.ServerID] {
		return false
	}
	target := d.petTarget[owner.ServerID]
	if !w.duelFoeLocked(owner, target) {
		target = foe
		d.petTarget[owner.ServerID] = foe
	}
	var tx, ty, tz float32
	level := 0
	if t := w.Players[target]; t != nil {
		tx, ty, tz, level = t.X, t.Y, t.Z, t.Level
	} else if tp := w.NPCs[target]; tp != nil {
		tx, ty, tz, level = tp.X, tp.Y, tp.Z, tp.Level
	} else {
		return false
	}
	if distXZ(owner.X, owner.Z, tx, tz) > petLeash {
		return false // too far from its owner: it follows instead
	}
	ps.target = 0
	dist := distXZ(pet.X, pet.Z, tx, tz)
	if dist > petReach {
		x := tx + (pet.X-tx)/dist*(petReach-1)
		z := tz + (pet.Z-tz)/dist*(petReach-1)
		pet.Y = ty
		w.stepNPCToward(pet, x, z, max(owner.SpeedNow(), petRunSpeed), dt)
		return true
	}
	pet.stopMoving()
	pet.Facing = headingTo(tx-pet.X, tz-pet.Z)
	length, delay, fx, hits := npcRound(pet)
	if pet.RoundTarget != target {
		pet.endRound()
		pet.RoundTarget = target
	}
	if pet.RoundStart.IsZero() || !now.Before(pet.RoundStart.Add(length)) {
		next := pet.RoundStart.Add(length)
		if pet.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		pet.RoundStart, pet.RoundHits = next, 0
		breakNPCInvis(pet)
		w.broadcastNPCSwingLocked(pet, target, uint32(delay/time.Millisecond), fx)
	}
	for pet.RoundHits < len(hits) && !now.Before(pet.RoundStart.Add(hits[pet.RoundHits])) {
		pet.RoundHits++
		pet.LastCombatTime = now
		n := petMeleeHit(pet, duelStandIn(level), rand.Intn)
		if t := w.Players[target]; t != nil {
			if w.duelHurtLocked(d, owner, pet.ServerID, t, n, now) {
				return true
			}
			continue
		}
		if tp := w.NPCs[target]; tp != nil {
			w.duelHurtPetLocked(d, owner, tp, n, now)
			if d.petOut[tp.Owner] {
				d.petTarget[owner.ServerID] = foe
				pet.endRound()
				return true
			}
		}
	}
	return true
}

// duelPetCommandLocked is PetCommand's attack (0x0041 8) at the owner's duel
// opponent or its pet: the pet joins the duel and fights it. It reports
// whether the target was the duel's.
func (w *World) duelPetCommandLocked(ps *petState, target Entity) bool {
	owner := ps.owner
	if target == nil || !w.duelFoeLocked(owner, target.GetEntityID()) {
		return false
	}
	d := w.duelOnLocked(owner.ServerID)
	if d.petOut[owner.ServerID] {
		return false
	}
	d.pets[owner.ServerID] = true
	d.petTarget[owner.ServerID] = target.GetEntityID()
	ps.target = 0
	ps.heldUntil = time.Time{}
	return true
}
