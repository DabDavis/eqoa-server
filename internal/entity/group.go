package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Groups, as CLIENT keeps one (VIClient's m_group, m_inGroup, m_p2BD78) and
// reads it off the wire (the decomp, modules/client/src/viclient.cpp):
//
//	0x0625 in   invite: u32 the target's id, string8 its name (InviteEntity), from any
//	            member here (GroupInvite)
//	0x0622 in   accept: u32 the inviter's id (AcceptInvite)
//	0x0624 in   decline: u32 the inviter's id, u8 1 declined / 2 already grouped
//	0x0623 in   leave;  0x0626 in  remove a member (u32 id);  0x0627 in  disband
//	0x0773 out  an invitation: u32 inviter id, u32 (unread), string8 name (Msg0773)
//	0x0774 out  an invitation withdrawn: u32 inviter id (Msg0774)
//	0x0772 out  the roster: u32 (unread), u32 count, count ids, count string8 names,
//	            the leader first (the receiver leads when it is member 0) (Msg0772)
//	0x069A out  out of the group: u8 1 ejected (630), 2 disbanded (631),
//	            3 not invited (632), 4 full (633)
//	0x069C out  to the inviter: u8 1 declined (558, + name), 2 already in a
//	            group (628, + name), 3 recipient not found (627)
//	0x40 state  the other members, 13 bytes each (Channel40): u32 id, u8 health
//	            known, u8 health of 255, s8, three s16 coordinates
//
// An id is the one the RECEIVER knows the member by: its own player object id
// for itself, the visibility channel's object id for another. A member out of
// sight has none, so it is given a stand-in (groupFarID) and the roster is sent
// again whenever the ids a receiver knows change.
//
// An ordinary invite stops at GroupWindowSize; only a leader's raid flag asks past
// it, and then it is a raid group and each member's client is told of
// GroupWindowSize of them alone, unless it asks for them all (raid.go, raidflag.go).

// GroupWindowSize is m_group's length, the most members a client is ever told
// of: Msg0772 reads a roster's count with no bound, and a fifth member's id
// lands on the count itself (raid.go).
const GroupWindowSize = 4

// Group is one group: its members in the order they joined, the leader first.
type Group struct {
	ID      uint32
	Members []EntityID
	// lastRoster is the roster each member was last sent, by member.
	lastRoster map[EntityID]string
	// raid is what its members were last told it is (raidChangedLocked)
	raid bool
	// flagged: its leader raised a raid flag (raidflag.go): a raid's group,
	// whatever its size, to its chat (GroupIsRaid)
	flagged bool
	// rules: a raid's loot rule, helper and main assist (raidrules.go)
	rules raidRules
}

// GroupMembers is p's group's members (the leader first), or none.
func (w *World) GroupMembers(p *Player) []*Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return nil
	}
	out := make([]*Player, 0, len(g.Members))
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil {
			out = append(out, m)
		}
	}
	return out
}

// groupFarID stands in for a member the receiver cannot see.
func groupFarID(id EntityID) uint32 { return 0x7F000000 | uint32(id) }

// sessionOfLocked finds a player's session. Must be called with lock held.
func (w *World) sessionOfLocked(p *Player) *session.Session {
	var found *session.Session
	if w.SessionManager == nil || p == nil {
		return nil
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if w.sessionPlayerLocked(s) == p {
			found = s
			return false
		}
		return true
	})
	return found
}

// idSeenByLocked is the id the receiver knows a member by.
func (w *World) idSeenByLocked(recv *session.Session, member *Player) uint32 {
	if w.sessionPlayerLocked(recv) == member {
		return recv.PlayerObjectID()
	}
	if id := w.objectIDForEntityLocked(recv, member.ServerID); id != 0 {
		return id
	}
	return groupFarID(member.ServerID)
}

// IDSeenBy is the id the receiver knows a player by (its own object id,
// the visibility channel's, or groupFarID's stand-in), for lists that name
// players: the who list's rows carry it, and an invite from one sends it back.
func (w *World) IDSeenBy(recv *session.Session, p *Player) uint32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.idSeenByLocked(recv, p)
}

