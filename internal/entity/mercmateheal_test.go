package entity

import (
	"testing"
	"time"
)

// Petition 2805: a group mate's heal lands on another player's merc, at it
// on the caster's channel and at it from the group window, whose roster names
// a merc the caster has no channel for by its stand-in (groupFarID); the
// mate's group heal reaches it too. The control: a stand-in for a merc of
// another group finds no target.
func TestAGroupMateHealsAnothersMerc(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercPassive) // the owner's tank
	sessB, b := r.player("B", 9, 0)          // a cleric of the group
	r.group(b)
	heal := mercHealSpell()
	b.Power = b.MaxPowerNow()
	r.run(0, time.Second)
	vs := GetOrCreateVisibility(sessB)
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = ms.npc.ServerID, 0x01312E98
	for i, id := range []uint32{0x01312E98, groupFarID(ms.npc.ServerID)} {
		hurtNPCTo(ms.npc, 30)
		before := ms.npc.HP
		at := time.Duration(i*30+2) * time.Second
		if !r.w.StartCast(sessB, heal, id, r.at(at)) {
			t.Fatalf("B's heal at the merc (id %08X) was refused", id)
		}
		r.run(at, at+5*time.Second)
		if ms.npc.HP < before+125 {
			t.Errorf("B's heal at the merc (id %08X): HP %d -> %d", id, before, ms.npc.HP)
		}
	}
	// B's group heal
	hurtNPCTo(ms.npc, 30)
	before := ms.npc.HP
	if !r.w.StartCast(sessB, mercGroupHealSpell(), 0, r.at(70*time.Second)) {
		t.Fatal("B's group heal was refused")
	}
	r.run(70*time.Second, 75*time.Second)
	if ms.npc.HP < before+300 {
		t.Errorf("B's group heal on the merc: HP %d -> %d", before, ms.npc.HP)
	}

	// the control: C, of no group, names the merc by its stand-in: no target
	sessC, c := r.player("C", 9, 0)
	c.Power = c.MaxPowerNow()
	if r.w.StartCast(sessC, heal, groupFarID(ms.npc.ServerID), r.at(80*time.Second)) {
		t.Error("C, of no group, healed the merc by its group stand-in")
	}
}
