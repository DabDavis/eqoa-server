package entity

import "math"

// WorldFloorY is where the client puts a character who is over nothing. Walking
// off the edge of terrain drops them to exactly -1000 and leaves them there;
// it is the client's floor, not a value any real ground has.
//
// It matters to the SERVER because a height is persisted on disconnect, and
// saving a fall makes the next login spawn at the floor and fall again — one
// bad step strands a character for good. savePlayerPosition refuses to write a
// height at or below this. Measured, not assumed: see the world-9 note below,
// where walking east out of the lifted zone put a character at y=-1000.
const WorldFloorY = float32(-1000)

// SpawnLift is added to a stored height at world entry, in a world that is not
// on every disc (SpawnLifted).
//
// A character is saved standing ON the ground, and a stored height that is even
// slightly low puts the camera UNDER the terrain, where the player falls
// instead of standing. World 22 does this reliably: its zone is placed at an
// offset, so a height measured in zone-local space is about a unit under the
// world-space ground (observed: characters stored at y=58.3 and 57.8 where the
// ground is 59.3).
//
// It is NOT added in the disc's own worlds, where every login lands now
// (handlers' loginSpawn resumes nowhere else): there a stored height is the
// client's own report of where it stood, and a client places a login exactly
// where the MemoryDump says (VIWalkControl::SetLocation, marked on the floor)
// and looks for the floor from its body's middle, 1.2 above its feet (parm38),
// so a rounding error under the ground is caught anyway. Three units up was a
// visible fall at every login, on a PS2 as in eqoa-rust-client-vanilla
// (F3 20261025-163229: "the character is slightly in the air ... in a fall
// animation briefly"), and a climb: a session that ended before its first
// report saved the lifted height, three more each time.
const SpawnLift = float32(3)

// DiscWorlds is how many worlds CLIENT's gWorldInfo names on the Frontiers
// disc (TUNARIA, RATHE, ODUS, LAVASTM, PLANESKY, SECRETS): the worlds every
// client has, a retail PS2 and a page reading the player's own disc alike.
const DiscWorlds = 6

// SpawnLifted reports whether a login into world is lifted by SpawnLift: only
// in a world that is not on every disc, where heights were measured low.
func SpawnLifted(world int) bool {
	return world < 0 || world >= DiscWorlds
}

// SafeSaveHeight decides which height to persist on disconnect: the one the
// player is at, or the last one that was written. It reports false when it
// refused the current height, so the caller can say so.
//
// A fall is not a position. Saving it makes the next login spawn at the floor
// and fall again, so one bad step strands a character permanently — two rows in
// the live database were already sitting at exactly the floor when this was
// written.
func SafeSaveHeight(current, stored float32) (float32, bool) {
	if current <= WorldFloorY {
		return stored, false
	}
	return current, true
}

// EntityID is a unique identifier for entities in the game world.
type EntityID uint32

// Entity is the interface implemented by both Player and NPC.
type Entity interface {
	GetEntityID() EntityID
	GetPosition() (x, y, z float32)
	IsPlayerEntity() bool
	IsDead() bool
}

// Faction constants.
const (
	FactionFriendly byte = 0 // Players — blue face, con=2
	FactionNeutral  byte = 1 // Merchants/quest NPCs (NPCType & 0x80) — neutral face, con=0
	FactionHostile  byte = 2 // Attackable NPCs — red face, con=0
)

// Range constants (in game units, roughly 1 unit = 1 meter).
const (
	MeleeRange float32 = 10.0 // Melee attack range
	// InteractRange must not be STRICTER than MeleeRange: being able to hit an
	// NPC from 10 units but not talk to it from 8 is backwards, and 8.0 is a
	// real rejection observed in the wild (a player stood next to a Freeport
	// NPC and the click was refused). The old 7.0 had no decompiled constant or
	// note behind it. Tied to MeleeRange rather than given a new invented
	// number, so the two cannot drift apart again.
	InteractRange float32 = MeleeRange
)

