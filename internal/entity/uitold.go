package entity

import (
	"encoding/binary"
	"sort"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// uitold.go — what vanilla is told for two of its windows, to a client that
// answered each offer (handlers/mercui.go, handlers/groupreg.go; the GM
// Jibrish's petitions 1475, 1476 and 1488, 2026-11-11):
//
//   - the mercs the player may see (protocol.OpcodeMercsData): every merc in
//     its world by name (with its hirer's, to a client that asked for layout
//     2: its "<Hirer's Mercenary>" tag), which the client draws in the pet's
//     colour tinted red
//     (a merc's channel says pet as a pet's does: statesync.go's byte 142, 3),
//     and whether its own is out, for Group > Dismiss Mercenary;
//   - its group search standing (OpcodeGroupRegState): registered or not, its
//     own "Allow anyone in my group to invite" (groupinvites.go), and whether
//     its group's members invite now.
//
// Each is made for every such session once a second (uiToldEvery) and sent
// only when it differs from what the session last had (session.TakeMercs,
// TakeGroupReg), so whatever changes them (a hire, a dismissal, a death, a
// logout, a world change, a registration, a group's leader or its setting)
// reaches the client within a second without a hook at each. A session that
// never answered (a PS2) is sent neither.

// uiToldEvery is how often uiToldSystem looks.
const uiToldEvery = time.Second

// uiToldLayout is the layout both messages carry.
const uiToldLayout = 1

// mercNameMost is the most bytes of a merc's name told: as many as its
// entity channel carries (statesync.go's 116-139).
const mercNameMost = 24

// uiToldSystem tells each client that asked what changed (the file's doc).
// Must be called with the lock held.
func (w *World) uiToldSystem(now time.Time) {
	if w.SessionManager == nil || now.Sub(w.uiTold) < uiToldEvery {
		return
	}
	w.uiTold = now
	var names map[uint8][]mercNamed // the mercs by world, made when one asks
	w.SessionManager.ForEach(func(s *session.Session) bool {
		merc, reg := s.MercsTold(), s.GroupReg()
		if !merc && !reg {
			return true
		}
		p := w.sessionPlayerLocked(s)
		if p == nil || !s.CharacterInWorld() {
			return true
		}
		if merc {
			if names == nil {
				names = w.mercNamesLocked()
			}
			body := mercsBody(s.MercsLayout(), w.ownMercOutLocked(p), names[p.World])
			if s.TakeMercs(string(body)) {
				queueOurs(s, protocol.OpcodeMercsData, body)
			}
		}
		if reg {
			body := w.groupRegBodyLocked(p)
			if s.TakeGroupReg(string(body)) {
				queueOurs(s, protocol.OpcodeGroupRegState, body)
			}
		}
		return true
	})
}

// mercNamed is a merc out by its name and its hirer's.
type mercNamed struct{ name, owner string }

// mercNamesLocked is every merc out by its world, each world's sorted by
// name, then hirer. Must be called with the lock held.
func (w *World) mercNamesLocked() map[uint8][]mercNamed {
	out := map[uint8][]mercNamed{}
	for _, ms := range w.allMercsLocked() {
		if ms == nil || ms.npc == nil || ms.npc.Name == "" {
			continue
		}
		owner := ""
		if ms.owner != nil {
			owner = ms.owner.Name
		}
		out[ms.npc.World] = append(out[ms.npc.World], mercNamed{ms.npc.Name, owner})
	}
	for _, n := range out {
		sort.Slice(n, func(i, j int) bool {
			if n[i].name != n[j].name {
				return n[i].name < n[j].name
			}
			return n[i].owner < n[j].owner
		})
	}
	return out
}

// ownMercOutLocked is whether p has a merc out, or one waiting outside
// capture the flag's arena (ctfaway.go): what /merc dismiss sends away. Must
// be called with the lock held.
func (w *World) ownMercOutLocked(p *Player) bool {
	if len(w.mercsOfLocked(p)) > 0 {
		return true
	}
	a := w.ctfAwayOfLocked(p)
	return a != nil && len(a.mercs) > 0
}

// mercsBody is OpcodeMercsData's body (past the opcode) in the layout the
// client asked for: 1 the names, 2 each with its hirer's.
func mercsBody(layout uint8, own bool, names []mercNamed) []byte {
	layout = min(max(layout, uiToldLayout), 2)
	b := []byte{layout, 0}
	if own {
		b[1] = 1
	}
	str := func(n string) {
		if len(n) > mercNameMost {
			n = n[:mercNameMost]
		}
		b = append(b, byte(len(n)))
		b = append(b, n...)
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(min(len(names), 0xFFFF)))
	for _, n := range names[:min(len(names), 0xFFFF)] {
		str(n.name)
		if layout >= 2 {
			str(n.owner)
		}
	}
	return b
}

// The flags of OpcodeGroupRegState.
const (
	groupRegRegistered = 1 << 0
	groupRegOwnOpen    = 1 << 1
	groupRegMembers    = 1 << 2
)

// groupRegBodyLocked is OpcodeGroupRegState's body for p. Must be called
// with the lock held.
func (w *World) groupRegBodyLocked(p *Player) []byte {
	var f byte
	if p.LFG {
		f |= groupRegRegistered
	}
	if w.groupInvitesOpenLocked(p) {
		f |= groupRegOwnOpen
	}
	if w.membersInviteLocked(p) {
		f |= groupRegMembers
	}
	return []byte{uiToldLayout, f}
}

// queueOurs queues one of our own messages: the opcode and body.
func queueOurs(s *session.Session, op protocol.GameOpcode, body []byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(op)), body...))
	s.Queue().AddWithSequence(msg)
}
