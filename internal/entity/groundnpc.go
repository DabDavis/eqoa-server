package entity

// Ground is where creatures stand (internal/ground): the floor at (x, z)
// under a height, false where it knows none.
type Ground interface {
	Floor(world uint8, x, y, z float32) (float32, bool)
}

// Walls is what stands in a creature's way (internal/ground's Wall): whether
// the way from one point to another passes through a wall, the terrain's or
// anything placed on it (a house, a fence, a tree's trunk). A Ground that is
// also Walls keeps a wander off the walls (movement.go's legOKLocked); one
// that is not leaves every way open, as before.
type Walls interface {
	Wall(world uint8, x0, y0, z0, x1, y1, z1 float32) bool
}

// Placed is what stands in a creature's way of what is placed on the ground
// alone, the terrain left out (internal/ground's Placed). A Ground that is
// also Placed keeps a wander where CLIENT sets the creature down on its own
// floor (movement.go's clearLocked); one that is not, as before.
type Placed interface {
	Placed(world uint8, x0, y0, z0, x1, y1, z1 float32) bool
}

// SetGround gives the world its ground: every move of an NPC (its wander,
// its chase, its way home, a pet's following) then sets it on the floor
// there, so CLIENT, which sets a creature down only on a floor within 1.5 of
// the height it is sent (0x00CBCE40), never shows one walking on air off a
// hill (F3 20261025-165427). Without one an NPC keeps the height it had.
func (w *World) SetGround(g Ground) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ground = g
	w.groundGen++
}

// How near the terrain a creature's spawn must be for it to stand on the
// ground as it moves: below it CLIENT's own reach (0x00CBCE40), above it a
// little more, for the spawns recorded a little high. Of eqoaclassic's
// 22,750 spawns about 20,000 lie within half a unit of the terrain, 613
// between half a unit and 1.5 above it, 148 between 1.5 and 2 and 72
// between 2 and 2.5, against a tail of 30 to 70 a half unit beyond, the NPCs
// that stand on what the ground leaves out (a bridge, a dock, a house, a
// keep's floor). The 1.5 it was until F3 20261026-021425 left a crocodile
// recorded 1.68 above the sand off the ground, and it wandered down the
// beach at its spawn's height, 3.9 above the sand: "Mobs are still floating
// in the air".
const (
	spawnedBelow float32 = 1.5
	spawnedAbove float32 = 2.5
)

// platformReach is how far from its spawn a creature that stands off the
// ground (on what the ground leaves out) keeps its spawn's height in a
// chase or on its way home; further off it has left whatever it stood on,
// and stands on the terrain.
const platformReach float32 = 10

// spawnOnGroundLocked reports whether a creature spawned on the terrain of
// the world's ground (within spawnedBelow below to spawnedAbove above it),
// so its moves take the floor; false with no ground. Must be called with the
// lock held.
func (w *World) spawnOnGroundLocked(npc *NPC) bool {
	if w.ground == nil {
		return false
	}
	if !npc.groundKnown {
		npc.groundKnown = true
		y, ok := w.ground.Floor(npc.World, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
		npc.onGround = ok && y-npc.SpawnY <= spawnedBelow && npc.SpawnY-y <= spawnedAbove
		npc.spawnLift = 0
		if npc.onGround && npc.SpawnY > y {
			npc.spawnLift = npc.SpawnY - y
		}
	}
	return npc.onGround
}

// groundNPCLocked sets an NPC that moved at the height it stands at now: on
// the floor there if it spawned on the ground; one that spawned swimming
// keeps its height, above the floor; one that stands on something the ground
// leaves out keeps its spawn's height near its spawn and takes the terrain
// once it has left it (a creature that never wanders off it: roam.go).
// Must be called with the lock held.
func (w *World) groundNPCLocked(npc *NPC) {
	if w.ground == nil {
		return
	}
	if w.spawnOnGroundLocked(npc) {
		if y, ok := w.floorOrBankLocked(npc.World, npc.X, npc.Y, npc.Z); ok {
			npc.Y = y
		}
		w.swimLocked(npc) // water.go: in deep water it swims at the surface
		return
	}
	if w.spawnInWaterLocked(npc) {
		if y, ok := w.floorOrBankLocked(npc.World, npc.X, npc.Y, npc.Z); ok && y > npc.Y {
			npc.Y = y
		}
		return
	}
	if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) <= platformReach {
		npc.Y = npc.SpawnY
		return
	}
	if y, ok := w.floorOrBankLocked(npc.World, npc.X, npc.Y, npc.Z); ok {
		npc.Y = y
	}
	w.swimLocked(npc)
}

