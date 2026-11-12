package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// mercgroup.go — a merc takes a place in its owner's group (the owner,
// 2026-11-10: "mercs will occupy a group slot", as EverQuest's do).
//
// A merc is a member of its owner's group (Group.Members holds its NPC's id
// after its owner's): it counts toward the group's GroupWindowSize, so a group
// of four takes no more mercs (a hire, a revival or a login's coming back goes
// on standby), and a player takes a place before a merc: a player's yes into a
// group of four with a merc in it puts the merc that joined last on standby
// until a place frees (mercbench.go, 2026-11-12: up to three mercs a player);
// a group whose leader raised a raid flag takes one whatever its size
// (raidflag.go: a raid group is the flag's). It shows in the group window as
// any member: the roster (0x0772) names it under the id each client knows it
// by (its channel's, else groupFarID's stand-in) and the group channel (0x40)
// carries its health and its place; CLIENT's window needs no more (DrawGroup:
// a name, a health gauge, and the status icons of the entity it finds by that
// id, a creature's flags as a player's), and nothing of CLIENT's asks a member
// to be a player but its invite (InviteEntity's m_p256 == 2), which no one
// sends a merc. A group's heal or buff reaches it (cast.go: castTargetsLocked's
// group, groupMercsLocked); its own group heals reach the group's players and
// mercs (merccast.go). Experience is the players' alone: killexp.go's split
// takes the group's players (groupMembersLocked), so a merc takes no share and
// cuts none; a raid group's cut is the raid flag's (only a flagged group passes
// four with a merc in it).
//
// CLIENT holds itself in a group only after its own Invite or Accept (m_inGroup,
// which no message sets: raidflag.go), and Msg0772 returns without it. So a
// player in no group with a merc is asked into one, in the group invitation's
// own box, as a raid flag's caller is asked: from itself, named "Mercenary
// <name>" (CLIENT's word 636 is "<name> invites you to join a group"); its yes
// makes the group of two, its owner leading (member 0), and its client shows
// the merc in it. A merc fights only in its owner's group (petition 1504,
// Lifeispain: "I declined but it is still following me around ... It still is
// fighting for me even though not in group"): while its box (or another's box
// it waits behind) is up it stands where it is and fights nothing, and its no
// dismisses it, as /merc dismiss does (its hire ended, its row dropped). The
// merc leaves the group when it is dismissed, dies, or its owner leaves the
// group or the world; the group is then shrunk as for any member (one left
// alone is told it is disbanded). Its owner out of a group of others (it left,
// was booted, or its leader disbanded it) is asked again, as at a hire; out of
// the group of it and its merc alone (its own leave or disband) it is
// dismissed, as at a no; in a group with no place for it, too. A leader's boot
// of a merc (0x0626 naming it) dismisses it, its owner told.
//
// A group of a player and its merc alone is no bar to another player's box (an
// invitation, a raid flag's): CLIENT shows none to a client in a group (Msg0773
// declines it for itself, 2), so that group gives way first, its client told
// with 0x069A's 8 (CLIENT's removal with no line: m_inGroup cleared); a yes
// takes the merc into the group its owner joins (room for both is checked
// before the box), a no asks its owner into a group with it again. A merc's own
// box waits while any other box is up on its owner's client.

// mercBoxWords is what a merc's own box says it is from (CLIENT's word 636);
// mercsBoxWords several mercs' one box.
const (
	mercBoxWords  = "Mercenary %s"
	mercsBoxWords = "Mercenaries (%d)"
)

// The lines of a merc in a group (ours).
const (
	mercGroupFull   = "Your group is full: a mercenary takes a place in it, and it has none."
	mercNoRoomLine  = "%s's mercenary needs a place in the group too, and the group has room for one."
	mercNoRoomYours = "The group has no room for you and your mercenary."
	mercBootedLine  = "%s removed your mercenary %s from the group: it is dismissed."
	mercAskLine     = "Accept the box to have %s in your group: a mercenary fights only in your group, and a no dismisses it."
	mercBoxUpLine   = "Answer the group box you have up first."
	mercOutLine     = "Your mercenary %s is dismissed: a mercenary fights only in your group. The Mercenary Recruitment board will hire you another."
	mercOutFullLine = "Your mercenary %s is dismissed: your group has no place for it."
)

