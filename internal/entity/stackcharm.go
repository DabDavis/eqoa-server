package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// stackcharm.go — a racial charm stacks (the owner, 2026-11-15: "Looks like
// having more than 1 pet isnt working", then "the kerran racial should stack
// only"). The Kerran's Cat Magnetism (spell 760303, the racial rank 1972: "an
// animal up to level 50 follows the khajii for 45 minutes") is a charm, and
// until now a charm was one pet at a time (charm.go: refused with a charmed pet
// out, a summoned pet dismissed as it took hold: petitions 1348, 1351). A
// charm a racial rank adds (racialSpellLocked: on this server Cat Magnetism
// alone) now takes its animal as an EXTRA companion:
//
//   - never refused for the pet out, summoned or charmed, and never taking its
//     place: a Kerran necromancer keeps his skeleton, a Kerran enchanter her
//     Entrancing charm, and the animal comes too. Nor does the animal refuse
//     or replace anything: with it out, any charm or summoning goes as if it
//     were not there. A second Cat Magnetism lets the first animal go (quietly,
//     not turned on its master) and takes the new one; one resisted leaves it.
//   - CLIENT's pet window and commands (0x007E, 0x0041: one m_petId) stay the
//     pet's. With no other pet out the animal IS the pet (World.pets, the bar,
//     the stances, dismiss); when a pet comes (a summoning, another charm) the
//     animal steps aside (sideStepLocked) to World.sidePets, off the bar, its
//     non-collide entry its own (sidePetNoCollideID); when that pet goes, the
//     animal is the pet again (sidePromoteLocked). So a player always has the
//     bar on one of them, and there is never more than one such animal.
//   - beside the pet it follows its owner in a slot of its own (slotSidePet)
//     and fights by itself as an aggressive pet does (petChooseTargetLocked:
//     its owner's auto-attack target, what fights its owner, it, the group),
//     else what the pet is fighting (the pet's attack command reaches it so);
//     the pet's passive keeps it to the pet's fight alone, the pet's back off
//     backs it off too. It is in no duel and no PvP fight (as a merc's pet).
//   - everything else is the charm's (charm.go): its time and CHA share, its
//     grace and leash, a coach breaking it, it let go when its master dies or
//     logs out, its death ending the charm (no loot, back on its spawn), its
//     hits and kills its master's (petLandHitLocked), its hate falling to its
//     master (pethate.go), group heals and the group's pets' fights counting it
//     (petgroupheal.go, groupPetsFight). A restart keeps it beside the pet
//     (keeppet.go: KeptPlayer.SidePet).
// Other charms keep their one-pet rules.

// sidePetNoCollideID is the animal's entry on its owner's non-collide list,
// past the pet's, the mercs' (1, 3, 4) and the familiar's (2).
const sidePetNoCollideID uint32 = petNoCollideID + 5

// The lines (ours).
const (
	// stackFreed follows the previous animal's name as a new one is taken
	stackFreed = " goes free as your charm takes another."
	// stackAside follows the animal's name as a pet comes and it steps aside
	stackAside = " stays at your side."
	// stackDied and stackWornOff are the animal's end beside a pet
	stackDied    = "Your charmed %s has died."
	stackWornOff = "Your charm on %s has worn off."
)

// noCollideID is the pet's entry on its owner's non-collide list.
func (ps *petState) noCollideID() uint32 {
	if ps.side {
		return sidePetNoCollideID
	}
	return petNoCollideID
}

// stackingCharmLocked reports a charm that stacks: one a racial rank adds (the
// file's doc). Must be called with the lock held.
func (w *World) stackingCharmLocked(spell *CastSpell) bool {
	return spell != nil && spell.Def.Charms() && w.racialSpellLocked(spell.SpellID)
}

// charmBlockedLocked reports a charm p may not cast: a charm that does not
// stack while a charmed pet out that no stacking charm took (charm.go's one
// charmed pet at a time). Must be called with the lock held.
func (w *World) charmBlockedLocked(p *Player, spell *CastSpell) bool {
	if !spell.Def.Charms() || w.stackingCharmLocked(spell) {
		return false
	}
	ps := w.petStateLocked(p)
	return ps != nil && ps.npc.Charm != nil && !ps.stacks
}

// newCharmHold is what the charm changes on npc, for caster's charm: its time
// with its caster's CHA share (charmDuration).
func newCharmHold(caster *Player, npc *NPC, spell *CastSpell, now time.Time) *charmHold {
	hold := &charmHold{npcType: npc.NPCType, faction: npc.Faction, world: npc.World, spell: spell.Name}
	if d := charmDuration(spell, caster.StatNow(StatCha)); d > 0 {
		hold.until = now.Add(d)
	}
	return hold
}

