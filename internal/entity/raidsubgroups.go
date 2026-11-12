package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// raidsubgroups.go — a raid's groups of four inside it (the GM Jibrish's petition 1687,
// 2026-11-12: "Even though everyone's in a raid group, sort them by group 1,2,3,4 etc. and
// allow the raid leader and assistant raid leader ... to move people from group to group.
// This is so raids can easily organize for fights and say things like 'Group 2 go here'").
//
// Every player member of a raid has a group number, 1 and up, of RaidSubgroupSize at
// most: a newcomer goes into the lowest numbered group with room, in the order they
// joined (so a raid of nine is 4, 4 and 1 until someone is moved), and keeps its number
// until it leaves or is moved; one leaving frees its place and moves no one else. A
// mercenary takes no place (it is in no roster). The leader (Members[0]) and its helper
// (the "co-leader", Group.rules.helper) move a member to another group with room
// (/raidgroup <name> <n>, vanilla's window: 0x7E48 op 12) or swap two members' places
// (/raidswap <a> <b>, op 13); a group full at RaidSubgroupSize refuses a move and names
// the swap. A group's number goes up to one past what the raid's size needs
// (raidSubgroupMost), so a leader may always open a new one.
//
// The numbers are for organising alone: experience, chat, loot, group spells (scope 2
// and a group's area: the whole raid, raid.go's doc) and quest credit read Group.Members
// as before, and a PS2's window of four (shownMembersLocked) is unchanged.
//
// To vanilla they go as one more byte after each 0x7E1A record, to a client that answered
// the roster's offer 2 (the offer is 2 since: an older vanilla answers 1 and is sent the
// records as before); a PS2 never asked for the roster. Everyone, a PS2 too, hears its own
// move in chat, and /raidgroup alone lists the groups. A keep restart keeps the numbers
// (KeptGroup.Subs).

// RaidSubgroupSize is a raid's group's most members: a group's (GroupWindowSize).
const RaidSubgroupSize = GroupWindowSize

// RaidRosterWithGroups is the 0x7E18 offer's value and the 0x7E19 answer's for the
// roster with each record's group number after it.
const RaidRosterWithGroups = 2

// raidSubgroupMost is the highest group number a member may be moved to in g, its
// numbers subs: one past what its players need at RaidSubgroupSize a group, or the
// highest in use.
func raidSubgroupMost(players int, subs map[EntityID]uint8) int {
	most := (players+RaidSubgroupSize-1)/RaidSubgroupSize + 1
	for _, n := range subs {
		most = max(most, int(n))
	}
	return min(most, 255)
}

// subgroupsLocked is every player member of g's group number: its own where it has
// one, else the lowest group with room, in the order they joined. It changes nothing
// (KeepGroups calls it under the read lock). Must be called with the lock held.
func (w *World) subgroupsLocked(g *Group) map[EntityID]uint8 {
	out := make(map[EntityID]uint8, len(g.Members))
	count := map[uint8]int{}
	var unplaced []EntityID
	for _, id := range g.Members {
		if w.Players[id] == nil {
			continue
		}
		if n := g.subs[id]; n > 0 && count[n] < RaidSubgroupSize {
			out[id] = n
			count[n]++
		} else {
			unplaced = append(unplaced, id)
		}
	}
	n := uint8(1)
	for _, id := range unplaced {
		for count[n] >= RaidSubgroupSize {
			n++
		}
		out[id] = n
		count[n]++
	}
	return out
}

// settleSubgroupsLocked writes g's numbers as subgroupsLocked finds them, so a member
// keeps its place as others come and go. Must be called with the write lock held.
func (w *World) settleSubgroupsLocked(g *Group) map[EntityID]uint8 {
	g.subs = w.subgroupsLocked(g)
	return g.subs
}

// subgroupMembers is the members of number n in subs, in g's order.
func subgroupMembers(g *Group, subs map[EntityID]uint8, n uint8) []EntityID {
	var out []EntityID
	for _, id := range g.Members {
		if subs[id] == n {
			out = append(out, id)
		}
	}
	return out
}

// raidMemberLocked is g's member named name, else the one the sender knows by id, for
// a move; nil with the refusal said. Must be called with the lock held.
func (w *World) raidMemberLocked(sess *session.Session, g *Group, name string, id uint32) *Player {
	var m *Player
	if name = strings.TrimSpace(name); name != "" {
		m = w.memberNamedLocked(g, name)
	} else if t := w.playerSeenByLocked(sess, id); t != nil && t.GroupID == g.ID {
		m = t
	}
	if m == nil {
		if w.raidMercLocked(sess, g, name, id) != nil {
			w.refuseLocked(sess, "A mercenary has no place in the raid's groups: it goes with its owner.")
		} else {
			w.refuseLocked(sess, fmt.Sprintf("%s is not in your raid.", orTarget(name)))
		}
	}
	return m
}

