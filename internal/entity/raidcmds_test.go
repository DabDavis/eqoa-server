package entity

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The raid's own rules and commands (raidrules.go, raidcmds.go; the GM Jibrish's
// petitions 1448, 1451, 1452, 1453): each beside its control.

// boxFrom is the name the last 0x0773 queued to session i's client named, drained; ""
// for none.
func (r *raidRig) boxFrom(i int) string {
	r.t.Helper()
	name := ""
	for _, m := range drain(r.sess[i]) {
		if opOfMsg(m) == protocol.OpcodeGroupInviteReceived && len(m) >= 14 {
			n := int(binary.LittleEndian.Uint32(m[10:]))
			name = string(m[14 : 14+n])
		}
	}
	return name
}

// accept is member i's yes to the box naming whom.
func (r *raidRig) accept(i int, whom *Player) {
	r.w.GroupAccept(r.sess[i], r.w.IDSeenBy(r.sess[i], whom))
}

// raidOf is player i's group.
func (r *raidRig) raidOf(i int) *Group { return r.w.groups[r.p[i].GroupID] }

// fight puts a creature on the World with player i on its hate list (a fight's
// beginning), at the group's place.
func (r *raidRig) fight(id EntityID, i int) *NPC {
	npc := &NPC{ServerID: id, Name: "a raid troll", Level: 50, HP: 1000, MaxHP: 1000, X: 1000, Y: 10, Z: 1000, Size: 1}
	r.w.mu.Lock()
	r.w.putNPCLocked(npc)
	if i >= 0 {
		r.w.addThreatLocked(npc, r.p[i].ServerID, 10)
	}
	r.w.mu.Unlock()
	return npc
}

