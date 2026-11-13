package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// instancemembers.go — who else an instance holds (the GM Jibrish's petition 1879, 2026-11-13: "Add the ability to
// invite or remove people from your instance. Under group or raid give an option each. How this should work: We set
// an instance player limit, the player instance control menu should allow you to add players up to that cap. For all
// heroic dungeons, such as heroic guk, set this cap to 4. If a player is in an instanced dungeon and gets removed from
// the instance this way, he gets a 60 second warning before being removed from the instance and set back to freeport.
// They will still get to roll on any loot they were a part of is need before greed is on, up to the point a group
// leader hit remove instance. Only the group or raid leader / co leader can remove or add from the instance. If
// players are in a dungeon in progress without 4 people, they can add someone with /addinstance playername or via the
// group menu up to the 4 player cap for heroic dungeons. For raids, temporarily set this cap to 50, tho we may change
// that later.").
//
// An instance (dungeons.go) was its owner's and its owner's group's alone. Now:
//
//   - Its party's leader (its group's first member), its raid's helper (the co leader: raidrules.go), or its owner
//     alone adds a player by name or target (/addinstance; the menu's 0x7E48 op 14): the player may then stand in it
//     and enter it from a Dungeon Warden or /dungeon enter, as its party does, though in no group of it. Up to the
//     instance's cap of players, its party's and those added counted together: a dungeon's own (the recipe's "cap"),
//     else world.dungeons.cap (DefaultDungeonCap, 4: every instanced dungeon is a heroic one, "Heroic Guk"); a raid's
//     instance world.dungeons.raid_cap (DefaultDungeonRaidCap, 50). A party already at the cap adds no one.
//   - The same may take a player out of it (/removeinstance; op 15): one added, or one of its own party (who stays in
//     the group). Standing in it, the player is warned and has DungeonRemoveWarning (60 s) before it is sent out to
//     Freeport (in front of the Dungeon Warden by Dabrish); outside, it may no longer go in. Either way it stays out
//     until added again (which takes back a removal) or the instance closes. Its owner is never taken out.
//   - The loot: a kill's need before greed seats (raidrules.go) leave out whoever the instance took out, from the moment
//     it was taken out: what it was part of before (a roll already offered at a kill) stays its.
//
// Kept over a keep restart with the instance (keepdungeon.go).

// The caps and the warning.
const (
	DefaultDungeonCap     = 4
	DefaultDungeonRaidCap = 50
	DungeonRemoveWarning  = 60 * time.Second
)

// The lines (ours).
const (
	instLineAdded      = "%s is added to your party's instance of %s (%d of %d players)."
	instLineAddedYou   = "%s adds you to their instance of %s: talk to a Dungeon Warden, or type /dungeon enter %s."
	instLineFull       = "Your party's instance of %s holds %d players, its most: no one more can be added."
	instLineThere      = "%s may stand in your party's instance of %s already."
	instLineRemoved    = "%s is taken out of your party's instance of %s."
	instLineRemovedIn  = "%s takes you out of their instance of %s: in %d seconds you are sent back to Freeport."
	instLineRemovedOut = "%s takes you out of their instance of %s: you may no longer go in."
	instLineSentOut    = "You are sent out of %s's instance of %s."
	instLineNotYours   = "Only your party's leader, its raid's helper, or the one who opened it may add to or take from an instance."
	instLineWhich      = "Your party has several dungeons open: name one, as /addinstance %s <dungeon> (%s)."
	instLineNone       = "Your party has no dungeon open. Its leader opens one at a Dungeon Warden."
	instLineNoOne      = "No player of that name is in the world."
	instLineOwner      = "The one who opened the instance cannot be taken out of it."
)

// dungeonCapLocked is how many players the instance holds (the file's doc). Must be called with the lock held.
func (w *World) dungeonCapLocked(in *dungeonInst) int {
	w.dungeonFollowOwnerLocked(in)
	if g := w.groups[in.group]; in.group != 0 && raidingLocked(g) {
		return w.dungeons.set.RaidCap
	}
	if c := w.dungeons.defs[in.def].Cap; c > 0 {
		return c
	}
	return w.dungeons.set.Cap
}

