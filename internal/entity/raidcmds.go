package entity

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// raidcmds.go — the raid's commands (the GM Jibrish's petitions 1448, 1451 and 1452,
// 2026-11-10; handlers/raidcmds.go routes the typed lines and vanilla's menu here):
//
//   - /raidinvite [name]: the leader or its helper invites a player by name, else its
//     target, in the raid flag's box ("<leader>'s raid"), the yes joining whatever the
//     size; a caller in no group takes a box of its own first (raidflag.go's doc), and
//     a plain group's leader's invite makes it a raid (flagged);
//   - /raidaoe, the old /raid: the flag (raidflag.go), the helper's too;
//   - /raidleave: any member out of its group, its client told (0x069A 8: CLIENT's case
//     with no word) and a line, wherever its client thinks it is (a replaced login's
//     shows none: raid.go's raidStillLine); the leader leaving hands the lead on, as any
//     group's (Members[0]);
//   - /raidkick <name>: the leader or its helper (never the leader), as Boot Member;
//   - /raidleader [name]: the lead handed to a member (the leader's); alone, who leads,
//     helps and assists, and the loot rule;
//   - /raidhelper [name|none]: the leader's one helper, a member, who may invite and
//     kick; /raidassist <name|none>: the leader's main assist; /raidassist alone,
//     anyone's: its target targeted (0x007B, the handler's);
//   - /raidloot [ffa|needgreed|master]: the leader's loot rule, never in a fight
//     (raidrules.go's doc);
//   - /invite <name>: a group invite by name, GroupInvite's rules (a caller in no group
//     takes a box of its own first, as for the flag);
//   - /target <name> (/tar): the nearest player, NPC or creature the client shows within
//     TargetByNameRange whose name is it or starts with it (TargetNear).
//
// The lines are ours, in a PS2's own characters; a refusal is red.

// TargetByNameRange is how far /target reaches (petition 1452: "within 100 meters").
const TargetByNameRange float32 = 100

// NoTargetInRange is /target's line when nothing of the name is within reach, the GM
// Jibrish's words.
const NoTargetInRange = "No target in range"

// raidOwnInviteWords is what a caller's own box is from for /invite in no group.
const raidOwnInviteWords = "Your own group"

// raidLeaveSub is the 0x069A case /raidleave sends: 8, CLIENT's case with no word (the
// decomp's viclient.cpp; 1 says "The group leader has ejected you.", 2 "The group has
// been disbanded.").
const raidLeaveSub = 8

func (w *World) refuseLocked(sess *session.Session, line string) {
	w.sendColoredLocked(sess, line, castRefusalRGB)
}

func (w *World) raidLineLocked(sess *session.Session, line string) {
	w.sendColoredLocked(sess, line, raidRGB)
}

// inWorldPlayerLocked is the player of that name in the world (any case), not a GM
// hidden from players, or nil. Must be called with the lock held.
func (w *World) inWorldPlayerLocked(name string) *Player {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for _, p := range w.Players {
		if strings.EqualFold(p.Name, name) && !p.GMHidden {
			if s := w.sessionOfLocked(p); s != nil && s.CharacterInWorld() {
				return p
			}
		}
	}
	return nil
}

// memberNamedLocked is g's member of that name (any case), or nil. Must be called with
// the lock held.
func (w *World) memberNamedLocked(g *Group, name string) *Player {
	name = strings.TrimSpace(name)
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil && strings.EqualFold(m.Name, name) {
			return m
		}
	}
	return nil
}

// raidCommandLocked is the caller's raid for a command only its leader (or, helper,
// its helper too) may give: the raid, or nil with the refusal said. Must be called with
// the lock held.
func (w *World) raidCommandLocked(sess *session.Session, p *Player, helper bool, what string) *Group {
	g := w.raidOfLocked(p)
	if g == nil {
		w.refuseLocked(sess, "You are not in a raid.")
		return nil
	}
	if g.Members[0] == p.ServerID || helper && g.rules.helper == p.ServerID {
		return g
	}
	if helper {
		w.refuseLocked(sess, fmt.Sprintf("Only your raid's leader or its helper can %s.", what))
	} else {
		w.refuseLocked(sess, fmt.Sprintf("Only your raid's leader can %s.", what))
	}
	return nil
}

