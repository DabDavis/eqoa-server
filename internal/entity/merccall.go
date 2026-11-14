package entity

// merccall.go — what a merc may take on: only what its owner called it to
// (GM ticket 1959, Jibwiz, approved by the owner 2026-11-13: "Mercenaries
// should not attack unless the player does a hostile action, or performs a
// heal on someone in combat (So healer mercs engage when the player heals
// someone). Hostile behavior should be removed for mercenaries and mercenary
// pets, but not regular player pets.").
//
// An owner calls his mercs onto a creature by:
//   - a hostile act on it: his auto-attack on it, a hit, a ranged shot, a
//     harmful spell or art landing or resisted (updateAggro with him the
//     attacker: a charm, a taunt, a pickpocket caught too);
//   - a heal or a beneficial spell landing on one (a player, a merc, a pet)
//     a creature is fighting: that creature (landOnPlayerLocked,
//     landOnPetLocked).
//
// A call holds until the creature dies, or its fight is over (no enemy, no
// hate left: it went home). No merc fights on its own any more for its
// owner's group or raid, for a creature with its owner on its hate list (one
// that aggroed on him), nor by its stance: aggressive is assist now.
//
// Self-defence (the decision taken with 1959): a merc fights back what is
// fighting IT (the creature's enemy the merc itself), and its pet what is
// fighting the pet or its merc; never on another's behalf, its owner's
// included, until its owner acts.
//
// Its heals (mercHealTurnLocked) wait the same way: while a creature fights
// one of its own team (its owner, itself, its owner's mercs) the merc heals
// only once its owner has a called fight (mercFightCalledLocked) or the merc
// itself is fought; out of a fight it tops them up as before, which angers
// no one.
//
// An aggressive merc (GM ticket 2025, Jibwiz, approved by the owner: "Mercenaries
// should assist group members when they commit an action if on the right stance,
// such as aggressive. If a group member needs heals, they should help heal them.
// If they attack, they should to. Same with when the player does") answers its
// owner's group mates' calls as its owner's: the other players of the group it is
// in (mercGroupMatesLocked), in a raid those of its owner's group number alone
// (raidsubgroups.go; the owner's rule of 2026-11-13, "not the whole raid", kept),
// in its world, none in a fight that is its own (ownfight.go: a duel, Mr. Moogie's
// challenge, the PvP flag). A mate's auto-attack's target, a creature a mate did a
// hostile act on, one fighting someone a mate healed or helped: it takes them on
// (mercCalledForLocked); and its heals reach the mates and their mercs
// (mercHealTeamLocked; its buffs stay its owner's own). Passive and assist keep
// the owner's call alone. Nothing it takes on is uncalled: no group mate's call,
// no fight of the merc's.

// mercCallLocked records p's call onto npc for his mercs, when he has any.
// Must be called with the lock held.
func (w *World) mercCallLocked(p *Player, npc *NPC) {
	if p == nil || npc == nil || npc.Dead || npc.Owner != 0 || npc.isMerc() || !w.mercCallsHeardLocked(p) {
		return
	}
	if w.mercCalls == nil {
		w.mercCalls = map[EntityID]map[EntityID]bool{}
	}
	calls := w.mercCalls[p.ServerID]
	if calls == nil {
		calls = map[EntityID]bool{}
		w.mercCalls[p.ServerID] = calls
	}
	calls[npc.ServerID] = true
}

// mercCallsHeardLocked reports a merc that may answer p's calls: his own, or
// one in his group (an aggressive one answers a group mate's: the file's doc).
// Must be called with the lock held.
func (w *World) mercCallsHeardLocked(p *Player) bool {
	if len(w.mercs[p.ServerID]) > 0 {
		return true
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return false
	}
	for _, id := range g.Members {
		if w.NPCs[id].isMerc() {
			return true
		}
	}
	return false
}

// mercCallHelpLocked is p's heal or beneficial spell landing on id: each
// creature fighting id (its enemy, or id on its hate list, in a fight and not
// on its way home) within healHateReach of where id stands is called. Must be
// called with the lock held.
func (w *World) mercCallHelpLocked(p *Player, id EntityID, world uint8, x, z float32) {
	if p == nil || !w.mercCallsHeardLocked(p) || w.SpatialIndex == nil {
		return
	}
	for _, nid := range w.SpatialIndex.QueryRadius(x, z, healHateReach) {
		npc := w.NPCs[nid]
		if npc == nil || npc.Dead || npc.World != world || npc.CurrentEnemy == 0 || npc.chase.returning {
			continue
		}
		if _, hates := npc.Threats[id]; hates || npc.CurrentEnemy == id {
			w.mercCallLocked(p, npc)
		}
	}
}

// mercCalledLocked reports npc called by the owner (the file's doc); a call
// whose creature died or whose fight is over is let go. Must be called with
// the lock held.
func (w *World) mercCalledLocked(owner *Player, npc *NPC) bool {
	if owner == nil || npc == nil {
		return false
	}
	calls := w.mercCalls[owner.ServerID]
	if !calls[npc.ServerID] {
		return false
	}
	if npc.Dead || npc.CurrentEnemy == 0 && len(npc.Threats) == 0 || npc.chase.returning {
		delete(calls, npc.ServerID)
		if len(calls) == 0 {
			delete(w.mercCalls, owner.ServerID)
		}
		return false
	}
	return true
}

