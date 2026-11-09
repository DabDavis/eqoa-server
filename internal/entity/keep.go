package entity

// keep.go — what a restart of the server process carries of the World for
// each session it keeps (claude-notes/session-keep-restart-20261106.md;
// session/keep.go has the connection's half, network/udp/keep.go the rest).
//
// A character comes back through its row, as at a login: the shutdown's save
// (SaveAll) writes its place, level, effects, standings, masteries, a rez's
// share and its hit points first. What a login does not carry is kept here:
// the id its client knows its target by, its lockouts, what its client asked
// for (power bars, the duel window), whether it was dead, and, per session,
// which entity sits on each of the client's entity slots under which id: the
// client keeps its scene through the restart, and each slot must go on
// describing the entity it shows (an NPC's id is its row's, a player's its
// character's, the same in the new process), or take it away. Groups are
// formed again among the characters that came back. A fight, a cast on its
// way, a duel, a trade and an invitation end. A pet comes through with its
// owner (KeptPet, keeppet.go: petition 1363). The creatures come through by
// keepnpc.go.

import (
	"fmt"
	"time"

	"github.com/eqoa/server/internal/session"
)

// KeptPlayer is what a restart carries of a player beyond its row.
type KeptPlayer struct {
	CharServerID uint32        `json:"char"`
	Name         string        `json:"name"`
	Dead         bool          `json:"dead,omitempty"`
	Target       uint32        `json:"target,omitempty"`
	Lockouts     []KeptLockout `json:"lockouts,omitempty"`
	PowerBars    bool          `json:"power_bars,omitempty"`
	RaidRoster   bool          `json:"raid_roster,omitempty"`
	DuelWindow   bool          `json:"duel_window,omitempty"`
	GM           bool          `json:"gm,omitempty"`
	Flying       bool          `json:"flying,omitempty"`
	Place        [3]float32    `json:"place"`
	World        uint8         `json:"world"`
	RaceLabels   bool          `json:"race_labels,omitempty"`
	// RecordWindow: its client has the combat record window (combatrecord.go)
	RecordWindow bool `json:"record_window,omitempty"`
	// PvP: its flag (pvp.go), and how long it must yet stay on
	PvP       bool  `json:"pvp,omitempty"`
	PvPHoldMS int64 `json:"pvp_hold_ms,omitempty"`
	// Jumps: its client jumps (jump.go)
	Jumps bool `json:"jumps,omitempty"`
	// Pet: its pet, summoned or charmed (keeppet.go), nil none
	Pet *KeptPet `json:"pet,omitempty"`
}

// KeptLockout is a spell's lockout and what is left of it.
type KeptLockout struct {
	Spell  int   `json:"spell"`
	Index  int   `json:"index"`
	LeftMS int64 `json:"left_ms"`
}

// KeptView is a session's entity slots past the player's own (index i is slot
// i+1), and the next id it numbers an entity with.
type KeptView struct {
	Slots []KeptSlot `json:"slots"`
	Next  uint32     `json:"next"`
}