// RaidInvite is /raidinvite: the player named, else the one the sender knows by
// targetID (its target). The file's doc.
func (w *World) RaidInvite(sess *session.Session, name string, targetID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	var t *Player
	if name = strings.TrimSpace(name); name != "" {
		if t = w.inWorldPlayerLocked(name); t == nil {
			w.refuseLocked(sess, fmt.Sprintf("No player named %s is in the world.", name))
			return
		}
	} else if t = w.playerSeenByLocked(sess, targetID); t == nil {
		w.refuseLocked(sess, "Target a player to invite to your raid, or name one: /raidinvite <name>.")
		return
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		if t == p {
			w.refuseLocked(sess, "You cannot invite yourself.")
			return
		}
		// its client must take the box first (raidflag.go's doc)
		w.askLocked(p, p, sess, groupAsk{raid: true, invite: t.ServerID}, raidOwnBoxWords)
		w.sendChatToSession(sess, fmt.Sprintf("Accept the box to lead a raid group and invite %s.", t.Name))
		log.Printf("Raid: %s invites %s, in no group: asked first", p.Name, t.Name)
		return
	}
	if g.Members[0] != p.ServerID && !(raidingLocked(g) && g.rules.helper == p.ServerID) {
		w.refuseLocked(sess, "Only your raid's leader or its helper can invite to the raid.")
		return
	}
	w.raidInviteLocked(sess, p, g, t)
}

// raidInviteLocked is inviter's raid invite of t into g: the raid flag's box, its yes
// joining whatever the size, and g a raid from now. Must be called with the lock held.
func (w *World) raidInviteLocked(sess *session.Session, inviter *Player, g *Group, t *Player) {
	ts := w.sessionOfLocked(t)
	switch {
	case t == nil || ts == nil || !ts.CharacterInWorld():
		w.refuseLocked(sess, "That player is not in the world.")
		return
	case t == inviter:
		w.refuseLocked(sess, "You cannot invite yourself.")
		return
	case t.GroupID == g.ID:
		w.refuseLocked(sess, fmt.Sprintf("%s is already in your raid.", t.Name))
		return
	case t.GroupID != 0:
		w.refuseLocked(sess, fmt.Sprintf("%s is already in a group.", t.Name))
		return
	}
	if ask, up := w.invites[t.ServerID][inviter.ServerID]; up && ask.declined.IsZero() {
		w.refuseLocked(sess, fmt.Sprintf("%s has not answered your invitation yet.", t.Name))
		return
	}
	g.flagged = true
	w.raidBoxLocked(t, inviter, ts, fmt.Sprintf("%s invites you to %s's raid.", inviter.Name, w.raidNameLocked(inviter)))
	w.raidLineLocked(sess, fmt.Sprintf("You invite %s to the raid.", t.Name))
	log.Printf("Raid: %s invites %s to group %d", inviter.Name, t.Name, g.ID)
}

// InviteByName is /invite <name>: a group invite, GroupInvite's rules. The file's doc.
func (w *World) InviteByName(sess *session.Session, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		w.refuseLocked(sess, "Name a player: /invite <name>.")
		return
	}
	t := w.inWorldPlayerLocked(name)
	if t == nil {
		w.refuseLocked(sess, fmt.Sprintf("No player named %s is in the world.", name))
		return
	}
	if t == p {
		w.refuseLocked(sess, "You cannot invite yourself.")
		return
	}
	if p.GroupID == 0 || w.groups[p.GroupID] == nil {
		if t.GroupID != 0 {
			w.sendGroupNotice(sess, 2, t.Name)
			return
		}
		// its client must take the box first: a group the server alone made would be none
		// to it (raidflag.go's doc)
		w.askLocked(p, p, sess, groupAsk{invite: t.ServerID}, raidOwnInviteWords)
		w.sendChatToSession(sess, fmt.Sprintf("Accept the box to form a group and invite %s.", t.Name))
		return
	}
	w.groupInviteLocked(sess, p, t)
}

