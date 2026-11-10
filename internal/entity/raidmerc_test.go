package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A raid and the mercs in it (mercgroup.go, raidrules.go, raidcmds.go): a merc takes a
// place in its owner's group but is no player, so it is never a roll seat, on the master
// looter's list, the leader, the helper or the main assist; kicked from a raid it is
// dismissed as Boot Member dismisses it, and holds no claim; its fight is the raid's; a
// player in a group of it and its merc alone steps aside for a raid invite. Each rule
// beside its control.

// mercRaid is the rig's owner with its merc, in a raid flagged of the owner (its leader),
// the merc and the players given.
func mercRaid(t *testing.T, others ...string) (*mercRig, *mercState, *Group, []*session.Session, []*Player) {
	t.Helper()
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0), MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID()) // the box of a group with its merc
	var ss []*session.Session
	var ps []*Player
	for _, n := range others {
		s, p := r.player(n, 0, 0)
		ss, ps = append(ss, s), append(ps, p)
	}
	g := r.group(ps...)
	g.flagged = true
	if g.Members[0] != r.p.ServerID || !groupHas(g, ms.npc.ServerID) {
		t.Fatalf("the raid %v: the owner leading, its merc in it", g.Members)
	}
	return r, ms, g, ss, ps
}

func ownerLines(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().GetPending() {
		if op := opOfMsg(m.Data()); op == protocol.OpcodeClientMessage || op == protocol.OpcodeColoredChat {
			out = append(out, chatText(m.Data()))
		}
	}
	return out
}

// A raid kill's seats and its master looter's list are its players: the merc in it is
// neither (the control: its owner and the other player are both).
func TestARaidsMercIsNoSeat(t *testing.T) {
	r, ms, g, _, ps := mercRaid(t, "Other")
	g.rules.loot = LootNeedGreed
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	npc := r.creature("a gnoll", 20, 2)
	r.w.mu.Lock()
	r.w.killNPC(npc)
	r.w.creditKillLocked(r.sess, r.p, npc)
	r.w.mu.Unlock()
	if len(heard) != 1 || heard[0].Raid == nil {
		t.Fatalf("heard %+v", heard)
	}
	k := heard[0].Raid
	names := func(ss []RaidSeat) string {
		var out []string
		for _, s := range ss {
			out = append(out, s.Name)
			if s.Entity == ms.npc.ServerID {
				t.Errorf("the merc %q is a seat", s.Name)
			}
		}
		return strings.Join(out, ",")
	}
	if got := names(k.Seats); got != "Owner,"+ps[0].Name {
		t.Fatalf("seats %s", got)
	}
	if got := names(k.Members); got != "Owner,"+ps[0].Name {
		t.Fatalf("members %s", got)
	}
}

// A merc is never the raid's leader, helper or main assist, by name or by target; a
// player is (the control).
func TestAMercLeadsHelpsAndAssistsNoRaid(t *testing.T) {
	r, ms, g, _, ps := mercRaid(t, "Other")
	name := ms.npc.Name
	r.sent()
	r.w.RaidHelper(r.sess, name, 0)
	r.w.RaidAssistSet(r.sess, name, 0)
	r.w.RaidLeader(r.sess, name, 0)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[3], vs.ChannelObjectID[3] = ms.npc.ServerID, 0x00B00001
	r.w.RaidHelper(r.sess, "", 0x00B00001)
	lines := ownerLines(r.sess)
	for _, want := range []string{"A mercenary cannot be the raid's helper", "A mercenary cannot be the raid's main assist",
		"A mercenary cannot lead the raid"} {
		if !saidPart(lines, want) {
			t.Errorf("no %q in %q", want, lines)
		}
	}
	if g.rules.helper != 0 || g.rules.assist != 0 || g.Members[0] != r.p.ServerID {
		t.Fatalf("a merc took a role: %+v, leader %d", g.rules, g.Members[0])
	}
	r.w.RaidHelper(r.sess, ps[0].Name, 0)
	if g.rules.helper != ps[0].ServerID {
		t.Fatal("the control: a player is no helper")
	}
}