// dungeonPartyLocked is the characters the instance holds now: its owner, its group's players (a merc is none) and
// those added, never one taken out. Must be called with the lock held.
func (w *World) dungeonPartyLocked(in *dungeonInst) map[uint32]bool {
	w.dungeonFollowOwnerLocked(in)
	out := map[uint32]bool{in.owner: true}
	if g := w.groups[in.group]; in.group != 0 && g != nil {
		for _, id := range g.Members {
			if n := w.NPCs[id]; n != nil {
				continue // a merc
			}
			char := uint32(id) // an offline member's id is its character's
			if p := w.Players[id]; p != nil {
				char = p.CharServerID
			}
			out[char] = true
		}
	}
	for c := range in.invited {
		out[c] = true
	}
	for c := range in.removed {
		if c != in.owner {
			delete(out, c)
		}
	}
	return out
}

// dungeonManagesLocked is whether p may add to or take from the instance: its owner, its group's leader, or its raid's
// helper. Must be called with the lock held.
func (w *World) dungeonManagesLocked(in *dungeonInst, p *Player) bool {
	w.dungeonFollowOwnerLocked(in)
	if p.CharServerID == in.owner {
		return true
	}
	g := w.groups[in.group]
	if in.group == 0 || g == nil || p.GroupID != in.group || len(g.Members) == 0 {
		return false
	}
	return g.Members[0] == p.ServerID || raidingLocked(g) && g.rules.helper == p.ServerID
}

// dungeonManagedLocked is the instance p's command is for: the one it stands in, else its party's of the dungeon named
// (key or name), else its party's only one; "" and a line when none or several. Must be called with the lock held.
func (w *World) dungeonManagedLocked(p *Player, dungeon, target string) (*dungeonInst, string) {
	if w.inDungeonsLocked(p) {
		if in := w.dungeons.insts[p.Instance]; in != nil && w.dungeonAllowsLocked(in, p) {
			return in, ""
		}
	}
	var open []*dungeonInst
	var keys []string
	for i, d := range w.dungeons.defs {
		in := w.dungeonInstForLocked(i, p)
		if in == nil {
			continue
		}
		if dungeon != "" && (strings.EqualFold(d.Key, dungeon) || strings.EqualFold(d.Name, dungeon)) {
			return in, ""
		}
		open = append(open, in)
		keys = append(keys, d.Key)
	}
	switch {
	case dungeon != "" || len(open) == 0:
		return nil, instLineNone
	case len(open) > 1:
		if target == "" {
			target = "<name>"
		}
		return nil, fmt.Sprintf(instLineWhich, target, strings.Join(keys, ", "))
	}
	return open[0], ""
}

// dungeonMemberCommandLocked is the instance and the player a /addinstance or /removeinstance names (by name, else by
// the id its client knows its target by), or a line. Must be called with the lock held.
func (w *World) dungeonMemberCommandLocked(sess *session.Session, name string, targetID uint32) (p, t *Player, in *dungeonInst, line string) {
	p = w.sessionPlayerLocked(sess)
	if p == nil || !w.dungeons.on {
		return nil, nil, nil, "The dungeons are closed on this server."
	}
	name, dungeon, _ := strings.Cut(strings.TrimSpace(name), " ")
	dungeon = strings.TrimSpace(dungeon)
	if name != "" {
		t = w.inWorldPlayerLocked(name)
	} else {
		t = w.playerSeenByLocked(sess, targetID)
	}
	in, line = w.dungeonManagedLocked(p, dungeon, name)
	if in == nil {
		return p, t, nil, line
	}
	if !w.dungeonManagesLocked(in, p) {
		return p, t, nil, instLineNotYours
	}
	return p, t, in, ""
}

// DungeonAddMember is /addinstance [name] [dungeon]: the player named (else the sender's target) added to its party's
// instance (the file's doc). It reports a line for the sender.
func (w *World) DungeonAddMember(sess *session.Session, name string, targetID uint32) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, t, in, line := w.dungeonMemberCommandLocked(sess, name, targetID)
	if in == nil {
		return line
	}
	if t == nil {
		return instLineNoOne
	}
	d := &w.dungeons.defs[in.def]
	party := w.dungeonPartyLocked(in)
	if party[t.CharServerID] {
		return fmt.Sprintf(instLineThere, t.Name, d.Name)
	}
	if cap := w.dungeonCapLocked(in); len(party) >= cap {
		return fmt.Sprintf(instLineFull, d.Name, cap)
	}
	delete(in.removed, t.CharServerID)
	inGroup := in.group != 0 && t.GroupID == in.group
	if !inGroup {
		if in.invited == nil {
			in.invited = map[uint32]string{}
		}
		in.invited[t.CharServerID] = t.Name
	}
	n, cap := len(w.dungeonPartyLocked(in)), w.dungeonCapLocked(in)
	if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil && t != p {
		w.sendColoredLocked(ts, fmt.Sprintf(instLineAddedYou, p.Name, d.Name, d.Key), dungeonColour)
	}
	log.Printf("Dungeons: %s adds %s to instance %d of %s (%s's): %d of %d", p.Name, t.Name, in.id, d.Name, in.ownerName, n, cap)
	return fmt.Sprintf(instLineAdded, t.Name, d.Name, n, cap)
}