// saidPart is whether a line holds part.
func saidPart(lines []string, part string) bool {
	for _, l := range lines {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// /raidinvite asks the player named, else the target, in the flag's box, and the yes
// joins past four; a member who neither leads nor helps is refused (the control).
func TestRaidInviteByNameOrTargetPastFour(t *testing.T) {
	r := newRaidRig(t, 7, 30)
	r.form(5) // a raid of five
	r.drainAll()
	r.w.RaidInvite(r.sess[0], "fay", 0) // any case
	msgs := drain(r.sess[5])
	if l := linesOf(msgs); !saidPart(l, "Ann invites you to Ann's raid. Past 4 members") {
		t.Fatalf("Fay's line: %q", l)
	}
	box := ""
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeGroupInviteReceived && len(m) >= 14 {
			n := int(binary.LittleEndian.Uint32(m[10:]))
			box = string(m[14 : 14+n])
		}
	}
	if box != "Ann's raid" {
		t.Fatalf("Fay's box: %q, want the raid's", box)
	}
	r.accept(5, r.p[0])
	if len(r.raidOf(0).Members) != 6 || r.p[5].GroupID != r.p[0].GroupID {
		t.Fatalf("Fay is in group %d, the raid has %d", r.p[5].GroupID, len(r.raidOf(0).Members))
	}
	// by the target, with no name
	r.w.RaidInvite(r.sess[0], "", r.w.IDSeenBy(r.sess[0], r.p[6]))
	if got := r.boxFrom(6); got != "Ann's raid" {
		t.Fatalf("Gus's box by target: %q", got)
	}
	// the control: a member with no role
	r.drainAll()
	r.w.RaidInvite(r.sess[1], "gus", 0)
	if !saidPart(r.heard(1), "Only your raid's leader or its helper") {
		t.Fatal("a plain member's raid invite was not refused")
	}
	if got := r.boxFrom(6); got != "" {
		t.Fatalf("a plain member's invite reached Gus: %q", got)
	}
}

// A raid invite from one in no group asks the caller first in its own box, and its yes
// makes it a flagged group of one and asks the one named; /invite <name> does the same
// with a plain group's invite, which stops at four (the control: no raid).
func TestRaidInviteAndInviteFromNoGroupAskTheCallerFirst(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	r.w.RaidInvite(r.sess[0], "Bob", 0)
	if got := r.boxFrom(0); got != raidOwnBoxWords {
		t.Fatalf("Ann's own box: %q", got)
	}
	if got := r.boxFrom(1); got != "" {
		t.Fatalf("Bob was asked before Ann had a group: %q", got)
	}
	r.accept(0, r.p[0])
	if g := r.raidOf(0); g == nil || !g.flagged {
		t.Fatalf("Ann's yes: group %v, want one flagged", g)
	}
	if got := r.boxFrom(1); got != "Ann's raid" {
		t.Fatalf("Bob's box: %q", got)
	}
	// /invite: a plain group's
	r.w.InviteByName(r.sess[2], "bob")
	if got := r.boxFrom(2); got != raidOwnInviteWords {
		t.Fatalf("Cal's own box: %q", got)
	}
	r.accept(2, r.p[2])
	g := r.raidOf(2)
	if g == nil || g.flagged {
		t.Fatalf("/invite made group %+v, want a plain one", g)
	}
	if got := r.boxFrom(1); got != "Cal" {
		t.Fatalf("Bob's box from /invite: %q, want Cal's own name as a group invite gives it", got)
	}
	// a name no one has
	r.w.InviteByName(r.sess[2], "Zed")
	if !saidPart(r.heard(2), "No player named Zed") {
		t.Fatal("an unknown name was not said")
	}
}

// /raidleave takes any member out with 0x069A 8 (CLIENT's case with no word) and a
// line; the leader's hands the lead on, told; a plain member's tells no new leader (the
// control).
func TestRaidLeaveTellsTheClientAndHandsTheLeadOn(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.raidOf(0).flagged = true // a flag's raid stays one at four
	r.drainAll()
	r.w.RaidLeave(r.sess[3])
	msgs := drain(r.sess[3])
	gone := false
	for _, m := range msgs {
		gone = gone || opOfMsg(m) == protocol.OpcodeRemoveGroupMember && len(m) == 3 && m[2] == raidLeaveSub
	}
	if !gone || !saidPart(linesOf(msgs), "You left the raid.") || r.p[3].GroupID != 0 {
		t.Fatalf("Dee's leave: 0x069A 8 %v, lines %q, group %d", gone, linesOf(msgs), r.p[3].GroupID)
	}
	if saidPart(r.heard(1), "leads the raid now") {
		t.Fatal("a plain member's leave named a new leader (the control)")
	}
	r.w.RaidHelper(r.sess[0], "Bob", 0)
	r.w.RaidLeave(r.sess[0])
	if g := r.raidOf(1); g == nil || g.Members[0] != r.p[1].ServerID || g.rules.helper != 0 {
		t.Fatalf("after the leader left: %+v, want Bob leading, helping no one", g)
	}
	if !saidPart(r.heard(2), "Bob leads the raid now.") {
		t.Fatal("the raid was not told who leads")
	}
	// one in no group
	r.w.RaidLeave(r.sess[0])
	if !saidPart(r.heard(0), "You are not in a raid.") {
		t.Fatal("a leave out of a raid was not refused")
	}
}

// /raidleader hands the lead to a member, first in every roster; a member's own is
// refused and moves no one (the control); alone, it says the raid.
func TestRaidLeaderHandsOnTheLead(t *testing.T) {
	r := newRaidRig(t, 5, 30)
	r.form(5)
	r.drainAll()
	r.w.RaidLeader(r.sess[1], "Dee", 0)
	if g := r.raidOf(0); g.Members[0] != r.p[0].ServerID {
		t.Fatal("a member handed on the lead")
	}
	r.w.RaidLeader(r.sess[0], "cal", 0)
	g := r.raidOf(0)
	if g.Members[0] != r.p[2].ServerID || g.Members[1] != r.p[0].ServerID {
		t.Fatalf("members %v: want Cal, then Ann and the rest in their order", g.Members)
	}
	for i := range r.p {
		ids, _ := r.lastRoster(i)
		if len(ids) == 0 || ids[0] != r.w.IDSeenBy(r.sess[i], r.p[2]) {
			t.Fatalf("%s's roster %x: Cal is not member 0", r.p[i].Name, ids)
		}
	}
	r.w.RaidLeader(r.sess[4], "", 0)
	if l := r.heard(4); !saidPart(l, "leader Cal") || !saidPart(l, "loot free for all") {
		t.Fatalf("the raid as it stands: %q", l)
	}
}

// The leader's one helper invites and kicks, never the leader; without the role a
// member does neither (the control); a second helper takes the first's place.
func TestRaidHelperInvitesAndKicks(t *testing.T) {
	r := newRaidRig(t, 7, 30)
	r.form(5)
	r.drainAll()
	r.w.RaidKick(r.sess[1], "Cal", 0)
	if r.p[2].GroupID == 0 || !saidPart(r.heard(1), "Only your raid's leader or its helper") {
		t.Fatal("a member with no role kicked")
	}
	r.w.RaidHelper(r.sess[0], "bob", 0)
	if r.raidOf(0).rules.helper != r.p[1].ServerID {
		t.Fatal("Bob is not the helper")
	}
	r.drainAll()
	r.w.RaidInvite(r.sess[1], "Fay", 0)
	if got := r.boxFrom(5); got != "Ann's raid" {
		t.Fatalf("the helper's invite: %q", got)
	}
	r.w.RaidKick(r.sess[1], "Cal", 0)
	kicked := false
	for _, m := range drain(r.sess[2]) {
		kicked = kicked || opOfMsg(m) == protocol.OpcodeRemoveGroupMember && m[2] == 1
	}
	if !kicked || r.p[2].GroupID != 0 {
		t.Fatalf("the helper's kick: told %v, group %d", kicked, r.p[2].GroupID)
	}
	r.w.RaidKick(r.sess[1], "Ann", 0)
	if r.p[0].GroupID == 0 || !saidPart(r.heard(1), "cannot kick its leader") {
		t.Fatal("the helper kicked the leader")
	}
	// the CLIENT's own Boot Member by the helper, too
	r.w.GroupBoot(r.sess[1], r.w.IDSeenBy(r.sess[1], r.p[3]))
	if r.p[3].GroupID != 0 {
		t.Fatal("the helper's Boot Member did nothing")
	}
	r.w.RaidHelper(r.sess[0], "Eve", 0)
	if g := r.raidOf(0); g.rules.helper != r.p[4].ServerID {
		t.Fatal("a second helper did not take the place")
	}
	r.w.RaidKick(r.sess[1], "Eve", 0)
	if r.p[4].GroupID == 0 {
		t.Fatal("Bob, no longer the helper, kicked Eve")
	}
	if !saidPart(r.heard(1), "Only your raid's leader or its helper") {
		t.Fatal("Bob, no longer the helper, was not refused")
	}
}

// The loot rule is the leader's and does not change in a fight: a creature with a
// member on its hate list; one fighting someone else is no fight of the raid's (the
// control).
func TestRaidLootRuleNotInAFight(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(5)
	r.drainAll()
	r.w.RaidLootRule(r.sess[1], "needgreed")
	if r.raidOf(0).rules.loot != LootFreeForAll {
		t.Fatal("a member set the rule")
	}
	outsider := r.fight(9100, 5) // Fay, in no raid
	_ = outsider
	r.w.RaidLootRule(r.sess[0], "nbg")
	if r.raidOf(0).rules.loot != LootNeedGreed || !saidPart(r.heard(2), "sets the raid's loot rule: need before greed") {
		t.Fatal("the rule did not change out of the raid's fight")
	}
	r.fight(9101, 3) // Dee fights
	if !r.w.RaidInCombat(r.p[0]) {
		t.Fatal("the raid is not in a fight with a member on a hate list")
	}
	r.w.RaidLootRule(r.sess[0], "master")
	if r.raidOf(0).rules.loot != LootNeedGreed || !saidPart(r.heard(0), "in combat") {
		t.Fatal("the rule changed in a fight")
	}
	// a pet's fight is its master's
	r2 := newRaidRig(t, 5, 30)
	r2.form(5)
	pet := &NPC{ServerID: 9200, Name: "Bob's pet", Owner: r2.p[1].ServerID, Level: 30, HP: 10, MaxHP: 10, X: 1000, Z: 1000}
	r2.w.mu.Lock()
	r2.w.putNPCLocked(pet)
	r2.w.mu.Unlock()
	foe := r2.fight(9201, -1)
	r2.w.mu.Lock()
	r2.w.addThreatLocked(foe, pet.ServerID, 5)
	r2.w.mu.Unlock()
	if !r2.w.RaidInCombat(r2.p[0]) {
		t.Fatal("a member's pet's fight is not the raid's")
	}
}

// In a fight a raid is not disbanded: its leader's disband leaves it to the next, as its
// client has let its group go; out of one it is disbanded (the control). One that no
// longer leads (its client still thinks so) leaves too.
func TestRaidDisbandInAFightLeavesInstead(t *testing.T) {
	r := newRaidRig(t, 5, 30)
	r.form(5)
	r.raidOf(0).flagged = true
	foe := r.fight(9300, 2)
	r.w.GroupDisband(r.sess[0])
	if r.p[0].GroupID != 0 || r.p[1].GroupID == 0 || r.raidOf(1).Members[0] != r.p[1].ServerID {
		t.Fatalf("a disband in a fight: Ann in %d, Bob in %d", r.p[0].GroupID, r.p[1].GroupID)
	}
	if !saidPart(r.heard(0), "cannot be disbanded") {
		t.Fatal("the leader was not told")
	}
	r.w.mu.Lock()
	foe.Threats = nil // the fight is over
	r.w.mu.Unlock()
	r.w.RaidLeader(r.sess[1], "Cal", 0) // Bob hands on the lead
	r.w.GroupDisband(r.sess[1])         // his CLIENT still thinks he leads
	if r.p[1].GroupID != 0 || r.p[2].GroupID == 0 {
		t.Fatalf("one who no longer leads disbanded: Bob %d, Cal %d", r.p[1].GroupID, r.p[2].GroupID)
	}
	r.w.GroupDisband(r.sess[2])
	for i := 2; i < 5; i++ {
		if r.p[i].GroupID != 0 {
			t.Fatalf("the control: %s is still in group %d", r.p[i].Name, r.p[i].GroupID)
		}
	}
}

// A member kicked in a fight keeps a claim on what the raid fought: a seat at its kill
// wherever it stands; kicked out of a fight it has none (the control), nor when the
// creature left the fight alive before it died (swept).
func TestAKickInAFightKeepsAClaim(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.raidOf(0).flagged = true // raised by a flag, as a raid past four is: it stays one at four
	r.raidOf(0).rules.loot = LootNeedGreed
	r.w.RaidKick(r.sess[0], "Bob", 0) // out of a fight
	foe := r.fight(9400, 2)
	r.w.RaidKick(r.sess[0], "Dee", 0) // in it
	if !saidPart(r.heard(3), "you keep your claim") {
		t.Fatal("Dee was not told of her claim")
	}
	r.p[3].X = 9000 // far off now
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	r.w.mu.Lock()
	r.w.killNPC(foe)
	r.w.creditKillLocked(r.sess[0], r.p[0], foe)
	r.w.mu.Unlock()
	if len(heard) != 1 || heard[0].Raid == nil {
		t.Fatalf("heard %+v", heard)
	}
	seats := map[string]bool{}
	for _, s := range heard[0].Raid.Seats {
		seats[s.Name] = s.Claim
	}
	if claim, ok := seats["Dee"]; !ok || !claim {
		t.Fatalf("seats %v: Dee's claim missing", seats)
	}
	if _, ok := seats["Bob"]; ok {
		t.Fatal("Bob, kicked out of a fight, has a seat")
	}
	if len(seats) != 5 { // Ann, Cal, Eve, Fay and Dee
		t.Fatalf("seats %v", seats)
	}
	// a claim on a creature that left the fight alive is swept
	foe2 := r.fight(9401, 2)
	r.w.RaidKick(r.sess[0], "Eve", 0)
	r.w.mu.Lock()
	if len(r.w.raids.claims[foe2.ServerID]) != 1 {
		r.w.mu.Unlock()
		t.Fatal("no claim on the second creature")
	}
	foe2.Threats = nil
	r.w.sweepRaidClaimsLocked()
	gone := len(r.w.raids.claims[foe2.ServerID]) == 0
	r.w.mu.Unlock()
	if !gone {
		t.Fatal("a claim outlived its fight")
	}
}

// A raid's kill tells its seats (members within KillShareRange, alive or dead), every
// member, and nothing under free for all or out of a raid (the controls).
func TestARaidsKillTellsItsSeats(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.p[4].X = 1000 + KillShareRange + 1 // Eve beyond the reach
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	kill := func(id EntityID) *RaidKill {
		npc := r.fight(id, 0)
		r.w.mu.Lock()
		r.w.killNPC(npc)
		r.w.creditKillLocked(r.sess[0], r.p[0], npc)
		r.w.mu.Unlock()
		return heard[len(heard)-1].Raid
	}
	if k := kill(9500); k != nil {
		t.Fatal("a free for all kill carries the raid")
	}
	r.raidOf(0).rules.loot = LootMaster
	k := kill(9501)
	if k == nil || k.Rule != LootMaster || k.Leader.Name != "Ann" || len(k.Members) != 6 || len(k.Seats) != 5 {
		t.Fatalf("the kill: %+v", k)
	}
	for _, s := range k.Seats {
		if s.Name == "Eve" {
			t.Fatal("Eve beyond the reach has a seat")
		}
	}
}

// The raid's state (0x7E44) goes to a client that answered, in its own ids, the flags
// its own; a client that did not answer gets none (the control); out of a raid, zeros.
func TestTheRaidStateGoesToAnsweredClients(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(5)
	r.w.SetRaidWindows(r.sess[1], true)
	r.w.RaidHelper(r.sess[0], "Bob", 0)
	r.w.RaidAssistSet(r.sess[0], "Cal", 0)
	r.w.RaidLootRule(r.sess[0], "master") // each sent at once, the last the whole state
	state := func(i int) []byte {
		var got []byte
		for _, m := range drain(r.sess[i]) {
			if opOfMsg(m) == protocol.OpcodeRaidState {
				got = m[2:]
			}
		}
		return got
	}
	b := state(1)
	if len(b) != 14 {
		t.Fatalf("Bob's state % x", b)
	}
	le := binary.LittleEndian
	if le.Uint32(b) != r.w.IDSeenBy(r.sess[1], r.p[0]) || le.Uint32(b[4:]) != r.sess[1].PlayerObjectID() ||
		le.Uint32(b[8:]) != r.w.IDSeenBy(r.sess[1], r.p[2]) || b[12] != byte(LootMaster) || b[13] != 4 {
		t.Fatalf("Bob's state % x", b)
	}
	if got := state(2); got != nil {
		t.Fatalf("Cal, who never answered, was sent % x", got)
	}
	// no change, nothing again; out of the raid, zeros once
	r.w.mu.Lock()
	r.w.TickCount = raidRosterTicks
	r.w.raidStateSystem()
	r.w.mu.Unlock()
	if got := state(1); got != nil {
		t.Fatal("an unchanged state was sent again")
	}
	if got := state(2); got != nil {
		t.Fatalf("the pass sent Cal, who never answered, % x", got)
	}
	r.w.RaidLeave(r.sess[1])
	if got := state(1); len(got) != 14 || le.Uint32(got) != 0 {
		t.Fatalf("out of the raid: % x", got)
	}
}

// /raidassist alone targets what the main assist targets, as the asker knows it; with
// no main assist it is refused (the control).
func TestRaidAssistTargetsTheMainAssistsTarget(t *testing.T) {
	r := newRaidRig(t, 5, 30)
	r.form(5)
	if _, ok := r.w.RaidAssist(r.sess[3]); ok || !saidPart(r.heard(3), "no main assist") {
		t.Fatal("an assist with no main assist was not refused")
	}
	foe := r.fight(9600, -1)
	for _, i := range []int{1, 3} {
		vs := GetOrCreateVisibility(r.sess[i])
		vs.ChannelEntity[0], vs.ChannelObjectID[0] = foe.ServerID, 0x00300000+uint32(i)
	}
	r.p[1].TargetObjectID = 0x00300001 // Bob's own id for it
	r.w.RaidAssistSet(r.sess[0], "Bob", 0)
	id, ok := r.w.RaidAssist(r.sess[3])
	if !ok || id != 0x00300003 {
		t.Fatalf("Dee's assist: %#x %v, want her own id for the troll", id, ok)
	}
}

// /target <name> takes the nearest the client shows within 100 whose name is it, else
// starts with it; an exact name beats a nearer start; past 100 nothing (the control).
func TestTargetNearByNameAndStart(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	vs := GetOrCreateVisibility(r.sess[0])
	put := func(slot int, id EntityID, name string, x float32) {
		r.w.mu.Lock()
		r.w.putNPCLocked(&NPC{ServerID: id, Name: name, Level: 5, HP: 5, MaxHP: 5, X: x, Y: 10, Z: 1000})
		r.w.mu.Unlock()
		vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = id, 0x00400000+uint32(id)
	}
	put(0, 7001, "Jibrish", 1090)      // 90 off
	put(1, 7002, "Jibber", 1020)       // 20 off
	put(2, 7003, "a moss snake", 1010) // 10 off
	put(3, 7004, "Jib", 1150)          // 150 off: out of reach
	cases := []struct {
		name string
		want EntityID
	}{
		{"jib", 7002},       // the exact Jib is out of reach: the nearest start
		{"JIBRISH", 7001},   // exact, any case
		{"moss", 7003},      // the article left off
		{"a moss", 7003},    // or not
		{"jibri", 7001},     // a start
		{"snake", 0},        // a word inside is no start (the control)
		{"Bob", 0x7F000000}, // a player in view by its name: below
	}
	vs.ChannelEntity[4], vs.ChannelObjectID[4] = r.p[1].ServerID, 0x00405000
	for _, c := range cases {
		id, ok := r.w.TargetNear(r.sess[0], c.name)
		switch {
		case c.want == 0x7F000000:
			if !ok || id != 0x00405000 {
				t.Errorf("%q: %#x %v, want Bob", c.name, id, ok)
			}
		case c.want == 0:
			if ok {
				t.Errorf("%q found %#x", c.name, id)
			}
		case !ok || id != 0x00400000+uint32(c.want):
			t.Errorf("%q: %#x %v, want %d", c.name, id, ok, c.want)
		}
	}
	// an exact name beats a nearer start
	put(5, 7005, "Jibbering", 1005)
	if id, _ := r.w.TargetNear(r.sess[0], "jibber"); id != 0x00400000+7002 {
		t.Errorf("exact Jibber lost to a start: %#x", id)
	}
	// itself, by its whole name alone
	if id, ok := r.w.TargetNear(r.sess[0], "Ann"); !ok || id != r.sess[0].PlayerObjectID() {
		t.Errorf("itself: %#x %v", id, ok)
	}
}

// A keep restart forms the raid again with its flag, its rule, its helper and main
// assist; a plain group comes back plain (the control).
func TestARaidsRulesAreKeptOverARestart(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(5)
	r.raidOf(0).flagged = true
	r.w.RaidHelper(r.sess[0], "Bob", 0)
	r.w.RaidAssistSet(r.sess[0], "Cal", 0)
	r.w.RaidLootRule(r.sess[0], "needgreed")
	b, err := json.Marshal(r.w.KeepGroups())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptGroup
	if err := json.Unmarshal(b, &kept); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.p {
		p.GroupID = 0
	}
	r.w.groups = nil
	if n, err := r.w.RestoreGroups(kept); n != 1 || err != nil {
		t.Fatalf("restored %d: %v", n, err)
	}
	g := r.raidOf(0)
	if !g.flagged || g.rules.loot != LootNeedGreed || g.rules.helper != r.p[1].ServerID || g.rules.assist != r.p[2].ServerID {
		t.Fatalf("restored %+v", g)
	}
	// a plain group's keeps nothing of the kind
	r2 := newRaidRig(t, 3, 30)
	r2.form(3)
	b, _ = json.Marshal(r2.w.KeepGroups())
	if strings.Contains(string(b), "loot") || strings.Contains(string(b), "flagged") {
		t.Fatalf("a plain group kept %s", b)
	}
}

// A corpse a raid's loot holds lies past its decay until the hold ends, then goes; a
// respawn forgets the hold; one not held decays as ever (the control).
func TestAHeldCorpseLiesUntilItsHoldEnds(t *testing.T) {
	w := NewWorld()
	w.SetCorpseTimes(time.Minute, time.Second)
	w.SetCorpseLinger(-1) // the creature's own corpse time, no linger (corpselinger.go)
	npc := &NPC{ServerID: 9700, Name: "Lady Vox", Level: 55, HP: 1, MaxHP: 1, X: 1, Z: 1, RespawnTime: time.Hour}
	w.mu.Lock()
	w.putNPCLocked(npc)
	w.killNPC(npc)
	died := npc.DeathTime
	w.mu.Unlock()
	if !w.HoldCorpse(npc.ServerID, died, died.Add(10*time.Minute)) {
		t.Fatal("not held")
	}
	if w.HoldCorpse(npc.ServerID, died.Add(time.Second), died.Add(time.Hour)) {
		t.Fatal("another death's hold taken")
	}
	w.mu.Lock()
	gone := w.corpseSystem(died.Add(5 * time.Minute))
	w.mu.Unlock()
	if len(gone) != 0 {
		t.Fatal("the held corpse decayed at its decay")
	}
	w.mu.Lock()
	gone = w.corpseSystem(died.Add(10*time.Minute + time.Second))
	ends := npc.CorpseGone
	w.mu.Unlock()
	if len(gone) != 1 || !ends.Equal(died.Add(10*time.Minute)) {
		t.Fatalf("at the hold's end: gone %v at %v", gone, ends)
	}
	w.mu.Lock()
	w.respawnNPCLocked(npc)
	held := npc.corpseHeld
	w.killNPC(npc)
	w.mu.Unlock()
	if !held.IsZero() {
		t.Fatal("a respawn kept the hold")
	}
	w.mu.Lock()
	gone = w.corpseSystem(npc.DeathTime.Add(time.Minute + time.Second))
	w.mu.Unlock()
	if len(gone) != 1 {
		t.Fatal("the control: an unheld corpse outlived its decay")
	}
}

var _ = session.NewManager

// A Metal Moogie killed by a raid is no raid's kill: his corpse (its Moogie
// Tribute) is his challenger's alone, never rolled (the owner, 2026-11-13).
// The control: another creature of the same raid's fight is the raid's.
func TestAMetalMoogieIsNoRaidsKill(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.raidOf(0).flagged = true
	r.raidOf(0).rules.loot = LootNeedGreed
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	foe := r.fight(9400, 2)
	foe.Name = MetalMoogieName
	r.w.mu.Lock()
	r.w.killNPC(foe)
	r.w.creditKillLocked(r.sess[0], r.p[0], foe)
	r.w.mu.Unlock()
	if len(heard) != 1 || heard[0].Raid != nil {
		t.Fatalf("a Metal Moogie's kill: %+v", heard)
	}
	other := r.fight(9401, 2)
	r.w.mu.Lock()
	r.w.killNPC(other)
	r.w.creditKillLocked(r.sess[0], r.p[0], other)
	r.w.mu.Unlock()
	if len(heard) != 2 || heard[1].Raid == nil {
		t.Fatalf("the control: %+v", heard)
	}
}