// RaidLeave is /raidleave (and vanilla's Leave Raid): out of the group. The file's doc.
func (w *World) RaidLeave(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		w.refuseLocked(sess, "You are not in a raid.")
		return
	}
	raid := raidingLocked(g)
	// CLIENT's 0x069A lets its group go whatever the case; 8 says nothing (a client
	// showing no group, InGroup 0, returns at once: harmless)
	w.sendGroupRemoved(p, raidLeaveSub)
	w.leaveGroupLocked(p)
	if raid {
		w.raidLineLocked(sess, "You left the raid.")
	} else {
		w.raidLineLocked(sess, "You left the group.")
	}
}

// mayKickLocked is whether p may take m out of g: its leader, or a raid's helper (never
// its leader). Must be called with the lock held.
func (w *World) mayKickLocked(g *Group, p, m *Player) bool {
	if len(g.Members) == 0 || m == nil || m == p || m.GroupID != g.ID {
		return false
	}
	if g.Members[0] == p.ServerID {
		return true
	}
	return raidingLocked(g) && g.rules.helper == p.ServerID && g.Members[0] != m.ServerID
}

// RaidKick is /raidkick <name> (or the target, by its id, with no name): the leader's or
// the helper's, as Boot Member (bootLocked: in a fight it keeps its claims). The file's doc.
func (w *World) RaidKick(sess *session.Session, name string, targetID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.raidCommandLocked(sess, p, true, "kick from the raid")
	if g == nil {
		return
	}
	var m *Player
	if name = strings.TrimSpace(name); name != "" {
		m = w.memberNamedLocked(g, name)
	} else if t := w.playerSeenByLocked(sess, targetID); t != nil && t.GroupID == g.ID {
		m = t
	}
	switch {
	case m == nil && name != "":
		w.refuseLocked(sess, fmt.Sprintf("%s is not in your raid.", name))
		return
	case m == nil:
		w.refuseLocked(sess, "Name a member: /raidkick <name>.")
		return
	case m == p:
		w.refuseLocked(sess, "Use /raidleave to leave the raid.")
		return
	case !w.mayKickLocked(g, p, m):
		w.refuseLocked(sess, "The raid's helper cannot kick its leader.")
		return
	}
	log.Printf("Raid: %s kicks %s from group %d", p.Name, m.Name, g.ID)
	who := m.Name
	w.bootLocked(g, m)
	w.raidLineLocked(sess, fmt.Sprintf("You kicked %s from the raid.", who))
}

// RaidLeader is /raidleader <name>: the lead handed to a member (the leader's), first in
// the roster so every member's client knows it (Msg0772's member 0); with no name, the
// raid as it stands (raidInfoLocked), anyone's.
func (w *World) RaidLeader(sess *session.Session, name string, targetID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" && targetID == 0 {
		w.raidInfoLocked(sess, p)
		return
	}
	g := w.raidCommandLocked(sess, p, false, "hand on the lead")
	if g == nil {
		return
	}
	m := w.memberNamedLocked(g, name)
	if name == "" {
		if t := w.playerSeenByLocked(sess, targetID); t != nil && t.GroupID == g.ID {
			m = t
		}
	}
	switch {
	case m == nil:
		w.refuseLocked(sess, fmt.Sprintf("%s is not in your raid.", orTarget(name)))
		return
	case m == p:
		w.refuseLocked(sess, "You lead the raid already.")
		return
	}
	for i, id := range g.Members {
		if id == m.ServerID {
			copy(g.Members[1:i+1], g.Members[:i])
			g.Members[0] = m.ServerID
			break
		}
	}
	if g.rules.helper == m.ServerID {
		g.rules.helper = 0 // the leader helps no one
	}
	log.Printf("Raid: %s hands group %d's lead to %s", p.Name, g.ID, m.Name)
	w.sendRosterLocked(g, true)
	w.groupSayColoredLocked(g, fmt.Sprintf("%s makes %s the raid's leader.", p.Name, m.Name), nil)
	w.raidStateNowLocked(g)
}

func orTarget(name string) string {
	if name == "" {
		return "Your target"
	}
	return name
}

// RaidHelper is /raidhelper <name|none>: the leader's one helper, a member; with no
// name, the raid as it stands.
func (w *World) RaidHelper(sess *session.Session, name string, targetID uint32) {
	w.raidPick(sess, name, targetID, "helper")
}

