package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// A raid's kill opened before the worker came to it is rolled by that opening, under its
// rules: no one takes it for free in between (the control: a kill with no raid opens as
// any corpse).
func TestARaidKillOpenedBeforeTheWorkerIsStillRolled(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1)
	seat := func(i int) entity.RaidSeat {
		return entity.RaidSeat{ServerID: r.p[i].CharServerID, Name: r.p[i].Name, Level: 50}
	}
	r.f.h.heardRaidKill(entity.KillCredit{NPCName: r.f.npc.Name, NPCID: r.f.npc.ServerID, Died: r.f.npc.DeathTime,
		Raid: &entity.RaidKill{Group: 1, Rule: entity.LootNeedGreed, Leader: seat(0), Seats: []entity.RaidSeat{seat(0), seat(1)},
			Members: []entity.RaidSeat{seat(0), seat(1)}}})
	r.drain()
	r.f.h.OpenCorpse(r.sess[1], r.f.npc) // no takeRaidKills first
	got := r.msgs(1)
	if len(got) == 0 || lootCount(t, got[len(got)-1]) != 1 {
		t.Fatalf("the opening: %s", ops(got))
	}
	if r.rollOf("Fine Steel Sword") == nil {
		t.Fatal("the sword was not rolled on")
	}
	if _, pending := r.f.h.raids.kills.Load(r.f.npc.ServerID); pending {
		t.Fatal("the kill is still pending: the worker would roll it again")
	}
	// the control
	r2 := newRaidLootRig(t, 2)
	r2.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r2.f.h.OpenCorpse(r2.sess[1], r2.f.npc)
	if got := r2.msgs(1); lootCount(t, got[len(got)-1]) != 1 || r2.rollOf("Fine Steel Sword") != nil {
		t.Fatal("a kill with no raid was rolled on")
	}
}

// A corpse that goes ends its rolls and closes their windows; one that lies keeps them
// (the control).
func TestACorpseGoneEndsItsRolls(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.windows[r.sess[0]] = true
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Fine Steel Sword")
	r.drain()
	r.f.h.CorpsesGone([]entity.EntityID{r.f.npc.ServerID + 1}) // another's
	if r.rollOf("Fine Steel Sword") == nil {
		t.Fatal("another corpse's going ended the roll")
	}
	r.f.h.CorpsesGone([]entity.EntityID{r.f.npc.ServerID})
	if r.rollOf("Fine Steel Sword") != nil {
		t.Fatal("the roll outlived its corpse")
	}
	closed := false
	for _, m := range r.msgs(0) {
		closed = closed || opOf(m) == protocol.OpcodeRaidRoll && binary.LittleEndian.Uint32(m[2:]) == uint32(roll.id) &&
			binary.LittleEndian.Uint16(m[6:]) == 0
	}
	if !closed {
		t.Fatal("its window was not closed")
	}
}

// A member kicked in the fight is a seat and on the master looter's list: the list names
// the members and the claim; one never a member is on neither (the control).
func TestAClaimIsOnTheMasterLootersList(t *testing.T) {
	r := newRaidLootRig(t, 4)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.windows[r.sess[0]] = true
	r.members = []int{0, 1, 2} // Rd is not in the raid now
	seat := func(i int, claim bool) entity.RaidSeat {
		return entity.RaidSeat{ServerID: r.p[i].CharServerID, Name: r.p[i].Name, Level: 50, Claim: claim}
	}
	k := entity.KillCredit{NPCName: r.f.npc.Name, NPCID: r.f.npc.ServerID, Died: r.f.npc.DeathTime,
		Raid: &entity.RaidKill{Group: 1, Rule: entity.LootMaster, Leader: seat(0, false),
			Seats:   []entity.RaidSeat{seat(0, false), seat(1, false), seat(3, true)},
			Members: []entity.RaidSeat{seat(0, false), seat(1, false), seat(3, true)}}}
	r.f.h.heardRaidKill(k)
	r.f.h.takeRaidKills()
	r.drain()
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	var names []string
	for _, m := range r.msgs(0) {
		if opOf(m) != protocol.OpcodeRaidMasterLoot || len(m) < 6 || m[2] != 1 {
			continue
		}
		names = nil
		b := m[6:]
		for i := 0; i < int(m[5]); i++ {
			n := int(b[4])
			names = append(names, string(b[5:5+n]))
			b = b[5+n:]
		}
	}
	got := strings.Join(names, ",")
	if got != "Ann,Rb,Rd,Rc" {
		t.Fatalf("the list: %s", got) // the master, the kill's members, then those of the raid since
	}
	// the claim hears the raid's lines
	r.f.h.MasterAssign(r.sess[0], 0, 0, "Rd")
	if r.owns(3, "Fine Steel Sword") != 1 || !raidSaid(r.lines(3), "Ann assigns Fine Steel Sword to Rd.") {
		t.Fatal("the claim was not given to, or not told")
	}
}
