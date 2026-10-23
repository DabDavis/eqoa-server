package entity

import "github.com/eqoa/server/internal/session"

// Assist: CLIENT's Assist Target and Assist Leader (the quick commands |22
// and |21, and the start menu's) ask the server whom to target, 0x00CB and
// 0x007B (handlers/target.go), and Msg007B targets the answer: an id as the
// asker knows it, or -1 for none (word 838, "There is no target for
// assisting."). Assist Target is whom the asker's target has targeted,
// Assist Leader whom the asker's group leader has: EverQuest's /assist. No
// capture of either was made, on Sandstorm or a retail server.
//
// A player's target is its last 0x0010, an id in ITS session's table; the
// answer is the same entity in the asker's. Object ids are per session (each
// visibility table counts its own), so both are read through the session the
// id belongs to.

// NoAssist is Msg007B's -1: word 838.
const NoAssist uint32 = 0xFFFFFFFF

// AssistTarget is whom the entity the sender knows by id has targeted, as
// the sender knows it: a player's last target, an NPC's enemy, the player a
// training dummy fights. ok is false when the session has no player.
func (w *World) AssistTarget(sess *session.Session, id uint32) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.sessionPlayerLocked(sess) == nil {
		return NoAssist, false
	}
	return w.assistIDLocked(sess, w.targetOfLocked(w.entitySeenByLocked(sess, id))), true
}

// AssistLeader is whom the sender's group leader has targeted, as the sender
// knows it; none out of a group. A leader asking gets its own target back.
func (w *World) AssistLeader(sess *session.Session) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return NoAssist, false
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil || len(g.Members) == 0 {
		return NoAssist, true
	}
	leader := w.Players[g.Members[0]]
	if leader == nil {
		return NoAssist, true
	}
	return w.assistIDLocked(sess, w.targetOfLocked(leader)), true
}

// entitySeenByLocked resolves an id the sender gave (its own, an object id
// from ITS visibility channels, or a group stand-in) to an entity, or nil:
// playerSeenByLocked, for NPCs too. Must be called with the lock held.
func (w *World) entitySeenByLocked(sender *session.Session, id uint32) Entity {
	if id == 0 || id == NoAssist {
		return nil
	}
	if id&0xFF000000 == 0x7F000000 {
		if p := w.Players[EntityID(id&0x00FFFFFF)]; p != nil {
			return p
		}
		return nil
	}
	if id == sender.PlayerObjectID() {
		if p := w.sessionPlayerLocked(sender); p != nil {
			return p
		}
		return nil
	}
	vs, ok := sender.VisibilityState().(*VisibilityState)
	if !ok || vs == nil {
		return nil
	}
	for i := 0; i < MaxVisibleEntities; i++ {
		if vs.ChannelObjectID[i] != id || vs.ChannelEntity[i] == 0 {
			continue
		}
		if npc, ok := w.NPCs[vs.ChannelEntity[i]]; ok {
			return npc
		}
		if p, ok := w.Players[vs.ChannelEntity[i]]; ok {
			return p
		}
	}
	return nil
}

// targetOfLocked is whom e has targeted: a player's last 0x0010, read in its
// own session's table; an NPC's enemy, or the player a dummy fights; none
// for a corpse. Must be called with the lock held.
func (w *World) targetOfLocked(e Entity) Entity {
	switch t := e.(type) {
	case *Player:
		s := w.sessionOfLocked(t)
		if s == nil {
			return nil
		}
		return w.entitySeenByLocked(s, t.TargetObjectID)
	case *NPC:
		if t.Dead {
			return nil
		}
		id := t.CurrentEnemy
		if t.Dummy != NotDummy {
			id = t.DummyTarget
		}
		if id == 0 {
			return nil
		}
		if p, ok := w.Players[id]; ok {
			return p
		}
		if n, ok := w.NPCs[id]; ok {
			return n
		}
	}
	return nil
}

// assistIDLocked is the id the sender knows e by (its own player's, or its
// visibility channel's), or NoAssist when it has none: an entity the sender
// cannot see is no target to give it. Must be called with the lock held.
func (w *World) assistIDLocked(sess *session.Session, e Entity) uint32 {
	if e == nil {
		return NoAssist
	}
	if id := w.objectIDSeenByLocked(sess, e.GetEntityID()); id != 0 {
		return id
	}
	return NoAssist
}
