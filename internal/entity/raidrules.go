package entity

import (
	"encoding/binary"
	"log"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raidrules.go — a raid's own rules beyond a group's (the GM Jibrish's petitions 1448 and
// 1451, 2026-11-10): its loot rule, its helper and its main assist, whether it is in a
// fight, the loot claims of those kicked from it during one, and what a raid's kill tells
// the loot (handlers/raidloot.go). The commands are raidcmds.go's.
//
// A raid is a group whose leader raised a raid flag or invited by /raidinvite (flagged),
// or one past four (raid.go). Its leader is Members[0], as every group's.
//
// IN COMBAT (ours, petition 1451's "if the raid is in combat"): a raid is in a fight while
// a living creature has one of its members, or a member's pet, on its hate list. Its loot
// rule does not change then and it is not disbanded (a PS2 leader's disband in a fight
// leaves the raid instead, as its CLIENT has already let its own group go: group.go's
// GroupDisband), and a member kicked then keeps a claim on the loot of every creature
// fighting it at that moment, for that fight: the creature's corpse offers it its rolls
// and the master looter's list names it, wherever it stands. The claim ends when that
// creature dies (its kill takes it), or leaves the fight alive (leashed or won: no one on
// its hate list), or goes; a kick out of a fight claims nothing.

// LootRule is a raid's loot rule.
type LootRule uint8

const (
	// LootFreeForAll: anyone loots anything (every group's way, and a raid's
	// until its leader picks another).
	LootFreeForAll LootRule = 0
	// LootNeedGreed: the rules' items of a raid's kill are rolled on, Need
	// before Greed (handlers/raidloot.go).
	LootNeedGreed LootRule = 1
	// LootMaster: a raid's kill holding a rules' item is its master looter's
	// (the leader's) to take, assign or abandon (handlers/raidloot.go).
	LootMaster LootRule = 2
)

// String is the rule in its lines' words.
func (r LootRule) String() string {
	switch r {
	case LootNeedGreed:
		return "need before greed"
	case LootMaster:
		return "master looter"
	}
	return "free for all"
}

// ParseLootRule reads a /raidloot word: ffa (free, all, freeforall), needgreed (need,
// nbg, needbeforegreed), master (ml, masterlooter); any case.
func ParseLootRule(s string) (LootRule, bool) {
	switch strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.TrimSpace(s))) {
	case "ffa", "free", "all", "freeforall":
		return LootFreeForAll, true
	case "needgreed", "need", "nbg", "needbeforegreed", "ng":
		return LootNeedGreed, true
	case "master", "ml", "masterlooter", "masterloot":
		return LootMaster, true
	}
	return 0, false
}

// raidRules is a raid's own (Group.rules): its loot rule, its one helper (the leader's
// pick, who may invite and kick) and its main assist (whom /raidassist assists).
type raidRules struct {
	loot   LootRule
	helper EntityID
	assist EntityID
}

// raidWorld is the World's raid state beyond its groups: the kicked members' claims, by
// the creature they claim the loot of.
type raidWorld struct {
	claims map[EntityID][]raidClaim
}

// raidClaim is one kicked member's claim on a creature's loot, made while it fought the
// raid group group.
type raidClaim struct {
	player EntityID
	group  uint32
}

// raidingLocked is whether g is a raid: flagged, or past four. Must be called with the
// lock held.
func raidingLocked(g *Group) bool { return g != nil && (g.flagged || g.isRaid()) }

// raidOfLocked is p's group when it is a raid, else nil. Must be called with the lock held.
func (w *World) raidOfLocked(p *Player) *Group {
	if p == nil || p.GroupID == 0 {
		return nil
	}
	if g := w.groups[p.GroupID]; raidingLocked(g) {
		return g
	}
	return nil
}

// leaderOf is g's leader, or nil.
func (w *World) leaderOfLocked(g *Group) *Player {
	if g == nil || len(g.Members) == 0 {
		return nil
	}
	return w.Players[g.Members[0]]
}

// groupOfIDLocked is the group a fighting entity's id belongs to: a player's own, a
// pet's (or a charmed creature's) master's; 0 for none.
func (w *World) groupOfIDLocked(id EntityID) uint32 {
	if p := w.Players[id]; p != nil {
		return p.GroupID
	}
	if n := w.NPCs[id]; n != nil && n.Owner != 0 {
		if o := w.Players[n.Owner]; o != nil {
			return o.GroupID
		}
	}
	return 0
}

