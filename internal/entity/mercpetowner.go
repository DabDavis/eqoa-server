package entity

import (
	"log"
	"time"
)

// mercpetowner.go — whom a merc's pet's abilities "on its owner" go to (Heirloom
// petition 3133, a shadow knight with a wizard merc: "I had a 5minute Familiar
// heal on me the character? If thats from the WIZ pet, that shouldn't be").
//
// A pet's template ability of the target owner (a heal of its owner, or its
// owner or itself, the worse off; a buff on its owner) is for the one who
// summoned it: a wizard's familiar's Familiar Heal and Familiar Blessing ("A
// buff that increases its owner's maximum hit points", the workbook's "Pet
// Heal" and "Pet Buff" of target Owner), the Undead Servant's Servitude on its
// shadow knight, Ghilan's Blessing on her magician. A merc's pet's petState
// owner is the merc's hirer (mercpet.go: its kills, its NPC.Owner), so those
// went on the hirer: the merc's Arch Familiar cast Familiar Heal (55) and
// Familiar Blessing (55) on the shadow knight who hired the wizard. They are
// its merc's now, the caster of its summoning, as a player's pet's are its
// player's; a member heal already put a merc's pet's merc first
// (petmemberheal.go). Nothing of a player's own pet changes.

// petOwnerMate is whom the pet's abilities on its owner land on: a merc's pet's
// merc, any other pet's owner.
func (ps *petState) petOwnerMate() mercMate {
	if ms := ps.merc; ms != nil && ms.npc != nil {
		return mercMate{n: ms.npc}
	}
	return mercMate{p: ps.owner}
}

// alive reports the mate alive (a player not dead, an NPC not dead).
func (mm mercMate) alive() bool {
	if mm.p != nil {
		return !mm.p.IsDead()
	}
	return !mm.n.Dead
}

// world is the world the mate stands in.
func (mm mercMate) world() uint8 {
	if mm.p != nil {
		return mm.p.World
	}
	return mm.n.World
}

// petSeesMateLocked is whether the pet sees the mate (sight.go). Must be
// called with the lock held.
func (w *World) petSeesMateLocked(pet *NPC, mm mercMate) bool {
	x, y, z := mm.place()
	return w.npcSeesLocked(pet, mm.world(), x, y, z)
}

// mercPetHealsMercLocked lands a merc's pet's heal of its owner on its merc
// (the file's doc): at most what it is missing (a share heal its share of the
// merc's most), its body kept in step. Must be called with the lock held.
func (w *World) mercPetHealsMercLocked(ps *petState, c *petCast, n int) {
	pet, m := ps.npc, ps.merc.npc
	if c.a.HealPct > 0 {
		n = petShareAmount(m.MaxHP, c.a.HealPct)
	}
	if n <= 0 || m.Dead || m.HP >= m.MaxHP {
		return
	}
	if !w.npcSeesLocked(pet, m.World, m.X, m.Y, m.Z) {
		log.Printf("LoS: %s's %q spares its merc %s with %q, out of its sight", ps.owner.Name, pet.Name, m.Name, c.spell.Name)
		return
	}
	n = min(n, m.MaxHP-m.HP)
	m.HP += n
	syncMercBody(ps.merc)
	w.dpsPetAddLocked(ps, dpsHeal, n) // its hirer's healing, as its pet's member heals (dpsheal.go); its merc's line (dpsmerc.go)
	log.Printf("Pet: %s's %q heals its merc %s with %q for %d (HP %d/%d)", ps.owner.Name, pet.Name, m.Name, c.spell.Name, n, m.HP, m.MaxHP)
}

// mercPetBuffsMercLocked lands a merc's pet's buff of its owner on its merc
// (the file's doc), as the merc's own buff on itself lands (merccast.go's
// mercHelpToLocked), its figures the merc's body's; one of the pet's that
// lasts only while it is out goes with it (petBuffsGoLocked). Must be called
// with the lock held.
func (w *World) mercPetBuffsMercLocked(ps *petState, c *petCast, now time.Time) {
	ms := ps.merc
	m := ms.npc
	if m.Dead || buffOn(m.Effects, c.spell) {
		return
	}
	cs := &castState{spell: *c.spell, caster: m.ServerID, targets: []EntityID{m.ServerID}}
	cs.spell.figures = figuresPet // numbertags.go
	w.landOnPetLocked(cs, ms.body, nil, m, now)
	if c.a.WhileOut {
		for _, e := range m.Effects {
			if e.SpellID == c.spell.SpellID {
				e.petBound = true
			}
		}
	}
	syncMercBody(ms)
	log.Printf("Pet: %s's %q buffs its merc %s with %q", ps.owner.Name, ps.npc.Name, m.Name, c.spell.Name)
}

// mercPetBuffGoesLocked takes the pet's buff that lasts only while it is out
// off its merc (petBuffsGoLocked); it reports one taken. Must be called with
// the lock held.
func (w *World) mercPetBuffGoesLocked(ps *petState, spellID int) bool {
	m := ps.merc.npc
	for i, e := range m.Effects {
		if e.SpellID == spellID {
			m.Effects = append(m.Effects[:i:i], m.Effects[i+1:]...)
			syncMercBody(ps.merc)
			return true
		}
	}
	return false
}
