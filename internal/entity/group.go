package entity

import (
	"log"
	"math"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Groups, as CLIENT keeps one (VIClient's m_group, m_inGroup, m_p2BD78) and
// reads it off the wire (the decomp, modules/client/src/viclient.cpp):
//
//	0x0625 in   invite: u32 the target's id, string8 its name (InviteEntity)
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

// MaxGroupSize is m_group's length.
const MaxGroupSize = 4

// Group is one group: its members, the leader first.
type Group struct {
	ID      uint32
	Members []EntityID
	// lastRoster is the roster each member was last sent, by member.
	lastRoster map[EntityID]string
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

// PlayerSeenBy resolves an id the sender gave to a player (playerSeenByLocked).
func (w *World) PlayerSeenBy(sender *session.Session, id uint32) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.playerSeenByLocked(sender, id)
}

// playerSeenByLocked resolves an id the sender gave (its own, an object id
// from ITS visibility channels, or a stand-in) to a player. Object ids are per
// session, so only the sender's own table is read: findEntityByObjectIDLocked
// searches every session's, and two clients that see each other first both
// call the other 100001.
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
	for i := 0; i < MaxVisibleEntities; i++ {
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

// sendRosterLocked sends each member the roster in its own ids, when it
// changed since the last one it was sent (or always, with force).
func (w *World) sendRosterLocked(g *Group, force bool) {
	for _, mid := range g.Members {
		m := w.Players[mid]
		sess := w.sessionOfLocked(m)
		if m == nil || sess == nil {
			continue
		}
		var ids []uint32
		var names []string
		for _, oid := range g.Members {
			if o := w.Players[oid]; o != nil {
				ids = append(ids, w.idSeenByLocked(sess, o))
				names = append(names, o.Name)
			}
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
// forms a group of one if it has none (the client already thinks so).
func (w *World) GroupInvite(sess *session.Session, targetID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	inviter := w.sessionPlayerLocked(sess)
	if inviter == nil {
		return
	}
	target := w.playerSeenByLocked(sess, targetID)
	tsess := w.sessionOfLocked(target)
	if target == nil || tsess == nil || target == inviter {
		w.sendGroupNotice(sess, 3, "")
		return
	}
	if target.GroupID != 0 {
		w.sendGroupNotice(sess, 2, target.Name)
		return
	}
	g := w.groups[inviter.GroupID]
	if g == nil {
		if w.groups == nil {
			w.groups = make(map[uint32]*Group)
		}
		w.nextGroupID++
		g = &Group{ID: w.nextGroupID, Members: []EntityID{inviter.ServerID}, lastRoster: map[EntityID]string{}}
		w.groups[g.ID] = g
		inviter.GroupID = g.ID
		log.Printf("Group: %s formed group %d", inviter.Name, g.ID)
	}
	if g.Members[0] != inviter.ServerID {
		return // only the leader invites; the client checks first
	}
	if w.invites == nil {
		w.invites = make(map[EntityID]map[EntityID]bool)
	}
	if w.invites[target.ServerID] == nil {
		w.invites[target.ServerID] = map[EntityID]bool{}
	}
	w.invites[target.ServerID][inviter.ServerID] = true
	log.Printf("Group: %s invites %s", inviter.Name, target.Name)
	sendGroupMsg(tsess, protocol.OpcodeGroupInviteReceived, func(b *buffer.Writer) {
		b.WriteUint32(w.idSeenByLocked(tsess, inviter))
		b.WriteUint32(0)
		writeName8(b, inviter.Name)
	})
}

// takeInviteLocked removes and reports an invitation from inviter to p.
func (w *World) takeInviteLocked(p, inviter *Player) bool {
	set := w.invites[p.ServerID]
	if set == nil || !set[inviter.ServerID] {
		return false
	}
	delete(set, inviter.ServerID)
	return true
}

// GroupAccept is 0x0622: into the inviter's group, and the roster to all.
func (w *World) GroupAccept(sess *session.Session, inviterID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	inviter := w.playerSeenByLocked(sess, inviterID)
	if p == nil {
		return
	}
	if inviter == nil || !w.takeInviteLocked(p, inviter) || p.GroupID != 0 {
		w.sendGroupRemoved(p, 3)
		return
	}
	g := w.groups[inviter.GroupID]
	if g == nil {
		w.sendGroupRemoved(p, 3)
		return
	}
	if len(g.Members) >= MaxGroupSize {
		w.sendGroupRemoved(p, 4)
		return
	}
	g.Members = append(g.Members, p.ServerID)
	p.GroupID = g.ID
	log.Printf("Group: %s joined %s's group %d (%d members)", p.Name, inviter.Name, g.ID, len(g.Members))
	w.sendRosterLocked(g, true)
}

// GroupDecline is 0x0624: the inviter told why (558 or 628).
func (w *World) GroupDecline(sess *session.Session, inviterID uint32, why byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	inviter := w.playerSeenByLocked(sess, inviterID)
	if p == nil || inviter == nil || !w.takeInviteLocked(p, inviter) {
		return
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

// leaveGroupLocked takes p out of its group (it has already cleared its
// own); one left alone is told the group is disbanded.
func (w *World) leaveGroupLocked(p *Player) {
	delete(w.invites, p.ServerID)
	g := w.groups[p.GroupID]
	p.GroupID = 0
	if g == nil {
		return
	}
	for i, id := range g.Members {
		if id == p.ServerID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	delete(g.lastRoster, p.ServerID)
	log.Printf("Group: %s left group %d (%d left)", p.Name, g.ID, len(g.Members))
	w.shrinkLocked(g)
}

// shrinkLocked dissolves a group of one (2: disbanded) or sends the rest
// the roster.
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
}

// GroupBoot is 0x0626 from the leader: the member is ejected (1).
func (w *World) GroupBoot(sess *session.Session, memberID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	m := w.playerSeenByLocked(sess, memberID)
	if p == nil || m == nil || m == p || p.GroupID == 0 || m.GroupID != p.GroupID {
		return
	}
	g := w.groups[p.GroupID]
	if g == nil || g.Members[0] != p.ServerID {
		return
	}
	for i, id := range g.Members {
		if id == m.ServerID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	m.GroupID = 0
	delete(g.lastRoster, m.ServerID)
	w.sendGroupRemoved(m, 1)
	w.shrinkLocked(g)
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
	if g == nil || g.Members[0] != p.ServerID {
		return
	}
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil {
			m.GroupID = 0
			if m != p {
				w.sendGroupRemoved(m, 2)
			}
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
		w.sendRosterLocked(g, false)
		for _, mid := range g.Members {
			m := w.Players[mid]
			sess := w.sessionOfLocked(m)
			if m == nil || sess == nil {
				continue
			}
			conn := sess.Connection()
			if conn == nil {
				continue
			}
			ch := conn.GetServerGroupUpdate()
			if ch == nil {
				continue
			}
			data := make([]byte, 13*(MaxGroupSize-1))
			k := 0
			for _, oid := range g.Members {
				o := w.Players[oid]
				if o == nil || o == m || k == MaxGroupSize-1 {
					continue
				}
				r := data[k*13:]
				id := w.idSeenByLocked(sess, o)
				r[0], r[1], r[2], r[3] = byte(id), byte(id>>8), byte(id>>16), byte(id>>24)
				r[4] = 1
				hp := 0
				if o.MaxHP > 0 {
					hp = o.HP * 255 / o.MaxHP
				}
				r[5] = byte(max(0, min(255, hp)))
				r[6] = 0
				for c, v := range []float32{o.X, o.Y, o.Z} {
					s := int16(max(math.MinInt16, min(math.MaxInt16, int(v))))
					r[7+2*c], r[8+2*c] = byte(s), byte(uint16(s)>>8)
				}
				k++
			}
			ch.SetData(data)
		}
	}
}