// groupsInCombatLocked is every group in a fight now: one of its members or their pets
// on a living creature's hate list (the file's doc). One pass over the fights' set
// (npcsets.go). Must be called with the lock held.
func (w *World) groupsInCombatLocked() map[uint32][]*NPC {
	w.syncNPCSetsLocked()
	out := map[uint32][]*NPC{}
	for id, npc := range w.sets.ai.m {
		if npc = w.setMember(w.sets.ai.m, id, npc); npc == nil || npc.Dead || len(npc.Threats) == 0 {
			continue
		}
		seen := map[uint32]bool{}
		for t := range npc.Threats {
			if g := w.groupOfIDLocked(t); g != 0 && !seen[g] {
				seen[g] = true
				out[g] = append(out[g], npc)
			}
		}
	}
	return out
}

// raidFoesLocked is the creatures g fights now (the file's doc). Must be called with the
// lock held.
func (w *World) raidFoesLocked(g *Group) []*NPC {
	if g == nil {
		return nil
	}
	return w.groupsInCombatLocked()[g.ID]
}

// RaidInCombat reports whether p's raid is in a fight (false out of one).
func (w *World) RaidInCombat(p *Player) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.raidOfLocked(p)
	return g != nil && len(w.raidFoesLocked(g)) > 0
}

// raidKickedLocked is m's kick from g, before it leaves: in a fight, a claim on every
// creature g fights (the file's doc), and m told. Must be called with the lock held.
func (w *World) raidKickedLocked(g *Group, m *Player) {
	if !raidingLocked(g) || m == nil {
		return
	}
	foes := w.raidFoesLocked(g)
	if len(foes) == 0 {
		return
	}
	if w.raids.claims == nil {
		w.raids.claims = map[EntityID][]raidClaim{}
	}
	var names []string
	for _, npc := range foes {
		have := false
		for _, c := range w.raids.claims[npc.ServerID] {
			have = have || c.player == m.ServerID && c.group == g.ID
		}
		if !have {
			w.raids.claims[npc.ServerID] = append(w.raids.claims[npc.ServerID], raidClaim{player: m.ServerID, group: g.ID})
		}
		names = append(names, npc.Name)
	}
	log.Printf("Raid: %s kicked from group %d in a fight: claims on %v", m.Name, g.ID, names)
	if s := w.sessionOfLocked(m); s != nil {
		w.sendColoredLocked(s, raidKickClaimLine, raidRGB)
	}
}

// raidKickClaimLine is a member kicked in a fight's line.
const raidKickClaimLine = "You were kicked from the raid during a fight: you keep your claim on the loot of what it was fighting."

// takeRaidClaimsLocked is (and forgets) the claims on npc's loot. Must be called with
// the lock held.
func (w *World) takeRaidClaimsLocked(npc *NPC) []raidClaim {
	c := w.raids.claims[npc.ServerID]
	delete(w.raids.claims, npc.ServerID)
	return c
}

// sweepRaidClaimsLocked forgets the claims on a creature that left its fight alive
// (nothing on its hate list), died uncredited, or went. Must be called with the lock
// held.
func (w *World) sweepRaidClaimsLocked() {
	for id := range w.raids.claims {
		if npc := w.NPCs[id]; npc == nil || npc.Dead || len(npc.Threats) == 0 {
			delete(w.raids.claims, id)
		}
	}
}

// RaidKill is what a raid's kill tells the loot (KillCredit.Raid): the rule its group
// had at the kill (never free for all: such a kill carries none), its leader (the master
// looter), whom the rolls are offered (Seats: its members in the creature's world within
// KillShareRange of it, alive or dead, and those it kicked in that fight wherever they
// are), and everyone the master looter may give to (Members: every member and the same
// kicked ones). Each seat as the kill found it.
type RaidKill struct {
	Group   uint32
	Rule    LootRule
	Leader  RaidSeat
	Seats   []RaidSeat
	Members []RaidSeat
	// TemplateHPPct is its class template's hit points of its level's (0 none): one
	// of the raid boss rules (handlers/raidloot.go)
	TemplateHPPct int
}

