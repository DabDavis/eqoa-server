package entity

import "github.com/eqoa/server/internal/database"

// mercdyes.go — a merc wears its owner's dyes (petition 2998, owner-approved: "I would like my
// mercenaries to have the same equipment dye as me, they are my pack after all"). The owner's dye of
// each armour slot, its trim and the robe (chardyes: Player.Dyes) is the merc's, read from its owner
// as each tick makes its channel, so a dye the owner changes (#dye, a dyer) is on the merc at the
// next tick with no rekit: the live look's colours and, for a PS2, the nearest disc tint
// (putNPCDyeTints). The hands and the cape are left out: the merc's weapons are its own, and it has
// no cape. A part its hirer hides (/merc helm off, /merc look off: MercLook.Hidden) has no dye; its
// look is bare. An owner with no dye on a slot leaves the merc's piece as it is.

// mercDyeSlots are the dye slots a merc takes from its owner: the armour's, their trims and the robe.
var mercDyeSlots = func() []int {
	out := []int{database.DyeRobe}
	for i := 0; i < 6; i++ {
		out = append(out, database.DyeChest+i, database.DyeTrimOf(database.DyeChest+i))
	}
	return out
}()

// dyeHides is the hidden part (HideChest ... HideHelm, HideRobe) that hides dye slot i, 0 none.
func dyeHides(i int) uint16 {
	switch {
	case i == database.DyeRobe:
		return HideRobe
	case i >= database.DyeChest && i <= database.DyeHelm:
		return 1 << (i - database.DyeChest)
	case database.IsTrimSlot(i):
		return 1 << (i - database.DyeChestTrim)
	}
	return 0
}

// mercDyesLocked is the dyes merc NPC n shows (the file's doc), nil for any other NPC or an owner with
// none. Must be called with the lock held.
func (w *World) mercDyesLocked(n *NPC) *Dyes {
	if !n.isMerc() || n.merc.owner == nil {
		return nil
	}
	ms := n.merc
	own := &ms.owner.Dyes
	if !ms.dyesMade || ms.dyesOf != *own || ms.dyesHid != n.HiddenParts {
		ms.dyes, ms.dyesOf, ms.dyesHid, ms.dyesMade = Dyes{}, *own, n.HiddenParts, true
		for _, i := range mercDyeSlots {
			if n.HiddenParts&dyeHides(i) == 0 {
				ms.dyes[i] = own[i]
			}
		}
	}
	if ms.dyes == (Dyes{}) {
		return nil
	}
	return &ms.dyes
}

// npcDyesLocked is the dyes NPC n is drawn in: a merc's its owner's (mercDyesLocked), any other's its
// name's (npcdyes.go). Must be called with the lock held.
func (w *World) npcDyesLocked(n *NPC) *Dyes {
	if d := w.mercDyesLocked(n); d != nil {
		return d
	}
	return npcDyesOfKey(n.classKey())
}
