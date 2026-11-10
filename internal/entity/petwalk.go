package entity

import (
	"time"
)

// petwalk.go — a pet's way round what stands between it and where it goes (the
// magician Waldorf's petition 1496, in Permafrost: "pets in general don't
// respect walls/barriers/floors/ceilings etc. They just take the quickest line
// to target").
//
// Why a pet went through them: a creature's chase looks ahead of each step and
// goes round what it cannot run over (chaseroute.go, since 2026-11-08), but a
// pet never chased. Following its owner and closing on its target it took
// petStepLocked: a straight step at (x, z), then the floor nearest the height of
// whom it goes to. So it walked through a cave's wall as through air, and under
// a floor its owner stood on it rose through the rock the moment a floor of that
// height was over it (the same look for a floor that keeps it on a hill it is
// sent down from and on a bridge's deck: pet.go). A mercenary moves by the same
// step (merc.go, mercfight.go).
//
// Now a pet or a mercenary on the terrain going to someone on the terrain
// (petRoutesLocked) moves as a chasing creature does, by the chase's own
// functions:
//   - it looks ahead of its step along the leg a chase runs (routeWayLocked: a
//     floor every unit, no steeper than a wander leg, nothing placed across it
//     at its knee), and while that is clear steps straight, as before;
//   - where it is not, it searches the ground for its way round (planRouteLocked,
//     the tick's share of points with the creatures' chases) and runs it
//     (followRouteLocked), on the floor under it as it goes, to its end near
//     whom it goes to on that one's floor, then straight;
//   - a straight step that would still drop it past legDrop or lift it past
//     routeClimb (the floor above or below taken for its own) is not taken, and
//     its way is searched at the next;
//   - whom it goes to over or under it, on another floor (past routeLevel)
//     within its reach across the ground: it searches at once, and neither
//     stands as arrived nor fights from there while it has a way.
// A search that finds nothing leaves it its old straight step for a while (the
// chase's own wait, routeAgain doubling), so where the ground has no way it
// moves as it did, never worse; its owner's teleport still brings it along
// (petLost). A pet's search is the shorter (petRouteNodes), and its leg is held
// to one thing more than a creature's: no wall of the terrain's from its knee
// to its head (movement.go's legHead), which a chase's leg leaves to the slope
// and a cave's thin wall between two level floors does not show.
//
// Measured on Permafrost's real ground (petwalk_real_test.go, 225 pets told to
// follow an owner 8 to 45 off, each on a creature's spawn): of the 96 with pet
// and owner on one floor, 3 found no way; every pet with a straight way or a
// way round but one went through nothing and all came to their owner, where
// the old step went through a wall or a floor in 49 of the 96. Of the 129 on
// different floors 123 found no way (with forty thousand points 39 of them
// have one within routeSpan: the caves' floors join far off, or by a way a leg
// does not take) and went as before: through. A search 0.1 to 5 ms, 2 ms the
// longest tick.
//
// What is left as it was, and why: a pet or its goal off the terrain (on a
// bridge's deck, a dock, a house's own floor more than petOnTerrain over the
// ground, swimming, in the air) takes the old step. The search knows the
// terrain's floors and what is placed on them as walls (movement.go's legRoute);
// a deck is no floor to it, so over one it would find no way, spend its points
// and stand. So a house's wall is gone round from the outside and inside a house
// built on the ground, not on a house's raised floor; and a familiar (never
// owned) follows as before.

// petOnTerrain is how far over the terrain's floor a pet or its goal may stand
// and still be on it: CLIENT's own reach for setting a creature down (1.5,
// groundnpc.go's spawnedBelow).
const petOnTerrain float32 = 1.5

// petRouteNodes is the most points one search of a pet's or a mercenary's
// tries (a creature's chase: routeNodes, 2000). A pet is near whom it goes to,
// and on Permafrost's real ground a way on its owner's floor was found within
// it nearly every time (petwalk_real_test.go); a search that finds none costs
// its points with the world held, so a pet's is the shorter.
const petRouteNodes = 800

// petWay is a pet's or a mercenary's way round, while it has one, and whom it
// was found to (a new goal gets a way of its own).
type petWay struct {
	route chaseRoute
	goal  EntityID
}

// walksTerrain reports an NPC whose floor in a route is the terrain where it
// stands, not its spawn's: a summoned pet or a mercenary (owned, of no row's
// spawn; a charmed creature keeps its own: groundnpc.go).
func (n *NPC) walksTerrain() bool {
	return n.Owner != 0 && n.Charm == nil
}