// groupView is one member as a client's group window shows it: a player of
// the group, or a merc.
type groupView struct {
	id      EntityID
	name    string
	hp, top int
	x, y, z float32
}

// memberViewLocked is a group member's view: a player, or a merc (its NPC),
// false for neither. Must be called with the lock held.
func (w *World) memberViewLocked(id EntityID) (groupView, bool) {
	if p := w.Players[id]; p != nil {
		return groupView{id: p.ServerID, name: p.Name, hp: p.HP, top: p.MaxHPNow(), x: p.X, y: p.Y, z: p.Z}, true
	}
	if n := w.NPCs[id]; n.isMerc() && !n.Dead {
		return groupView{id: n.ServerID, name: n.Name, hp: n.HP, top: n.MaxHP, x: n.X, y: n.Y, z: n.Z}, true
	}
	return groupView{}, false
}

// shownViewsLocked is shownMembersLocked with the group's mercs: the members
// of g that m's client is told of, in the order they joined, m itself and at
// most GroupWindowSize - 1 others (raid.go). Must be called with the lock held.
func (w *World) shownViewsLocked(g *Group, m *Player) []groupView {
	out := make([]groupView, 0, GroupWindowSize)
	others := GroupWindowSize - 1
	for _, id := range g.Members {
		v, ok := w.memberViewLocked(id)
		switch {
		case !ok:
		case id == m.ServerID:
			out = append(out, v)
		case others > 0:
			out = append(out, v)
			others--
		}
	}
	return out
}

// groupIDSeenByLocked is the id the receiver knows a member by: its own player
// object id for itself, its channel's for one it sees, else groupFarID's
// stand-in. Must be called with the lock held.
func (w *World) groupIDSeenByLocked(recv *session.Session, id EntityID) uint32 {
	if p := w.sessionPlayerLocked(recv); p != nil && p.ServerID == id {
		return recv.PlayerObjectID()
	}
	if oid := w.objectIDForEntityLocked(recv, id); oid != 0 {
		return oid
	}
	return groupFarID(id)
}

// groupRecord is a member's 13 bytes of the group channel (0x40, CLIENT's
// Channel40): its id, its health known and of 255, a byte, and its place in
// whole units.
func groupRecord(r []byte, id uint32, v groupView) {
	r[0], r[1], r[2], r[3] = byte(id), byte(id>>8), byte(id>>16), byte(id>>24)
	r[4] = 1
	hp := 0
	if v.top > 0 {
		hp = int(healthBar(v.hp, v.top))
	}
	r[5] = byte(max(0, min(255, hp)))
	r[6] = 0
	for c, f := range []float32{v.x, v.y, v.z} {
		s := int16(max(math.MinInt16, min(math.MaxInt16, int(f))))
		r[7+2*c], r[8+2*c] = byte(s), byte(uint16(s)>>8)
	}
}

// groupHasLocked reports id among g's members.
func groupHas(g *Group, id EntityID) bool {
	for _, m := range g.Members {
		if m == id {
			return true
		}
	}
	return false
}

// mercGroupJoinLocked puts a merc just come into its owner's group, its roster
// sent; with its owner in no group, asks its owner into one with it (the
// file's doc). sess is its owner's. Must be called with the lock held.
func (w *World) mercGroupJoinLocked(ms *mercState, sess *session.Session) {
	o := ms.owner
	if g := w.groups[o.GroupID]; o.GroupID != 0 && g != nil {
		w.mercIntoGroupLocked(ms, g)
		return
	}
	w.mercAskLocked(ms, sess)
}