// Under the ground (petition 1500, Jibrish: "Players are able to run monsters
// into the water, swim down, then run quickly on land and the mobs can end up
// underneath the world floor, unable to hit the player"). The ground's floor
// at a place is the highest within its step (floorStepUp) over the height it
// is asked from, and a creature that moved where none was kept the height it
// had: a swimmer, its feet swimDepth under the surface, that came to a bank
// rising more than half a unit over the water went on under it, and any
// creature that ran into a face steeper than the step (a chase stepping a unit
// a tick, five after a stalled tick) went on inside the rock, its height left
// as it was, until the ground came down to it again. CLIENT sets a creature
// down only on a floor within 1.5 under the height it is sent, so it showed
// it under the terrain, out of every player's reach.
//
// Now a creature's floor is the one within its step under it, else the one
// over it within bankClimb (a swimmer out onto a bank, a step a little higher
// than the ground's); and a move that would still leave it in the ground (no
// floor under it, the ground over it: inGroundLocked) is not taken (chase.go,
// chaseroute.go, npcmove.go), and one found there already is set on the floor
// over it (liftOutLocked).
const (
	// floorStepUp is the ground's own step (ground.StepUp): how far over the
	// height it is asked from a floor may be
	floorStepUp float32 = 2
	// bankClimb is how far over its height a creature with no floor under it
	// climbs onto one: a swimmer's depth and a step on a route (routeClimb), a
	// bank 2.5 over the water's surface
	bankClimb float32 = swimDepth + routeClimb
	// groundOver is how far over a creature the ground is looked for when it
	// has no floor under it
	groundOver float32 = 400
)

// floorOrBankLocked is the floor a creature at (x, y, z) stands on: the
// ground's within its step under y, else the one over it within bankClimb;
// false with neither. Must be called with the lock held.
func (w *World) floorOrBankLocked(world uint8, x, y, z float32) (float32, bool) {
	if f, ok := w.ground.Floor(world, x, y, z); ok {
		return f, true
	}
	if f, ok := w.ground.Floor(world, x, y+bankClimb-floorStepUp, z); ok && f > y {
		return f, true
	}
	return y, false
}

// inGroundLocked reports a place in the ground: no floor within the step under
// it, and the ground over it. Must be called with the lock held.
func (w *World) inGroundLocked(world uint8, x, y, z float32) bool {
	if w.ground == nil {
		return false
	}
	if _, ok := w.ground.Floor(world, x, y, z); ok {
		return false
	}
	_, over := w.ground.Floor(world, x, y+groundOver, z)
	return over
}

// liftOutLocked sets a creature found in the ground on the floor over it, the
// lowest one (a creature put there before this rule, or by anything else), and
// reports whether it moved it. Must be called with the lock held.
func (w *World) liftOutLocked(npc *NPC) bool {
	if !w.inGroundLocked(npc.World, npc.X, npc.Y, npc.Z) {
		return false
	}
	for up := floorStepUp; up <= groundOver; up += floorStepUp {
		if f, ok := w.ground.Floor(npc.World, npc.X, npc.Y+up, npc.Z); ok && f > npc.Y {
			npc.Y = f
			w.swimLocked(npc)
			return true
		}
	}
	return false
}