// RaidAssistSet is /raidassist <name|none>: the leader's main assist, a member.
func (w *World) RaidAssistSet(sess *session.Session, name string, targetID uint32) {
	w.raidPick(sess, name, targetID, "main assist")
}

// raidPick sets g's helper or main assist to the member named (or targeted), none for
// "none".
func (w *World) raidPick(sess *session.Session, name string, targetID uint32, role string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" && targetID == 0 && role == "helper" {
		w.raidInfoLocked(sess, p)
		return
	}
	g := w.raidCommandLocked(sess, p, false, "pick its "+role)
	if g == nil {
		return
	}
	slot := &g.rules.helper
	if role != "helper" {
		slot = &g.rules.assist
	}
	if strings.EqualFold(name, "none") || strings.EqualFold(name, "off") || strings.EqualFold(name, "clear") {
		*slot = 0
		w.groupSayColoredLocked(g, fmt.Sprintf("%s: the raid has no %s now.", p.Name, role), nil)
		w.raidStateNowLocked(g)
		return
	}
	m := w.memberNamedLocked(g, name)
	if name == "" {
		if t := w.playerSeenByLocked(sess, targetID); t != nil && t.GroupID == g.ID {
			m = t
		}
	}
	switch {
	case m == nil:
		w.refuseLocked(sess, fmt.Sprintf("%s is not in your raid.", orTarget(name)))
		return
	case role == "helper" && m == p:
		w.refuseLocked(sess, "The leader cannot be its own helper.")
		return
	}
	*slot = m.ServerID
	log.Printf("Raid: %s makes %s group %d's %s", p.Name, m.Name, g.ID, role)
	w.groupSayColoredLocked(g, fmt.Sprintf("%s makes %s the raid's %s.", p.Name, m.Name, role), nil)
	w.raidStateNowLocked(g)
}

// RaidAssist is /raidassist alone: whom the raid's main assist has targeted, as the
// sender knows it (Assist's rules, assist.go), and whether it has a raid with one (else
// the refusal said).
func (w *World) RaidAssist(sess *session.Session) (uint32, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return NoAssist, false
	}
	g := w.raidOfLocked(p)
	if g == nil {
		w.refuseLocked(sess, "You are not in a raid.")
		return NoAssist, false
	}
	ma := w.Players[g.rules.assist]
	if ma == nil || ma.GroupID != g.ID {
		w.refuseLocked(sess, "Your raid has no main assist: its leader names one with /raidassist <name>.")
		return NoAssist, false
	}
	if ma == p {
		w.refuseLocked(sess, "You are the raid's main assist.")
		return NoAssist, false
	}
	return w.assistIDLocked(sess, w.targetOfLocked(ma)), true
}

// RaidLootRule is /raidloot <rule> (and vanilla's menu): the leader's, never in a fight
// (raidrules.go's doc); with no rule, the raid as it stands.
func (w *World) RaidLootRule(sess *session.Session, word string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	word = strings.TrimSpace(word)
	if word == "" {
		w.raidInfoLocked(sess, p)
		return
	}
	rule, ok := ParseLootRule(word)
	if !ok {
		w.refuseLocked(sess, "Loot rules: /raidloot ffa, /raidloot needgreed or /raidloot master.")
		return
	}
	w.raidLootRuleLocked(sess, p, rule)
}

// RaidLootRuleTo is vanilla's menu's loot rule (protocol.OpcodeRaidCommand 1).
func (w *World) RaidLootRuleTo(sess *session.Session, rule LootRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && rule <= LootMaster {
		w.raidLootRuleLocked(sess, p, rule)
	}
}

func (w *World) raidLootRuleLocked(sess *session.Session, p *Player, rule LootRule) {
	g := w.raidCommandLocked(sess, p, false, "set its loot rule")
	if g == nil {
		return
	}
	if rule == g.rules.loot {
		w.raidLineLocked(sess, fmt.Sprintf("Your raid's loot rule is %s already.", rule))
		return
	}
	if len(w.raidFoesLocked(g)) > 0 {
		w.refuseLocked(sess, "Your raid is in combat: its loot rule cannot change until the fight is over.")
		return
	}
	g.rules.loot = rule
	log.Printf("Raid: %s sets group %d's loot rule to %s", p.Name, g.ID, rule)
	w.groupSayColoredLocked(g, fmt.Sprintf("%s sets the raid's loot rule: %s.", p.Name, rule), nil)
	w.raidStateNowLocked(g)
}