// stackCharmLandLocked is a stacking charm taking npc (its resist passed):
// the animal an earlier one took let go, then npc the pet when none is out,
// else beside it. Must be called with the lock held.
func (w *World) stackCharmLandLocked(caster *Player, sess *session.Session, npc *NPC, spell *CastSpell, now time.Time) {
	id := caster.ServerID
	for _, old := range []*petState{w.sidePets[id], w.pets[id]} {
		if old == nil || !old.stacks || old.npc.Charm == nil {
			continue
		}
		tell, line := sess, ""
		switch {
		case old.owner != caster:
			tell = nil // an earlier login's
		case !old.npc.Dead:
			line = old.npc.Name + stackFreed
		}
		log.Printf("Charm: %s's %q lets %q go for %q", caster.Name, spell.Name, old.npc.Name, npc.Name)
		w.releaseCharmLocked(old, tell, line, false)
	}
	hold := newCharmHold(caster, npc, spell, now)
	if w.petStateLocked(caster) == nil {
		if old := w.pets[id]; old != nil {
			// a dead one's or an earlier login's (as charmLandLocked)
			tell := sess
			if old.owner != caster {
				tell = nil
			}
			w.dismissPetLocked(old, tell, "", now)
		}
		w.holdCharmLocked(caster, sess, npc, hold, now)
		w.pets[id].stacks = true
		return
	}
	w.holdSideLocked(caster, sess, npc, hold, now)
}

// holdSideLocked makes npc caster's animal beside its pet under hold: a
// stacking charm's landing, or one a restart carried (keeppet.go). Must be
// called with the lock held.
func (w *World) holdSideLocked(caster *Player, sess *session.Session, npc *NPC, hold *charmHold, now time.Time) {
	w.takeCharmedLocked(caster, npc, hold)
	ps := &petState{owner: caster, npc: npc, stance: petAggressive, last: now, stacks: true}
	ps.tmpl = w.charmTemplateLocked(npc)
	w.putSideLocked(ps, sess)
	lasts := "until it is let go"
	if !hold.until.IsZero() {
		lasts = "for " + hold.until.Sub(now).String()
	}
	log.Printf("Charm: %s's %q takes %s (level %d, %d/%d HP) beside its pet, %s", caster.Name, hold.spell, npc.Name, npc.Level,
		npc.HP, npc.MaxHP, lasts)
}

// putSideLocked keeps ps beside its owner's pet (World.sidePets), an animal
// there before let go. Must be called with the lock held.
func (w *World) putSideLocked(ps *petState, sess *session.Session) {
	id := ps.owner.ServerID
	if old := w.sidePets[id]; old != nil && old != ps {
		w.releaseCharmLocked(old, sess, "", false)
	}
	if w.sidePets == nil {
		w.sidePets = make(map[EntityID]*petState)
	}
	ps.side, ps.stance = true, petAggressive
	ps.noCollide, ps.told = false, 0
	w.sidePets[id] = ps
	if sess != nil {
		w.announceSideLocked(ps, sess)
	}
}

// sideStepLocked moves the pet old off the bar to beside the pet coming in
// its place, when a stacking charm took it (its owner caster's, alive and
// charmed), and reports whether it did; any other is the caller's to
// dismiss. Must be called with the lock held.
func (w *World) sideStepLocked(old *petState, caster *Player, sess *session.Session) bool {
	if !old.stacks || old.side || old.owner != caster || old.npc.Dead || old.npc.Charm == nil {
		return false
	}
	id := caster.ServerID
	if w.pets[id] == old {
		delete(w.pets, id)
	}
	if sess != nil {
		sendClearPet(sess) // the bar takes the new pet's name (its 0x007E follows)
		if old.noCollide {
			sendNoCollideRemove(sess, petNoCollideID)
		}
		w.sendChatToSession(sess, old.npc.Name+stackAside)
	}
	old.target = 0
	old.npc.endRound()
	w.putSideLocked(old, sess)
	log.Printf("Charm: %s's %q steps beside the pet coming", caster.Name, old.npc.Name)
	return true
}

// sidePromoteLocked makes the animal beside no pet any longer the pet (the
// bar, the stances, dismiss). Must be called with the lock held.
func (w *World) sidePromoteLocked(ps *petState, sess *session.Session) {
	id := ps.owner.ServerID
	delete(w.sidePets, id)
	if sess != nil && ps.noCollide {
		sendNoCollideRemove(sess, sidePetNoCollideID)
	}
	ps.side, ps.noCollide, ps.told = false, false, 0
	ps.stance = petDefensive // a new pet's
	if w.pets == nil {
		w.pets = make(map[EntityID]*petState)
	}
	w.pets[id] = ps
	if sess != nil {
		w.announcePetLocked(ps, sess)
	}
	log.Printf("Charm: %s's %q is its pet again", ps.owner.Name, ps.npc.Name)
}

