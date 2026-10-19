package entity

// Buddy and ignore lists, as CLIENT drives them. Kept in memory for the
// server's life; nothing is written to the database. Both are sent again at
// each world entry: the ignore list as 0x1005, the buddies 0x03D5 apiece.
//
// Ignore list (VIClientPart38300): the client sends 0x1000 with a name and
// 0x1002 with an entry's id; the server answers 0x1003 {string name, u32 id}
// and 0x1004 {u32 id}. The same list makes those players non-collideable.
//
// Buddy list (VIClient's m_buddies): 0x0366 asks for a name; the player
// named gets 0x076F {u32 request id, string name} and answers 0x0367
// {u32 id} (accept) or 0x0368 {u32 id, u8 reason}. An accept puts each on
// the other's list: 0x03D5 {zigzag id, string name, zigzag class, race,
// level, u8 online, wide guild} to both, and 0x0770 {string name} to the
// asker. A refusal goes back to the asker as 0x0771 {u8 reason, string
// name}, the reason as the refuser sent it (CLIENT's Msg0771 words: 1 busy,
// 2 declined, 8 a full list); a name nobody online has is 0x0771 type 5.
// 0x0369 {zigzag id} removes one; the answer is 0x03D6 {zigzag id}.

import (
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

type ignoreEntry struct {
	Name string
	ID   uint32
}

// buddyInfo is a buddy as last seen, for a list sent while it is offline.
type buddyInfo struct {
	Name               string
	Class, Race, Level int
}

type socialLists struct {
	ignores map[EntityID][]ignoreEntry
	buddies map[EntityID]map[EntityID]buddyInfo
	// requests: asker -> the player asked
	requests map[EntityID]EntityID
}

func (w *World) socialInit() {
	if w.social.ignores == nil {
		w.social.ignores = map[EntityID][]ignoreEntry{}
		w.social.buddies = map[EntityID]map[EntityID]buddyInfo{}
		w.social.requests = map[EntityID]EntityID{}
	}
}

// playerNamedLocked finds an online player by name, any case.
func (w *World) playerNamedLocked(name string) *Player {
	for _, p := range w.Players {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// IgnoreAdd puts a player on the sender's ignore list (by name, online or
// not: an unknown name gets an id of its own) and answers 0x1003.
func (w *World) IgnoreAdd(sess *session.Session, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil || name == "" || strings.EqualFold(name, me.Name) {
		return
	}
	list := w.social.ignores[me.ServerID]
	for _, e := range list {
		if strings.EqualFold(e.Name, name) {
			return
		}
	}
	id := uint32(len(list) + 1)
	for _, e := range list {
		if e.ID >= id {
			id = e.ID + 1
		}
	}
	if p := w.playerNamedLocked(name); p != nil {
		name = p.Name
	}
	w.social.ignores[me.ServerID] = append(list, ignoreEntry{Name: name, ID: id})
	sendGroupMsg(sess, protocol.OpcodeNoCollideAdd, func(b *buffer.Writer) {
		writeName8(b, name)
		b.WriteUint32(id)
	})
}

// IgnoreList is a player's ignore list, for 0x1005 at the world entry.
func (w *World) IgnoreList(sess *session.Session) (names []string, ids []uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return nil, nil
	}
	for _, e := range w.social.ignores[me.ServerID] {
		names = append(names, e.Name)
		ids = append(ids, e.ID)
	}
	return names, ids
}

// IgnoreRemove takes an entry off by id and answers 0x1004.
func (w *World) IgnoreRemove(sess *session.Session, id uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return
	}
	list := w.social.ignores[me.ServerID]
	for i, e := range list {
		if e.ID == id {
			w.social.ignores[me.ServerID] = append(list[:i:i], list[i+1:]...)
			sendGroupMsg(sess, protocol.OpcodeNoCollideRemove, func(b *buffer.Writer) {
				b.WriteUint32(id)
			})
			return
		}
	}
}

func infoOf(p *Player) buddyInfo {
	return buddyInfo{Name: p.Name, Class: p.Class, Race: p.Race, Level: p.Level}
}

func writeBuddy(b *buffer.Writer, id EntityID, i buddyInfo, online bool) {
	b.Write7BitEncodedInt64(int64(id))
	writeName8(b, i.Name)
	b.Write7BitEncodedInt64(int64(i.Class))
	b.Write7BitEncodedInt64(int64(i.Race))
	b.Write7BitEncodedInt64(int64(i.Level))
	if online {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	b.WriteStringUnicode("") // the guild column
}

// SendBuddyList sends a player's list at the world entry, one 0x03D5 a
// buddy, online or as last seen.
func (w *World) SendBuddyList(sess *session.Session) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return 0
	}
	for id, info := range w.social.buddies[me.ServerID] {
		online := false
		if p := w.Players[id]; p != nil && w.sessionOfLocked(p) != nil {
			info, online = infoOf(p), true
		}
		sendGroupMsg(sess, protocol.OpcodeBuddyListEntry, func(b *buffer.Writer) { writeBuddy(b, id, info, online) })
	}
	return len(w.social.buddies[me.ServerID])
}

