package entity

import "github.com/eqoa/server/internal/session"

// safemode.go — a player's safe mode, /testinvis (petition 1424, the GM
// Jibrish, 2026-11-10: "Add a safe mode command to everyone on the server.
// This should turn on gm safe mode where no mobs agro though while its
// toggled. The toggle is /testinvis. If they willfully engage in combat that
// still works as normal, and they can be killed. It should make it so nothing
// agros them at all and typing it again shuts it off.").
//
// Safe, no creature attacks the character on sight (aggroSystem), and its
// heals draw none of the free creatures round it on it (healAggroLocked), as
// a GM's #aggro off. Unlike the GM's, a fight it starts works as any other: a
// creature it hits, or whose fight it joins, fights it and may kill it, and
// that creature's area spells take it as anyone near (templateAreaLocked). A
// creature's area spell leaves it out only while it is no part of that
// creature's fight. A fight already on when the mode goes on goes on.
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
