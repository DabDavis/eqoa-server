package entity

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Petition 2213 (Drizzt, 2026-11-16): "mercenaries drop group after a reset.
// seemingly still with me but dont help." The keep restart at 01:52:45 put
// his cleric Talanus back (grouped true) and formed a new group of the two of
// them (86, it had been 84) after the players' groups; his client froze
// through the restart and logged in again 14 s later, a takeover: CLIENT's
// Connect clears m_inGroup and Msg0772 returns without it, so every roster of
// the group the server still held was ignored, the merc out of his group
// window, and its buffs were gone with the restart. These hold that a keep
// restart carries a merc in its group in its place, under the group's id,
// with its effects, and that a client that does not hold the group is asked
// with the merc's box until it does.

// sentOn is what sess was sent since the last look, by opcode.
func sentOn(sess *session.Session) map[protocol.GameOpcode]int {
	out := map[protocol.GameOpcode]int{}
	for _, m := range sess.Queue().GetPending() {
		out[opOfMsg(m.Data())]++
	}
	return out
}

// hireSlot hires the kit's merc into the slot, which must come.
func (r *mercRig) hireSlot(slot int, k MercKit) *mercState {
	r.t.Helper()
	if _, why, _ := r.w.HireMercAt(r.sess, slot, k, MercAssist, r.at(0)); why != "" {
		r.t.Fatalf("the hire of slot %d was refused: %s", slot, why)
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ms := r.w.mercAtLocked(r.p, slot)
	if ms == nil {
		r.t.Fatalf("no merc in slot %d", slot)
	}
	return ms
}

// A keep restart carries a group of its owner, two mercs and another player
// in its members' order under its own id, the mercs with the spells on them
// and their share of hit points, the owner's client probed with the mercs'
// box (its answer 2 for itself, as a client in a group gives, keeps all as it
// is); and the merc heals its owner after it. The control: a snapshot without
// the order (a process's before it) still forms the group, the mercs at the
// upkeep.
func TestMercKeepRestartKeepsItsGroupPlace(t *testing.T) {
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	r := newMercRig(t)
	_, friend := r.player("Friend", 0, 0) // before the mercs take ids: its character's id is the rig's again
	k1 := r.kit(9, 0, 0, heal)
	k2 := r.kit(0, 0, 0)
	k2.Name = "Belthas"
	a := r.hire(k1, MercAssist)
	b := r.hireSlot(2, k2)
	g := r.group(friend)
	if len(g.Members) != 4 || g.Members[1] != a.npc.ServerID || g.Members[2] != b.npc.ServerID || g.Members[3] != friend.ServerID {
		t.Fatalf("the group before the restart: %v", g.Members)
	}
	// a buff on the cleric: 500 more hit points, 50 more armour
	a.npc.Effects = append(a.npc.Effects, &ActiveEffect{SpellID: 700805, Name: "Order", Until: r.at(time.Hour),
		Mods: StatMods{MaxHP: 500, AC: 50}})
	r.w.mu.Lock()
	syncMercBody(a)
	r.w.mu.Unlock()
	buffed := a.npc.MaxHP
	a.npc.HP = buffed * 3 / 4
	kp, ok := r.w.KeepPlayer(r.sess, r.at(0))
	if !ok || len(kp.Mercs) != 2 || len(kp.Mercs[0].Effects) != 1 {
		t.Fatalf("kept: %+v", kp.Mercs)
	}
	raw, err := json.Marshal(r.w.KeepGroups())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptGroup
	if err := json.Unmarshal(raw, &kept); err != nil || len(kept) != 1 || len(kept[0].Order) != 4 {
		t.Fatalf("kept groups: %s (%v)", raw, err)
	}

	restart := func(groups []KeptGroup) (*mercRig, *Player) {
		r2 := newMercRig(t)
		_, f2 := r2.player("Friend", 0, 0)
		if r2.p.CharServerID != r.p.CharServerID || f2.CharServerID != friend.CharServerID {
			t.Fatalf("the rig's characters moved: %d %d", r2.p.CharServerID, f2.CharServerID)
		}
		r2.p.X, r2.p.Z = r.p.X, r.p.Z
		for i, km := range KeptMercsOf(&kp) {
			kit := k1
			if i == 1 {
				kit = k2
			}
			if !r2.w.RestoreMerc(r2.sess, kit, km, r2.at(0)) {
				t.Fatalf("merc %d did not come back", km.Slot)
			}
		}
		if n, err := r2.w.RestoreGroups(groups); n != 1 || err != nil {
			t.Fatalf("groups formed again: %d (%v)", n, err)
		}
		return r2, f2
	}

	r2, f2 := restart(kept)
	r2.w.mu.Lock()
	a2, b2 := r2.w.mercAtLocked(r2.p, 1), r2.w.mercAtLocked(r2.p, 2)
	r2.w.mu.Unlock()
	g2 := r2.w.groups[r2.p.GroupID]
	if g2 == nil || g2.ID != g.ID {
		t.Fatalf("the group is %+v, want its id %d", g2, g.ID)
	}
	if len(g2.Members) != 4 || g2.Members[0] != r2.p.ServerID || g2.Members[1] != a2.npc.ServerID ||
		g2.Members[2] != b2.npc.ServerID || g2.Members[3] != f2.ServerID {
		t.Fatalf("members %v, want owner %d, mercs %d %d, friend %d", g2.Members, r2.p.ServerID, a2.npc.ServerID, b2.npc.ServerID, f2.ServerID)
	}
	if len(a2.npc.Effects) != 1 || a2.npc.MaxHP != buffed || a2.npc.HP != buffed*3/4 {
		t.Errorf("the cleric came back with %d effects, %d/%d HP; want 1, %d/%d", len(a2.npc.Effects), a2.npc.HP, a2.npc.MaxHP,
			buffed*3/4, buffed)
	}
	r2.sent()
	r2.run(0, time.Second)
	got := r2.sent()
	if got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("the owner's client was not probed with its mercs' box: %v", got)
	}
	// its client holds the group: it answers 2 for itself (Msg0773)
	r2.w.GroupDecline(r2.sess, r2.sess.PlayerObjectID(), 2)
	r2.run(time.Second, 2*time.Second)
	if r2.w.groups[r2.p.GroupID] != g2 || len(g2.Members) != 4 || r2.w.NPCs[a2.npc.ServerID] == nil {
		t.Fatalf("the probe's answer changed the group: %v", g2.Members)
	}
	if got := r2.sent(); got[protocol.OpcodeGroupInviteReceived] != 0 {
		t.Errorf("probed again after its answer: %v", got)
	}
	// and it heals its owner as before
	top := r2.p.MaxHPNow()
	r2.p.HP = top / 2
	r2.run(2*time.Second, 8*time.Second)
	if r2.p.HP <= top/2 {
		t.Errorf("the restored cleric did not heal its owner: %d/%d", r2.p.HP, top)
	}

	// the control: a snapshot's groups without Order
	old := []KeptGroup{{ID: kept[0].ID, Members: kept[0].Members}}
	r3, _ := restart(old)
	r3.run(0, time.Second)
	g3 := r3.w.groups[r3.p.GroupID]
	if g3 == nil || len(g3.Members) != 4 {
		t.Fatalf("an old snapshot's group: %+v", g3)
	}
}

