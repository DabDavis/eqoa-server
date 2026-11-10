package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// raidflag.go — the raid flag: the one way a group goes past four (the user,
// 2026-11-07: "we should refractor it to an aoe 60 m flag to have raid group invite").
//
// A group's leader, or one in no group who then leads, raises it with /raid. Every
// player within RaidFlagRadius in its world who is in no group gets the same box as a
// group invite (0x0773, its answers 0x0622 and 0x0624), named "<leader>'s raid", and
// each yes joins whatever the group's size. No line of sight. The leader is told who
// was asked. Another /raid asks whoever has come into reach since: no one with a box
// still unanswered, no one who said no in the last raidAskAgain (ours).
//
// On a PS2 with no S2 /raid never reaches a server: ChatLine answers an unknown /command
// itself (word 826) and neither of its keyboards has a #; an S2 sends it on. Either way
// it sends /t on as typed, so "/t raid" is the flag there, on every client (a tell with
// no text is no tell). Both shown in the tracer.
//
// A caller in no group is asked first, in the same box. A client knows it is in a
// group only from its own Invite or Accept (m_inGroup, which no message sets), and
// Msg0772 returns without it, so a caller the server merely made a leader would see no
// group and could not leave it. Its yes (ownBoxYesLocked) makes it a group of one
// and raises the flag; CLIENT sets m_inGroup, and the roster's member 0 makes it the
// leader.

// RaidFlagRadius is how far a raid flag reaches.
const RaidFlagRadius float32 = 60

// raidAskAgain is how long a no to a leader's flag keeps the next /raid from asking
// again (ours).
const raidAskAgain = time.Minute

// raidOwnBoxWords is what a caller's own box is from.
const raidOwnBoxWords = "The raid flag"

// raidAskedNames is the most names a line to the leader lists.
const raidAskedNames = 12

// groupAsk is an invitation: raid, a raid flag's; declined, when it was refused, for a
// flag's refusal is kept for raidAskAgain (it no longer waits on an answer).
type groupAsk struct {
	raid     bool
	declined time.Time
	// invite is, on a caller's own box, whom its yes invites (raidcmds.go's /raidinvite and
	// /invite from one in no group); 0 is the flag's ask of everyone within reach
	invite EntityID
	// sent is the id the box named its inviter by: what the answer sends back,
	// though the asked one's view may since have moved the inviter to another
	// slot or out of it (answerInviterLocked)
	sent uint32
}

// RaidCall is /raid from sess's player. The lines are the World's.
func (w *World) RaidCall(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		// its client must take the box first (the file's doc), once
		if _, up := w.invites[p.ServerID][p.ServerID]; !up {
			w.askLocked(p, p, sess, groupAsk{raid: true}, raidOwnBoxWords)
			log.Printf("Group: %s raises a raid flag, in no group: asked first", p.Name)
		}
		w.sendChatToSession(sess, fmt.Sprintf("Accept the box to lead a raid group and ask everyone within %d.", int(RaidFlagRadius)))
		return
	}
	if len(g.Members) == 0 || g.Members[0] != p.ServerID && !(raidingLocked(g) && g.rules.helper == p.ServerID) {
		// a raid's helper raises it too (petition 1451: "The raid helper can invite other
		// people to the raid"), its box naming the leader's raid (raidAskLocked)
		w.sendColoredLocked(sess, "Only your group's leader (or a raid's helper) can raise a raid flag.", castRefusalRGB)
		return
	}
	w.raidFlagLocked(sess, p, time.Now())
}

// ownBoxYesLocked is a caller's yes to its own box: a group of one, its roster (member
// 0 is how its client knows it leads), and what the box was for: the flag, or the
// invite of ask.invite (raidcmds.go), a raid's when ask.raid. Must be called with the
// lock held.
func (w *World) ownBoxYesLocked(sess *session.Session, p *Player, ask groupAsk) {
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil { // a group since: its roster, its client in it now
		w.sendRosterLocked(g, true)
		return
	}
	g := w.formGroupLocked(p)
	w.sendRosterLocked(g, true)
	switch t := w.Players[ask.invite]; {
	case ask.invite == 0:
		w.raidFlagLocked(sess, p, time.Now())
	case ask.raid:
		w.raidInviteLocked(sess, p, g, t)
	default:
		w.groupInviteLocked(sess, p, t)
	}
}