// PlayerByEntity is the player of that entity id in the world, or nil.
func (w *World) PlayerByEntity(id EntityID) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Players[id]
}

// PlayerSeenBy resolves an id the sender gave to a player (playerSeenByLocked).
func (w *World) PlayerSeenBy(sender *session.Session, id uint32) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.playerSeenByLocked(sender, id)
}

// playerSeenByLocked resolves an id the sender gave (its own, an object id
// from ITS visibility channels, or a stand-in) to a player. Object ids are per
// session, so only the sender's own table is read (two clients that see each
// other first both call the other 100001), as findEntityByObjectIDLocked
// reads it too.
func (w *World) playerSeenByLocked(sender *session.Session, id uint32) *Player {
	if id&0xFF000000 == 0x7F000000 {
		return w.Players[EntityID(id&0x00FFFFFF)]
	}
	if id == sender.PlayerObjectID() {
		return w.sessionPlayerLocked(sender)
	}
	vs, ok := sender.VisibilityState().(*VisibilityState)
	if !ok || vs == nil {
		return nil
	}
	for i := range vs.ChannelObjectID {
		if vs.ChannelObjectID[i] == id {
			return w.Players[vs.ChannelEntity[i]]
		}
	}
	return nil
}

func sendGroupMsg(sess *session.Session, op protocol.GameOpcode, body func(*buffer.Writer)) {
	if sess == nil {
		return
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(op))
	body(w)
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

func writeName8(w *buffer.Writer, s string) {
	b := []byte(s)
	if len(b) > 23 {
		b = b[:23]
	}
	w.WriteUint32(uint32(len(b)))
	w.WriteBytes(b)
}

func (w *World) sendGroupNotice(sess *session.Session, sub byte, name string) {
	sendGroupMsg(sess, protocol.OpcodeGroupStuff, func(b *buffer.Writer) {
		b.WriteByte(sub)
		if sub != 3 {
			writeName8(b, name)
		}
	})
}

func (w *World) sendGroupRemoved(p *Player, sub byte) {
	sendGroupMsg(w.sessionOfLocked(p), protocol.OpcodeRemoveGroupMember, func(b *buffer.Writer) { b.WriteByte(sub) })
}

// sendRosterLocked sends each member the roster its client holds
// (shownViewsLocked: its players and mercs) in its own ids, when it changed
// since the last one it was sent (or always, with force).
func (w *World) sendRosterLocked(g *Group, force bool) {
	for _, mid := range g.Members {
		m := w.Players[mid]
		sess := w.sessionOfLocked(m)
		if m == nil || sess == nil {
			continue
		}
		var ids []uint32
		var names []string
		for _, v := range w.shownViewsLocked(g, m) { // its players and mercs (mercgroup.go)
			ids = append(ids, w.groupIDSeenByLocked(sess, v.id))
			names = append(names, v.name)
		}
		key := ""
		for k := range ids {
			key += string(rune(ids[k]&0xFFFF)) + string(rune(ids[k]>>16)) + names[k]
		}
		if !force && g.lastRoster[mid] == key {
			continue
		}
		g.lastRoster[mid] = key
		sendGroupMsg(sess, protocol.OpcodeCreateGroup, func(b *buffer.Writer) {
			b.WriteUint32(0)
			b.WriteUint32(uint32(len(ids)))
			for _, id := range ids {
				b.WriteUint32(id)
			}
			for _, n := range names {
				writeName8(b, n)
			}
		})
	}
}

// GroupInvite is 0x0625 from inviter: the invitee gets 0x0773; the inviter
// forms a group of one if it has none (the client already thinks so). Any
// member invites into its group, not only the leader (the user, 2026-11-06:
// "lets allow any group member to invite to group"): CLIENT refuses a member's
// invite itself (InviteEntity and InviteToGroup: word 640), so a PS2's member
// never sends one, and vanilla sends one where this server offers it
// (protocol.OpcodeMemberInvites, handlers/group.go). The invitee joins the
// inviter's group; the leader stays the leader. A group of GroupWindowSize takes
// no one by an invite: its leader's raid flag asks past it (raidflag.go), so no one
// takes a raid group's cut by accident.
func (w *World) GroupInvite(sess *session.Session, targetID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	inviter := w.sessionPlayerLocked(sess)
	if inviter == nil {
		return
	}
	w.groupInviteLocked(sess, inviter, w.playerSeenByLocked(sess, targetID))
}

// groupInviteLocked is inviter's invite of target (nil: no one), GroupInvite's rules,
// for its 0x0625 and /invite <name> (raidcmds.go). Must be called with the lock held.
func (w *World) groupInviteLocked(sess *session.Session, inviter, target *Player) {
	tsess := w.sessionOfLocked(target)
	if target == nil || tsess == nil || target == inviter {
		w.sendGroupNotice(sess, 3, "")
		return
	}
	if target.GroupID != 0 && !w.mercOnlyGroupLocked(target) { // its and its merc's alone gives way (mercgroup.go)
		w.sendGroupNotice(sess, 2, target.Name)
		return
	}
	g := w.groups[inviter.GroupID]
	if g != nil && len(g.Members) >= GroupWindowSize {
		w.sendColoredLocked(sess, raidFullLine, castRefusalRGB)
		return
	}
	// a merc out takes a place of its own (mercgroup.go)
	if have := 1 + w.followersLocked(inviter); (g == nil || !g.flagged) && w.followersLocked(target) > 0 &&
		max(len(g.membersOrNone()), have)+1+w.followersLocked(target) > GroupWindowSize {
		w.sendColoredLocked(sess, fmt.Sprintf(mercNoRoomLine, target.Name), castRefusalRGB)
		return
	}
	if g == nil {
		w.formGroupLocked(inviter)
	}
	w.mercGivesWayLocked(target)
	w.askLocked(target, inviter, tsess, groupAsk{}, inviter.Name)
	log.Printf("Group: %s invites %s", inviter.Name, target.Name)
}

// formGroupLocked makes leader a group of one. Must be called with the lock held.
func (w *World) formGroupLocked(leader *Player) *Group {
	if w.groups == nil {
		w.groups = make(map[uint32]*Group)
	}
	w.nextGroupID++
	g := &Group{ID: w.nextGroupID, Members: []EntityID{leader.ServerID}, lastRoster: map[EntityID]string{}}
	w.groups[g.ID] = g
	leader.GroupID = g.ID
	log.Printf("Group: %s formed group %d", leader.Name, g.ID)
	w.mercFollowIntoLocked(leader) // its merc a member too (mercgroup.go)
	return g
}

// askLocked is target's invitation from inviter: kept as ask, and the box CLIENT
// shows for it, under the words the box names it by. Must be called with the lock held.
func (w *World) askLocked(target, inviter *Player, tsess *session.Session, ask groupAsk, words string) {
	if w.invites == nil {
		w.invites = make(map[EntityID]map[EntityID]groupAsk)
	}
	if w.invites[target.ServerID] == nil {
		w.invites[target.ServerID] = map[EntityID]groupAsk{}
	}
	ask.sent = w.idSeenByLocked(tsess, inviter)
	w.invites[target.ServerID][inviter.ServerID] = ask
	sendGroupMsg(tsess, protocol.OpcodeGroupInviteReceived, func(b *buffer.Writer) {
		b.WriteUint32(ask.sent)
		b.WriteUint32(0)
		writeName8(b, words)
	})
}

// answerInviterLocked is whom p's answer naming id answers: the player id names
// now, when p has a box of that player's up; else the one whose box was sent
// naming id (the user, 2026-11-08: "dwarfrish tried to /raid after we all
// accepted and folks couldnt join": a raid's crowd moved the inviter to another
// slot of the answerer's view between the box and the yes, and the yes was
// dropped, the box left waiting). Must be called with the lock held.
func (w *World) answerInviterLocked(sess *session.Session, p *Player, id uint32) *Player {
	inviter := w.playerSeenByLocked(sess, id)
	if inviter != nil {
		if _, up := w.invites[p.ServerID][inviter.ServerID]; up {
			return inviter
		}
	}
	for from, ask := range w.invites[p.ServerID] {
		if ask.sent == id && ask.declined.IsZero() {
			if o := w.Players[from]; o != nil {
				return o
			}
		}
	}
	return inviter
}

// takeInviteLocked removes and reports an invitation from inviter to p still
// waiting on its answer.
func (w *World) takeInviteLocked(p, inviter *Player) (groupAsk, bool) {
	ask, ok := w.invites[p.ServerID][inviter.ServerID]
	if !ok || !ask.declined.IsZero() {
		return groupAsk{}, false
	}
	delete(w.invites[p.ServerID], inviter.ServerID)
	return ask, true
}

// GroupAccept is 0x0622: into the inviter's group and the roster to all. An
// invite's stops at GroupWindowSize, a raid flag's does not (raid.go, raidflag.go).
func (w *World) GroupAccept(sess *session.Session, inviterID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	inviter := w.answerInviterLocked(sess, p, inviterID)
	ask, asked := groupAsk{}, false
	if inviter != nil {
		ask, asked = w.takeInviteLocked(p, inviter)
	}
	if asked && inviter == p {
		if ask.merc {
			w.mercOwnYesLocked(sess, p) // mercgroup.go
			return
		}
		w.ownBoxYesLocked(sess, p, ask) // raidflag.go
		return
	}
	if !asked || p.GroupID != 0 {
		who := "no one"
		if inviter != nil {
			who = inviter.Name
		}
		log.Printf("Group: %s's yes to %#x (%s) not taken: invited %v, in group %d", p.Name, inviterID, who, asked, p.GroupID)
		w.sendGroupRemoved(p, 3)
		return
	}
	g := w.groups[inviter.GroupID]
	if g == nil {
		log.Printf("Group: %s's yes to %s not taken: %s is in no group now", p.Name, inviter.Name, inviter.Name)
		w.sendGroupRemoved(p, 3)
		return
	}
	// a raid flag's yes counts at any size; it goes with its leader (leaveGroupLocked)
	if len(g.Members) >= GroupWindowSize && !ask.raid {
		w.sendGroupRemoved(p, 4)
		w.sendColoredLocked(sess, raidFullLine, castRefusalRGB)
		if is := w.sessionOfLocked(inviter); is != nil {
			w.sendColoredLocked(is, raidFullLine, castRefusalRGB)
		}
		return
	}
	// and its merc's place (mercgroup.go)
	if !ask.raid && !g.flagged && len(g.Members)+1+w.followersLocked(p) > GroupWindowSize {
		w.sendGroupRemoved(p, 4)
		w.sendColoredLocked(sess, mercNoRoomYours, castRefusalRGB)
		return
	}
	g.Members = append(g.Members, p.ServerID)
	p.GroupID = g.ID
	log.Printf("Group: %s joined %s's group %d (%d members)", p.Name, inviter.Name, g.ID, len(g.Members))
	w.mercFollowIntoLocked(p) // its merc with it (mercgroup.go)
	w.sendRosterLocked(g, true)
	w.raidJoinedLocked(g, p, sess)
}

// GroupDecline is 0x0624: the inviter told why (558 or 628).
func (w *World) GroupDecline(sess *session.Session, inviterID uint32, why byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	inviter := w.answerInviterLocked(sess, p, inviterID)
	if inviter == nil {
		log.Printf("Group: %s's no to %#x: no one", p.Name, inviterID)
		return
	}
	ask, asked := w.takeInviteLocked(p, inviter)
	if !asked {
		return
	}
	if inviter == p {
		if ask.merc {
			w.mercOwnNoLocked(sess, p, why) // mercgroup.go
			return
		}
		w.ownBoxNoLocked(sess, p, why, ask) // raidflag.go
		return
	}
	if ask.raid && why == 1 {
		ask.declined = time.Now()
		w.invites[p.ServerID][inviter.ServerID] = ask // raidflag.go: not asked again at once
	}
	sub := byte(1)
	if why == 2 {
		sub = 2
	}
	w.sendGroupNotice(w.sessionOfLocked(inviter), sub, p.Name)
}

// GroupLeave is 0x0623.
func (w *World) GroupLeave(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		w.leaveGroupLocked(p)
	}
}

