package handlers

import (
	"encoding/binary"
	"sort"
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

// Two of a Lore item off one kill (a GM's petition 2248): a seat that wins the
// first does not win the second; the next highest does, and with no one else rolling it
// lies on the corpse as though every one passed. The control: an item that is not Lore
// goes to the same winner twice.
func TestALoreItemsSecondCopyGoesToTheNextRoller(t *testing.T) {
	for _, c := range []struct {
		name               string
		lore               bool
		rbChoice           rollChoice
		ann, rb, freeAfter int
	}{
		{"Lore, a second roller", true, rollGreed, 1, 1, 0},
		{"Lore, no second roller", true, rollPass, 1, 0, 1},
		{"the control, not Lore", false, rollGreed, 2, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRaidLootRig(t, 3)
			id := r.item("Ring of Vox", 1, 7, 1, 0, 0, 1)
			if c.lore {
				if _, err := r.f.db.Exec(`UPDATE itempattern SET lore = 1 WHERE patternid = ?`, id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 100, ?, 9)`, corpseNPCID, id); err != nil {
				t.Fatal(err)
			}
			r.kill(entity.LootNeedGreed)
			ls := r.f.h.loots()
			ls.mu.Lock()
			var rolls []*lootRoll
			for _, x := range ls.rolls {
				rolls = append(rolls, x)
			}
			ls.mu.Unlock()
			if len(rolls) != 2 {
				t.Fatalf("%d rolls, want 2", len(rolls))
			}
			sort.Slice(rolls, func(i, j int) bool { return rolls[i].id < rolls[j].id })
			for _, roll := range rolls {
				r.dice = []int{90}
				r.f.h.RaidRollChoice(r.sess[0], roll.id, rollNeed)
				if c.rbChoice == rollGreed {
					r.dice = []int{20}
				}
				r.f.h.RaidRollChoice(r.sess[1], roll.id, c.rbChoice)
				r.f.h.RaidRollChoice(r.sess[2], roll.id, rollPass)
			}
			if a, b := r.owns(0, "Ring of Vox"), r.owns(1, "Ring of Vox"); a != c.ann || b != c.rb {
				t.Errorf("Ann holds %d, Rb %d; want %d, %d", a, b, c.ann, c.rb)
			}
			if n := len(r.freeNames()); n != c.freeAfter {
				t.Errorf("%d free on the corpse, want %d", n, c.freeAfter)
			}
			if r.corpse() != nil && len(r.corpse().raid.held) != 0 {
				t.Errorf("%d held for a winner", len(r.corpse().raid.held))
			}
		})
	}
}

// The owner, 2026-11-18: "rolls from players are not showing for the whole raid". A raid's
// loot rolls and their winner are said to every member of the raid (the World's
// RaidSessions: Group.Members, all its groups, wherever each is), a member with no seat on
// the kill included, not only to the roll's seats.
func TestARaidsLootRollsAreSaidToEveryMember(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.kill(entity.LootNeedGreed, 2) // Rc: no seat (left out of the kill)
	roll := r.rollOf("Fine Steel Sword")
	r.drain()
	r.dice = []int{40}
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollNeed)
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollPass)
	if l := r.lines(2); !raidSaid(l, "Ann rolls Need on Fine Steel Sword: 40.") || !raidSaid(l, "Ann wins Fine Steel Sword with Need 40.") {
		t.Fatalf("a member with no seat heard %q", l)
	}
}