// A group of its owner and its merc alone is kept under its id (it was formed
// anew under the next until 2026-11-16: Drizzt's 80, 84, 86), and again
// through a second restart.
func TestMercKeepRestartKeepsTheOwnersGroupOfMercs(t *testing.T) {
	r := newMercRig(t)
	k := r.kit(9, 0, 0)
	r.hire(k, MercAssist)
	id := r.p.GroupID
	for round := range 2 {
		kp, _ := r.w.KeepPlayer(r.sess, r.at(0))
		kept := r.w.KeepGroups()
		if len(kept) != 1 || kept[0].ID != id || len(kept[0].Members) != 1 || len(kept[0].Order) != 2 {
			t.Fatalf("round %d: kept groups %+v", round, kept)
		}
		r2 := newMercRig(t)
		r2.p.X, r2.p.Z = r.p.X, r.p.Z
		if !r2.w.RestoreMerc(r2.sess, k, KeptMercsOf(&kp)[0], r2.at(0)) {
			t.Fatal("the merc did not come back")
		}
		if n, err := r2.w.RestoreGroups(kept); n != 1 || err != nil {
			t.Fatalf("round %d: %d groups (%v)", round, n, err)
		}
		r2.run(0, time.Second)
		g := r2.w.groups[r2.p.GroupID]
		if r2.p.GroupID != id || g == nil || len(g.Members) != 2 {
			t.Fatalf("round %d: the owner is in group %d (%+v), want %d with its merc", round, r2.p.GroupID, g, id)
		}
		r = r2
	}
}