// ownBoxNoLocked is a caller's answer other than yes to its own box. Its client
// answers 2 for itself when it holds itself in a group already, where the server has
// none: it needs no box then, so what the box was for goes on. Must be called with the
// lock held.
func (w *World) ownBoxNoLocked(sess *session.Session, p *Player, why byte, ask groupAsk) {
	if why == 2 && p.GroupID == 0 {
		w.ownBoxYesLocked(sess, p, ask)
	}
}

// raidAskedLocked reports leader's flag having nothing to ask of t: a box of its still
// up, or a refusal within raidAskAgain.
func (w *World) raidAskedLocked(t, leader *Player, now time.Time) (waiting, refused bool) {
	ask, ok := w.invites[t.ServerID][leader.ServerID]
	if !ok {
		return false, false
	}
	if ask.declined.IsZero() {
		return true, false
	}
	return false, now.Sub(ask.declined) < raidAskAgain
}

// raidFlagLocked raises leader's flag: everyone it reaches in no group, in the box
// (the file's doc), and a line to the leader; a GM hidden from players is not one of
// them. Must be called with the lock held.
func (w *World) raidFlagLocked(sess *session.Session, leader *Player, now time.Time) {
	if g := w.groups[leader.GroupID]; leader.GroupID != 0 && g != nil {
		g.flagged = true // a raid's group to its chat (raid.go's GroupIsRaid)
	}
	var asked []string
	waiting := 0
	for _, id := range w.SpatialIndex.QueryRadius(leader.X, leader.Z, RaidFlagRadius) {
		t := w.Players[id]
		if t == nil || t == leader || t.GroupID != 0 || t.World != leader.World || t.GMHidden ||
			dist3(t.X, t.Y, t.Z, leader.X, leader.Y, leader.Z) > RaidFlagRadius {
			continue
		}
		ts := w.sessionOfLocked(t)
		if ts == nil || !ts.CharacterInWorld() {
			continue
		}
		wait, refused := w.raidAskedLocked(t, leader, now)
		if wait {
			waiting++
		}
		if wait || refused {
			continue
		}
		w.raidAskLocked(t, leader, ts)
		asked = append(asked, t.Name)
	}
	sort.Strings(asked)
	log.Printf("Group: %s raises a raid flag: asks %v, %d still to answer", leader.Name, asked, waiting)
	w.sendChatToSession(sess, raidAskedLine(asked, waiting))
}

// raidAskLocked is t's box from leader's flag, with what it means. Must be called
// with the lock held.
func (w *World) raidAskLocked(t, leader *Player, ts *session.Session) {
	w.raidBoxLocked(t, leader, ts, leader.Name+" raises a raid flag.")
}

// raidBoxLocked is t's raid box from inviter, named for its group's leader's raid (its
// helper's ask's too, raidcmds.go), and a line: what brought it, and what a raid costs.
// Must be called with the lock held.
func (w *World) raidBoxLocked(t, inviter *Player, ts *session.Session, why string) {
	w.askLocked(t, inviter, ts, groupAsk{raid: true}, w.raidNameLocked(inviter)+"'s raid")
	w.sendChatToSession(ts, fmt.Sprintf("%s Past %d members a group takes %d%% less XP from kills.",
		why, GroupWindowSize, 100-RaidXPPct))
}

// raidNameLocked is the name a raid inviter's raid goes by: its group's leader's, else its
// own. Must be called with the lock held.
func (w *World) raidNameLocked(inviter *Player) string {
	if l := w.leaderOfLocked(w.groups[inviter.GroupID]); inviter.GroupID != 0 && l != nil {
		return l.Name
	}
	return inviter.Name
}

// raidAskedLine is the leader's line for a flag: who it asked, the first
// raidAskedNames by name, and how many are still to answer the last one.
func raidAskedLine(asked []string, waiting int) string {
	var b strings.Builder
	switch n := len(asked); {
	case n == 0:
		fmt.Fprintf(&b, "Raid flag: no one new within %d in no group.", int(RaidFlagRadius))
	case n > raidAskedNames:
		fmt.Fprintf(&b, "Raid flag: asked %s and %d more.", strings.Join(asked[:raidAskedNames], ", "), n-raidAskedNames)
	default:
		fmt.Fprintf(&b, "Raid flag: asked %s.", strings.Join(asked, ", "))
	}
	if waiting > 0 {
		fmt.Fprintf(&b, " %d still to answer.", waiting)
	}
	return b.String()
}
