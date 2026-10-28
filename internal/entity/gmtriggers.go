package entity

// What a GM's trigger needs of the world (handlers/gmtriggers.go): copies of
// an NPC placed round a point on the ground, hostile ones attacking any
// player on sight, and where a player stands.

// triggerFloorReach is how far from the trigger's height the ground's floor
// may be for a spawn to stand on it; past it (a cave under the place, a
// cliff's top above) the spawn keeps the place's height.
const triggerFloorReach float32 = 20

// SpawnEventNPC places a copy of the NPC named name (SpawnNPCCopy's pick) at
// (x, z) in world, on the ground's floor under cy when the server has the
// ground files and one lies within triggerFloorReach, else at cy, facing
// (cx, cz): a GM trigger's spawn. A hostile one may be attacked (the NPC bit
// 0x80 cleared) and attacks any player on sight (EventAggro). It returns the
// copy's id and the name as the NPC spells it.
func (w *World) SpawnEventNPC(name string, world uint8, cx, cy, cz, x, z float32, hostile bool) (EntityID, string, bool) {
	y := cy
	if fy, ok := w.GroundFloor(world, x, cy+3, z); ok && fy-cy <= triggerFloorReach && cy-fy <= triggerFloorReach {
		y = fy
	}
	facing := float32(0)
	if dx, dz := cx-x, cz-z; dx != 0 || dz != 0 {
		facing = headingTo(dx, dz)
	}
	id, spelled, ok := w.SpawnNPCCopy(name, x, y, z, facing, world)
	if !ok {
		return 0, "", false
	}
	if hostile {
		w.mu.Lock()
		if c := w.NPCs[id]; c != nil {
			c.NPCType &^= 0x80
			c.EventAggro = true
		}
		w.mu.Unlock()
	}
	return id, spelled, true
}

// PlayerSpot is where a player stands and whether it is dead; ok false for
// one that is no longer the world's.
func (w *World) PlayerSpot(p *Player) (world uint8, x, y, z float32, dead, ok bool) {
	if p == nil {
		return 0, 0, 0, 0, false, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	cur, found := w.Players[p.ServerID]
	if !found || cur != p {
		return 0, 0, 0, 0, false, false
	}
	return p.World, p.X, p.Y, p.Z, p.IsDead(), true
}
