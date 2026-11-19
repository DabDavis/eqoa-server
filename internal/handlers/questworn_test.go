package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// questworn_test.go — a quest's item worn counts as had (2026-11-18, the
// owner: "for the level 1 quest where players get item from merchant if
// player equips it the quest giver doesnt acknowledge until they
// unequipped"; Live's player, "New Gear" step 4, told four times he
// lacked the plain robe he wore). A show step keeps it; a give step takes a
// carried copy first, else takes the worn one off and takes it.

const (
	wornMerchantNPC = 9301 // npcs.npcid of the merchant's catalog
	wornMerchantID  = 802  // its entity id
)

// wornFixture is a quest fixture with a merchant to buy from and the item
// menu to wear with, as a player does it.
type wornFixture struct {
	*questFixture
	robe  int    // a robe (wear 2: equip slot 21), sold for nothing
	giver uint32 // Kellina's object id
	shop  *NPCWindowHandler
	equip *EquipHandler
}

func newWornFixture(t *testing.T, kind string) *wornFixture {
	t.Helper()
	f := &wornFixture{questFixture: newQuestFixture(t)}
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	f.robe = f.gearPattern("plain robe", 2, 1)
	f.exec(`UPDATE itempattern SET itemcost = 0 WHERE patternid = ?`, f.robe)
	f.giver = f.tw.add(1, &entity.NPC{ServerID: 801, Name: "Kellina", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	merchant := &entity.NPC{ServerID: wornMerchantID, NPCID: wornMerchantNPC, Name: "Merchant Yulia", NPCType: 0x81,
		SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5}
	f.tw.add(2, merchant)
	f.exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 300, ?, 0)`, wornMerchantNPC, f.robe)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline) VALUES (20020, 'New Gear', 1, 'Kellina',
		'Buy a plain robe.', '', '')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, xp) VALUES
		(20020, 1, '%s', 'Kellina', '%d', 'Bring Kellina a plain robe (Merchant Yulia).', 'Kellina: Well done.', 430)`, kind, f.robe))
	f.reload()

	f.shop = NewNPCWindowHandler(nil, f.inv)
	f.shop.playerOf = func(*session.Session) *entity.Player { return f.tw.p }
	f.shop.find = func(_ *session.Session, id uint32) (entity.Entity, bool) {
		if id == wornMerchantID {
			return merchant, true
		}
		return nil, false
	}
	f.equip = NewEquipHandler(nil, f.inv, nil, nil)
	f.equip.playerOf = func(*session.Session) *entity.Player { return f.tw.p }
	f.qe.takeOff = f.equip.TakeOff // as the registry wires it

	f.tw.talk(f.giver) // no answer to take it: taken at once
	f.msgs()
	if step, _ := f.progress(20020); step != 1 {
		t.Fatalf("the quest is at step %d, want 1", step)
	}
	return f
}

// buy is the merchant's 0x004A for its first ware.
func (f *wornFixture) buy() {
	f.t.Helper()
	if !f.shop.buy(f.tw.s, wornMerchantID, 0, 1) {
		f.t.Fatal("the merchant refused the robe")
	}
	f.msgs()
}

// use is the item menu's Use (0x003E, interaction 0) on the list key: an
// unworn item is put on, a worn one taken off.
func (f *wornFixture) use(key uint32) {
	f.t.Helper()
	payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(0xFFFFFFFF)
		w.WriteByte(0)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(payload)
	if err := f.equip.HandleInteractItem(f.tw.s, msg); err != nil {
		f.t.Fatal(err)
	}
	f.msgs()
}

// worn is the robe rows' equip slots, in item id order.
func (f *wornFixture) worn() []int {
	f.t.Helper()
	rows, err := f.db.Query(`SELECT equiploc FROM charinventory WHERE serverid = ? AND patternid = ? ORDER BY itemid`, f.sid, f.robe)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var e int
		rows.Scan(&e)
		out = append(out, e)
	}
	return out
}

