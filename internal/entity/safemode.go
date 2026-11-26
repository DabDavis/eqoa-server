package entity

import "github.com/eqoa/server/internal/session"

// safemode.go — a player's safe mode, /testinvis (petition 1424, the GM
// a GM, 2026-11-10: "Add a safe mode command to everyone on the server.
// This should turn on gm safe mode where no mobs agro though while its
// toggled. The toggle is /testinvis. If they willfully engage in combat that
// still works as normal, and they can be killed. It should make it so nothing
// agros them at all and typing it again shuts it off.").
//
// Safe, no creature attacks the character on sight (aggroSystem), and its
// heals draw none of the free creatures round it on it (healAggroLocked), as
// a GM's #aggro off. Unlike the GM's, a fight it starts works as any other: a
// creature it hits, or whose fight it joins, fights it and may kill it, and
// that creature's area spells take it as anyone near (areaTakesPlayerLocked). A
// creature's area spell leaves it out only while it is no part of that
// creature's fight. A GM's Safe Mode (#aggro off, aggro.go) is the same to a
// creature's area abilities. A fight already on when the mode goes on goes on.
//
// Keyed by the character, so the mode outlives a change of world and, kept in
// table charsafemode by the handler (handlers/safemode.go), a logout and every
// restart, until /testinvis is typed again.

// SafeMode reports whether the session's character is in safe mode; ok false
// out of the world.
func (w *World) SafeMode(sess *session.Session) (on, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, false
	}
	return w.aggro.safe[p.CharServerID], true
}

// SetSafeMode puts the session's character in safe mode or takes it out. It
// reports false out of the world.
func (w *World) SetSafeMode(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	if on {
		if w.aggro.safe == nil {
			w.aggro.safe = make(map[uint32]bool)
		}
		w.aggro.safe[p.CharServerID] = true
	} else {
		delete(w.aggro.safe, p.CharServerID)
	}
	return true
}

// safeLocked reports p in safe mode. Must be called with the lock held.
func (w *World) safeLocked(p *Player) bool {
	return w.aggro.safe[p.CharServerID]
}

// inFightLocked reports p part of npc's fight: on its hate list or its enemy.
// Must be called with the lock held.
func inFightLocked(npc *NPC, p *Player) bool {
	if npc.CurrentEnemy == p.ServerID {
		return true
	}
	_, hated := npc.Threats[p.ServerID]
	return hated
}

// areaTakesPlayerLocked reports whether npc's area abilities take the player
// p: its area spells, breaths and nukes (templateAreaLocked), a sethp and its
// damage over time (setHPReachLocked), an area rampage (areaRampageLocked) and
// a swarm (swarmReachLocked), each by this one rule. p is taken when it is
// there (presentLocked), where npc is (npcSharesLocked, the water's edge:
// apartLocked) and seen by it (an invisible player only by one that sees
// invisibility, seesInvisLocked), and, in a player's safe mode (/testinvis) or
// a GM's Safe Mode (#aggro off, aggro.go), only while it is part of npc's
// fight (inFightLocked: on its hate list, a hit of either's put it there, or
// the one it fights). Neither mode spares a player a fight it is in (the
// owner, 2026-11-26: Garrak, a sub-GM in Safe Mode meleeing the dreadnaught,
// never took its Dread Curse or its damage over time); both still keep the
// creatures from setting on it on sight (aggroSystem, healAggroLocked). Must
// be called with the lock held.
func (w *World) areaTakesPlayerLocked(npc *NPC, p *Player) bool {
	if p == nil || !w.presentLocked(p) || !w.npcSharesLocked(npc, p) || w.apartLocked(p, npc) {
		return false
	}
	if p.Invisible() && !w.seesInvisLocked(npc, p) {
		return false
	}
	if w.aggro.ignored[p.CharServerID] || w.safeLocked(p) {
		return inFightLocked(npc, p)
	}
	return true
}