// announceSideLocked puts the animal on its owner's non-collide list once.
func (w *World) announceSideLocked(ps *petState, sess *session.Session) {
	if !ps.noCollide {
		sendNoCollideAdd(sess, ps.npc.Name, sidePetNoCollideID)
		ps.noCollide = true
	}
}

// isSidePetLocked reports npc an animal beside a pet. Must be called with the
// lock held.
func (w *World) isSidePetLocked(npc *NPC) bool {
	if npc == nil || npc.Owner == 0 {
		return false
	}
	ps := w.sidePets[npc.Owner]
	return ps != nil && ps.npc == npc
}

// sideBackOffLocked backs the animal beside p's pet off with the pet (0x0041
// 7). Must be called with the lock held.
func (w *World) sideBackOffLocked(p *Player, until time.Time) {
	if ps := w.sidePets[p.ServerID]; ps != nil && ps.owner == p {
		ps.target, ps.heldUntil = 0, until
		ps.npc.endRound()
	}
}

// sidePetSystem keeps or ends each animal beside a pet (the file's doc): let
// go as a charmed pet is (its owner gone, dead; its charm's time, grace), the
// pet again when its owner's pet is gone, else following and fighting. Every
// tick after petSystem; must be called with the lock held.
func (w *World) sidePetSystem(now time.Time) {
	for id, ps := range w.sidePets {
		owner, pet := ps.owner, ps.npc
		if w.Players[id] != owner {
			w.releaseCharmLocked(ps, nil, "", false) // logged out: nobody to tell
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if pet.Charm == nil {
			delete(w.sidePets, id) // let go otherwise (a GM's hand)
			continue
		}
		if pet.Dead {
			w.releaseCharmLocked(ps, sess, fmt.Sprintf(stackDied, pet.Name), false)
			continue
		}
		if owner.IsDead() || owner.respawnPending() {
			w.releaseCharmLocked(ps, sess, "", false)
			continue
		}
		h := pet.Charm
		if !h.until.IsZero() && !now.Before(h.until) {
			w.releaseCharmLocked(ps, sess, fmt.Sprintf(stackWornOff, pet.Name), true)
			continue
		}
		if charmFar(pet, h.world, pet.World, pet.X, pet.Z) {
			w.releaseCharmLocked(ps, sess, charmTooFar, false)
			continue
		}
		if w.pets[id] == nil {
			w.sidePromoteLocked(ps, sess) // the pet gone: the bar's again (petSystem's from the next tick)
			continue
		}
		dt := float32(now.Sub(ps.last).Seconds())
		ps.last = now
		dt = min(max(dt, 0), 0.5)
		w.petEffectsLocked(ps, now)
		if pet.World != owner.World || distXZ(pet.X, pet.Z, owner.X, owner.Z) > petLost {
			if charmFar(pet, h.world, owner.World, owner.X, owner.Z) {
				w.releaseCharmLocked(ps, sess, fmt.Sprintf(stackWornOff, pet.Name), false)
				continue
			}
			oldX, oldZ := pet.X, pet.Z
			pet.World = owner.World
			pet.X, pet.Y, pet.Z = w.followPlaceLocked(pet, owner, slotSidePet)
			pet.stopMoving()
			ps.target = 0
			pet.endRound()
			w.SpatialIndex.Update(pet.ServerID, oldX, oldZ, pet.X, pet.Z)
		}
		w.sideChooseTargetLocked(ps, now)
		if t := w.NPCs[ps.target]; ps.target != 0 && t != nil {
			w.petFightLocked(ps, t, dt, now)
		} else {
			ps.target = 0
			if !w.petTemplateTurnLocked(ps, nil, now) {
				w.followLocked(pet, owner, slotSidePet, dt, now)
			} else {
				pet.stopMoving()
			}
		}
		if sess != nil {
			w.announceSideLocked(ps, sess)
		}
	}
}

// sideChooseTargetLocked gives the animal its fight (the file's doc): with
// the pet passive only the pet's; else an aggressive pet's choice, else the
// pet's fight. Must be called with the lock held.
func (w *World) sideChooseTargetLocked(ps *petState, now time.Time) {
	owner, pet := ps.owner, ps.npc
	main := w.petStateLocked(owner)
	mainFight := func() EntityID {
		if main == nil || main.target == 0 {
			return 0
		}
		t := w.NPCs[main.target]
		if t == nil || !petMayFight(pet, t) || distXZ(owner.X, owner.Z, t.X, t.Z) > petLeash {
			return 0
		}
		return t.ServerID
	}
	if main != nil && main.stance == petPassive {
		if id := mainFight(); id != ps.target {
			ps.target = id
			pet.endRound()
		}
		return
	}
	ps.stance = petAggressive
	w.petChooseTargetLocked(ps, now)
	if ps.target == 0 && !now.Before(ps.heldUntil) {
		ps.target = mainFight()
	}
}
