package entity

import (
	"fmt"
	"log"
	"math"

	"github.com/eqoa/server/internal/session"
)

// mercgroup.go — a merc takes a place in its owner's group (the owner,
// 2026-11-10: "mercs will occupy a group slot", as EverQuest's do).
//
// A merc is a member of its owner's group (Group.Members holds its NPC's id
// after its owner's): it counts toward the group's GroupWindowSize, so a group
// of four takes no merc (a hire, a revival or a login's coming back is refused
// with a line) and a player with its merc out joins a group only with room for
// both; a group whose leader raised a raid flag takes one whatever its size
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
// the merc in it. Its no leaves the merc out of any group until /merc group asks
// again, its owner joins one (the merc with it) or invites someone (the merc in
// the group that makes). The merc leaves the group when it is dismissed, dies,
// or its owner leaves the group or the world; the group is then shrunk as for
// any member (one left alone is told it is disbanded). A leader's boot of a
// merc (0x0626 naming it) dismisses it, its owner told.

// mercBoxWords is what a merc's own box says it is from (CLIENT's word 636).
const mercBoxWords = "Mercenary %s"

// The lines of a merc in a group (ours).
const (
	mercGroupFull   = "Your group is full: a mercenary takes a place in it, and it has none."
	mercNoRoomLine  = "%s's mercenary needs a place in the group too, and the group has room for one."
	mercNoRoomYours = "The group has no room for you and your mercenary."
	mercBootedLine  = "%s removed your mercenary %s from the group: it is dismissed."
	mercAskLine     = "Accept the box to have %s in your group."
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

// followersLocked is how many places p's merc takes in a group p joins: 1 with
// one out, else 0. Must be called with the lock held.
func (w *World) followersLocked(p *Player) int {
	if w.mercOfLocked(p) != nil {
		return 1
	}
	return 0
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

// mercGroupRoomLocked is why p's group has no room for a merc of its (a hire
// or a revival in place of old, which gives back its place), "" when it has
// one or p is in no group. Must be called with the lock held.
func (w *World) mercGroupRoomLocked(p *Player, old *mercState) string {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil || g.flagged {
		return ""
	}
	n := len(g.Members)
	if old != nil && groupHas(g, old.npc.ServerID) {
		n--
	}
	if n >= GroupWindowSize {
		return mercGroupFull
	}
	return ""
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

// mercAskLocked asks the merc's owner, in no group, into one with its merc, in
// the group invitation's own box (the file's doc), once a coming. Must be
// called with the lock held.
func (w *World) mercAskLocked(ms *mercState, sess *session.Session) {
	o := ms.owner
	if sess == nil || o.GroupID != 0 || ms.asked {
		return
	}
	if _, up := w.invites[o.ServerID][o.ServerID]; up {
		return // its own box is up already (a raid flag's): its yes forms the group, the merc in it
	}
	ms.asked = true
	w.askLocked(o, o, sess, groupAsk{merc: true}, fmt.Sprintf(mercBoxWords, ms.npc.Name))
	w.sendChatToSession(sess, fmt.Sprintf(mercAskLine, ms.npc.Name))
	log.Printf("Merc: %s asked into a group with %q", o.Name, ms.npc.Name)
}

// AskMercGroup asks the session's player into a group with its merc again
// (/merc group). It reports a line for the player, "" when the box was sent.
func (w *World) AskMercGroup(sess *session.Session) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	ms := w.mercOfLocked(p)
	switch {
	case ms == nil:
		return "You have no mercenary out."
	case p.GroupID != 0:
		if g := w.groups[p.GroupID]; g != nil && groupHas(g, ms.npc.ServerID) {
			w.sendRosterLocked(g, true)
			return fmt.Sprintf("%s is in your group.", ms.npc.Name)
		}
		return mercGroupFull
	}
	ms.asked = false
	w.mercAskLocked(ms, sess)
	return ""
}

// mercIntoGroupLocked adds the merc to g after its owner, the roster to all,
// unless g has no room for it (told its owner once). Must be called with the
// lock held.
func (w *World) mercIntoGroupLocked(ms *mercState, g *Group) {
	id := ms.npc.ServerID
	if groupHas(g, id) {
		return
	}
	if !g.flagged && len(g.Members) >= GroupWindowSize {
		if !ms.full {
			ms.full = true
			if sess := w.sessionOfLocked(ms.owner); sess != nil {
				w.sendColoredLocked(sess, mercGroupFull, castRefusalRGB)
			}
		}
		return
	}
	ms.full = false
	g.Members = append(g.Members, id)
	log.Printf("Group: %s's mercenary %q joined group %d (%d members)", ms.owner.Name, ms.npc.Name, g.ID, len(g.Members))
	w.sendRosterLocked(g, true)
	w.raidChangedLocked(g)
}

// mercFollowIntoLocked puts p's merc in the group p is in now (its join, its
// forming one). Must be called with the lock held.
func (w *World) mercFollowIntoLocked(p *Player) {
	ms := w.mercOfLocked(p)
	g := w.groups[p.GroupID]
	if ms == nil || p.GroupID == 0 || g == nil {
		return
	}
	w.mercIntoGroupLocked(ms, g)
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
			w.raidLeftLocked(g, ms.npc.Name)
			w.shrinkLocked(g)
			return
		}
	}
}

// mercGroupUpkeepLocked keeps the merc in its owner's group, and in no other:
// its owner joined one, or its group went (a disband). Every 5 ticks, with the
// group system; must be called with the lock held.
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
		w.mercIntoGroupLocked(ms, own)
	case ms.regroup:
		// a keep restart's: its owner's client holds the group of the two of
		// them, which no box need ask again (keepmerc.go)
		ms.regroup = false
		w.sendRosterLocked(w.formGroupLocked(o), true)
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
	if w.mercOfLocked(p) == nil {
		w.sendGroupRemoved(p, 3) // its merc gone since: no group
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
	log.Printf("Merc: %s declined a group with its mercenary", p.Name)
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
		if !n.isMerc() || n.Dead || n.World != p.World || outOfRange(p, n.X, n.Y, n.Z, rng) {
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