// RaidGroupMove is /raidgroup <name> <n> and vanilla's op 12: the member (by name, else
// the id the sender knows it by) moved to group n, the leader's or its helper's. The
// file's doc.
func (w *World) RaidGroupMove(sess *session.Session, name string, id uint32, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.raidCommandLocked(sess, p, true, "move members between groups")
	if g == nil {
		return
	}
	m := w.raidMemberLocked(sess, g, name, id)
	if m == nil {
		return
	}
	subs := w.settleSubgroupsLocked(g)
	most := raidSubgroupMost(len(subs), subs)
	switch {
	case n < 1 || n > most:
		w.refuseLocked(sess, fmt.Sprintf("Groups go from 1 to %d in this raid.", most))
		return
	case int(subs[m.ServerID]) == n:
		w.refuseLocked(sess, fmt.Sprintf("%s is in group %d already.", m.Name, n))
		return
	}
	if in := subgroupMembers(g, subs, uint8(n)); len(in) >= RaidSubgroupSize {
		w.refuseLocked(sess, fmt.Sprintf("Group %d is full (%d). Swap two members: /raidswap %s <name>.", n, RaidSubgroupSize, m.Name))
		return
	}
	from := subs[m.ServerID]
	subs[m.ServerID] = uint8(n)
	log.Printf("Raid: %s moves %s from group %d to %d in raid %d", p.Name, m.Name, from, n, g.ID)
	w.subgroupMovedLocked(sess, g, p, fmt.Sprintf("%s moves %s to group %d.", p.Name, m.Name, n), m)
}

// RaidGroupSwap is /raidswap <a> <b> and vanilla's op 13: two members' groups traded,
// the leader's or its helper's. Names, else the ids the sender knows them by.
func (w *World) RaidGroupSwap(sess *session.Session, nameA, nameB string, idA, idB uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.raidCommandLocked(sess, p, true, "move members between groups")
	if g == nil {
		return
	}
	byName := strings.TrimSpace(nameA) != "" && strings.TrimSpace(nameB) != ""
	if !byName && (idA == 0 || idB == 0) {
		w.refuseLocked(sess, "Name two members: /raidswap <name> <name>.")
		return
	}
	if !byName {
		nameA, nameB = "", ""
	}
	a := w.raidMemberLocked(sess, g, nameA, idA)
	if a == nil {
		return
	}
	b := w.raidMemberLocked(sess, g, nameB, idB)
	if b == nil {
		return
	}
	subs := w.settleSubgroupsLocked(g)
	if subs[a.ServerID] == subs[b.ServerID] {
		w.refuseLocked(sess, fmt.Sprintf("%s and %s are both in group %d.", a.Name, b.Name, subs[a.ServerID]))
		return
	}
	subs[a.ServerID], subs[b.ServerID] = subs[b.ServerID], subs[a.ServerID]
	log.Printf("Raid: %s swaps %s (now %d) and %s (now %d) in raid %d", p.Name, a.Name, subs[a.ServerID], b.Name, subs[b.ServerID], g.ID)
	w.subgroupMovedLocked(sess, g, p, fmt.Sprintf("%s swaps %s (now group %d) and %s (now group %d).", p.Name, a.Name, subs[a.ServerID], b.Name, subs[b.ServerID]), a, b)
}

// subgroupMovedLocked tells a move: the mover the line, each one moved its own group
// ("You are now in group 2 of the raid."), and every roster sent now. Must be called
// with the lock held.
func (w *World) subgroupMovedLocked(sess *session.Session, g *Group, mover *Player, line string, moved ...*Player) {
	w.raidLineLocked(sess, line)
	for _, m := range moved {
		if m == mover {
			continue
		}
		if s := w.sessionOfLocked(m); s != nil {
			w.raidLineLocked(s, fmt.Sprintf("%s puts you in group %d of the raid.", mover.Name, g.subs[m.ServerID]))
		}
	}
	w.raidRosterNowLocked(g)
}

// RaidGroups is /raidgroup alone: the raid's groups, a line each ("Group 1: Ann, Bo"),
// for anyone in it (a PS2's way to see them).
func (w *World) RaidGroups(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.raidOfLocked(p)
	if g == nil {
		w.refuseLocked(sess, "You are not in a raid.")
		return
	}
	subs := w.settleSubgroupsLocked(g)
	for _, l := range subgroupLines(g, subs, func(id EntityID) string { return w.Players[id].Name }) {
		w.raidLineLocked(sess, l)
	}
	w.raidLineLocked(sess, fmt.Sprintf("You are in group %d. The leader or its helper moves: /raidgroup <name> <n>, /raidswap <name> <name>.", subs[p.ServerID]))
}

// subgroupLines is a line a group, in number order: "Group 1: Ann, Bo".
func subgroupLines(g *Group, subs map[EntityID]uint8, name func(EntityID) string) []string {
	var nums []int
	seen := map[uint8]bool{}
	for _, n := range subs {
		if !seen[n] {
			seen[n] = true
			nums = append(nums, int(n))
		}
	}
	sort.Ints(nums)
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		var names []string
		for _, id := range subgroupMembers(g, subs, uint8(n)) {
			names = append(names, name(id))
		}
		out = append(out, fmt.Sprintf("Group %d: %s", n, strings.Join(names, ", ")))
	}
	return out
}
