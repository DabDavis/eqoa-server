package entity

import "github.com/eqoa/server/internal/session"

// hidemercs.go — /hide mercs and /show mercs (petition 1858, the GM Jibrish,
// 2026-11-12: "add the command /hide mercs and /show mercs — this hide the
// character model and nameplate for mercenaries, or restore them";
// handlers/hidemercs.go has the commands).
//
// A player that hid them is sent no other player's mercenary, nor a merc's
// pet: the visibility system leaves them out of its candidates
// (visibilitySystem), so a merc in view when the player hides them leaves its
// slot as any entity leaving view does (MarkRemoved: the client takes its body
// and its name plate away), and comes back on a slot, made anew, at the next
// pass after /show mercs. Its own mercs stay in view: it commands them through
// the merc menu (□ on its own merc), which needs the merc on a slot.
//
// It is the viewer's view alone. Nothing of the game looks here: a hidden merc
// fights, heals, is healed and buffed, takes its group place and is found by
// every check of who stands where (npcSharesLocked and the rest) as before,
// and the other players see it as ever.
//
// Keyed by the character, so it outlives a change of world and, kept in table
// charhidemercs by the handler, a logout and every restart, until /show mercs.

// MercsHidden reports whether the session's character hid other players'
// mercenaries; ok false out of the world.
func (w *World) MercsHidden(sess *session.Session) (on, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, false
	}
	return w.mercsHidden[p.CharServerID], true
}

// SetMercsHidden hides other players' mercenaries from the session's
// character's view or shows them again, from the next visibility pass. It
// reports false out of the world.
func (w *World) SetMercsHidden(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	if on {
		if w.mercsHidden == nil {
			w.mercsHidden = make(map[uint32]bool)
		}
		w.mercsHidden[p.CharServerID] = true
	} else {
		delete(w.mercsHidden, p.CharServerID)
	}
	return true
}

// mercHiddenFromLocked reports npc kept out of viewer's view by /hide mercs:
// a mercenary or a merc's pet (Owner the hirer's) of another player's, while
// viewer hid them. Its own are never hidden. Must be called with the lock
// held.
func (w *World) mercHiddenFromLocked(npc *NPC, viewer *Player) bool {
	if viewer == nil || !w.mercsHidden[viewer.CharServerID] {
		return false
	}
	if !npc.isMerc() && !npc.isMercPet() {
		return false
	}
	return npc.Owner != viewer.ServerID
}
