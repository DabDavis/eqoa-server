package entity

import (
	"log"
)

// petgroupheal.go — a pet's heal of everyone of its side in reach at once (the
// magician Waldorf's petition 1495: "Ghilan Noble should have three heals, the
// large one, small one, and an AOE heal for 150 that heals players and pets in
// range. First two seem to work fine but is missing the third").
//
// A pet's heal landed on its owner, on itself, or on the worse off of the two
// (TemplateAbility.Target owner, self, either). Target "group" is the third:
// cast, as any of its heals, when someone of its side in reach is below the
// ability's threshold, and landing on all of them who are hurt, each at most
// what it is missing. Its side (petSideLocked) is the pet, its owner, the other
// players of its owner's group, and the pets and mercenaries of all of those;
// its reach its spell's range from the pet, petGroupHealReach when the spell
// says none. The petition's "players ... in range" is read as the group's: a
// group heal was the group's in the game, and a heal of every player in range
// would heal a duel's opponent or a PvP foe. Of the group's other players, one
// in a fight that is its own (ownfight.go: a duel, PvP, Mr. Moogie's challenge)
// is left out with its pet; the pet's own owner is healed whatever it fights,
// as its other heals do (a class pet is a duel's own).

// petGroupHealReach is how far from the pet its group heal reaches when its
// spell has no range of its own: a spell's usual 30.
const petGroupHealReach float32 = 30

// petSide is the pet's side in its group heal's reach: players and the pets
// and mercenaries with them.
type petSide struct {
	players []*Player
	pets    []*NPC
}

// petSideLocked is the pet's side within reach of it (the file's doc), alive
// and in its world. Must be called with the lock held.
func (w *World) petSideLocked(ps *petState, reach float32) petSide {
	pet, owner := ps.npc, ps.owner
	var out petSide
	near := func(world uint8, x, y, z float32) bool {
		return world == pet.World && dist3(pet.X, pet.Y, pet.Z, x, y, z) <= reach
	}
	addNPC := func(n *NPC) {
		if n != nil && !n.Dead && n.MaxHP > 0 && near(n.World, n.X, n.Y, n.Z) {
			out.pets = append(out.pets, n)
		}
	}
	addPlayer := func(p *Player) {
		if p == nil || p.IsDead() || p.respawnPending() || (p != owner && w.ownFightLocked(p)) {
			return
		}
		if near(p.World, p.X, p.Y, p.Z) {
			out.players = append(out.players, p)
		}
		if mate := w.pets[p.ServerID]; mate != nil && mate.npc != pet {
			addNPC(mate.npc)
		}
		for _, ms := range w.mercs[p.ServerID] {
			addNPC(ms.npc)
		}
	}
	out.pets = append(out.pets, pet)
	addPlayer(owner)
	if g := w.groups[owner.GroupID]; owner.GroupID != 0 && g != nil {
		for _, id := range g.Members {
			if id != owner.ServerID {
				addPlayer(w.Players[id]) // a merc's place in the group is its owner's to bring (nil here)
			}
		}
	}
	return out
}

// petGroupReach is the reach of a pet's group heal a: its spell's range, else
// petGroupHealReach.
func petGroupReach(ps *petState, a *templateAbility) float32 {
	if s := a.spellFor(petSpellClass(a, ps.npc), ps.npc.Level); s != nil && s.Range > 0 {
		return s.Range
	}
	return petGroupHealReach
}

// petGroupHurtLocked is whether anyone of the pet's side in its group heal's
// reach is below the heal's threshold. Must be called with the lock held.
func (w *World) petGroupHurtLocked(ps *petState, a *templateAbility) bool {
	side := w.petSideLocked(ps, petGroupReach(ps, a))
	for _, p := range side.players {
		if top := p.MaxHPNow(); top > 0 && p.HP*100 < top*a.BelowOr() {
			return true
		}
	}
	for _, n := range side.pets {
		if n.HP*100 < n.MaxHP*a.BelowOr() {
			return true
		}
	}
	return false
}

// petGroupHealLocked lands a pet's group heal of n: each of its side in reach
// healed n, at most what it is missing; a player told the figure from the pet
// (a pet's heal: numbertags.go) when its client is shown the pet, the pet's
// owner the figure on its own pet, and a mercenary's body kept in step. Must be
// called with the lock held.
func (w *World) petGroupHealLocked(ps *petState, spell *CastSpell, n int) {
	if n <= 0 {
		return
	}
	pet := ps.npc
	reach := petGroupHealReach
	if spell.Range > 0 {
		reach = spell.Range
	}
	side := w.petSideLocked(ps, reach)
	healed, total := 0, 0
	for _, p := range side.players {
		top := p.MaxHPNow()
		h := min(n, top-p.HP)
		if h <= 0 {
			continue
		}
		p.HP += h
		p.StatsDirty = true
		healed, total = healed+1, total+h
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			if id := w.objectIDForEntityLocked(sess, pet.ServerID); id != 0 {
				w.sendDamageIndication(sess, id, h, sess.PlayerObjectID(), numberHeal.byPet())
			}
		}
	}
	for _, t := range side.pets {
		h := min(n, t.MaxHP-t.HP)
		if h <= 0 {
			continue
		}
		t.HP += h
		healed, total = healed+1, total+h
		if t == pet {
			w.tellPetHealLocked(ps, h)
		}
		if t.isMerc() {
			syncMercBody(t.merc)
		}
	}
	log.Printf("Pet: %s's %q heals its group with %q: %d healed, %d in all", ps.owner.Name, pet.Name, spell.Name, healed, total)
}