// RaidSeat is one player a raid's kill counts for.
type RaidSeat struct {
	Session  *session.Session
	Entity   EntityID
	ServerID uint32 // the character's (characters.serverid)
	Name     string
	Class    int
	Race     int
	Level    int
	// Claim: kicked from the raid in the fight (the file's doc), not in it now
	Claim bool
}

func seatOf(w *World, p *Player, claim bool) RaidSeat {
	return RaidSeat{Session: w.sessionOfLocked(p), Entity: p.ServerID, ServerID: p.CharServerID, Name: p.Name,
		Class: p.Class, Race: p.Race, Level: p.Level, Claim: claim}
}

// raidKillLocked is killer's kill of npc as a raid's (RaidKill), or nil: no raid, or its
// rule free for all. It takes the creature's claims either way. Must be called with the
// lock held.
func (w *World) raidKillLocked(killer *Player, npc *NPC) *RaidKill {
	claims := w.takeRaidClaimsLocked(npc)
	g := w.raidOfLocked(killer)
	if g == nil {
		g = w.plainGroupNeedGreedLocked(killer) // a group's own need before greed (grouploot.go)
	}
	if g == nil || g.rules.loot == LootFreeForAll {
		return nil
	}
	leader := w.leaderOfLocked(g)
	if leader == nil {
		return nil
	}
	k := &RaidKill{Group: g.ID, Rule: g.rules.loot, Leader: seatOf(w, leader, false)}
	if npc.tmpl != nil {
		k.TemplateHPPct = npc.tmpl.HPPct
	}
	for _, id := range g.Members {
		m := w.Players[id]
		if m == nil {
			continue
		}
		s := seatOf(w, m, false)
		k.Members = append(k.Members, s)
		if m.World != npc.World {
			continue
		}
		if dx, dz := m.X-npc.X, m.Z-npc.Z; dx*dx+dz*dz <= KillShareRange*KillShareRange {
			k.Seats = append(k.Seats, s)
		}
	}
	for _, c := range claims {
		m := w.Players[c.player]
		if c.group != g.ID || m == nil || m.GroupID == g.ID {
			continue // another raid's, gone, or back in this one (a seat already)
		}
		s := seatOf(w, m, true)
		k.Seats = append(k.Seats, s)
		k.Members = append(k.Members, s)
	}
	log.Printf("Raid: group %d's kill of %q under %s: %d seats, %d members", g.ID, npc.Name, g.rules.loot, len(k.Seats), len(k.Members))
	return k
}

// raidRGB is a raid's lines' colour: its chat's orange (handlers/chat.go's raidChatRGB).
var raidRGB = [3]byte{255, 140, 0}

// --- The raid's state for a client's windows (protocol.OpcodeRaidState) ---

// SetRaidWindows is sess's client's answer to the raid offer (protocol.OpcodeRaidAnswer):
// on, it is sent the raid's state (at the next pass, whatever was sent before), its rolls
// as windows and the master looter's list; off, chat lines. False when sess has no player.
func (w *World) SetRaidWindows(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.RaidWindows, p.raidStateSent = on, ""
	return true
}

// RaidWindows reports whether the player's client shows the raid's windows.
func (w *World) RaidWindows(p *Player) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return p != nil && p.RaidWindows
}

// raidStateBodyLocked is p's client's 0x7E44 body (protocol.OpcodeRaidState): all zero
// out of a raid. Must be called with the lock held.
func (w *World) raidStateBodyLocked(sess *session.Session, p *Player, fighting map[uint32][]*NPC) []byte {
	b := make([]byte, 0, 14)
	g := w.raidOfLocked(p)
	leader := w.leaderOfLocked(g)
	if g == nil || leader == nil {
		return w.groupLootStateLocked(p, b) // a plain group's need before greed, else all zero (grouploot.go)
	}
	id := func(e EntityID) uint32 {
		if o := w.Players[e]; o != nil && o.GroupID == g.ID {
			return w.idSeenByLocked(sess, o)
		}
		return 0
	}
	b = binary.LittleEndian.AppendUint32(b, w.idSeenByLocked(sess, leader))
	b = binary.LittleEndian.AppendUint32(b, id(g.rules.helper))
	b = binary.LittleEndian.AppendUint32(b, id(g.rules.assist))
	var flags byte
	if len(fighting[g.ID]) > 0 {
		flags |= 1
	}
	if leader == p {
		flags |= 2
	}
	if g.rules.helper == p.ServerID {
		flags |= 4
	}
	return append(b, byte(g.rules.loot), flags)
}