// /raidkick of a merc dismisses it, as Boot Member does, and it holds no claim in a fight;
// the helper may dismiss another's merc but never the leader's; a player kicked in the
// fight holds one (the control).
func TestARaidKickOfAMercDismissesIt(t *testing.T) {
	r, ms, g, ss, ps := mercRaid(t, "Helper", "Third")
	foe := r.creature("a raid troll", 40, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(foe, ps[1].ServerID, 10) // the raid in a fight
	r.w.mu.Unlock()
	g.rules.helper = ps[0].ServerID
	r.w.RaidKick(ss[0], ms.npc.Name, 0) // the helper, the leader's merc
	if r.w.mercs[r.p.ServerID] == nil || !groupHas(g, ms.npc.ServerID) {
		t.Fatal("the helper dismissed the leader's merc")
	}
	r.w.RaidKick(r.sess, ms.npc.Name, 0) // the leader
	if r.w.mercs[r.p.ServerID] != nil || groupHas(g, ms.npc.ServerID) {
		t.Fatal("the leader's /raidkick of a merc did not dismiss it")
	}
	r.w.mu.Lock()
	claims := len(r.w.raids.claims[foe.ServerID])
	r.w.mu.Unlock()
	if claims != 0 {
		t.Fatalf("a merc kicked in a fight holds %d claims", claims)
	}
	r.w.RaidKick(r.sess, ps[1].Name, 0)
	r.w.mu.Lock()
	claims = len(r.w.raids.claims[foe.ServerID])
	r.w.mu.Unlock()
	if claims != 1 {
		t.Fatalf("the control: a player kicked in the fight holds %d claims", claims)
	}
}

// A merc's fight is its raid's: a creature with only the merc on its hate list puts the
// raid in combat; another player's merc does not (the control).
func TestAMercsFightIsItsRaids(t *testing.T) {
	r, ms, _, _, _ := mercRaid(t, "Other")
	foe := r.creature("a raid troll", 40, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(foe, ms.npc.ServerID, 10)
	r.w.mu.Unlock()
	if !r.w.RaidInCombat(r.p) {
		t.Fatal("the merc's fight is not its raid's")
	}
	r2 := newMercRig(t)
	ms2 := r2.hire(r2.kit(9, 0, 0), MercAssist)
	_, other := r2.player("Raider", 0, 0)
	r2.w.mu.Lock()
	g := r2.w.formGroupLocked(other)
	g.flagged = true
	foe2 := &NPC{ServerID: 7777, Name: "a troll", Level: 40, HP: 9, MaxHP: 9, X: other.X, Z: other.Z}
	r2.w.putNPCLocked(foe2)
	r2.w.addThreatLocked(foe2, ms2.npc.ServerID, 10)
	r2.w.mu.Unlock()
	if r2.w.RaidInCombat(other) {
		t.Fatal("the control: a merc of no member put the raid in combat")
	}
}

// /raidinvite reaches a player in a group of it and its merc alone: that group gives way
// (0x069A 8, then the raid's box), and its yes brings it and its merc into the raid past
// four; a player in a group of others is refused (the control). /invite <name> likewise.
func TestARaidInviteReachesAMercOnlyGroup(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0), MercAssist)
	r.w.GroupAccept(r.sess, r.sess.PlayerObjectID()) // the owner and its merc, a group
	own := r.p.GroupID
	lsess, leader := r.player("Leader", 0, 0)
	var three []*Player
	for _, n := range []string{"Two", "Three", "Four"} { // four players already
		_, p := r.player(n, 0, 0)
		three = append(three, p)
	}
	r.w.mu.Lock()
	g := r.w.formGroupLocked(leader)
	g.flagged = true
	for _, p := range three {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
	r.w.mu.Unlock()
	r.sent()
	r.w.RaidInvite(lsess, "owner", 0)
	var ops []protocol.GameOpcode
	for _, m := range r.sess.Queue().GetPending() {
		ops = append(ops, opOfMsg(m.Data()))
		if opOfMsg(m.Data()) == protocol.OpcodeRemoveGroupMember && m.Data()[2] != 8 {
			t.Errorf("the owner's client was told %d, want 8", m.Data()[2])
		}
	}
	if r.p.GroupID != 0 || r.w.groups[own] != nil || len(ops) < 2 || ops[0] != protocol.OpcodeRemoveGroupMember ||
		ops[1] != protocol.OpcodeGroupInviteReceived {
		t.Fatalf("no step-aside: group %d, sent %04X", r.p.GroupID, ops)
	}
	r.w.GroupAccept(r.sess, r.w.IDSeenBy(r.sess, leader))
	if r.p.GroupID != g.ID || !groupHas(g, ms.npc.ServerID) || len(g.Members) != 6 {
		t.Fatalf("after the yes: %v, the owner in %d", g.Members, r.p.GroupID)
	}
	// the control: one in a group of others
	osess, other := r.player("Busy", 0, 0)
	_, mate := r.player("Mate", 0, 0)
	r.w.mu.Lock()
	og := r.w.formGroupLocked(other)
	og.Members = append(og.Members, mate.ServerID)
	mate.GroupID = og.ID
	r.w.mu.Unlock()
	_ = osess
	r.w.RaidInvite(lsess, "busy", 0)
	if other.GroupID != og.ID {
		t.Fatal("a player in a group of others was taken out of it")
	}
	// /invite <name> from no group to a player in a group of it and its merc alone
	r3 := newMercRig(t)
	r3.hire(r3.kit(9, 0, 0), MercAssist)
	r3.w.GroupAccept(r3.sess, r3.sess.PlayerObjectID())
	isess, _ := r3.player("Asker", 0, 0)
	r3.w.InviteByName(isess, "owner")
	r3.w.GroupAccept(isess, isess.PlayerObjectID()) // the asker's own box
	r3.sent()
	if r3.p.GroupID != 0 {
		t.Fatalf("/invite <name>: the owner's group %d did not give way", r3.p.GroupID)
	}
}

// /raidleave takes the merc out with its owner (0x069A 8 to the owner, the raid two
// smaller), and the owner is asked into a group with its merc again; a member with no
// merc leaves alone (the control).
func TestRaidLeaveWithAMerc(t *testing.T) {
	r, ms, g, ss, ps := mercRaid(t, "Other", "Third")
	n := len(g.Members)
	r.sent()
	r.w.RaidLeave(r.sess)
	if r.p.GroupID != 0 || groupHas(g, ms.npc.ServerID) || len(g.Members) != n-2 {
		t.Fatalf("after /raidleave: %v, the owner in %d", g.Members, r.p.GroupID)
	}
	got := r.sent()
	if got[protocol.OpcodeRemoveGroupMember] != 1 {
		t.Fatalf("the owner's client: %v", got)
	}
	r.run(0, time.Second) // the upkeep asks the owner into a group with its merc
	if got := r.sent(); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("the owner was not asked into a group with its merc again: %v", got)
	}
	m := len(g.Members)
	r.w.RaidLeave(ss[0])
	if ps[0].GroupID != 0 || len(g.Members) != m-1 {
		t.Fatal("the control: a member without a merc did not leave alone")
	}
}