// A character taken over (its client froze, crashed or left, and logged in
// again: takeover.go) keeps its merc in its group, healing; its new client,
// which holds no group, is asked with the merc's box once it is in the world
// (not before), and its yes sends it the roster. A merc whose box was up on
// the old client is asked again on the new one, not dismissed. A no
// dismisses it, as at any merc's box.
func TestMercTakeoverAsksTheNewClientToShowTheGroup(t *testing.T) {
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0, heal), MercAssist)
	gid := r.p.GroupID
	takeover := func(port int) *session.Session {
		t.Helper()
		s := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 1, 0, 1), Port: port}, uint32(port), 1, 1, 2, true)
		s.SetPlayerObjectID(0x00B00000 + uint32(port))
		if p, _ := r.w.TakeOverPlayer(s, r.p.CharServerID); p != r.p {
			t.Fatalf("not taken over: %v", p)
		}
		r.sess = s
		return s
	}
	ns := takeover(9001)
	r.run(0, time.Second)
	if got := sentOn(ns); got[protocol.OpcodeGroupInviteReceived] != 0 {
		t.Fatalf("the new client was asked before it was in the world: %v", got)
	}
	if r.p.GroupID != gid || !groupHas(r.w.groups[gid], ms.npc.ServerID) {
		t.Fatal("the takeover took the merc out of its group")
	}
	ns.SetCharacterInWorld(true)
	r.run(time.Second, 2*time.Second)
	if got := sentOn(ns); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("the new client in the world was not asked to show its group: %v", got)
	}
	// it heals meanwhile, in the group the server holds
	top := r.p.MaxHPNow()
	r.p.HP = top / 2
	r.run(2*time.Second, 8*time.Second)
	if r.p.HP <= top/2 {
		t.Errorf("the merc did not heal its owner while its box was up: %d/%d", r.p.HP, top)
	}
	r.w.GroupAccept(ns, ns.PlayerObjectID())
	if got := sentOn(ns); got[protocol.OpcodeCreateGroup] == 0 {
		t.Errorf("no roster after the yes: %v", got)
	}
	if r.p.GroupID != gid || len(r.w.groups[gid].Members) != 2 {
		t.Fatalf("after the yes: group %d", r.p.GroupID)
	}

	// /merc group with the group held: the probe again (a client in a group
	// answers it for itself)
	if line := r.w.AskMercGroup(ns); line == "" {
		t.Error("/merc group in a group said nothing")
	}
	if got := sentOn(ns); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Errorf("/merc group did not probe the client: %v", got)
	}
	r.w.GroupDecline(ns, ns.PlayerObjectID(), 2)

	// a box up on the old client at the takeover: asked again on the new one
	r.w.GroupLeave(ns) // the group of the two of them: the merc dismissed (mercgroup.go)
	r.run(8*time.Second, 9*time.Second)
	ms = r.hireAsked(r.kit(9, 0, 0, heal), MercAssist)
	_ = sentOn(ns)
	ns2 := takeover(9002)
	ns2.SetCharacterInWorld(true)
	r.run(9*time.Second, 11*time.Second)
	if r.w.merc1(r.p.ServerID) == nil {
		t.Fatal("a merc whose box was up on the old client was dismissed at the takeover")
	}
	if got := sentOn(ns2); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("the new client was not asked into a group with the merc: %v", got)
	}
	// its box lost to the client: /merc group sends it again
	if line := r.w.AskMercGroup(ns2); line != "" {
		t.Errorf("/merc group with its own box up: %q", line)
	}
	if got := sentOn(ns2); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Errorf("/merc group did not send its box again: %v", got)
	}
	r.w.GroupAccept(ns2, ns2.PlayerObjectID())
	if g := r.w.groups[r.p.GroupID]; g == nil || !groupHas(g, ms.npc.ServerID) {
		t.Fatal("the yes made no group with the merc")
	}

	// a takeover's no to the box: dismissed
	ns3 := takeover(9003)
	ns3.SetCharacterInWorld(true)
	r.run(11*time.Second, 13*time.Second)
	if got := sentOn(ns3); got[protocol.OpcodeGroupInviteReceived] != 1 {
		t.Fatalf("the third client was not asked: %v", got)
	}
	r.w.GroupDecline(ns3, ns3.PlayerObjectID(), 1)
	r.run(13*time.Second, 14*time.Second)
	if r.w.merc1(r.p.ServerID) != nil || r.p.GroupID != 0 {
		t.Errorf("the no kept the merc (group %d)", r.p.GroupID)
	}
}
