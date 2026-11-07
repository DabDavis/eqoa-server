package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/session"
)

// raid.go — a group of more than four is a raid group (the user, 2026-11-07: "lets
// remove group member cap, but if over 4 it is a raid group, with 80% xp cut"), and
// only a leader's raid flag takes one past four (raidflag.go).
//
// A raid group has no cap on the server. CLIENT holds four: VIClient's m_group[4] is
// followed at once by its count, m_p2BE4C, and Msg0772 (decomp viclient.cpp) reads
// the count with no bound and writes each member through it, so a fifth id lands on
// the count and the loop runs on through memory, 3 bytes in every 52 (a tracer's call of
// it never came back from one id, and took 9 M instructions from another). Vanilla stops
// at four but reads the names from where the fifth id starts.
// So each member's client is told of four at most (shownMembersLocked), in its own
// roster and its own 0x40 records, and to any client a raid group looks like a group
// of four: the leader (member 0 is how CLIENT knows who leads), the member itself,
// and the earliest of the others to join. That choice moves only when one of those
// leaves: a newcomer changes no one's window, and each window keeps its order.
//
// The rest is the server's alone: experience, chat, loot lines, group spells, quest
// credit, invites and the rest all read Group.Members.
//
// The cut is on a kill's whole award (awardKillXPLocked) by the group's size, whoever
// of it is near the kill, so a boon's, a debt's and the masteries' shares (awardXP)
// are shares of what is left. A quest's experience is not a kill's and is not cut.

// RaidXPPct is the share of a kill's experience a raid group's member gets (the
// user's 80% cut).
const RaidXPPct = 20

// The lines a raid group's members are told. Ours, in a PS2's own characters
// (`%` is safe: Msg0A7A hands the wide string to AddText, no format).
var (
	raidNowLine    = fmt.Sprintf("Your group is now a raid group: %d%% less XP from kills. Bars show %d others at most.", 100-RaidXPPct, GroupWindowSize-1)
	raidJoinedLine = fmt.Sprintf("You joined a raid group: %d%% less XP from kills. Bars show %d others at most.", 100-RaidXPPct, GroupWindowSize-1)
	raidEndLine    = "Your group is no longer a raid group: the XP cut ends."
	raidFullLine   = fmt.Sprintf("This group is full at %d. Its leader's /raid (on a PS2, /t raid) asks everyone within %d to a raid group: %d%% less XP from kills.", GroupWindowSize, int(RaidFlagRadius), 100-RaidXPPct)
	raidStillLine  = fmt.Sprintf("You are still in a raid group: %d%% less XP from kills. Logging out leaves it.", 100-RaidXPPct)
)

// isRaid is whether g is a raid group.
func (g *Group) isRaid() bool { return len(g.Members) > GroupWindowSize }

// inRaidLocked is whether p is in a raid group. Must be called with the lock held.
func (w *World) inRaidLocked(p *Player) bool {
	g := w.groups[p.GroupID]
	return p.GroupID != 0 && g != nil && g.isRaid()
}

// raidShare is what a raid group's member gets of a kill's xp.
func raidShare(xp int) int { return xp * RaidXPPct / 100 }

// shownMembersLocked is the members of g that m's client is told of, in the order
// they joined: all of them in a group of GroupWindowSize or fewer, as ever; past
// that m itself and the earliest of the others (the leader first among them), so
// never more than GroupWindowSize. Must be called with the lock held.
func (w *World) shownMembersLocked(g *Group, m *Player) []*Player {
	out := make([]*Player, 0, GroupWindowSize)
	others := GroupWindowSize - 1 // one place is m's own
	for _, id := range g.Members {
		switch o := w.Players[id]; {
		case o == nil:
		case o == m:
			out = append(out, o)
		case others > 0:
			out = append(out, o)
			others--
		}
	}
	return out
}

// groupSayLocked tells every member of g a line, but except. Must be called with the
// lock held.
func (w *World) groupSayLocked(g *Group, line string, except *Player) {
	for _, id := range g.Members {
		if o := w.Players[id]; o != except {
			if s := w.sessionOfLocked(o); s != nil {
				w.sendChatToSession(s, line)
			}
		}
	}
}

// raidChangedLocked tells g's members when it has become a raid group or stopped
// being one. Must be called with the lock held.
func (w *World) raidChangedLocked(g *Group) {
	now := g.isRaid()
	if now == g.raid {
		return
	}
	g.raid = now
	log.Printf("Group: group %d, %d members, raid group %v", g.ID, len(g.Members), now)
	if now {
		w.groupSayLocked(g, raidNowLine, nil)
	} else {
		w.groupSayLocked(g, raidEndLine, nil)
	}
}

// raidJoinedLocked is p's joining g, its session sess: a client's window may never
// show a raid group's newcomer, so the group is told by name, and the newcomer of a
// group already raiding is told what it joined. Must be called with the lock held.
func (w *World) raidJoinedLocked(g *Group, p *Player, sess *session.Session) {
	p.raidReminder = false // it has just been told
	if g.isRaid() {
		w.groupSayLocked(g, fmt.Sprintf("%s joined the raid group (%d members).", p.Name, len(g.Members)), p)
	}
	if g.raid {
		w.sendChatToSession(sess, raidJoinedLine)
	}
	w.raidChangedLocked(g)
}

// raidLeftLocked is one of a raid group's members having left it, named for those
// whose windows may never have shown it. Must be called with the lock held.
func (w *World) raidLeftLocked(g *Group, name string) {
	if g.raid {
		w.groupSayLocked(g, fmt.Sprintf("%s left the raid group (%d members).", name, len(g.Members)), nil)
	}
}

// raidRemindLocked tells a member whose client was replaced that its group is a raid
// group, once it is in the world: a new client is told nothing of a group it did not
// join (Msg0772 returns unless InGroup, which only an invite or an accept sets), and
// the cut goes on. Must be called with the lock held.
func (w *World) raidRemindLocked(g *Group, m *Player, sess *session.Session) {
	if !m.raidReminder {
		return
	}
	if !g.isRaid() {
		m.raidReminder = false
	} else if sess.CharacterInWorld() {
		m.raidReminder = false
		w.sendChatToSession(sess, raidStillLine)
	}
}
