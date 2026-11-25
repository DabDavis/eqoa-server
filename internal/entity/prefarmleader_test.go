package entity

import "testing"

// /prefarm (handlers/prefarm.go) is the group leader's for the group's kills: a
// kill credit carries the leader at the death (KillCredit.Leader), whoever of the
// group killed; a kill of no group carries none.
func TestAKillCreditCarriesItsGroupsLeader(t *testing.T) {
	r := newRaidRig(t, 3, 60)
	r.form(2) // Ann leads Bob; Cal alone
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	kill := func(by int, id EntityID) KillCredit {
		t.Helper()
		n := &NPC{ServerID: id, Name: "a bat", Level: 1, HP: 10, MaxHP: 10, NPCType: 0x0100,
			X: r.p[by].X + 2, Y: r.p[by].Y, Z: r.p[by].Z, World: r.p[by].World}
		r.w.mu.Lock()
		r.w.putNPCLocked(n)
		r.w.mu.Unlock()
		if !r.w.KillNPCNowBy(r.sess[by], id) {
			t.Fatalf("kill %d refused", id)
		}
		return heard[len(heard)-1]
	}
	if k := kill(1, 9101); k.Group == 0 || k.Leader != r.p[0].CharServerID {
		t.Fatalf("Bob's kill: group %d leader %d, want Ann's %d", k.Group, k.Leader, r.p[0].CharServerID)
	}
	if k := kill(0, 9102); k.Leader != r.p[0].CharServerID {
		t.Fatalf("Ann's kill: leader %d, want %d", k.Leader, r.p[0].CharServerID)
	}
	if k := kill(2, 9103); k.Group != 0 || k.Leader != 0 {
		t.Fatalf("Cal's kill alone: group %d leader %d, want none", k.Group, k.Leader)
	}
}
