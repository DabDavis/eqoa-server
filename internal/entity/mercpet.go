package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// mercpet.go — a merc's own pet (the owner, 2026-11-11: "let mercenaries cast
// their pets"). A merc whose book holds a summoning (a magician's elementals,
// a necromancer's bones and flesh, an enchanter's animations and golems, a
// shaman's spirits, a wizard's familiars: the classes whose books have one)
// casts its best (mercPetSpell: the highest level of its book's, the dearest
// of those) as a player of its class does: its cast time, its power, its
// lockout (merccast.go's mercCastLocked), and its landing makes the pet a
// player's casting of it would (pet.go's makePetLocked: its body, its level
// the spell's at most the merc's, its workbook sheet and template), beside
// the merc.
//
// Our choices, the owner's brief's:
//   - the pet is the merc's (petState.merc, NPC.mercPet), not its hirer's
//     class pet: nothing of CLIENT's pet slot is its (no 0x007E, no 0x0041, no
//     non-collide entry), the hirer's own pet stays out beside it, and no
//     player command reaches it. Its NPC is owned by the hirer (NPC.Owner), as
//     the merc's is, so the server keeps it on the players' side as every pet:
//     no player's hit or harmful spell, no guard's, a creature fights it as a
//     pet (npcFightPetLocked);
//   - it assists the merc and its hirer, by the merc's stance
//     (mercPetChooseTargetLocked): its hirer's auto-attack's target, the
//     merc's, what fights it, the merc or the hirer or the hirer hurt; a
//     passive merc's only what the merc was told to attack. It follows the
//     merc out of a fight; never a player, never a duel's or Mr.
//     Moogie's (mercMayFight), so it is in no duel, no PvP fight and no
//     challenge (ownfight.go; duel.go leaves it out as it does the merc);
//   - a kill of its is the hirer's, as the merc's (pet.go's petLandHitLocked:
//     the players' experience split, no share of its own), its figures the
//     hirer's pet's;
//   - it takes no group seat (it is no member: mercgroup.go is the merc's);
//   - it dies as a pet dies; the merc summons another once its spell's
//     lockout from the last cast is over (its recast);
//   - it goes (seen dying, as a dismissed pet: petDiesLocked) whenever the
//     merc goes: dismissed, dead, its hirer's logout, a new hire.
//   - a summoning's reagent is not asked of a merc (it carries nothing; no
//     summoning of a class's book on this server has one).

// The line a merc's hirer is told when the merc's pet dies (ours).
const mercPetDiedLine = "%s's pet has died."

// isMercPet reports a merc's own pet (mercpet.go).
func (n *NPC) isMercPet() bool { return n != nil && n.mercPet != nil }

// petStateOfNPCLocked is the pet's state: its owner's class pet's, or a merc's
// pet's; nil for an NPC that is neither (or no longer). Must be called with
// the lock held.
func (w *World) petStateOfNPCLocked(pet *NPC) *petState {
	if pet == nil {
		return nil
	}
	if ps := w.pets[pet.Owner]; ps != nil && ps.npc == pet {
		return ps
	}
	if ms := pet.mercPet; ms != nil && ms.pet != nil && ms.pet.npc == pet {
		return ms.pet
	}
	return nil
}

// petLiveLocked reports the pet still out: its owner's pet, or its merc's
// while the merc is out. Must be called with the lock held.
func (w *World) petLiveLocked(ps *petState) bool {
	if ms := ps.merc; ms != nil {
		return ms.pet == ps && w.mercLiveLocked(ms)
	}
	return w.pets[ps.owner.ServerID] == ps
}

// petMasterLocked is the player a pet's figures go by (the balance's level and
// class): a merc's pet's merc's body, any other's owner. Must be called with
// the lock held.
func (w *World) petMasterLocked(npc *NPC) *Player {
	if ms := npc.mercPet; ms != nil {
		return ms.body
	}
	return w.Players[npc.Owner]
}

// mercPetSpell is the summoning the merc casts: the highest level of its
// book's (a pet's, not a charm's, a familiar item's or a trip's), the dearest
// of those, the highest id of those; nil for a book with none. Its hirer's
// pick of a kind (MercKit.PetPick, /merc pet: petition 1564) takes the best of
// that kind's, and the best of all when its book has none of it yet.
func (ms *mercState) mercPetSpell() *CastSpell {
	if s := ms.mercPetSpellOf(ms.kit.PetPick); s != nil {
		return s
	}
	return ms.mercPetSpellOf(nil)
}

