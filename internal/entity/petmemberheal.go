package entity

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// petmemberheal.go — a pet's heal of the worst off of its side (the user,
// 2026-11-11: "lets allow mage water pets to heal other group/raid members also
// on heirloom server").
//
// A pet's single-target heal landed on its owner, on itself, or on the worse
// off of the two (TemplateAbility.Target owner, self, either). Target "member"
// is a fourth: the one of its side (petSideLocked: itself, its owner, its
// owner's group's other players, a raid group's every member among them, and
// the pets and mercenaries of all of those, none of the group's others in a
// fight of its own) in its spell's reach of the pet (petGroupReach) with the
// lowest share of its hit points, cast when that one is below the ability's
// threshold. Its owner counts wherever it stands, as an owner or either heal
// healed it, and comes first: below the threshold it is healed whoever else is
// worse off (petitions 1819/1820); ties among the rest go to itself, then the
// group's order.
//
// world.pet_heal_group (SetPetHealGroup; the Heirloom Test Server sets it)
// makes a magician's water pet's heals on its owner or on either of the two
// member heals, at the moment it wants and casts them: nothing in its row
// changes, and with the switch off they are as they were. A water pet is one of
// the pet template "pet water" (its pettemplate row's, a workbook pet's base
// among them) or of a petstats row of the element water. A heal of its own
// share (a self heal) stays on itself, and a group heal (target group) on all
// of its side: a raid group's members were its side already (raid.go: a raid
// group is one Group of more than four).

// SetPetHealGroup turns world.pet_heal_group on or off.
func (w *World) SetPetHealGroup(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.petHealGroup = on
}

// PetHealGroup reports world.pet_heal_group.
func (w *World) PetHealGroup() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.petHealGroup
}

// petWaterTemplate is the magician's water pets' pet template.
const petWaterTemplate = "pet water"

// waterPet is whether the pet is a magician's water pet (the file's doc).
func waterPet(ps *petState) bool {
	if ps == nil || ps.npc == nil {
		return false
	}
	if s := ps.npc.sheet; s != nil && strings.EqualFold(s.Element, "water") {
		return true
	}
	if ps.tmpl == nil || ps.tmpl.tmpl == nil {
		return false
	}
	name := strings.ToLower(ps.tmpl.tmpl.Name)
	return name == petWaterTemplate || strings.HasPrefix(name, petWaterTemplate+" (")
}

// petHealTargetLocked is who the pet's ability a lands on: its own target, but
// a water pet's heal on its owner or on either a member heal while
// world.pet_heal_group is on. Must be called with the lock held.
func (w *World) petHealTargetLocked(ps *petState, a *templateAbility) string {
	t := a.TargetOr()
	if w.petHealGroup && a.Kind == database.AbilityHeal && (t == database.AbilityOnOwner || t == database.AbilityOnEither) &&
		waterPet(ps) {
		return database.AbilityOnMember
	}
	return t
}

// petMembersLocked is who a member heal of the pet may land on, in the order a
// tie goes: its owner (alive, wherever it stands), itself, then the rest of its
// side in reach. Must be called with the lock held.
func (w *World) petMembersLocked(ps *petState, reach float32) petSide {
	pet, owner := ps.npc, ps.owner
	side := w.petSideLocked(ps, reach)
	var out petSide
	if owner != nil && !owner.IsDead() && !owner.respawnPending() {
		out.players = append(out.players, owner)
	}
	if !pet.Dead {
		out.pets = append(out.pets, pet)
	}
	for _, p := range side.players {
		if p != owner {
			out.players = append(out.players, p)
		}
	}
	for _, n := range side.pets {
		if n != pet {
			out.pets = append(out.pets, n)
		}
	}
	return out
}

// petMemberPickLocked is the one a member heal a of the pet goes to now: its
// owner (a merc's pet: its merc, in reach) whenever that one is below the
// heal's threshold (petitions 1819/1820,
// the GM Jibrish: "They should always prioritize the mage for heals but
// otherwise use them on other damaged group or raid members"), else the lowest
// share of its hit points among the rest of petMembersLocked below it; 0 for
// none. Must be called with the lock held.
func (w *World) petMemberPickLocked(ps *petState, a *templateAbility) EntityID {
	side := w.petMembersLocked(ps, petGroupReach(ps, a))
	if ms := ps.merc; ms != nil {
		// a merc's pet: the merc is its mage, first when in reach
		for _, n := range side.pets {
			if n == ms.npc && n.MaxHP > 0 && n.HP*100 < n.MaxHP*a.BelowOr() {
				return n.ServerID
			}
		}
	} else if o := ps.owner; o != nil && len(side.players) > 0 && side.players[0] == o {
		if top := o.MaxHPNow(); top > 0 && o.HP*100 < top*a.BelowOr() {
			return o.ServerID // its owner first, however the rest stand
		}
	}
	var best EntityID
	low := 2.0
	try := func(id EntityID, hp, top int) {
		if top <= 0 || hp*100 >= top*a.BelowOr() {
			return
		}
		if share := float64(hp) / float64(top); share < low {
			best, low = id, share
		}
	}
	// players first, then pets: the owner before itself on a tie
	for _, p := range side.players {
		try(p.ServerID, p.HP, p.MaxHPNow())
	}
	for _, n := range side.pets {
		try(n.ServerID, n.HP, n.MaxHP)
	}
	return best
}

// petMemberHealLocked lands a pet's member heal of n on its cast's target, if
// that one is still of its side in reach: at most what it is missing (a share
// heal its share of the target's most hit points), a player told the figure
// from the pet (numbertags.go: a pet's heal), the pet's owner the figure on
// its own pet, a mercenary's body kept in step. Must be called with the lock
// held.
func (w *World) petMemberHealLocked(ps *petState, c *petCast, n int) {
	pet := ps.npc
	side := w.petMembersLocked(ps, petGroupReach(ps, c.a))
	for _, p := range side.players {
		if p.ServerID != c.target {
			continue
		}
		top := p.MaxHPNow()
		if c.a.HealPct > 0 {
			n = petShareAmount(top, c.a.HealPct)
		}
		if n = min(n, top-p.HP); n <= 0 {
			return
		}
		p.HP += n
		p.StatsDirty = true
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			if id := w.objectIDForEntityLocked(sess, pet.ServerID); id != 0 {
				w.sendDamageIndication(sess, id, n, sess.PlayerObjectID(), numberHeal.byPet())
			}
		}
		log.Printf("Pet: %s's %q heals %s with %q for %d (HP %d/%d)", ps.owner.Name, pet.Name, p.Name, c.spell.Name, n, p.HP, top)
		return
	}
	for _, t := range side.pets {
		if t.ServerID != c.target {
			continue
		}
		if c.a.HealPct > 0 {
			n = petShareAmount(t.MaxHP, c.a.HealPct)
		}
		if n = min(n, t.MaxHP-t.HP); n <= 0 {
			return
		}
		t.HP += n
		if t == pet {
			w.tellPetHealLocked(ps, n)
		}
		if t.isMerc() {
			syncMercBody(t.merc)
		}
		log.Printf("Pet: %s's %q heals %s with %q for %d (HP %d/%d)", ps.owner.Name, pet.Name, t.Name, c.spell.Name, n, t.HP, t.MaxHP)
		return
	}
}
