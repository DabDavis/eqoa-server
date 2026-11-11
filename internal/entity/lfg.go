package entity

import (
	"math"
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
// What a search returns (the GM Jibrish's petition 1582, 2026-11-11: "When
// players register with group by hitting community > group > register, the
// search menu should display them on the list"): every registered character
// the query's classes, races and levels take, of either type (the window's
// type 0 is a group's leader looking for players, type 1 a character in no
// group looking for one; the window's title is always "Search for Player"),
// but the searcher, its own group and a member of a group it does not lead
// (registered before it joined: a member cannot register). Until then a type 0 search left out
// every registered group leader and a type 1 search a full group's (petition
// 1476's rule, inferred: no capture of SOE's server holds a search).
//
// The levels: the window opens with the range its searcher hunts with
// (LevelRange, the live client's 0x012EC0C0: a level 60 searcher's is 45-60),
// which hid every registered character out of it, though the player chose no
// levels. A query whose levels are that range (lfgDefaultRange) takes every
// level; levels the player typed are a filter.

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
	anyLevel := w.lfgDefaultRangeLocked(me, q.MinLevel, q.MaxLevel)
	var found []*Player
	for _, p := range w.Players {
		if p == me || !p.LFG || !inWorld[p] {
			continue
		}
		if me.GroupID != 0 && p.GroupID == me.GroupID {
			continue
		}
		if q.Type != 0 && q.Type != 1 { // the window sends no other
			continue
		}
		// one registered before it joined a group it does not lead looks
		// for none now (a member cannot register: SetLFG)
		if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil && len(g.Members) > 0 && g.Members[0] != p.ServerID {
			continue
		}
		if !lfgBit(q.Classes, p.Class) || !lfgBit(q.Races, WireRace(p.Race)) || (!anyLevel && (p.Level < q.MinLevel || p.Level > q.MaxLevel)) {
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

// lfgLevelRange is the live client's LevelRange (0x012EC0C0, as vanilla's
// groupsearch.rs level_range ports it) over a list's lowest and highest
// levels: down to three quarters of the highest (at least six under it), up to
// a third over the lowest (more than five over it), the low end no higher
// than the high, both 1..60. The third's product is the EE's, which chops
// where IEEE rounds: both are reported (the same but at a tie).
func lfgLevelRange(lo, hi int) [2][2]int {
	third := float64(lo) * float64(math.Float32frombits(0x3FAA3D71))
	rounded := float32(third)
	chopped := rounded
	if float64(chopped) > third {
		chopped = math.Nextafter32(chopped, 0)
	}
	var out [2][2]int
	for i, f := range [2]float32{chopped, rounded} {
		low := int(float32(hi) * 0.75) // exact: three quarters, truncated
		high := int(f + 0.5)
		if high-lo <= 5 {
			high = lo + 6
		}
		if hi-low < 6 {
			low = max(hi-6, 1)
		}
		low = min(low, high)
		out[i] = [2]int{min(max(low, 1), 60), min(max(high, 1), 60)}
	}
	return out
}

// lfgDefaultRangeLocked is whether a search's levels are the range the window
// opened with for me (its LevelRange over its group's levels: each member's
// as the client sees its creature, its own for one out of its view, so each
// other member either; out of a group its own alone). Must be called with
// the lock held.
func (w *World) lfgDefaultRangeLocked(me *Player, low, high int) bool {
	var others []int
	if g := w.groups[me.GroupID]; me.GroupID != 0 && g != nil {
		for _, id := range g.Members {
			if id == me.ServerID {
				continue
			}
			if p := w.Players[id]; p != nil {
				others = append(others, p.Level)
			} else if n := w.NPCs[id]; n != nil {
				others = append(others, n.Level)
			}
		}
	}
	others = others[:min(len(others), 8)]
	for set := 0; set < 1<<len(others); set++ { // which members are seen at their own level
		lo, hi := me.Level, me.Level
		for i, l := range others {
			if set&(1<<i) == 0 {
				l = me.Level
			}
			lo, hi = min(lo, l), max(hi, l)
		}
		for _, r := range lfgLevelRange(lo, hi) {
			if low == r[0] && high == r[1] {
				return true
			}
		}
	}
	return false
}