// MercPetFor is the summoning a merc of the kit casts (mercPetSpell), nil for
// none: the handlers' real-data test's.
func MercPetFor(kit MercKit) *CastSpell {
	return (&mercState{kit: kit}).mercPetSpell()
}

// MercPetKindOf reports whether a summoning's name is of a kind: holding one
// of its words (lower case); any summoning, for no words.
func MercPetKindOf(name string, words []string) bool {
	if len(words) == 0 {
		return true
	}
	name = strings.ToLower(name)
	for _, w := range words {
		if strings.Contains(name, w) {
			return true
		}
	}
	return false
}

// mercPetSpellOf is mercPetSpell's best of the book's summonings of the kind
// whose words are given (MercPetKindOf), nil for none.
func (ms *mercState) mercPetSpellOf(words []string) *CastSpell {
	var best *CastSpell
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if !s.Def.Summons() || s.Def.Charms() || s.Familiar != nil || s.Travel != nil || !MercPetKindOf(s.Name, words) {
			continue
		}
		if best == nil || s.Level > best.Level || s.Level == best.Level && (s.PowerCost > best.PowerCost ||
			s.PowerCost == best.PowerCost && s.SpellID > best.SpellID) {
			best = s
		}
	}
	return best
}

// mercPetOutLocked is the merc's pet alive and out, or nil. Must be called
// with the lock held.
func (w *World) mercPetOutLocked(ms *mercState) *petState {
	ps := ms.pet
	if ps == nil || ps.npc.Dead || w.NPCs[ps.npc.ServerID] != ps.npc {
		return nil
	}
	return ps
}

// mercSummonTurnLocked begins the merc's summoning when it has no pet out and
// its book has one ready (its lockout over, its power). It reports a cast
// begun. Must be called with the lock held.
func (w *World) mercSummonTurnLocked(ms *mercState, now time.Time) bool {
	if w.mercPetOutLocked(ms) != nil {
		return false
	}
	for _, c := range w.mercCasts {
		if c.ms == ms && c.life == ms.life && c.spell.Def.Summons() {
			return false // one on its way
		}
	}
	s := ms.mercPetSpell()
	if s == nil || !ms.mercReady(s, now) {
		return false
	}
	w.mercCastLocked(ms, s, ms.npc.ServerID, false, now)
	return true
}

// mercSummonLandLocked lands the merc's summoning: its pet (the file's doc)
// beside it, in place of one it has. Must be called with the lock held.
func (w *World) mercSummonLandLocked(ms *mercState, s *CastSpell, now time.Time) {
	syncMercBody(ms) // its body where the merc stands
	ps := w.makePetLocked(s, ms.body, ms.owner, ms, now)
	if ps == nil {
		return
	}
	w.mercPetGoesLocked(ms, now)
	ps.stance = petDefensive
	pet := ps.npc
	w.putNPCLocked(pet)
	w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	ms.pet = ps
	log.Printf("Merc: %s's %q summons its pet %q (level %d, %d HP)", ms.owner.Name, ms.npc.Name, pet.Name, pet.Level, pet.MaxHP)
}

// mercPetGoesLocked takes the merc's pet away, seen dying where it stands
// (petDiesLocked), out of every fight. Must be called with the lock held.
func (w *World) mercPetGoesLocked(ms *mercState, now time.Time) {
	ps := ms.pet
	if ps == nil {
		return
	}
	if ps.npc.Charm != nil {
		w.mercCharmReleaseLocked(ms, false) // a charmed one is let go where it stands (mercharm.go)
		return
	}
	ms.pet = nil
	pet := ps.npc
	w.dropAggroLocked(pet.ServerID)
	if !pet.Dead && w.NPCs[pet.ServerID] == pet {
		w.petDiesLocked(pet, now)
	}
	log.Printf("Merc: %s's %q's pet %q goes", ms.owner.Name, ms.npc.Name, pet.Name)
}

