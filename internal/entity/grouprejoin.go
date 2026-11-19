package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// A character back in the world under a new client is still in its group here
// (rejoinGroupLocked, a takeover's handedOverLocked), but CLIENT holds itself
// in a group only after its own Invite or Accept, and Connect clears it, so
// the new client ignored every roster (Msg0772 returns out of a group): the
// group window empty, Leave Group "you are not in a group", an invite of its
// own refused as "full" or "already in a group" and its group still listing it
// (petitions 2338, 2348, 2351: two players' boxes after a reload).
// mercRejoinAskLocked's probe (petition 2213) did this for a group of an owner
// and its mercs alone; groupRejoinAskLocked does it for one with other
// players: its own box, once its client is in the world. A client in the group
// already answers it 2 for itself and shows nothing (Msg0773): the roster
// again. One in none shows it: its yes (AcceptInvite: m_inGroup set) is sent
// the roster, its no takes it out of the group, as its Leave Group would.

// groupRejoinLine is said with the box.
const groupRejoinLine = "You are still in your group: accept the box to show it again (no leaves the group)."

// markGroupRejoinLocked has p's client asked to show its group once it is in
// the world, when other players are in it with p. Must be called with the lock
// held.
func (w *World) markGroupRejoinLocked(p *Player, g *Group) {
	p.groupRejoin = g != nil && w.otherPlayerInGroupLocked(g, p)
}

// otherPlayerInGroupLocked reports a player of g other than p.
func (w *World) otherPlayerInGroupLocked(g *Group, p *Player) bool {
	for _, id := range g.Members {
		if id != p.ServerID && w.Players[id] != nil {
			return true
		}
	}
	return false
}

// groupRejoinAskLocked sends m's client its own box asking it to show g (the
// file's doc), when it is marked, in the world and has no other box up. A
// merc's probe of its own (mercRejoinAskLocked) does it instead when one is
// due. Must be called with the lock held.
func (w *World) groupRejoinAskLocked(g *Group, m *Player, sess *session.Session) {
	if !m.groupRejoin || !mercClientReady(sess) || w.mercAskHeld[m.ServerID] || w.boxWaitingLocked(m) {
		return
	}
	m.groupRejoin = false
	if m.GroupID != g.ID || !w.otherPlayerInGroupLocked(g, m) {
		return
	}
	for _, ms := range w.mercsOfLocked(m) {
		if ms.rejoin && w.mercInOwnGroupLocked(ms) {
			return // its mercs' box probes the same group
		}
	}
	words := m.Name
	for _, id := range g.Members {
		if o := w.Players[id]; o != nil && o != m {
			words = o.Name // the leader, else the first other player
			break
		}
	}
	w.askLocked(m, m, sess, groupAsk{rejoin: true}, words)
	w.sendChatToSession(sess, groupRejoinLine)
	log.Printf("Group: %s's client asked to show its group %d", m.Name, g.ID)
}

// groupRejoinYesLocked is a yes to that box: the roster, its client in the
// group now; the group gone since, its client told so (2: disbanded). Must be
// called with the lock held.
func (w *World) groupRejoinYesLocked(p *Player) {
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		w.sendRosterLocked(g, true)
		return
	}
	w.sendGroupRemoved(p, 2)
}

// groupRejoinNoLocked is any other answer: 2, its client in the group already,
// the roster again; else out of the group. Must be called with the lock held.
func (w *World) groupRejoinNoLocked(p *Player, why byte) {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return
	}
	if why == 2 {
		w.sendRosterLocked(g, true)
		return
	}
	log.Printf("Group: %s declined showing its group %d: it leaves it", p.Name, g.ID)
	w.leaveGroupLocked(p)
}
