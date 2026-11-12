package entity

import (
	"strings"
	"testing"
)

// The TEST menu's KILL (KillNPCNowBy) is credited as #kill is: to the GM and
// its group's members near, so their quests' kill and loot steps hear it (the
// owner's Quil'Eyne of 2026-11-12, killed so twice with no blood dropped),
// and still gives no experience. The controls: a member past KillShareRange
// is not credited; the panel's KillNPCNow (no GM) credits no one; a dead or
// missing NPC is refused.
func TestTheTestMenusKillIsCreditedToTheGMsGroup(t *testing.T) {
	r := newRaidRig(t, 3, 60)
	r.form(3)
	r.p[2].X += KillShareRange + 50 // Cal stands too far to share
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	newNPC := func(id EntityID) *NPC {
		n := &NPC{ServerID: id, Name: "Quil'Eyne", Level: 65, HP: 1000, MaxHP: 1000, NPCType: 0x0100,
			X: r.p[0].X + 2, Y: r.p[0].Y, Z: r.p[0].Z, World: r.p[0].World}
		r.w.mu.Lock()
		r.w.putNPCLocked(n)
		r.w.mu.Unlock()
		return n
	}
	quil := newNPC(9001)
	xp := r.p[0].TotalXP
	if !r.w.KillNPCNowBy(r.sess[0], quil.ServerID) {
		t.Fatal("KillNPCNowBy refused a living NPC")
	}
	if !quil.Dead {
		t.Fatal("KillNPCNowBy left it alive")
	}
	if len(heard) != 1 {
		t.Fatalf("kills heard %d, want 1", len(heard))
	}
	var names []string
	for _, kp := range heard[0].Players {
		names = append(names, kp.Name)
	}
	if got := strings.Join(names, ","); got != "Ann,Bob" {
		t.Fatalf("credited %s, want Ann,Bob (Cal too far)", got)
	}
	if heard[0].NPCName != "Quil'Eyne" || heard[0].NPCID != quil.ServerID || heard[0].Group != r.p[0].GroupID {
		t.Fatalf("the credit %+v", heard[0])
	}
	if r.p[0].TotalXP != xp {
		t.Fatalf("the GM's kill gave experience: %d -> %d", xp, r.p[0].TotalXP)
	}
	if r.w.KillNPCNowBy(r.sess[0], quil.ServerID) || r.w.KillNPCNowBy(r.sess[0], 4242) {
		t.Fatal("a dead or missing NPC was killed")
	}
	if len(heard) != 1 {
		t.Fatalf("a refused kill was heard: %d", len(heard))
	}

	other := newNPC(9002)
	if !r.w.KillNPCNow(other.ServerID) || !other.Dead {
		t.Fatal("KillNPCNow refused")
	}
	if len(heard) != 1 {
		t.Fatalf("the panel's KillNPCNow was credited: %d", len(heard))
	}
}