// mercAskLocked asks the merc's owner, in no group, into one with its mercs,
// in the group invitation's own box (the file's doc): one box for every merc
// of its out of a group and not asked yet. Must be called with the lock held.
func (w *World) mercAskLocked(ms *mercState, sess *session.Session) {
	o := ms.owner
	if sess == nil || o.GroupID != 0 || ms.asked || w.mercAskHeld[o.ServerID] {
		return
	}
	if w.boxWaitingLocked(o) {
		// a box up already (another player's, or its own raid flag's, whose yes
		// forms the group, the merc in it): asked once it is answered no
		return
	}
	var names []string
	for _, m := range w.mercsOfLocked(o) {
		if !m.asked && !w.mercInOwnGroupLocked(m) {
			m.asked = true
			names = append(names, m.npc.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	// the box's name is short in CLIENT's box (a rig's "Mercenaries
	// Ironwall, M"): several are counted there, named in the line
	who := strings.Join(names, " and ")
	words := fmt.Sprintf(mercBoxWords, who)
	if len(names) > 1 {
		who = strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
		words = fmt.Sprintf(mercsBoxWords, len(names))
	}
	w.askLocked(o, o, sess, groupAsk{merc: true}, words)
	w.sendChatToSession(sess, fmt.Sprintf(mercAskLine, who))
	log.Printf("Merc: %s asked into a group with %q", o.Name, names)
}

// HoldMercAsk holds the box asking the session's player into a group with
// its mercs (on) while the handlers bring several back at once (a login's
// every hire), so one box asks for all of them; off lets it go and asks it.
func (w *World) HoldMercAsk(sess *session.Session, on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if on {
		if w.mercAskHeld == nil {
			w.mercAskHeld = map[EntityID]bool{}
		}
		w.mercAskHeld[p.ServerID] = true
		return
	}
	delete(w.mercAskHeld, p.ServerID)
	for _, ms := range w.mercsOfLocked(p) {
		if !w.mercInOwnGroupLocked(ms) {
			if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
				w.mercIntoGroupLocked(ms, g)
				continue
			}
			w.mercAskLocked(ms, sess)
		}
	}
}

// AskMercGroup asks the session's player into a group with its mercs again
// (/merc group). It reports a line for the player, "" when the box was sent.
func (w *World) AskMercGroup(sess *session.Session) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	list := w.mercsOfLocked(p)
	switch {
	case len(list) == 0:
		return "You have no mercenary out."
	case p.GroupID != 0:
		g := w.groups[p.GroupID]
		if g == nil {
			return mercGroupFull
		}
		for _, ms := range list {
			w.mercIntoGroupLocked(ms, g)
		}
		w.sendRosterLocked(g, true)
		var in []string
		for _, ms := range w.mercsOfLocked(p) {
			if groupHas(g, ms.npc.ServerID) {
				in = append(in, ms.npc.Name)
			}
		}
		if len(in) == 0 {
			return mercGroupFull
		}
		return strings.Join(in, ", ") + " in your group."
	case w.boxWaitingLocked(p):
		return mercBoxUpLine
	}
	for _, ms := range list {
		ms.asked = false
	}
	w.mercAskLocked(list[0], sess)
	return ""
}

// boxWaitingLocked reports a group box up on p's client still to be answered:
// an invitation, a raid flag's, or its own. Must be called with the lock held.
func (w *World) boxWaitingLocked(p *Player) bool {
	for _, ask := range w.invites[p.ServerID] {
		if ask.declined.IsZero() {
			return true
		}
	}
	return false
}

// mercOnlyGroupLocked reports p's group is p and its mercs alone, with no
// raid flag raised: one that gives way to another player's box (the file's
// doc). Must be called with the lock held.
func (w *World) mercOnlyGroupLocked(p *Player) bool {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil || g.flagged || len(g.Members) < 2 || g.Members[0] != p.ServerID {
		return false
	}
	for _, id := range g.Members[1:] {
		if !w.mercHiredByLocked(id, p.ServerID) {
			return false
		}
	}
	return true
}

// mercGivesWayLocked ends p's group of it and its mercs alone, for another
// player's box to come (the file's doc): the group gone, p's client told with
// 0x069A's 8, and p asked into a group with its mercs again should it say no.
// Must be called with the lock held.
func (w *World) mercGivesWayLocked(p *Player) {
	if !w.mercOnlyGroupLocked(p) {
		return
	}
	g := w.groups[p.GroupID]
	delete(w.groups, g.ID)
	p.GroupID = 0
	for _, ms := range w.mercsOfLocked(p) {
		ms.asked = false
	}
	w.sendGroupRemoved(p, 8)
	log.Printf("Group: %s's group %d with its mercenaries gives way to a box", p.Name, g.ID)
}

// mercIntoGroupLocked adds the merc to g after its owner, the roster to all;
// g with no place for it (a plain group of four), it goes on standby
// (mercbench.go). Must be called with the lock held.
func (w *World) mercIntoGroupLocked(ms *mercState, g *Group) {
	id := ms.npc.ServerID
	if groupHas(g, id) {
		return
	}
	if !g.flagged && len(g.Members) >= GroupWindowSize {
		w.benchMercLocked(ms, mercStandbyFullLine, "", time.Now())
		return
	}
	ms.full = false
	g.Members = append(g.Members, id)
	log.Printf("Group: %s's mercenary %q joined group %d (%d members)", ms.owner.Name, ms.npc.Name, g.ID, len(g.Members))
	w.sendRosterLocked(g, true)
	w.raidChangedLocked(g)
}

// mercFollowIntoLocked puts p's mercs in the group p is in now (its join, its
// forming one), by slot, those with no place on standby. Must be called with
// the lock held.
func (w *World) mercFollowIntoLocked(p *Player) {
	for _, ms := range w.mercsOfLocked(p) {
		g := w.groups[p.GroupID]
		if p.GroupID == 0 || g == nil {
			return
		}
		w.mercIntoGroupLocked(ms, g)
	}
}

// mercsOutOfGroupLocked takes p's mercs out of g's members (its leaving it,
// its boot), each asked into a group with its owner again when others are
// left in g (again: true; whenOthers: when g has members left without
// them). Must be called with the lock held.
func (w *World) mercsOutOfGroupLocked(g *Group, p *Player, again, whenOthers bool) {
	var mine []*mercState
	for _, ms := range w.mercs[p.ServerID] {
		if ms.owner != p {
			continue
		}
		for i, id := range g.Members {
			if id == ms.npc.ServerID {
				g.Members = append(g.Members[:i], g.Members[i+1:]...)
				break
			}
		}
		delete(g.lastRoster, ms.npc.ServerID)
		mine = append(mine, ms)
	}
	if again || whenOthers && len(g.Members) > 0 {
		for _, ms := range mine {
			ms.asked = false
		}
	}
}

// mercLeaveGroupLocked takes the merc out of any group it is in, the group
// shrunk as for any member who leaves. Must be called with the lock held.
func (w *World) mercLeaveGroupLocked(ms *mercState) {
	id := ms.npc.ServerID
	for _, g := range w.groups {
		for i, m := range g.Members {
			if m != id {
				continue
			}
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			delete(g.lastRoster, id)
			log.Printf("Group: %s's mercenary %q left group %d (%d left)", ms.owner.Name, ms.npc.Name, g.ID, len(g.Members))
			// no "left the raid group" line for a merc: a merc dying in a raid
			// read as a player leaving (the owner, 2026-11-11: "dont show it in
			// the raid chat as them leaving, it looks like a player left and is
			// confusing"); its owner is told its own line (died, dismissed)
			w.shrinkLocked(g)
			return
		}
	}
}

// mercInOwnGroupLocked reports the merc a member of its owner's group. Must
// be called with the lock held.
func (w *World) mercInOwnGroupLocked(ms *mercState) bool {
	g := w.groups[ms.owner.GroupID]
	return ms.owner.GroupID != 0 && g != nil && groupHas(g, ms.npc.ServerID)
}

// mercOutLocked dismisses a merc out of its owner's group with nothing to
// ask (the file's doc): gone, its owner told the line, the handlers told its
// hire is over. Must be called with the lock held.
func (w *World) mercOutLocked(ms *mercState, sess *session.Session, line string) {
	name := ms.npc.Name
	w.mercGoesLocked(ms, sess, ms.last)
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(line, name))
	}
	w.noteMercLocked(MercNote{Kind: MercNoteDismissed, Owner: ms.owner, Sess: sess, Name: name})
	log.Printf("Merc: %s's %q dismissed out of its owner's group", ms.owner.Name, name)
}