// mercPetTurnLocked is the merc's pet's turn: a dead one forgotten (its hirer
// told), then, when fight, its target (mercPetChooseTargetLocked) fought as
// a pet fights; else it follows the merc (its
// template's heals and buffs while it does). With fight false (its merc
// standing down: its hirer down, out of its group) it stands where it is.
// Must be called with the lock held.
func (w *World) mercPetTurnLocked(ms *mercState, sess *session.Session, fight bool, dt float32, now time.Time) {
	ps := ms.pet
	if ps == nil {
		return
	}
	pet, m := ps.npc, ms.npc
	if pet.Dead || w.NPCs[pet.ServerID] != pet {
		ms.pet = nil
		if pet.Charm != nil {
			pet.mercPet = nil // a charmed creature's corpse, its charm's hold kept to its return (corpse.go)
		}
		w.dropAggroLocked(pet.ServerID)
		if pet.Dead && sess != nil {
			w.sendChatToSession(sess, fmt.Sprintf(mercPetDiedLine, m.Name))
		}
		log.Printf("Merc: %s's %q's pet %q is gone (dead %v)", ms.owner.Name, m.Name, pet.Name, pet.Dead)
		return
	}
	if w.mercCharmTurnLocked(ms, sess, now) { // a charm's end (mercharm.go)
		return
	}
	ps.last = now
	w.petEffectsLocked(ps, now)
	if pet.World != m.World || distXZ(pet.X, pet.Z, m.X, m.Z) > petLost {
		// its merc taken along (a teleport, another world): it comes too
		oldX, oldZ := pet.X, pet.Z
		pet.World = m.World
		pet.X, pet.Y, pet.Z = w.followPlaceLocked(pet, ms.body, slotMercPet)
		pet.stopMoving()
		ps.target = 0
		pet.endRound()
		w.SpatialIndex.Update(pet.ServerID, oldX, oldZ, pet.X, pet.Z)
	}
	if !fight {
		ps.target = 0
		pet.endRound()
		pet.stopMoving()
		return
	}
	w.mercPetChooseTargetLocked(ms, ps)
	if t := w.NPCs[ps.target]; ps.target != 0 && t != nil {
		w.petFightLocked(ps, t, dt, now)
		return
	}
	ps.target = 0
	if !w.petTemplateTurnLocked(ps, nil, now) { // its heals and buffs while it follows (pettemplate.go)
		w.followLocked(pet, ms.body, slotMercPet, dt, now)
	} else {
		pet.stopMoving()
	}
}

// mercPetChooseTargetLocked gives the merc's pet its target by its merc's
// stance (the owner, 2026-11-11: "the mercenary pets should attack when player
// or merc attacks"): a passive merc's pet only what its merc was told to
// attack (/merc attack: the merc's target), nothing of its own; any other's,
// in turn, its hirer's auto-attack's target (so it switches when its hirer
// starts on another), its merc's target, the one it fights while that still
// fights it or its merc or is still called, else the nearest creature within
// petAssist of its hirer its hirer called his mercs onto (GM ticket 1959,
// merccall.go: a hostile act of his, or a help on one it fights) or fighting
// the pet or its merc (self-defence). Never one only because it fights its
// hirer's group or raid, nor by an aggressive stance. A creature past
// petLeash from the hirer is none of its, as the merc's. Must be called with
// the lock held.
func (w *World) mercPetChooseTargetLocked(ms *mercState, ps *petState) {
	pet, m, owner := ps.npc, ms.npc, ms.owner
	valid := func(id EntityID) *NPC {
		t := w.NPCs[id]
		if id == 0 || t == nil || !mercMayFight(pet, t) || distXZ(owner.X, owner.Z, t.X, t.Z) > petLeash {
			return nil
		}
		return t
	}
	set := func(t *NPC) {
		id := EntityID(0)
		if t != nil {
			id = t.ServerID
		}
		if ps.target != id {
			ps.target = id
			pet.endRound()
		}
	}
	if ms.stance == MercPassive {
		set(valid(ms.target))
		return
	}
	if owner.Attacking() {
		if t := valid(owner.AttackTarget); t != nil {
			set(t)
			return
		}
	}
	if t := valid(ms.target); t != nil {
		set(t)
		return
	}
	// its hirer's call (merccall.go), or what fights the pet or its merc
	// (self-defence: GM ticket 1959)
	mine := func(npc *NPC) bool {
		return npc.CurrentEnemy == pet.ServerID || npc.CurrentEnemy == m.ServerID || w.mercCalledLocked(owner, npc)
	}
	if t := valid(ps.target); t != nil && mine(t) {
		return // still its fight
	}
	var best *NPC
	var bestD float32
	for _, id := range w.SpatialIndex.QueryRadius(owner.X, owner.Z, petAssist) {
		npc := valid(id)
		if npc == nil || !mine(npc) {
			continue
		}
		if d := distXZ(pet.X, pet.Z, npc.X, npc.Z); best == nil || d < bestD {
			best, bestD = npc, d
		}
	}
	set(best)
}
