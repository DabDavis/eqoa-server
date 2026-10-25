package entity

// Ground is where creatures stand (internal/ground): the floor at (x, z)
// under a height, false where it knows none.
type Ground interface {
	Floor(world uint8, x, y, z float32) (float32, bool)
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

// spawnedOnGround is how near the terrain a creature's spawn must be for it
// to stand on the ground as it moves: CLIENT's own reach (0x00CBCE40). Of
// eqoaclassic's 16,646 creatures 15,508 spawn so; most of the rest stand on
// what the ground leaves out (a bridge, a ruin, a house), and keep their
// heights as before, which a fall to the terrain beneath would not.
const spawnedOnGround float32 = 1.5

// groundNPCLocked sets an NPC that moved on the floor where it stands now,
// if it spawned on the ground. Must be called with the lock held.
func (w *World) groundNPCLocked(npc *NPC) {
	if w.ground == nil {
		return
	}
	if !npc.groundKnown {
		npc.groundKnown = true
		y, ok := w.ground.Floor(npc.World, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
		npc.onGround = ok && y-npc.SpawnY <= spawnedOnGround && npc.SpawnY-y <= spawnedOnGround
	}
	if !npc.onGround {
		return
	}
	if y, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok {
		npc.Y = y
	}
}
