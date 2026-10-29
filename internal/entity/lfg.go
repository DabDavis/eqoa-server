package entity

import (
	"sort"

	"github.com/eqoa/server/internal/session"
)

// The group search of SOE's live client (VIWndGroupSearch, Community > Group >
// Search; handlers/lfg.go has the wire, the live-client research's
// livediff/specs/lfg.md the client, outside this repository): a character registers as looking (0x0801 sub 2, or
// /lfg on), and a search lists the registered characters a group could use or
// the groups a character could join. The client keeps no registration state;
// the flag is the Player's, so a logout ends it (the next login is a new
// Player).
//
// What a search returns is inferred (no capture of SOE's server holds one):
// the window's type 0 is a group's leader with room looking for players, so
// it lists registered characters in no group; type 1 is a character in no
// group looking for one, so it lists the registered leaders of groups with
// room. Rows are players either way (the window's title is always "Search for
// Player"), filtered by the row's own class, race and level.

// LFGMaxRows is the most rows a search returns (ours: the client allocates 56
// bytes a row and shows ten at a time).
const LFGMaxRows = 50

// LFGQuery is a search: 0x0801 sub 4.
type LFGQuery struct {
	Type       byte   // 0 players for a group, 1 groups for a player
	Classes    uint32 // 1 << class; all bits: any (the picker's ANY is -1)
	Races      uint32 // 1 << race
	MinLevel   int
	MaxLevel   int
	MaxResults int // 0: LFGMaxRows
}

// LFGRow is one player a search found, as 0x0840 sub 0x40 carries it.
type LFGRow struct {
	Name    string
	Level   int
	Class   int
	Race    int
	X, Y, Z float32
	World   uint8
	ID      uint32 // the id the searcher knows it by: what 0x0625 (invite) takes
}

// LFGStatus is what a registration did.
type LFGStatus int

const (
	LFGNoPlayer  LFGStatus = iota // the session has no player
	LFGNotLeader                  // in a group and not its leader: refused (live word 1848)
	LFGSolo                       // registered or not, in no group
	LFGLeader                     // registered or not, as its group's leader
)

// SetLFG registers (on) or unregisters the session's character as looking
// for a group, or its group as looking for members when it leads one. A
// member of a group who does not lead it is refused, as the live client
// refuses before it sends (word 1848). It reports the player's standing and
// whether it was registered before.
func (w *World) SetLFG(sess *session.Session, on bool) (status LFGStatus, was bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return LFGNoPlayer, false
	}
	was = p.LFG
	switch g := w.groups[p.GroupID]; {
	case p.GroupID == 0 || g == nil:
		status = LFGSolo
	case len(g.Members) > 0 && g.Members[0] == p.ServerID:
		status = LFGLeader
	default:
		return LFGNotLeader, was
	}
	p.LFG = on
	return status, was
}

// LFGStanding is the session's character's registration and standing, for
// /lfg's status line.
func (w *World) LFGStanding(sess *session.Session) (status LFGStatus, registered bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return LFGNoPlayer, false
	}
	switch g := w.groups[p.GroupID]; {
	case p.GroupID == 0 || g == nil:
		return LFGSolo, p.LFG
	case len(g.Members) > 0 && g.Members[0] == p.ServerID:
		return LFGLeader, p.LFG
	}
	return LFGNotLeader, p.LFG
}

// LFGSearch is a search from the session's character: the registered
// characters in the world the query's type, classes, races and levels take,
// by name, at most MaxResults; never the searcher, nor its own group.
func (w *World) LFGSearch(sess *session.Session, q LFGQuery) []LFGRow {
	w.mu.RLock()
	defer w.mu.RUnlock()
	me := w.sessionPlayerLocked(sess)
	if me == nil {
		return nil
	}
	limit := q.MaxResults
	if limit <= 0 || limit > LFGMaxRows {
		limit = LFGMaxRows
	}
	// only players a session holds in the world (one being made, or whose
	// session is gone, is no one to invite)
	inWorld := map[*Player]bool{}
	if w.SessionManager != nil {
		w.SessionManager.ForEach(func(s *session.Session) bool {
			if p := w.sessionPlayerLocked(s); p != nil && s.CharacterInWorld() {
				inWorld[p] = true
			}
			return true
		})
	}
	var found []*Player
	for _, p := range w.Players {
		if p == me || !p.LFG || !inWorld[p] {
			continue
		}
		if me.GroupID != 0 && p.GroupID == me.GroupID {
			continue
		}
		g := w.groups[p.GroupID]
		switch q.Type {
		case 0: // players for a group: in none
			if p.GroupID != 0 && g != nil {
				continue
			}
		case 1: // groups for a player: their leaders, with room
			if p.GroupID == 0 || g == nil || len(g.Members) == 0 || g.Members[0] != p.ServerID || len(g.Members) >= MaxGroupSize {
				continue
			}
		default:
			continue
		}
		if !lfgBit(q.Classes, p.Class) || !lfgBit(q.Races, p.Race) || p.Level < q.MinLevel || p.Level > q.MaxLevel {
			continue
		}
		found = append(found, p)
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Name != found[j].Name {
			return found[i].Name < found[j].Name
		}
		return found[i].ServerID < found[j].ServerID
	})
	if len(found) > limit {
		found = found[:limit]
	}
	rows := make([]LFGRow, 0, len(found))
	for _, p := range found {
		rows = append(rows, LFGRow{Name: p.Name, Level: p.Level, Class: p.Class, Race: p.Race,
			X: p.X, Y: p.Y, Z: p.Z, World: p.World, ID: w.idSeenByLocked(sess, p)})
	}
	return rows
}

// lfgBit is whether a picker's mask takes an id: its bit (1 << id); a mask
// of -1 (ANY) takes every id, 0 (CLEAR) none.
func lfgBit(mask uint32, id int) bool {
	return id >= 0 && id < 32 && mask&(1<<uint(id)) != 0
}

// SetGroupChatShown is the Group menu's Show Chat (true) or Hide Chat
// (false): the disc's 0x0B01 {1, show}, the live client's 0x0801 sub 1. A
// player who hid it is not sent its group's chat (handlers/chat.go's
// groupChat) until it shows it again or logs in again.
func (w *World) SetGroupChatShown(sess *session.Session, show bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.GroupChatHidden = !show
	return true
}

// GroupChatHearers is who hears p's group chat: its group's members who have
// not hidden it, and p itself whatever it chose (its own line comes back to
// it), the leader first; none out of a group.
func (w *World) GroupChatHearers(p *Player) []*Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return nil
	}
	out := make([]*Player, 0, len(g.Members))
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil && (m == p || m.ServerID == p.ServerID || !m.GroupChatHidden) {
			out = append(out, m)
		}
	}
	return out
}
