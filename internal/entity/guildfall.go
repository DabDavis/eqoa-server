package entity

import (
	"log"
	"sort"
)

// guildfall.go: a Hardcore guild master's fall hands the lead on (the owner,
// 2026-11-18, petition 2384: "Pass the lead on death"). A guild has one master
// (GuildRankLeader; since petition 2976 it may have several), and its lead passed only when the master left the guild or
// its character was deleted; a fallen Hardcore character stays on its roster
// but can never log in again, so its guild was left with a master who could not
// act. Now the fall passes the lead to the highest-ranked member still playing
// (an officer first: highest()'s order, rank then the lowest id), never to
// another fallen member, and the fallen stays on the roster as a member (rank
// 0). A guild of fallen members alone keeps its master as it is.

// GuildLeadChange is a lead handed on by a fall: the guild, the fallen master
// now a member, and the new master.
type GuildLeadChange struct {
	Guild   *Guild
	Fallen  GuildMember
	NewLead GuildMember
}

// GuildFall notes character id as fallen and, when it is its guild's master,
// hands the lead on. It reports the change (false: none).
func (w *World) GuildFall(id uint32) (GuildLeadChange, bool) {
	var writes []guildWrite
	defer writeGuilds(&writes)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.guildFallen == nil {
		w.guildFallen = make(map[uint32]bool)
	}
	w.guildFallen[id] = true
	for _, g := range w.guilds {
		if m := g.Members[id]; m != nil {
			c, ok := w.handFallenLeadLocked(g, m)
			if ok {
				w.keepGuildLocked(&writes, g, false)
			}
			return c, ok
		}
	}
	return GuildLeadChange{}, false
}

// FallenGuildLeads is GuildFall for the guilds kept when the world starts:
// fallen holds every fallen Hardcore character, and each guild whose master is
// one of them has its lead handed on now (a master fallen before this rule).
// It reports how many guilds changed hands.
func (w *World) FallenGuildLeads(fallen []uint32) int {
	var writes []guildWrite
	defer writeGuilds(&writes)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.guildFallen == nil {
		w.guildFallen = make(map[uint32]bool)
	}
	for _, id := range fallen {
		w.guildFallen[id] = true
	}
	n := 0
	for _, g := range w.guilds {
		// each fallen master (a guild may have several), lowest id first
		var fallenLeads []*GuildMember
		for _, m := range g.Members {
			if m.Rank == GuildRankLeader && w.guildFallen[m.ID] {
				fallenLeads = append(fallenLeads, m)
			}
		}
		sort.Slice(fallenLeads, func(i, j int) bool { return fallenLeads[i].ID < fallenLeads[j].ID })
		changed := false
		for _, lead := range fallenLeads {
			if _, ok := w.handFallenLeadLocked(g, lead); ok {
				changed = true
			}
		}
		if changed {
			w.keepGuildLocked(&writes, g, false)
			n++
		}
	}
	return n
}

// handFallenLeadLocked hands g's lead from m, fallen, to the highest member
// still playing; nothing when m is not the master or none is left to take it.
// Must be called with the lock held.
func (w *World) handFallenLeadLocked(g *Guild, m *GuildMember) (GuildLeadChange, bool) {
	if m.Rank != GuildRankLeader {
		return GuildLeadChange{}, false
	}
	best := w.highestPlayingLocked(g, m.ID)
	if best == nil {
		log.Printf("Guild: %q's master %s has fallen; no member still playing to take the lead", g.Name, m.Name)
		return GuildLeadChange{}, false
	}
	m.Rank = GuildRankMember
	best.Rank = GuildRankLeader
	if p := w.memberPlayerLocked(m.ID); p != nil {
		p.GuildRank = GuildRankMember
	}
	if p := w.memberPlayerLocked(best.ID); p != nil {
		p.GuildRank = GuildRankLeader
	}
	log.Printf("Guild: %q's master %s has fallen; %s leads it now", g.Name, m.Name, best.Name)
	return GuildLeadChange{Guild: g, Fallen: *m, NewLead: *best}, true
}

// highestPlayingLocked is highest() among g's members but skip and the fallen,
// or nil. Must be called with the lock held.
func (w *World) highestPlayingLocked(g *Guild, skip uint32) *GuildMember {
	var best *GuildMember
	for _, o := range g.Members {
		if o.ID == skip || w.guildFallen[o.ID] {
			continue
		}
		if best == nil || o.Rank > best.Rank || (o.Rank == best.Rank && o.ID < best.ID) {
			best = o
		}
	}
	return best
}
