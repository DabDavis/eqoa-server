package entity

// hotzonelevels.go — the creature levels of a place (petition 2963): the Hot Zone Herald, /marks and
// #hotzone show each camp Hot zone's creatures' levels, so a player can choose one, or delevel to it.

// CreatureLevelsNear is the lowest and highest level of the creatures spawned in world within radius
// of (x, z): the attackable (no NPC bit), no training dummy, nobody's pet, merc or familiar, none a
// passing spawn (Temporary). A creature lying dead counts: its camp is the same. ok false for none.
func (w *World) CreatureLevelsNear(world uint8, x, z, radius float32) (lo, hi int, ok bool) {
	if w == nil || radius <= 0 {
		return 0, 0, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	r2 := radius * radius
	for _, npc := range w.NPCs {
		if npc == nil || npc.World != world || npc.NPCType&0x80 != 0 || npc.Dummy != NotDummy || npc.Owner != 0 ||
			npc.Temporary || npc.isMerc() || npc.isFamiliar() || npc.Level <= 0 {
			continue
		}
		if dx, dz := npc.SpawnX-x, npc.SpawnZ-z; dx*dx+dz*dz > r2 {
			continue
		}
		if !ok || npc.Level < lo {
			lo = npc.Level
		}
		if !ok || npc.Level > hi {
			hi = npc.Level
		}
		ok = true
	}
	return lo, hi, ok
}
