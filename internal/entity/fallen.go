package entity

import "log"

// fallen.go — a group's or raid's member who died in a fight still shares its kill (the
// owner, 2026-11-18: "if the raid boss dies and players are dead they dont get to roll on
// loot").
//
// What happened (Heirloom, 02:09:46 UTC 2026-11-18, group 154's kill of Veeshan's Talon:
// "7 seats, 11 members"): four players died to the Talon in the
// fight and, PlayerRespawnDelay later (respawn.go), were set down at their binds in
// another world before it died, a player in the very second of the kill. A kill's share
// went by where a member stood at the kill (raidKillLocked: in its world within
// KillShareRange; creditKillLocked: the same, and alive), so the four had no roll, no No
// Trade share, no quest credit and no quest drop. And a member lying dead in reach had a
// raid's seat but no KillCredit (creditKillLocked passed the dead by): no quest credit, and
// in a plain group no share of a No Trade stack (handlers/lootshare.go's killCharsOf).
//
// The rule now: a member's death in a fight is a mark on every creature its group was
// fighting then (a creature with the group's member or pet on its hate list, as a living
// member's fight is told: groupsInCombatLocked) that stood in the member's world (and
// instance) within KillShareRange of the body, the reach a living member's share is
// judged by. The member keeps the mark released or not, wherever its bind sets it down;
// at the kill a member of the killer's group who holds a mark of that group on the
// creature counts as if it stood there alive: a raid's seat (its rolls, its share), and
// one of the KillCredit's players (the kill group's corpse, its No Trade share, quest
// credit and quest drops). A member lying dead in reach counts as a living one does. Not
// the experience (killSharersLocked: the living alone) and not the tunar (CoinSharers),
// which go to those standing.
//
// A mark is lost when its member logs out (its entity id is no longer a player, and a
// login is a new one), leaves the group (it is no member at the kill), or the creature
// leaves its fight alive (no one on its hate list: sweepRaidClaimsLocked), dies (its kill
// takes the marks) or goes.

// markFallenLocked marks p's death on every creature its group fights near the body (the
// file's doc). Called at the death, before the hate lists drop it. Must be called with the
// lock held.
func (w *World) markFallenLocked(p *Player) {
	if p == nil || p.GroupID == 0 || w.groups[p.GroupID] == nil {
		return
	}
	var names []string
	for _, npc := range w.groupsInCombatLocked()[p.GroupID] {
		if npc.Dead || !w.npcSharesLocked(npc, p) {
			continue
		}
		if dx, dz := p.X-npc.X, p.Z-npc.Z; dx*dx+dz*dz > KillShareRange*KillShareRange {
			continue
		}
		if w.raids.fallen == nil {
			w.raids.fallen = map[EntityID][]raidClaim{}
		}
		have := false
		for i, c := range w.raids.fallen[npc.ServerID] {
			if c.player == p.ServerID {
				w.raids.fallen[npc.ServerID][i].group, have = p.GroupID, true
			}
		}
		if !have {
			w.raids.fallen[npc.ServerID] = append(w.raids.fallen[npc.ServerID], raidClaim{player: p.ServerID, group: p.GroupID})
		}
		names = append(names, npc.Name)
	}
	if len(names) > 0 {
		log.Printf("Fallen: %s died in group %d's fight: still shares the kill of %v", p.Name, p.GroupID, names)
	}
}

// takeFallenLocked is (and forgets) the marks on npc: each fallen member's group by its
// entity id. Must be called with the lock held.
func (w *World) takeFallenLocked(npc *NPC) map[EntityID]uint32 {
	marks := w.raids.fallen[npc.ServerID]
	delete(w.raids.fallen, npc.ServerID)
	if len(marks) == 0 {
		return nil
	}
	out := make(map[EntityID]uint32, len(marks))
	for _, c := range marks {
		out[c.player] = c.group
	}
	return out
}

// fellInFightLocked reports whether m, a member of group g at npc's kill, holds a mark of
// g's on it (fallen: takeFallenLocked's), and its dungeon did not take it out. Must be
// called with the lock held.
func (w *World) fellInFightLocked(fallen map[EntityID]uint32, g uint32, m *Player, npc *NPC) bool {
	return m != nil && g != 0 && fallen[m.ServerID] == g && m.GroupID == g &&
		!w.dungeonTookOutLocked(npc.Instance, m.CharServerID)
}
