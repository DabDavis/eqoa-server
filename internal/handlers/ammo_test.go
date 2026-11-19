package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// rangedItem adds an item record: its kind field (unk3), wear slot and
// attack type (petition 2390).
func (f *scribeFixture) rangedItem(name string, unk3, wear, attackType, maxStack int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 209763529, 0, ?, ?, 0, 1, 0, ?, 20, 30, 1, ?, 0, 50, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
		wear, unk3, attackType, maxStack, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// A bow's shot takes one arrow off the pack's stack and tells the client by
// 0x003C (the arrow's index, sound 0, one), as a thrown potion's does; bolts
// are no arrows, and a spare bow (kind 1 in its field, but wearable) is none.
func TestAShotTakesAnArrow(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	bow := f.rangedItem("spare longbow", entity.AmmoArrows, 16, 7, 1)
	bolt := f.rangedItem("bolt", entity.AmmoBolts, -1, 0, 10000)
	arrow := f.rangedItem("arrow", entity.AmmoArrows, -1, 0, 9999)
	f.own(bow, 1)
	f.own(bolt, 50)
	f.own(arrow, 9999)
	f.loadList()

	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	l := sessionInvList(f.sess)
	if n := ammoHeld(l, rows, entity.AmmoArrows); n != 9999 {
		t.Errorf("arrows held %d, want 9999 (the spare bow and the bolts are none)", n)
	}
	if n := ammoHeld(l, rows, entity.AmmoBolts); n != 50 {
		t.Errorf("bolts held %d, want 50", n)
	}

	f.h.ammoSpent(f.sess, f.player.CharServerID, entity.AmmoArrows)
	if n := f.stackOf("arrow"); n != 9998 {
		t.Errorf("arrows after a shot %d, want 9998", n)
	}
	if n := f.stackOf("bolt"); n != 50 {
		t.Errorf("bolts after a bow's shot %d, want 50", n)
	}
	var removed []byte
	for _, m := range f.sess.Queue().GetPending() {
		d := m.Data()
		if protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeRemoveInvItem {
			removed = append([]byte(nil), d...)
		}
	}
	if want := mustHex(t, "3c00020002"); !bytes.Equal(removed, want) { // the arrow is the third row: index 2
		t.Errorf("0x003C % X, want % X", removed, want)
	}
}