// raidInfoLocked tells p its raid as it stands: who leads, helps and assists, the loot
// rule and how many. Must be called with the lock held.
func (w *World) raidInfoLocked(sess *session.Session, p *Player) {
	g := w.raidOfLocked(p)
	if g == nil {
		w.raidLineLocked(sess, "You are not in a raid. A leader's /raidaoe or /raidinvite <name> makes one.")
		return
	}
	name := func(id EntityID) string {
		if m := w.Players[id]; m != nil && m.GroupID == g.ID {
			return m.Name
		}
		return "none"
	}
	fight := ""
	if len(w.raidFoesLocked(g)) > 0 {
		fight = " In combat."
	}
	w.raidLineLocked(sess, fmt.Sprintf("Raid of %d: leader %s, helper %s, main assist %s, loot %s.%s",
		len(g.Members), name(g.Members[0]), name(g.rules.helper), name(g.rules.assist), g.rules.loot, fight))
}

// disbandRefusedLocked is a disband of a raid in a fight (raidrules.go's doc): refused,
// said, and true (GroupDisband has its leader leave instead). Must be called with the
// lock held.
func (w *World) disbandRefusedLocked(sess *session.Session, g *Group, p *Player) bool {
	if !raidingLocked(g) || len(w.raidFoesLocked(g)) == 0 {
		return false
	}
	log.Printf("Raid: %s's disband of group %d refused in a fight: leaves it", p.Name, g.ID)
	w.refuseLocked(sess, "Your raid is in combat: it cannot be disbanded. You left it; it fights on.")
	return true
}

// TargetNear is /target <name> (/tar): the nearest the sender's client shows within
// TargetByNameRange whose name is name, any case (an NPC's with or without its "a",
// "an" or "the"), else the nearest whose name starts with it (petition 1453: "if i
// /target jib and the player name is jibrish ... it will target that"); an exact name
// wins over a start, the nearer over the farther. Players, NPCs, creatures and corpses
// alike; the sender itself by its whole name alone. Its id as the client knows it, or
// false.
func (w *World) TargetNear(sess *session.Session, name string) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	want := strings.ToLower(strings.TrimSpace(name))
	if p == nil || want == "" {
		return 0, false
	}
	if strings.EqualFold(p.Name, want) {
		return sess.PlayerObjectID(), true
	}
	vs, ok := sess.VisibilityState().(*VisibilityState)
	if !ok || vs == nil {
		return 0, false
	}
	best, bestRank, bestD := uint32(0), 3, float32(0)
	for i, e := range vs.ChannelEntity {
		if e == 0 || vs.ChannelObjectID[i] == 0 {
			continue
		}
		var n string
		var x, y, z float32
		if npc, ok := w.NPCs[e]; ok {
			n, x, y, z = npc.Name, npc.X, npc.Y, npc.Z
		} else if o, ok := w.Players[e]; ok {
			n, x, y, z = o.Name, o.X, o.Y, o.Z
		} else {
			continue
		}
		rank := nameRank(n, want)
		if rank > 1 {
			continue
		}
		d := dist3(x, y, z, p.X, p.Y, p.Z)
		if d > TargetByNameRange {
			continue
		}
		if rank < bestRank || rank == bestRank && d < bestD {
			best, bestRank, bestD = vs.ChannelObjectID[i], rank, d
		}
	}
	return best, best != 0
}

// nameRank is how a name meets what /target typed (lower case): 0 it is the name, 1 the
// name starts with it, 2 neither. An NPC's article ("a moss snake") may be left off.
func nameRank(name, want string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	bare := n
	for _, a := range []string{"a ", "an ", "the "} {
		if strings.HasPrefix(n, a) {
			bare = n[len(a):]
			break
		}
	}
	switch {
	case n == want || bare == want:
		return 0
	case strings.HasPrefix(n, want) || strings.HasPrefix(bare, want):
		return 1
	}
	return 2
}