// rejoinGroupLocked gives a character logging in again the group its old
// Player is still in: a login that replaces a session makes a new Player
// under the same ServerID, the old one never leaves (its session's removal
// finds the new one in the map and does nothing), and the group kept the id.
// The new Player came with no GroupID, so the roster still named both while
// every test of p.GroupID (the group bar's channel, voice, the group's chat)
// found none (F3 pass 18, voice's two-client run). Its roster is sent again,
// in the new session's ids. Called by CreatePlayer before p takes the map.
func (w *World) rejoinGroupLocked(p *Player) {
	delete(w.invites, p.ServerID) // the new client has no box up
	for _, g := range w.groups {
		for _, id := range g.Members {
			if id == p.ServerID {
				p.GroupID = g.ID
				delete(g.lastRoster, p.ServerID)
				p.raidReminder = g.isRaid()
				return
			}
		}
	}
}

// leaveGroupLocked takes p out of its group (it has already cleared its
// own); one left alone is told the group is disbanded.
func (w *World) leaveGroupLocked(p *Player) {
	delete(w.invites, p.ServerID)
	for _, set := range w.invites {
		delete(set, p.ServerID) // what it asked: its boxes answer "not invited" now
	}
	g := w.groups[p.GroupID]
	p.GroupID = 0
	if g == nil {
		return
	}
	led := len(g.Members) > 0 && g.Members[0] == p.ServerID
	for i, id := range g.Members {
		if id == p.ServerID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	delete(g.lastRoster, p.ServerID)
	// its merc goes with it, the two of them asked into a group again when
	// others are left in this one (mercgroup.go)
	if ms := w.mercs[p.ServerID]; ms != nil && ms.owner == p {
		for i, id := range g.Members {
			if id == ms.npc.ServerID {
				g.Members = append(g.Members[:i], g.Members[i+1:]...)
				break
			}
		}
		delete(g.lastRoster, ms.npc.ServerID)
		if len(g.Members) > 0 {
			ms.asked = false
		}
	}
	log.Printf("Group: %s left group %d (%d left)", p.Name, g.ID, len(g.Members))
	w.raidMemberGoneLocked(g, p.ServerID) // raidrules.go
	w.raidLeftLocked(g, p.Name)
	if led {
		w.raidLeadPassedLocked(g) // raidrules.go
	}
	w.shrinkLocked(g)
	w.raidStateNowForLocked(p)
}

// shrinkLocked dissolves a group of one (2: disbanded) or sends the rest
// the roster, and says when that ended a raid group.
func (w *World) shrinkLocked(g *Group) {
	if len(g.Members) <= 1 {
		for _, id := range g.Members {
			if m := w.Players[id]; m != nil {
				m.GroupID = 0
				w.sendGroupRemoved(m, 2)
			}
		}
		delete(w.groups, g.ID)
		return
	}
	w.sendRosterLocked(g, true)
	w.raidChangedLocked(g)
}

// GroupBoot is 0x0626 from the leader: the member is ejected (1).
func (w *World) GroupBoot(sess *session.Session, memberID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p != nil && p.GroupID != 0 {
		// a merc of the group: dismissed (mercgroup.go), by its leader or a raid's
		// helper (raidcmds.go)
		if g := w.groups[p.GroupID]; g != nil && len(g.Members) > 0 {
			if ms := w.mercSeenByLocked(sess, g, memberID); ms != nil {
				if w.mayDismissMercLocked(g, p, ms) {
					w.mercBootedLocked(p, ms)
				}
				return
			}
		}
	}
	m := w.playerSeenByLocked(sess, memberID)
	if p == nil || m == nil || m == p || p.GroupID == 0 || m.GroupID != p.GroupID {
		return
	}
	g := w.groups[p.GroupID]
	if g == nil || !w.mayKickLocked(g, p, m) { // the leader, or a raid's helper (raidcmds.go)
		return
	}
	w.bootLocked(g, m)
}

// bootLocked takes m out of g as ejected (0x069A 1), a raid's kick in a fight keeping
// its claims (raidrules.go). Must be called with the lock held.
func (w *World) bootLocked(g *Group, m *Player) {
	w.raidKickedLocked(g, m)
	for i, id := range g.Members {
		if id == m.ServerID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	m.GroupID = 0
	delete(g.lastRoster, m.ServerID)
	if ms := w.mercs[m.ServerID]; ms != nil && ms.owner == m {
		for i, id := range g.Members {
			if id == ms.npc.ServerID {
				g.Members = append(g.Members[:i], g.Members[i+1:]...) // its merc with it (mercgroup.go)
				break
			}
		}
		ms.asked = false // the two of them asked into a group again
	}
	w.sendGroupRemoved(m, 1)
	w.raidMemberGoneLocked(g, m.ServerID)
	w.raidLeftLocked(g, m.Name)
	w.shrinkLocked(g)
	w.raidStateNowForLocked(m)
}

// GroupDisband is 0x0627 from the leader: the rest are told (2).
func (w *World) GroupDisband(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.groups[p.GroupID]
	if g == nil {
		return
	}
	// CLIENT's DisbandGroup lets its own group go before it sends: one that no longer
	// leads (a lead handed on, /raidleader) and a raid's leader in a fight leave
	// instead, the rest going on (raidcmds.go)
	if g.Members[0] != p.ServerID || w.disbandRefusedLocked(sess, g, p) {
		w.leaveGroupLocked(p)
		return
	}
	players := 0
	for _, id := range g.Members {
		if w.Players[id] != nil {
			players++
		}
	}
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil {
			m.GroupID = 0
			if m != p {
				w.sendGroupRemoved(m, 2)
			}
			// a merc's owner out of a group of others is asked into one with
			// it again (mercgroup.go)
			if ms := w.mercs[m.ServerID]; ms != nil && ms.owner == m && players > 1 {
				ms.asked = false
			}
			w.raidStateNowForLocked(m)
		}
	}
	delete(w.groups, g.ID)
}

// groupSyncSystem keeps each member's group channel (0x40) and roster ids
// current. Every 5 ticks, with the visibility system.
func (w *World) groupSyncSystem() {
	if w.TickCount%5 != 0 {
		return
	}
	for _, g := range w.groups {
		w.raidChangedLocked(g)
		w.sendRosterLocked(g, false)
		for _, mid := range g.Members {
			m := w.Players[mid]
			sess := w.sessionOfLocked(m)
			if m == nil || sess == nil {
				continue
			}
			w.raidRemindLocked(g, m, sess)
			conn := sess.Connection()
			if conn == nil {
				continue
			}
			ch := conn.GetServerGroupUpdate()
			if ch == nil {
				continue
			}
			data := make([]byte, 13*(GroupWindowSize-1))
			k := 0
			for _, v := range w.shownViewsLocked(g, m) { // its players and mercs (mercgroup.go)
				if v.id == m.ServerID || k == GroupWindowSize-1 {
					continue
				}
				groupRecord(data[k*13:], w.groupIDSeenByLocked(sess, v.id), v)
				k++
			}
			ch.SetData(data)
		}
	}
}

// membersOrNone is g's members, none for no group.
func (g *Group) membersOrNone() []EntityID {
	if g == nil {
		return nil
	}
	return g.Members
}