// BuddyAdd asks the named player (0x076F); one nobody online has is
// answered 0x0771 type 5.
func (w *World) BuddyAdd(sess *session.Session, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return
	}
	p := w.playerNamedLocked(name)
	var to *session.Session
	if p != nil && p != me {
		to = w.sessionOfLocked(p)
	}
	if to == nil {
		sendGroupMsg(sess, protocol.OpcodeBuddyListStuff, func(b *buffer.Writer) {
			b.WriteByte(5)
			writeName8(b, name)
		})
		return
	}
	w.social.requests[me.ServerID] = p.ServerID
	sendGroupMsg(to, protocol.OpcodeBuddyInvite, func(b *buffer.Writer) {
		b.WriteUint32(uint32(me.ServerID))
		writeName8(b, me.Name)
	})
}

// BuddyAnswer is 0x0367 (accept) or 0x0368 (refuse, with the reason) to the
// request id 0x076F carried.
func (w *World) BuddyAnswer(sess *session.Session, id uint32, accept bool, reason byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	asker := w.Players[EntityID(id)]
	if me == nil || asker == nil || w.social.requests[asker.ServerID] != me.ServerID {
		return
	}
	delete(w.social.requests, asker.ServerID)
	to := w.sessionOfLocked(asker)
	if !accept {
		sendGroupMsg(to, protocol.OpcodeBuddyListStuff, func(b *buffer.Writer) {
			b.WriteByte(reason)
			writeName8(b, me.Name)
		})
		return
	}
	for _, pair := range [][2]*Player{{asker, me}, {me, asker}} {
		if w.social.buddies[pair[0].ServerID] == nil {
			w.social.buddies[pair[0].ServerID] = map[EntityID]buddyInfo{}
		}
		w.social.buddies[pair[0].ServerID][pair[1].ServerID] = infoOf(pair[1])
	}
	sendGroupMsg(to, protocol.OpcodeBuddyListEntry, func(b *buffer.Writer) { writeBuddy(b, me.ServerID, infoOf(me), true) })
	sendGroupMsg(sess, protocol.OpcodeBuddyListEntry, func(b *buffer.Writer) { writeBuddy(b, asker.ServerID, infoOf(asker), true) })
	sendGroupMsg(to, protocol.OpcodeGroupInviteAcceptedMessage, func(b *buffer.Writer) {
		writeName8(b, me.Name)
	})
}

// BuddyRemove takes a buddy off the sender's list and answers 0x03D6.
func (w *World) BuddyRemove(sess *session.Session, id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.socialInit()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return
	}
	delete(w.social.buddies[me.ServerID], EntityID(id))
	sendGroupMsg(sess, protocol.OpcodeBuddyRemoved, func(b *buffer.Writer) {
		b.Write7BitEncodedInt64(id)
	})
}
