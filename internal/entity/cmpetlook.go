package entity

// cmpetlook.go — a mastery rank that changes the look of its owner's pet and
// nothing else (the owner, 2026-11-16: "lets give zerkziee a custom construct
// for his tinker pet as another option cm only for him. the drachnid for jarek
// n toxis lvl 10 delf quest model"). A rank whose effects carry PetLook
// (cmcatalog's effects, "PetLook:360074664": Jarek's Creation's body,
// 0x15764DA8, the dark elf torso on the Tiny Spiderbot's legs that the dark
// elves' level 10 quest "Jarek's Creation" sends them to see) makes the pets of
// the spells it requires (its require_spells: Tinker's Construct, 901835, and
// the looks that replace it, 901836 and 901837) wear that body. Each is the
// pet its spell makes in every other way: level, hit points, stats,
// abilities, stance; only ModelID and the size its body's wearers have
// change. A rank that requires no spell changes every pet its owner
// summons. "OnlyFor:747" keeps the rank to one character (CMRank.OnlyFor).
// Removing the rank brings the spell's own body back at the next summoning.

const (
	// cmPetLook is the effects key of a pet's look (parseStatList's spelling
	// of "PetLook").
	cmPetLook = "petlook"
	// cmOnlyFor is the effects key of the one character a rank is for.
	cmOnlyFor = "onlyfor"
)

// appliesTo is whether the rank's PetLook is for a pet of the spell: one its
// Requires name (or one of their alternatives), or any pet when it requires
// none.
func (r *CMRank) appliesTo(spellID int) bool {
	if r.PetLook == 0 {
		return false
	}
	if len(r.Requires) == 0 {
		return true
	}
	for _, a := range r.Requires {
		if a.SpellID == spellID {
			return true
		}
		for _, alt := range a.Alts {
			if alt == spellID {
				return true
			}
		}
	}
	return false
}

// cmPetLookLocked is the body the player's ranks give a pet of the spell, 0
// none: the highest-id rank's of those that apply. Must be called with the
// lock held.
func (w *World) cmPetLookLocked(p *Player, spellID int) uint32 {
	if p == nil || w.cms == nil || spellID == 0 {
		return 0
	}
	best, look := 0, uint32(0)
	for id := range p.CMs {
		r := w.cms.byID[id]
		if r == nil || !r.Enabled || !r.appliesTo(spellID) || id < best {
			continue
		}
		best, look = id, r.PetLook
	}
	return look
}

// putCMPetLookLocked dresses a new pet of the spell, summoned by caster, in
// the body caster's ranks give it (cmPetLookLocked), at its wearers' size
// (petLookSizeLocked); a pet of no such rank is left as it is. Must be called
// with the lock held.
func (w *World) putCMPetLookLocked(pet *NPC, caster *Player, spellID int) {
	look := w.cmPetLookLocked(caster, spellID)
	if pet == nil || look == 0 || uint32(pet.ModelID) == look {
		return
	}
	pet.ModelID = int64(look)
	pet.Size = w.petLookSizeLocked(look)
}

// petLookSizeLocked is the size a pet in a rank's body takes: its wearers'
// (bodySizeLocked) where that is within a person's (petSizeLeast to
// petSizeMost, as a workbook pet's: petBodyLocked), else 1.
func (w *World) petLookSizeLocked(look uint32) float32 {
	if s := w.bodySizeLocked(look); s >= petSizeLeast && s <= petSizeMost {
		return s
	}
	return 1
}
