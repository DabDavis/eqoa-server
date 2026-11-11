package entity

import (
	"fmt"
	"time"

	"github.com/eqoa/server/internal/session"
)

// groupinvites.go — "Allow anyone in my group to invite" (the GM Jibrish's
// petition 1476, 2026-11-11: "A setting box bottom right 'Allow anyone in my
// group to invite' yes/no, per character, persistent, applies to regular groups
// only, live for the existing group when its leader toggles it").
//
// Since 2026-11-06 any member of a group invites into it (the user's "lets
// allow any group member to invite to group": GroupInvite). The setting is
// each character's, kept in table chargroupinvites (handlers/groupreg.go), and
// on unless a character turns it off, so a group whose leader never touched
// it goes on as it did. It is read when a member invites: in a regular group
// (not a raid group: one past GroupWindowSize or flagged, raidflag.go) whose
// leader has it off, only the leader's invite goes; in a raid group, or with
// it on, any member's. The leader's toggle is in force at once, for the group
// it leads now. Vanilla's group search shows it and sets it
// (protocol.OpcodeGroupInvitesSetting) and is told the group's rule
// (OpcodeGroupRegState, uitold.go) so it refuses a member's invite itself, as
// CLIENT refuses one (word 640); a PS2's member never invites (CLIENT's own
// rule), so the setting changes nothing for it.

// groupLeaderInvitesLine is said to a member whose invite the leader's
// setting refuses.
const groupLeaderInvitesLine = "Only %s, your group's leader, invites into this group."

// SetGroupInvitesShut loads the characters whose setting is off (table
// chargroupinvites at the start), in place of any held.
func (w *World) SetGroupInvitesShut(shut map[uint32]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.groupInvitesShut = make(map[uint32]bool, len(shut))
	for id, off := range shut {
		if off {
			w.groupInvitesShut[id] = true
		}
	}
}

// SetGroupInvitesOpen sets the session's character's "Allow anyone in my
// group to invite" (open: yes). It reports the character's id (0: no player)
// and whether it leads a regular group, where it is in force at once.
func (w *World) SetGroupInvitesOpen(sess *session.Session, open bool) (charID uint32, leads bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, false
	}
	if w.groupInvitesShut == nil {
		w.groupInvitesShut = map[uint32]bool{}
	}
	if open {
		delete(w.groupInvitesShut, p.CharServerID)
	} else {
		w.groupInvitesShut[p.CharServerID] = true
	}
	g := w.groups[p.GroupID]
	leads = p.GroupID != 0 && g != nil && len(g.Members) > 0 && g.Members[0] == p.ServerID && !g.flagged && !g.isRaid()
	w.uiTold = time.Time{} // told at the next tick
	return p.CharServerID, leads
}

// groupInvitesOpenLocked is p's own setting. Must be called with the lock
// held.
func (w *World) groupInvitesOpenLocked(p *Player) bool {
	return p != nil && !w.groupInvitesShut[p.CharServerID]
}

// membersInviteLocked is whether any member of p's group invites into it: in
// none, in a raid group, or in a regular group whose leader's setting is on.
// Must be called with the lock held.
func (w *World) membersInviteLocked(p *Player) bool {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil || len(g.Members) == 0 || g.flagged || g.isRaid() {
		return true
	}
	leader := w.Players[g.Members[0]]
	return leader == nil || w.groupInvitesOpenLocked(leader)
}

// groupInviteRefusedLocked says to inviter why its invite does not go, true
// when its group's leader lets only the leader invite. Must be called with
// the lock held.
func (w *World) groupInviteRefusedLocked(sess *session.Session, inviter *Player) bool {
	g := w.groups[inviter.GroupID]
	if g == nil || len(g.Members) == 0 || g.Members[0] == inviter.ServerID || w.membersInviteLocked(inviter) {
		return false
	}
	name := "its leader"
	if leader := w.Players[g.Members[0]]; leader != nil {
		name = leader.Name
	}
	w.sendColoredLocked(sess, fmt.Sprintf(groupLeaderInvitesLine, name), castRefusalRGB)
	return true
}