// mercGroupUpkeepLocked keeps the merc in its owner's group, and in no other:
// its owner joined one, or its group went (a disband); one out of it with no
// box to wait on is dismissed (the file's doc). Every 5 ticks, with the group
// system; must be called with the lock held.
func (w *World) mercGroupUpkeepLocked(ms *mercState, sess *session.Session) {
	if w.TickCount%5 != 0 {
		return
	}
	o, id := ms.owner, ms.npc.ServerID
	own := w.groups[o.GroupID]
	if o.GroupID == 0 {
		own = nil
	}
	for _, g := range w.groups {
		if g != own && groupHas(g, id) {
			ms.asked = false
			w.mercLeaveGroupLocked(ms) // a group its owner left behind
			break
		}
	}
	switch {
	case own != nil:
		ms.regroup = false
		w.mercIntoGroupLocked(ms, own) // no place: on standby (mercbench.go)
	case ms.regroup:
		// a keep restart's: its owner's client holds the group of the two of
		// them, which no box need ask again (keepmerc.go)
		ms.regroup = false
		w.sendRosterLocked(w.formGroupLocked(o), true)
	case !ms.asked:
		// out of a group of others, or a box its group gave way to answered no:
		// asked again, once no other box is up (mercAskLocked)
		w.mercAskLocked(ms, sess)
	case sess != nil && !w.boxWaitingLocked(o):
		// asked, and out of a group with no box up: its no, or its owner's
		// leave of the group of the two of them
		w.mercOutLocked(ms, sess, mercOutLine)
	}
}

