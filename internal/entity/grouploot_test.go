package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// The GM Jibrish's petition 1878: "Create an option under the group menu to allow players to turn on need before
// green loot system that raids has". A group of three (no raid): its kill carries no loot rule while it loots free
// for all (the control); a member's /grouploot is refused, the leader's sets need before greed, said to the group;
// then its kill is a raid's kill to the loot, under need before greed, its seats its members in reach; master
// looter is refused for a group; not in a fight; vanilla's menu's rule (0x7E48 1) and /raidloot do the same in a
// group; its clients that show the raid's windows are told it in 0x7E44 (leader 0, rule 1, flag 8, the leader's 2);
// off, its kill is free for all again.
func TestAGroupsNeedBeforeGreed(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	r.form(3)
	r.drainAll()
	g := r.raidOf(0)
	if g == nil || raidingLocked(g) {
		t.Fatal("not a plain group of three")
	}
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	kill := func(id EntityID) *RaidKill {
		npc := r.fight(id, 0)
		r.w.mu.Lock()
		npc.Threats, npc.CurrentEnemy = nil, 0
		r.w.killNPC(npc)
		r.w.creditKillLocked(r.sess[0], r.p[0], npc)
		r.w.mu.Unlock()
		return heard[len(heard)-1].Raid
	}
	if k := kill(9600); k != nil {
		t.Fatalf("a group's free for all kill carries a rule: %+v", k)
	}
	r.w.SetRaidWindows(r.sess[1], true)
	drain(r.sess[1])

	r.w.GroupLootRule(r.sess[1], "needgreed")
	if g.rules.loot != LootFreeForAll || !saidPart(r.heard(1), "Only your group's leader") {
		t.Fatal("a member set the group's rule")
	}
	r.w.GroupLootRule(r.sess[0], "master")
	if g.rules.loot != LootFreeForAll || !saidPart(r.heard(0), "Master looter is a raid's") {
		t.Fatal("a group took master looter")
	}
	r.w.GroupLootRule(r.sess[0], "on")
	if g.rules.loot != LootNeedGreed || !saidPart(r.heard(2), "sets the group's loot rule: need before greed") {
		t.Fatal("the leader's /grouploot on did not set need before greed")
	}
	var state []byte
	for _, m := range drain(r.sess[1]) {
		if opOfMsg(m) == protocol.OpcodeRaidState {
			state = m[2:]
		}
	}
	if len(state) != 14 || binary.LittleEndian.Uint32(state) != 0 || state[12] != byte(LootNeedGreed) || state[13] != groupRuleFlag {
		t.Fatalf("Bob's state: % x", state)
	}
	k := kill(9601)
	if k == nil || k.Rule != LootNeedGreed || k.Leader.Name != "Ann" || len(k.Seats) != 3 || len(k.Members) != 3 {
		t.Fatalf("the group's kill under need before greed: %+v", k)
	}

	// not in a fight
	r.fight(9602, 1)
	r.w.GroupLootRule(r.sess[0], "off")
	if g.rules.loot != LootNeedGreed || !saidPart(r.heard(0), "in combat") {
		t.Fatal("the group's rule changed in a fight")
	}
	r.w.mu.Lock()
	r.w.NPCs[9602].Threats, r.w.NPCs[9602].CurrentEnemy = nil, 0
	r.w.mu.Unlock()
	// vanilla's menu, then /raidloot, in a group
	r.w.RaidLootRuleTo(r.sess[0], LootFreeForAll)
	if g.rules.loot != LootFreeForAll {
		t.Fatal("the menu's rule did not reach a group")
	}
	r.w.RaidLootRule(r.sess[0], "needgreed")
	if g.rules.loot != LootNeedGreed || raidingLocked(g) {
		t.Fatal("/raidloot in a group did not set its rule, or made it a raid")
	}
	r.w.GroupLootRule(r.sess[0], "ffa")
	if k := kill(9603); k != nil {
		t.Fatalf("off again: the kill carries %+v", k)
	}
}