// KeptSlot is an entity on a slot: an NPC by its row's id, a player by its
// character's (its entity id may be another, CreatePlayer), with its name to
// tell the same one; and the id the client knows it by.
type KeptSlot struct {
	Kind   string `json:"kind,omitempty"` // "npc", "player", "" empty
	ID     uint32 `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Object uint32 `json:"object,omitempty"`
}

// KeptGroup is a group by its members' characters, the leader first.
type KeptGroup struct {
	ID      uint32   `json:"id"`
	Members []uint32 `json:"members"`
}

// KeepPlayer is what a restart carries of sess's player, false when it has
// none.
func (w *World) KeepPlayer(sess *session.Session, now time.Time) (KeptPlayer, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return KeptPlayer{}, false
	}
	k := KeptPlayer{CharServerID: p.CharServerID, Name: p.Name, Dead: p.IsDead() || p.respawnPending(),
		Target: p.TargetObjectID, PowerBars: p.PowerBars, RaidRoster: p.RaidRoster, DuelWindow: p.DuelWindow, GM: p.GM, Flying: p.Flying,
		Place: [3]float32{p.X, p.Y, p.Z}, World: p.World}
	k.RaceLabels = p.RaceLabels
	k.RecordWindow = p.RecordWindow
	k.Jumps = p.Jumps
	if p.pvp.on {
		k.PvP, k.PvPHoldMS = true, max(0, p.pvp.hold.Sub(now).Milliseconds())
	}
	for _, l := range p.Lockouts {
		if left := l.until.Sub(now); left > 0 {
			k.Lockouts = append(k.Lockouts, KeptLockout{Spell: l.spellID, Index: l.displayIndex, LeftMS: left.Milliseconds()})
		}
	}
	k.Pet = w.keptPetLocked(p, now) // keeppet.go
	return k, true
}

// RestoreKeptPlayer gives sess's player, built anew from its row, what its
// last process kept of it: its target, its lockouts (their time running on
// from now) and what its client asked for. It reports whether sess has a
// player.
func (w *World) RestoreKeptPlayer(sess *session.Session, k KeptPlayer, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.TargetObjectID = k.Target
	p.Lockouts = p.Lockouts[:0]
	for _, l := range k.Lockouts {
		p.Lockouts = append(p.Lockouts, castLockout{spellID: l.Spell, displayIndex: l.Index,
			until: now.Add(time.Duration(l.LeftMS) * time.Millisecond)})
	}
	p.RaceLabels, p.raceLabelsSent = k.RaceLabels, "" // the client keeps its list; a data step may have changed it
	p.PowerBars, p.powerBarsSent, p.powerBarsDue = k.PowerBars, "", k.PowerBars
	p.RaidRoster, p.raidRosterSent = k.RaidRoster, ""
	p.GM = k.GM
	p.DuelWindow = k.DuelWindow
	// its record's summary is read again (handlers/keep.go)
	p.RecordWindow = k.RecordWindow
	if k.PvP && w.pvpAllowed { // a server since turned off drops it
		p.pvp = pvpFlag{on: true, hold: now.Add(time.Duration(k.PvPHoldMS) * time.Millisecond)}
	}
	p.Jumps = k.Jumps && w.jumpsAllowed // and its jumps (its client is told: handlers/keep.go)
	w.restoreFlyingLocked(p, k)
	return true
}

// KeepView is sess's entity slots as the client has them.
func (w *World) KeepView(sess *session.Session) KeptView {
	w.mu.RLock()
	defer w.mu.RUnlock()
	vs, ok := sess.VisibilityState().(*VisibilityState)
	if !ok || vs == nil {
		return KeptView{}
	}
	k := KeptView{Next: vs.nextObjectID, Slots: make([]KeptSlot, len(vs.ChannelEntity))}
	for i, id := range vs.ChannelEntity {
		if id == 0 {
			continue
		}
		if npc, ok := w.NPCs[id]; ok {
			k.Slots[i] = KeptSlot{Kind: "npc", ID: uint32(id), Name: npc.Name, Object: vs.ChannelObjectID[i]}
		} else if p, ok := w.Players[id]; ok {
			k.Slots[i] = KeptSlot{Kind: "player", ID: p.CharServerID, Name: p.Name, Object: vs.ChannelObjectID[i]}
		}
	}
	return k
}

// RestoreView puts sess's entity slots back as its client has them, once
// every kept character is in the world again: each slot whose entity is in
// this world (an NPC of the row and name, not one made for the moment; a
// player of the character and name) goes on describing it under the same id,
// and each other is taken away on the client (its channel holds the last
// value it sent: StateUpdateData.MarkRemoved). The next new entity gets an id
// past every one the client was given. It reports how many slots were kept
// and taken away.
func (w *World) RestoreView(sess *session.Session, k KeptView) (kept, gone int) {
	conn := sess.Connection()
	if conn == nil {
		return 0, 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	vs := GetOrCreateVisibility(sess)
	vs.fit(conn)
	vs.nextObjectID = max(vs.nextObjectID, k.Next)
	for i, s := range k.Slots {
		if s.Kind == "" || i >= len(vs.ChannelEntity) {
			continue
		}
		vs.nextObjectID = max(vs.nextObjectID, s.Object)
		var id EntityID
		switch s.Kind {
		case "npc":
			if npc, ok := w.NPCs[EntityID(s.ID)]; ok && !npc.Temporary && npc.Name == s.Name {
				id = npc.ServerID
			}
		case "player":
			if p := w.playerByCharacterLocked(s.ID); p != nil && p.Name == s.Name {
				id = p.ServerID
			}
		}
		if id != 0 {
			vs.ChannelEntity[i], vs.ChannelObjectID[i] = id, s.Object
			kept++
			continue
		}
		vs.ChannelEntity[i], vs.ChannelObjectID[i] = 0, 0
		if state := conn.GetServerObject(i + 1); state != nil {
			state.MarkRemoved()
		}
		gone++
	}
	return kept, gone
}

// KeepGroups is every group of two players or more, by their characters.
func (w *World) KeepGroups() []KeptGroup {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []KeptGroup
	for _, g := range w.groups {
		k := KeptGroup{ID: g.ID}
		for _, id := range g.Members {
			if p := w.Players[id]; p != nil {
				k.Members = append(k.Members, p.CharServerID)
			}
		}
		if len(k.Members) >= 2 {
			out = append(out, k)
		}
	}
	return out
}

// RestoreGroups forms the kept groups again among the characters in the
// world, leader first as they were, any number of them; a group left with fewer
// than two is not formed. Each member's client is sent its roster by the group
// system (no roster sent yet), and a raid group's members are told nothing unless
// it stopped being one (raidChangedLocked). It returns how many groups were formed.
func (w *World) RestoreGroups(groups []KeptGroup) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.groups == nil {
		w.groups = map[uint32]*Group{}
	}
	n := 0
	var errs []string
	for _, k := range groups {
		if _, taken := w.groups[k.ID]; taken || k.ID == 0 {
			errs = append(errs, fmt.Sprintf("group %d: its id is taken", k.ID))
			continue
		}
		g := &Group{ID: k.ID, lastRoster: map[EntityID]string{}, raid: len(k.Members) > GroupWindowSize}
		for _, c := range k.Members {
			if p := w.playerByCharacterLocked(c); p != nil && p.GroupID == 0 {
				g.Members = append(g.Members, p.ServerID)
			}
		}
		if len(g.Members) < 2 {
			continue
		}
		for _, id := range g.Members {
			w.Players[id].GroupID = g.ID
		}
		w.groups[g.ID] = g
		w.nextGroupID = max(w.nextGroupID, g.ID)
		n++
	}
	if len(errs) > 0 {
		return n, fmt.Errorf("%v", errs)
	}
	return n, nil
}

// PlaceKeptDead puts a character that was dead when the restart came at its
// home (home: its bind point, else where a login starts), whole, as the
// respawn it was waiting for would have; the caller sends the dump.
func (w *World) PlaceKeptDead(sess *session.Session, x, y, z float32, world uint8) bool {
	p := w.SessionPlayer(sess)
	if p == nil {
		return false
	}
	_, _, ok := w.PlaceRespawn(p.ServerID, x, y, z, world)
	return ok
}