// mercOwnYesLocked is a merc owner's yes to its own box: a group with its merc
// (formGroupLocked puts the merc in), its roster to its client (member 0, the
// owner, is how its client knows it leads). In a group already (it joined one
// since), the merc is in that one. Must be called with the lock held.
func (w *World) mercOwnYesLocked(sess *session.Session, p *Player) {
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		w.mercFollowIntoLocked(p)
		w.sendRosterLocked(g, true)
		return
	}
	if len(w.mercsOfLocked(p)) == 0 {
		w.sendGroupRemoved(p, 3) // its mercs gone since: no group
		return
	}
	g := w.formGroupLocked(p)
	w.sendRosterLocked(g, true)
}

// mercOwnNoLocked is a merc owner's answer other than yes to its own box. Its
// client answers 2 for itself when it holds itself in a group already, where
// the server has none (raidFlagOwnNoLocked's case): that is a yes. Must be
// called with the lock held.
func (w *World) mercOwnNoLocked(sess *session.Session, p *Player, why byte) {
	if why == 2 && p.GroupID == 0 {
		w.mercOwnYesLocked(sess, p)
		return
	}
	log.Printf("Merc: %s declined a group with its mercenaries", p.Name)
	for _, ms := range w.mercsOfLocked(p) {
		if ms.asked && !w.mercInOwnGroupLocked(ms) {
			w.mercOutLocked(ms, sess, mercOutLine) // petition 1504: a declined merc goes
		}
	}
}

// groupMercsLocked adds to a group spell's targets the mercs of the caster's
// group, alive, in its world and its range (cast.go's castTargetsLocked). Must
// be called with the lock held.
func (w *World) groupMercsLocked(p *Player, ids []EntityID, rng float32) []EntityID {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return ids
	}
	for _, id := range g.Members {
		n := w.NPCs[id]
		if !n.isMerc() || n.Dead || !w.npcSharesLocked(n, p) || outOfRange(p, n.X, n.Y, n.Z, rng) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// mercSeenByLocked is the merc of g an id the sender gave names (its channel's
// id, or groupFarID's stand-in), nil for none. Must be called with the lock
// held.
func (w *World) mercSeenByLocked(sender *session.Session, g *Group, id uint32) *mercState {
	var e EntityID
	if id&0xFF000000 == 0x7F000000 {
		e = EntityID(id & 0x00FFFFFF)
	} else if vs, ok := sender.VisibilityState().(*VisibilityState); ok && vs != nil {
		for i := range vs.ChannelObjectID {
			if vs.ChannelObjectID[i] == id {
				e = vs.ChannelEntity[i]
				break
			}
		}
	}
	n := w.NPCs[e]
	if e == 0 || !n.isMerc() || !groupHas(g, e) {
		return nil
	}
	return n.merc
}

// mercBootedLocked is a leader's boot of a merc in its group: dismissed, its
// owner told, the handlers told its hire is over. Must be called with the
// lock held.
func (w *World) mercBootedLocked(leader *Player, ms *mercState) {
	sess := w.sessionOfLocked(ms.owner)
	name := ms.npc.Name
	w.mercGoesLocked(ms, sess, ms.last)
	if sess != nil && ms.owner != leader {
		w.sendColoredLocked(sess, fmt.Sprintf(mercBootedLine, leader.Name, name), castRefusalRGB)
	}
	w.noteMercLocked(MercNote{Kind: MercNoteDismissed, Owner: ms.owner, Sess: sess, Name: name})
	log.Printf("Group: %s booted %s's mercenary %q", leader.Name, ms.owner.Name, name)
}
