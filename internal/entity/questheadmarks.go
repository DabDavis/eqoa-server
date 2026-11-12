package entity

import (
	"encoding/binary"
	"strings"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// questheadmarks.go — the quest marks over the NPCs' heads (the GM Jibrish's
// petition 1813, 2026-11-12: "Add the ability for us to add yellow ! and ?
// over NPCs in game, visible to the player ... The exclamation point over
// their head should only happen if there's a quest they can log right then,
// and the question mark if there's a quest they can progress right then such
// as via a turn in or completion"). The quest engine works out each
// character's marks by NPC name (and by spawn, for a step pinned to one) when
// something they hang on changes (handlers/questheadmarks.go) and tells the
// World here; every other tick the World sends each client that asked the
// marks of the NPCs on its entity slots, by the ids the client knows them by,
// when that set changed: so an NPC coming into view gets its mark with no
// work by the engine, and nothing is sent while nothing changes. A session
// that never asked (every PS2) is sent nothing.

// The marks.
const (
	QuestMarkTake byte = 1 // "!": a quest the player may take here now
	QuestMarkStep byte = 2 // "?": a step the player can do here now
)

// questHeadTicks is how often (in ticks) the sets in view are looked at.
const questHeadTicks = 2

// QuestHeadMarks is one character's marks: by NPC name (folded: trimmed, in
// lower case) and by spawn (a step pinned to one NPC marks that one alone).
// A spawn's mark goes before its name's. Never changed once given to the World.
type QuestHeadMarks struct {
	Names  map[string]byte
	Spawns map[EntityID]byte
}

// markOf is the mark of the NPC, 0 none.
func (m *QuestHeadMarks) markOf(npc *NPC) byte {
	if m == nil || npc == nil {
		return 0
	}
	if k := m.Spawns[npc.ServerID]; k != 0 {
		return k
	}
	return m.Names[strings.ToLower(strings.TrimSpace(npc.Name))]
}

// SetQuestHeadMarks keeps the character's marks (nil forgets them): the next
// look sends its client what changed in view.
func (w *World) SetQuestHeadMarks(charID uint32, m *QuestHeadMarks) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if m == nil {
		delete(w.qhead, charID)
		return
	}
	if w.qhead == nil {
		w.qhead = map[uint32]*QuestHeadMarks{}
	}
	w.qhead[charID] = m
}

// SetQuestHeadMarksWanted is the client's answer (OpcodeQuestHeadMarksAnswer):
// whether the session's player is sent the marks. On, the next look sends the
// set whatever it was before (the client dropped what it held); off, nothing
// more is sent. It reports the player, nil when there is none in the world.
func (w *World) SetQuestHeadMarksWanted(sess *session.Session, on bool) *Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	p.QuestHeadMarks, p.questHeadDue, p.questHeadSent = on, on, ""
	return p
}

// WantQuestHeadMarks is SetQuestHeadMarksWanted for a player at hand (a
// keep restart's, which the client asked before).
func (w *World) WantQuestHeadMarks(p *Player, on bool) {
	if p == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p.QuestHeadMarks, p.questHeadDue, p.questHeadSent = on, on, ""
}

// QuestHeadMarksWanted is whether p's client asked for the marks.
func (w *World) QuestHeadMarksWanted(p *Player) bool {
	if p == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return p.QuestHeadMarks
}

// questHeadInViewLocked is what p's client is shown: each NPC on one of its
// entity slots with a mark, alive, by the slot's id, in slot order. Must be
// called with the lock held.
func (w *World) questHeadInViewLocked(sess *session.Session, p *Player) []byte {
	m := w.qhead[p.CharServerID]
	vs, _ := sess.VisibilityState().(*VisibilityState)
	var ids []uint32
	var marks []byte
	if m != nil && vs != nil {
		for i, id := range vs.ChannelEntity {
			if id == 0 || i >= len(vs.ChannelObjectID) {
				continue
			}
			npc := w.NPCs[id]
			if npc == nil || npc.Dead {
				continue
			}
			if k := m.markOf(npc); k != 0 {
				ids = append(ids, vs.ChannelObjectID[i])
				marks = append(marks, k)
			}
		}
	}
	return EncodeQuestHeadMarks(ids, marks)
}

// EncodeQuestHeadMarks is OpcodeQuestHeadMarks's body: a u16 count, then each
// NPC's u32 id and u8 mark, little-endian (the first 65,535).
func EncodeQuestHeadMarks(ids []uint32, marks []byte) []byte {
	n := min(len(ids), len(marks), 0xFFFF)
	out := make([]byte, 2, 2+5*n)
	binary.LittleEndian.PutUint16(out, uint16(n))
	for i := 0; i < n; i++ {
		out = binary.LittleEndian.AppendUint32(out, ids[i])
		out = append(out, marks[i])
	}
	return out
}

// questHeadMarksSystem sends each client that asked the marks its view shows
// when they changed, or when they are due (just asked). Every questHeadTicks,
// after the visibility system has put this tick's entities on their slots.
// Must be called with the lock held.
func (w *World) questHeadMarksSystem() {
	if w.TickCount%questHeadTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		w.sendQuestHeadLocked(s, w.sessionPlayerLocked(s))
		return true
	})
}

// sendQuestHeadLocked sends p's client the marks in its view when they
// changed or are due. Must be called with the lock held.
func (w *World) sendQuestHeadLocked(s *session.Session, p *Player) {
	if p == nil || !p.QuestHeadMarks {
		return
	}
	body := w.questHeadInViewLocked(s, p)
	if !p.questHeadDue && string(body) == p.questHeadSent {
		return
	}
	p.questHeadSent, p.questHeadDue = string(body), false
	s.Queue().AddWithSequence(capesMessage(protocol.OpcodeQuestHeadMarks, body))
}

// SendQuestHeadMarks is the system's look for one session now: its client is
// sent the marks in its view when they changed or are due.
func (w *World) SendQuestHeadMarks(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sendQuestHeadLocked(sess, w.sessionPlayerLocked(sess))
}