// mercForgetCallsLocked lets go of every call onto the creature (its death).
// Must be called with the lock held.
func (w *World) mercForgetCallsLocked(id EntityID) {
	for o, calls := range w.mercCalls {
		delete(calls, id)
		if len(calls) == 0 {
			delete(w.mercCalls, o)
		}
	}
}

// mercFightCalledLocked reports its owner's fight one the merc is in: its
// owner's auto-attack on, or a call onto, a creature still fighting. Must be
// called with the lock held.
func (w *World) mercFightCalledLocked(ms *mercState) bool {
	o := ms.owner
	if o.Attacking() {
		if t := w.NPCs[o.AttackTarget]; t != nil && !t.Dead {
			return true
		}
	}
	for _, p := range append([]*Player{o}, w.mercGroupMatesLocked(ms)...) {
		if p != o && p.Attacking() {
			if t := w.NPCs[p.AttackTarget]; t != nil && !t.Dead {
				return true
			}
		}
		for id := range w.mercCalls[p.ServerID] {
			if w.mercCalledLocked(p, w.NPCs[id]) {
				return true
			}
		}
	}
	return false
}

// mercGroupMatesLocked is the other players whose calls an aggressive merc
// answers (the file's doc): those of the group it is in, in a raid those of its
// owner's group number, in its world, none in a fight of its own; none for a
// merc of another stance or out of its owner's group. Must be called with the
// lock held.
func (w *World) mercGroupMatesLocked(ms *mercState) []*Player {
	o := ms.owner
	if ms.stance != MercAggressive || o == nil || !w.mercInOwnGroupLocked(ms) {
		return nil
	}
	g := w.groups[o.GroupID]
	var subs map[EntityID]uint8
	if g.isRaid() || g.flagged {
		subs = w.subgroupsLocked(g)
	}
	var out []*Player
	for _, id := range g.Members {
		p := w.Players[id]
		if p == nil || p == o || p.World != ms.npc.World || w.ownFightLocked(p) {
			continue
		}
		if subs != nil && subs[id] != subs[o.ServerID] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// mercCalledForLocked reports npc one the merc may take on by a call: its
// owner's, or a group mate's (mates, mercGroupMatesLocked: his auto-attack on
// it or his call). Must be called with the lock held.
func (w *World) mercCalledForLocked(ms *mercState, mates []*Player, npc *NPC) bool {
	if w.mercCalledLocked(ms.owner, npc) {
		return true
	}
	for _, p := range mates {
		if p.Attacking() && p.AttackTarget == npc.ServerID || w.mercCalledLocked(p, npc) {
			return true
		}
	}
	return false
}

// mercHealTeamLocked is whom the merc's heals reach: its own team
// (mercOwnTeamLocked), and for an aggressive merc its group mates
// (mercGroupMatesLocked) alive, and their mercs. Must be called with the lock
// held.
func (w *World) mercHealTeamLocked(ms *mercState) []mercMate {
	out := w.mercOwnTeamLocked(ms)
	mates := w.mercGroupMatesLocked(ms)
	if len(mates) == 0 {
		return out
	}
	m := ms.npc
	mine := map[*Player]bool{}
	for _, p := range mates {
		mine[p] = true
		if !p.IsDead() && !p.respawnPending() {
			out = append(out, mercMate{p: p})
		}
	}
	for _, id := range w.groups[ms.owner.GroupID].Members {
		if n := w.NPCs[id]; n.isMerc() && n != m && !n.Dead && n.World == m.World && n.merc != nil && mine[n.merc.owner] {
			out = append(out, mercMate{n: n})
		}
	}
	return out
}

// mercHealsMateLocked reports id one of the merc's group mates its heals reach
// (mercHealTeamLocked) beyond its own team. Must be called with the lock held.
func (w *World) mercHealsMateLocked(ms *mercState, id EntityID) bool {
	for _, mm := range w.mercHealTeamLocked(ms) {
		if mm.id() == id {
			return true
		}
	}
	return false
}

// mercHealHeldLocked reports the merc's heals held (the file's doc): a
// creature fighting one of its own team while its owner has no called fight
// and the merc itself is not fought. Must be called with the lock held.
func (w *World) mercHealHeldLocked(ms *mercState) bool {
	o, m := ms.owner, ms.npc
	team := map[EntityID]bool{}
	for _, mm := range w.mercHealTeamLocked(ms) {
		team[mm.id()] = true
	}
	fought := false
	for _, id := range w.SpatialIndex.QueryRadius(o.X, o.Z, petAssist) {
		npc := w.NPCs[id]
		if npc == nil || npc.Dead || npc.CurrentEnemy == 0 || npc.Owner != 0 {
			continue
		}
		if npc.CurrentEnemy == m.ServerID {
			return false // it is fought itself: it may heal (self-defence)
		}
		if team[npc.CurrentEnemy] {
			fought = true
		}
	}
	return fought && !w.mercFightCalledLocked(ms)
}