// sendRaidStateLocked sends p's client its raid's state when it changed (or always, with
// force). Must be called with the lock held.
func (w *World) sendRaidStateLocked(sess *session.Session, p *Player, fighting map[uint32][]*NPC, force bool) {
	if sess == nil || p == nil || !p.RaidWindows {
		return
	}
	body := w.raidStateBodyLocked(sess, p, fighting)
	if !force && string(body) == p.raidStateSent {
		return
	}
	p.raidStateSent = string(body)
	sendGroupMsg(sess, protocol.OpcodeRaidState, func(b *buffer.Writer) { b.WriteBytes(body) })
}

// raidStateNowLocked sends every member of g whose client shows the raid's windows its
// state at once (a command changed it). Must be called with the lock held.
func (w *World) raidStateNowLocked(g *Group) {
	if g == nil {
		return
	}
	fighting := w.groupsInCombatLocked()
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil && m.RaidWindows {
			w.sendRaidStateLocked(w.sessionOfLocked(m), m, fighting, false)
		}
	}
}

// raidStateNowForLocked sends p's client its state at once (it left its raid). Must be
// called with the lock held.
func (w *World) raidStateNowForLocked(p *Player) {
	if p != nil && p.RaidWindows {
		w.sendRaidStateLocked(w.sessionOfLocked(p), p, nil, false)
	}
}

// raidStateSystem sends each client that shows the raid's windows its state when it
// changed, and forgets the claims whose fight is over; with the roster, once a second.
// Must be called with the lock held.
func (w *World) raidStateSystem() {
	if w.TickCount%raidRosterTicks != 0 || w.SessionManager == nil {
		return
	}
	w.sweepRaidClaimsLocked()
	var fighting map[uint32][]*NPC
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.RaidWindows {
			return true
		}
		if fighting == nil {
			fighting = w.groupsInCombatLocked()
		}
		w.sendRaidStateLocked(s, p, fighting, false)
		return true
	})
}

// raidMemberGoneLocked is id's leaving g (left, kicked, logged out): it is no longer its
// helper or its main assist. Must be called with the lock held.
func (w *World) raidMemberGoneLocked(g *Group, id EntityID) {
	if g.rules.helper == id {
		g.rules.helper = 0
	}
	if g.rules.assist == id {
		g.rules.assist = 0
	}
}

// raidLeadPassedLocked tells g's members who leads it now, the old leader having left.
// Must be called with the lock held.
func (w *World) raidLeadPassedLocked(g *Group) {
	l := w.leaderOfLocked(g)
	if l != nil && g.rules.helper == l.ServerID {
		g.rules.helper = 0 // the helper leads now, and helps no one
	}
	if l != nil && raidingLocked(g) && len(g.Members) > 1 {
		w.groupSayColoredLocked(g, l.Name+" leads the raid now.", nil)
	}
}

// groupSayColoredLocked tells every member of g but except a line in the raid's colour.
// Must be called with the lock held.
func (w *World) groupSayColoredLocked(g *Group, line string, except *Player) {
	for _, id := range g.Members {
		if o := w.Players[id]; o != nil && o != except {
			if s := w.sessionOfLocked(o); s != nil {
				w.sendColoredLocked(s, line, raidRGB)
			}
		}
	}
}

// RaidSessions is the sessions of the members of group id now, in the order they joined,
// the leader first (for the loot's lines).
func (w *World) RaidSessions(id uint32) []*session.Session {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[id]
	if g == nil {
		return nil
	}
	var out []*session.Session
	for _, mid := range g.Members {
		if s := w.sessionOfLocked(w.Players[mid]); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// RaidLeaderOf is the character id of group id's leader now (0 for none).
func (w *World) RaidLeaderOf(id uint32) uint32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if l := w.leaderOfLocked(w.groups[id]); l != nil {
		return l.CharServerID
	}
	return 0
}

// NPCByID is the NPC of that id in the world, or nil.
func (w *World) NPCByID(id EntityID) *NPC {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.NPCs[id]
}