// legStandLocked is how a leg is checked for the NPC (movement.go's
// legFromLocked): whether it walks the ground's floor, and how far over it its
// own floor is. A creature's by its spawn (spawnOnGroundLocked); a summoned
// pet's and a mercenary's the terrain itself (their routes are run only there:
// petRoutesLocked). Must be called with the lock held.
func (w *World) legStandLocked(npc *NPC) (onGround bool, lift float32) {
	if npc.walksTerrain() {
		return w.ground != nil, 0
	}
	return w.spawnOnGroundLocked(npc), npc.spawnLift
}

// onTerrainLocked is whether (x, y, z) is on the terrain's floor: one there no
// more than petOnTerrain under y (and within the ground's step over it). Must
// be called with the lock held.
func (w *World) onTerrainLocked(world uint8, x, y, z float32) bool {
	f, ok := w.ground.Floor(world, x, y, z)
	return ok && y-f <= petOnTerrain
}

// petRoutesLocked is whether the pet's move to a goal at (gx, gy, gz) is looked
// at as a chase's: the routes on, a ground, the pet owned, not swimming, and
// both on the terrain's floor (the file's doc). Must be called with the lock
// held.
func (w *World) petRoutesLocked(pet *NPC, gx, gy, gz float32) bool {
	if !chaseRoutes || w.ground == nil || pet.Owner == 0 || pet.swimming || !w.routesFor(pet) {
		return false
	}
	return w.onTerrainLocked(pet.World, pet.X, pet.Y, pet.Z) && w.onTerrainLocked(pet.World, gx, gy, gz)
}

// petStackedLocked is whether the pet stands over or under its goal, on another
// floor, where a way round is its only way to it: both on the terrain, past
// routeLevel apart in height, and it has not just looked for a way and found
// none. Must be called with the lock held.
func (w *World) petStackedLocked(pet *NPC, gx, gy, gz float32, now time.Time) bool {
	return absf(gy-pet.Y) > routeLevel && !now.Before(pet.way.route.failedUntil) && w.petRoutesLocked(pet, gx, gy, gz)
}

// petWalkLocked moves a pet or a mercenary dt seconds at speed toward (x, z),
// where it would stand by the one of id goal at (gx, gy, gz): round what stands
// between where the ground has a way, else the straight step it always took
// (the file's doc). Must be called with the lock held.
func (w *World) petWalkLocked(pet *NPC, goal EntityID, x, z, gx, gy, gz, speed, dt float32, now time.Time) {
	way := &pet.way
	if way.goal != goal {
		*way = petWay{goal: goal}
	}
	if dt <= 0 || speed <= 0 || !w.petRoutesLocked(pet, gx, gy, gz) {
		way.route.pts = nil
		w.petStepLocked(pet, x, z, gy, speed, dt)
		return
	}
	d := distXZ(pet.X, pet.Z, gx, gz)
	stacked := absf(gy-pet.Y) > routeLevel && d <= routeFinish
	routing, hold := w.routeWayLocked(pet, &way.route, gx, gy, gz, d, speed*dt, stacked, now)
	if routing {
		w.followRouteLocked(pet, &way.route, speed, dt)
		return
	}
	if hold || (x == pet.X && z == pet.Z) {
		// its search put off to the next tick; or no step across the ground was
		// wanted (under its goal's floor) and no way up was found: it stands
		pet.stopMoving()
		return
	}
	// straight, as it always went; but not off an edge or up through a floor
	// the look ahead stepped over: that step is not taken, and its way is
	// searched at the next
	guarded := !now.Before(way.route.failedUntil)
	oldX, oldY, oldZ, oldSwim, oldFacing := pet.X, pet.Y, pet.Z, pet.swimming, pet.Facing
	w.petStepLocked(pet, x, z, gy, speed, dt)
	if guarded && (pet.Y < oldY-legDrop || pet.Y > oldY+routeClimb) {
		if w.SpatialIndex != nil {
			w.SpatialIndex.Update(pet.ServerID, pet.X, pet.Z, oldX, oldZ)
		}
		pet.X, pet.Y, pet.Z, pet.swimming, pet.Facing = oldX, oldY, oldZ, oldSwim, oldFacing
		way.route.edge = true
		pet.stopMoving()
	}
}
