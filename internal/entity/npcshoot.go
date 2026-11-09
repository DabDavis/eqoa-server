package entity

import "time"

// npcshoot.go — a creature that shoots the bow in its hand (the user, 2026-11-09,
// of Erollisi Marr: "has arrows on back", "let her use her bow").
//
// CLIENT's GetSwing gives a ranger with a ranged weapon and nothing in hand the
// ranged swings (Range1, Range2, Range4: attack.go), and SPELLFX.CSF's Range
// effects play the attack on the second hand and loose the second hand's
// projectile at the target, Range3 and Range4 two of them (a bow's arrow,
// 0x845FECDD: VICreatureEntity::ApplyItems), where the Attack effects play the
// hands' melee attacks and loose nothing. An NPC carries its bow in item 1 with
// attack type 7 (npchands.go), but its swing was a melee one whatever it held
// (npcSwingKind: a level 65 ranger Attack4), so an archer drew its bow at arm's
// length and no arrow flew.
//
// A creature that fights by a ranger's class template (class 1:
// classtemplate.go), holds a ranged weapon in its second hand and nothing in its
// first now shoots: its swing is GetSwing's with the ranged weapon worn (slot
// 18), each of Range3's and Range4's arrows is a hit of its own, and it fights
// from npcShootReach of its enemy where it sees it (sight.go), closing in only
// to there (chase.go). Every other creature fights as before: of the 77 rows
// holding a bow in their second hand on 2026-11-09 none fights by a ranger's
// template (the six names of them that fight by one fight by the roamer's, a
// warrior's).

// npcShootReach is how far a shooting creature fights from: the catalog's bows'
// reach (itempattern.unk5: 25 for every bow but the heirlooms' 55).
const npcShootReach float32 = 25

// npcShoots reports whether the NPC shoots the ranged weapon in its hand (the
// file's doc): a creature (no pet) of a ranger's template, its first hand empty
// and its second holding a bow, a crossbow or a thrown weapon.
func npcShoots(n *NPC) bool {
	if n == nil || n.Owner != 0 || n.tmpl == nil || n.tmpl.Class != classRanger || n.PrimaryWeapon != 0 || n.SecondaryWeapon == 0 {
		return false
	}
	switch n.SecondaryType {
	case 7, 9, 10:
		return true
	}
	return false
}

// npcShotHits is the times of a shooting creature's hits in its round: an arrow
// each, as the Range effects loose them (Range3 and Range4 two, the second as
// Attack3's second hit; Range1 and Range2 one).
func npcShotHits(kind int) []time.Duration {
	switch SwingEffectName(kind) {
	case "Range3", "Range4":
		return []time.Duration{500 * time.Millisecond, time.Second}
	}
	return []time.Duration{500 * time.Millisecond}
}

// shootReachLocked is the reach a shooting creature fights from at its enemy d
// off at (x, y, z): npcShootReach when the enemy is within it and in its sight,
// else none (ok false: it closes in as any creature). Must be called with the
// lock held.
func (w *World) shootReachLocked(npc *NPC, d, x, y, z float32) (float32, bool) {
	if !npcShoots(npc) || d > npcShootReach || !w.inSightLocked(npc.World, npc.X, npc.Y, npc.Z, x, y, z) {
		return 0, false
	}
	return npcShootReach, true
}
