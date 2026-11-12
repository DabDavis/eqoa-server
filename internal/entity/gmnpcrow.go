package entity

// gmnpcrow.go — what a GM's /delete and /clone (handlers/gmnpcrow.go) may
// take: an NPC of a row, standing in the world as its row makes it. Anything
// else is refused, said why: a spawn of no row (a GM's, a quest's, a
// trigger's or an event's), a dungeon instance's copy, a pet (charmed or
// summoned), a mercenary or a familiar, a genie or a portal, a spawn-only
// template (the quests' and triggers' spawns copy it) and anything in an
// instance. A board's name plate is never an NPC of the world (boardplate.go):
// it cannot be targeted.

// RowNPCRefusal is why the NPC of that id is no row a GM may delete or clone,
// or "" when it is one. Its name is given back either way ("" when it is not
// in the world).
func (w *World) RowNPCRefusal(id EntityID) (name, why string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc, ok := w.NPCs[id]
	if !ok {
		return "", "it has left the world"
	}
	return npc.Name, rowNPCRefusalOf(npc)
}

func rowNPCRefusalOf(npc *NPC) string {
	switch {
	case npc.isMerc() || npc.mercPet != nil:
		return "is a mercenary's (or its pet): it has no row"
	case npc.isFamiliar():
		return "is a familiar: it has no row"
	case npc.Owner != 0 || npc.Charm != nil:
		return "is a player's pet: it has no row of its own to change"
	case npc.GenieOf != 0:
		return "is a genie: it has no row"
	case npc.PortalOf != 0:
		return "is a summoning portal: it has no row"
	case npc.QuestSpawn:
		return "is a quest's spawn: it has no row (its quest step makes it)"
	case npc.Temporary && npc.CopyOf != 0 && npc.Instance != 0:
		return "is a dungeon instance's copy: it has no row (its template does; it stands outside the world)"
	case npc.Temporary:
		return "is a spawn with no row (a GM's, a trigger's or an event's)"
	case npc.Instance != 0:
		return "stands in an instance: its row is not changed from inside one"
	case npc.SpawnOnly:
		return "is a spawn-only template (npcspawnonly): the quests' and triggers' spawns copy it"
	}
	return ""
}
