package entity

// landwater.go — creatures of the land keep out of the water but in a fight
// (the owner, 2026-11-13: "land mobs should not path into water, but can chase
// players of course through it just fine").
//
// The water and what keeps to it are water.go's (the zones' leaf flags off the
// disc; npcmove's word, else in the water at its spawn a creature of the water,
// else of the land). Wandering, a creature of the land already never stepped
// into the water (movement.go's legs: every point of a leg out of it), and the
// GM paths are walked as a GM laid them. What took them into it was the way
// home after a fight: a chase runs straight at its enemy, across a river or a
// lake if that is where its enemy went, and the way back to where the chase
// began went straight back through it (or round a ledge by a search that knew
// no water). Measured on the real ground and creatures (landwater_real_test.go)
// before this: of 247 chases that crossed the water to an enemy on its far
// shore, 230 came home through it.
//
// Now a creature of the land on its way home searches as a chase does
// (chaseroute.go) when the way straight ahead goes into the water (its look
// ahead must be dry as well as clear) and searches dry: each leg's length in
// the water costs routeWetCost times its length more, so a way round on the
// land is taken wherever the search finds one (within homeDryNodes points), and
// the water only where none is (out of it by the nearest bank, for one left in
// it). A search that finds nothing leaves the rest of that way home as it was
// before, through the water if that is the way: it is never stranded, and not
// searched for again and again. A chase is as it was: into the water after its enemy.
//
// And a creature of the land left standing in the water (its way home cut by
// another fight, which began its chase there) walks out of it: its wander's leg
// may be in the water up to its first point out of it, not after; and a chase
// begun on the way home keeps the home it had, so it is not brought back to a
// place in the water.

// routeWetCost is how much dearer a unit of a dry search's way in the water is
// than one on the land: past the search's span (routeSpan) a way round is
// never that much longer.
const routeWetCost float32 = 20

// homeDryNodes is the most points a dry way home's search tries (routeNodes a
// chase's): on the real ground every dry way but three of 169 was found within
// 600, most within 100, and a search that finds nothing costs all it may try.
const homeDryNodes = 600

// homesDry turns the dry way home on; a test's control turns it off.
var homesDry = true

// homeDryLocked reports a creature that comes home on the land where it can: one
// of the land, the water known. Must be called with the lock held.
func (w *World) homeDryLocked(npc *NPC) bool {
	return homesDry && w.env != nil && npc.Owner == 0 && w.keepsToLocked(npc) == keepsToLand
}