// DungeonRemoveMember is /removeinstance [name] [dungeon]: the player named (else the sender's target, else one added
// of that name out of the world) taken out of its party's instance (the file's doc). It reports a line for the sender.
func (w *World) DungeonRemoveMember(sess *session.Session, name string, targetID uint32, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, t, in, line := w.dungeonMemberCommandLocked(sess, name, targetID)
	if in == nil {
		return line
	}
	d := &w.dungeons.defs[in.def]
	var char uint32
	who := ""
	if t != nil {
		char, who = t.CharServerID, t.Name
	} else if n, _, _ := strings.Cut(strings.TrimSpace(name), " "); n != "" {
		for c, nm := range in.invited { // one added, logged out
			if strings.EqualFold(nm, n) {
				char, who = c, nm
			}
		}
	}
	if char == 0 {
		return instLineNoOne
	}
	if char == in.owner {
		return instLineOwner
	}
	if !w.dungeonPartyLocked(in)[char] {
		return fmt.Sprintf("%s may not stand in your party's instance of %s.", who, d.Name)
	}
	delete(in.invited, char)
	if in.removed == nil {
		in.removed = map[uint32]time.Time{}
	}
	in.removed[char] = now
	if t != nil {
		if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
			if w.inDungeonsLocked(t) && t.Instance == in.id {
				w.sendColoredLocked(ts, fmt.Sprintf(instLineRemovedIn, p.Name, d.Name, int(DungeonRemoveWarning/time.Second)), dungeonColour)
			} else {
				w.sendColoredLocked(ts, fmt.Sprintf(instLineRemovedOut, p.Name, d.Name), dungeonColour)
			}
		}
	}
	log.Printf("Dungeons: %s takes %s out of instance %d of %s (%s's)", p.Name, who, in.id, d.Name, in.ownerName)
	return fmt.Sprintf(instLineRemoved, who, d.Name)
}

// dungeonRemovedTurnLocked is dungeonsSystem's look at p, standing in an instance it may not stand in: one that took
// it out waits out its warning, then sends it to Freeport (in front of the Dungeon Warden outside). It reports whether
// it was such a one (false: the usual send-out). Must be called with the lock held.
func (w *World) dungeonRemovedTurnLocked(in *dungeonInst, p *Player, now time.Time) bool {
	at, out := in.removed[p.CharServerID]
	if !out {
		return false
	}
	if now.Sub(at) < DungeonRemoveWarning {
		return true
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	to, facing, ok := w.dungeonWardenSpotLocked()
	if sess == nil || !ok {
		return true
	}
	w.dungeons.movedAt[p.ServerID] = now
	delete(w.dungeons.badAt, p.ServerID)
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: "the way out of the dungeon",
		Facing: facing, HasFacing: true})
	w.sendColoredLocked(sess, fmt.Sprintf(instLineSentOut, in.ownerName, w.dungeons.defs[in.def].Name), dungeonColour)
	log.Printf("Dungeons: %s, taken out of instance %d, goes to %s (%.1f, %.1f, %.1f) world=%d", p.Name, in.id, to.Place,
		to.X, to.Y, to.Z, to.World)
	return true
}

// dungeonTookOutLocked is whether the instance id took the character out (its kills' seats leave it out:
// raidrules.go). Must be called with the lock held.
func (w *World) dungeonTookOutLocked(inst, char uint32) bool {
	if inst == 0 || !w.dungeons.on {
		return false
	}
	in := w.dungeons.insts[inst]
	if in == nil {
		return false
	}
	_, out := in.removed[char]
	return out
}

// DungeonMembers is (for /dungeon and tests) the instance's added and taken-out characters' names, each in order.
func (w *World) DungeonMembers(inst uint32) (added, out []string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	in := w.dungeons.insts[inst]
	if in == nil {
		return nil, nil
	}
	for _, n := range in.invited {
		added = append(added, n)
	}
	for c := range in.removed {
		n := fmt.Sprint(c)
		if p := w.playerByCharLocked(c); p != nil {
			n = p.Name
		}
		out = append(out, n)
	}
	sort.Strings(added)
	sort.Strings(out)
	return added, out
}