// worldBounds gives the XZ extents of worlds we can state precisely, used to
// notice a world index that the position contradicts.
//
// Only the worlds WE build are listed, and that is the point rather than a gap:
// their extents are known exactly, where the disc worlds share one huge
// coordinate space and have no bounds worth asserting, so those are
// deliberately absent and accepted.
//
//	8 — DELIBERATELY ABSENT since worlds moved from the memory card to the hard
//	    disk. It bounded guk (X 750..1700, Z 750..1250, out of guk.esf), and guk
//	    was reached by the SD's "mc:" prefix, which the SD we now serve does not
//	    hook. Entry 8 of that table is a clone of entry 2 (Odus), so a position
//	    in world 8 is now an ODUS position — and guk's bound would reject every
//	    one of them, teleporting the character to world 0 on save. A bound that
//	    outlives the world it measured is worse than no bound: it strands people
//	    on purpose.
//	9 — Tunaria zone 127 (Widows Peak) lifted by cmd/zonelift and streamed off
//	    the PS2 hard disk. Its grid is 8x13 cells from origin (1000, 1000), but
//	    only the ONE lifted zone has terrain, so the bound is that zone's bbox
//	    rather than the grid's. Measured, not assumed: walking east out of it
//	    put a character at x=14086 and dropped them to the world floor at
//	    y=-1000, which is exactly the stranding this table exists to undo.
//	22 — a standalone authored world, one zone. DELIBERATELY ABSENT until its
//	    extents are known in WORLD space, because an entry here was drafted
//	    twice from the file and both readings were suspect:
//
//	      WorldZoneProxies  centre (9000, 132.1, 9000), a +/-1001 box — 2002
//	                        units across where the zone's tiles are 10000
//	      cmd/groundpick    tiles X 0..10000, Z 0..10000, Y -2.5..267.1 — but
//	                        those are ZONE-LOCAL, and this zone is placed at an
//	                        offset (spawning at the local ground height put the
//	                        camera under the terrain)
//
//	    A bound is only safe when it is measured in the frame the SERVER stores
//	    positions in. Absent means accepted, which costs nothing here; a wrong
//	    bound teleports every real position to world 0, which is exactly the
//	    stranding this table exists to undo. Add it from an observed in-world
//	    position, not from the ESF.
var worldBounds = map[uint8]struct{ MinX, MaxX, MinZ, MaxZ float32 }{
	9: {12000, 14000, 22000, 24000},
}

// CoherentWorld returns the world index to trust for a player at (x, z).
//
// A cross-world teleport commits the new world index the moment it is sent,
// because there is no opcode by which the client confirms it arrived. If the
// world then fails to LOAD — which is exactly what a broken guk loader does —
// the client stays where it was and keeps reporting its old position, while the
// server believes the player has moved worlds. Persisted, that strands the
// character permanently: visibility filters to the new world, nothing is in
// range, and no NPC ever appears again. That happened on 2026-08-06 (world 8 at
// Freeport's coordinates), and it is not self-correcting because the player has
// no way to reach a travel NPC they cannot see.
//
// So: a position that cannot exist in the recorded world means the world change
// did not really happen. Fall back to world 0, which is always reachable.
func CoherentWorld(world uint8, x, z float32) (uint8, bool) {
	b, known := worldBounds[world]
	if !known {
		return world, true
	}
	if x < b.MinX || x > b.MaxX || z < b.MinZ || z > b.MaxZ {
		return 0, false
	}
	return world, true
}

// EntityName returns an entity's display name, or "?" for a type that has
// none. Diagnostics only — the Entity interface deliberately stays narrow, but
// a log line naming an entity is worth far more than one naming its ID.
func EntityName(e Entity) string {
	switch t := e.(type) {
	case *NPC:
		return t.Name
	case *Player:
		return t.Name
	}
	return "?"
}

// DistanceBetween returns the 3D distance between two entities.
func DistanceBetween(a, b Entity) float32 {
	ax, ay, az := a.GetPosition()
	bx, by, bz := b.GetPosition()
	dx := ax - bx
	dy := ay - by
	dz := az - bz
	return float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
}

// InRange returns true if two entities are within the given distance.
func InRange(a, b Entity, maxDist float32) bool {
	return DistanceBetween(a, b) <= maxDist
}