// Bought, put on, then the giver: a show step is done and the robe kept,
// worn; the control, bought and not put on, the same.
func TestShowStepBoughtAndWorn(t *testing.T) {
	for _, wear := range []bool{true, false} {
		f := newWornFixture(t, "show")
		f.buy()
		if wear {
			f.use(0)
			if w := f.worn(); len(w) != 1 || w[0] != 21 {
				t.Fatalf("the robe was not put on: %v", w)
			}
		}
		f.tw.talk(f.giver)
		msgs := f.msgs()
		if _, done := f.progress(20020); done != 1 {
			t.Fatalf("worn %v: the giver did not take the show: %q", wear, msgs)
		}
		f.want(msgs, "box:Kellina: Well done.")
		if w := f.worn(); len(w) != 1 || (wear && w[0] != 21) || (!wear && w[0] != -1) {
			t.Errorf("worn %v: the robe afterwards %v", wear, w)
		}
	}
}

// Bought, put on, then the giver: a give step takes the robe off and takes
// it (UnequipItem for its slot, then the record out of the list); the
// control, bought and not put on, takes it from the pack.
func TestGiveStepBoughtAndWorn(t *testing.T) {
	for _, wear := range []bool{true, false} {
		f := newWornFixture(t, "give")
		f.buy()
		if wear {
			f.use(0)
		}
		f.tw.talk(f.giver)
		msgs := f.msgs()
		if _, done := f.progress(20020); done != 1 {
			t.Fatalf("worn %v: the giver did not take the robe: %q", wear, msgs)
		}
		if n := f.held(f.robe); n != 0 {
			t.Errorf("worn %v: %d robes left", wear, n)
		}
		f.want(msgs, "take:0*1", "box:Kellina: Well done.")
		unequip := fmt.Sprintf("op:%04X", uint16(protocol.OpcodeUnequipItem))
		if wear != hasMsg(msgs, unequip) {
			t.Errorf("worn %v: UnequipItem sent %v: %q", wear, !wear, msgs)
		}
		if wear && indexOf(msgs, unequip) > indexOf(msgs, "take:0*1") {
			t.Errorf("the robe was taken before it came off: %q", msgs)
		}
	}
}

// One robe carried and one worn: the carried one is handed in, the worn one
// stays on.
func TestGiveStepTakesTheCarriedCopyFirst(t *testing.T) {
	f := newWornFixture(t, "give")
	f.buy()
	f.use(0) // the first worn
	f.buy()  // a second, carried
	if w := f.worn(); len(w) != 2 || w[0] != 21 || w[1] != -1 {
		t.Fatalf("the robes %v, want one worn and one carried", w)
	}
	f.tw.talk(f.giver)
	msgs := f.msgs()
	if _, done := f.progress(20020); done != 1 {
		t.Fatalf("the giver did not take a robe: %q", msgs)
	}
	if w := f.worn(); len(w) != 1 || w[0] != 21 {
		t.Errorf("the robes left %v, want the worn one", w)
	}
	f.want(msgs, "take:1*1")
	f.not(msgs, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeUnequipItem)))
}

// A worn copy taken with no TakeOff wired (no equip handler) still comes
// off: the slot written -1 and UnequipItem sent.
func TestGiveStepTakesWornWithoutTheEquipHandler(t *testing.T) {
	f := newWornFixture(t, "give")
	f.qe.takeOff = nil
	item, err := f.inv.AddCharItem(f.sid, f.robe, 1, 300, 0, database.LocPack)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.inv.SetEquipLoc(f.sid, item, 21); err != nil {
		t.Fatal(err)
	}
	rows, _ := f.inv.GetCharacterItems(f.sid)
	f.tw.s.SetInventoryList(newInvList(rows))
	f.tw.talk(f.giver)
	msgs := f.msgs()
	if _, done := f.progress(20020); done != 1 || f.held(f.robe) != 0 {
		t.Fatalf("the worn robe was not handed in: %q", msgs)
	}
	f.want(msgs, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeUnequipItem)), "take:0*1")
}

func indexOf(msgs []string, want string) int {
	for i, m := range msgs {
		if m == want {
			return i
		}
	}
	return -1
}
