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

// SetGround gives the world its ground: every move of an NPC (its wander,
// its chase, its way home, a pet's following) then sets it on the floor
// there, so CLIENT, which sets a creature down only on a floor within 1.5 of
// the height it is sent (0x00CBCE40), never shows one walking on air off a
// hill (F3 20261025-165427). Without one an NPC keeps the height it had.
func (w *World) SetGround(g Ground) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ground = g
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
		if y, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok {
			npc.Y = y
		}
		return
	}
	if w.spawnInWaterLocked(npc) {
		if y, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok && y > npc.Y {
			npc.Y = y
		}
		return
	}
	if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) <= platformReach {
		npc.Y = npc.SpawnY
		return
	}
	if y, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok {
		npc.Y = y
	}
}
