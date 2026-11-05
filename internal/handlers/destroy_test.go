package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// destroyMsgs sends the item menu's Destroy, 0x003C {u32 key, u32 how many}
// (VIClient::UseItemOn), and returns what the server answered.
func (f *gearFixture) destroyItem(key, n uint32) [][]byte {
	return f.send(protocol.OpcodeRemoveInvItem, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(n)
	})
}

// stack puts a stack of n of an item in the pack and returns its itemid.
func (f *gearFixture) stack(pattern, n int) int {
	f.t.Helper()
	id, err := f.inv.AddCharItem(f.sid, pattern, n, 0, 0, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

// rowsLeft is the inventory's rows by itemid, each its stack.
func (f *gearFixture) rowsLeft() map[int]int {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[int]int{}
	for _, r := range rows {
		out[r.ItemID] = r.StackLeft
	}
	return out
}

// The item menu's Destroy: the record a key names leaves the database and
// the client's list, answered 0x003C {list index, sound 0, zigzag how many}
// (Msg003C takes them and lets the menu send the next Destroy); part of a
// stack leaves the rest; the keys still name their records after the list
// packs (VIItemList::Took never renumbers). Until 2026-11-05 nothing
// answered and the item stayed.
func TestDestroyTakesTheItemOut(t *testing.T) {
	f := newGearFixture(t)
	apple := f.own(f.pattern("Apple", -1, 0, 0), -1)
	rations := f.stack(f.pattern("Ration", -1, 0, 0), 20)
	dice := f.own(f.pattern("Dice", -1, 0, 0), -1)
	f.loadList() // keys 0 1 2 by position

	got := f.destroyItem(0, 1)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3C, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("the apple: % x", got)
	}
	if rows := f.rowsLeft(); len(rows) != 2 || rows[apple] != 0 || rows[rations] != 20 || rows[dice] != 1 {
		t.Fatalf("after the apple: %v", rows)
	}
	l := sessionInvList(f.sess)
	if ids := l.itemIDs(); len(ids) != 2 || ids[0] != rations || ids[1] != dice {
		t.Fatalf("the list after the apple: %v", ids)
	}

	// 5 of the 20 rations (key 1, now list index 0), then the other 15
	got = f.destroyItem(1, 5)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3C, 0x00, 0x00, 0x00, 0x0A}) {
		t.Fatalf("5 rations: % x", got)
	}
	if rows := f.rowsLeft(); rows[rations] != 15 {
		t.Fatalf("the stack after 5: %v", rows)
	}
	if rec, _ := l.at(0); rec.qty != 15 {
		t.Fatalf("the list's stack after 5: %d", rec.qty)
	}
	got = f.destroyItem(1, 15)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3C, 0x00, 0x00, 0x00, 0x1E}) {
		t.Fatalf("the other 15: % x", got)
	}
	if rows := f.rowsLeft(); len(rows) != 1 || rows[dice] != 1 {
		t.Fatalf("after the rations: %v", rows)
	}

	// the dice (key 2) are at list index 0 now
	got = f.destroyItem(2, 1)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3C, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("the dice: % x", got)
	}
	if rows := f.rowsLeft(); len(rows) != 0 || len(l.itemIDs()) != 0 {
		t.Fatalf("after the dice: %v, the list %v", rows, l.itemIDs())
	}
}

// A worn item comes off first (UnequipItem 0x0040 for its slot, before the
// 0x003C), as CLIENT's "Destroy equipped item?" box lets a player destroy
// one; a prepared item leaves its quick slot.
func TestDestroyAWornOrPreparedItem(t *testing.T) {
	f := newGearFixture(t)
	sword := f.own(f.pattern("Sword", 12, 1, 0), 16) // in the main hand
	potion := f.own(f.pattern("Potion", -1, 0, 0), -1)
	f.loadList()
	if err := f.inv.SetQuickSlot(f.sid, 0, potion); err != nil {
		t.Fatal(err)
	}

	got := f.destroyItem(0, 1)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{0x40, 0x00, 16}) ||
		!bytes.Equal(got[1], []byte{0x3C, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("the worn sword: % x", got)
	}
	if rows := f.rowsLeft(); len(rows) != 1 || rows[sword] != 0 {
		t.Fatalf("after the sword: %v", rows)
	}
	if w := f.worn(); len(w) != 0 {
		t.Fatalf("still worn after the sword: %v", w)
	}

	got = f.destroyItem(1, 1)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3C, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("the prepared potion: % x", got)
	}
	slots, err := f.inv.QuickSlotsOf(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Errorf("the potion's quick slot is still kept: %v", slots)
	}
}

// What the list does not hold is refused with 0x00D7 alone (it clears the
// client's held Destroy and shows nothing), and nothing changes: a key no
// record has, none, more than the stack holds.
func TestDestroyRefusesWhatIsNotHeld(t *testing.T) {
	f := newGearFixture(t)
	rations := f.stack(f.pattern("Ration", -1, 0, 0), 20)
	f.loadList()
	refused := []byte{0xD7, 0x00}
	for _, c := range []struct {
		name   string
		key, n uint32
	}{{"a key no record has", 7, 1}, {"none", 0, 0}, {"more than the stack", 0, 21}, {"a negative count", 0, 0xFFFFFFFF}} {
		got := f.destroyItem(c.key, c.n)
		if len(got) != 1 || !bytes.Equal(got[0], refused) {
			t.Errorf("%s: % x, want only 0x00D7", c.name, got)
		}
	}
	if rows := f.rowsLeft(); len(rows) != 1 || rows[rations] != 20 {
		t.Errorf("a refusal changed the inventory: %v", rows)
	}
	if rec, _ := sessionInvList(f.sess).at(0); rec.qty != 20 {
		t.Errorf("a refusal changed the list: %d", rec.qty)
	}
	// a short message is refused too
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRemoveInvItem)
	msg.SetData([]byte{1, 2, 3})
	if err := f.h.HandleDestroyItem(f.sess, msg); err != nil {
		t.Fatal(err)
	}
	if got := f.sess.Queue().GetPending(); len(got) != 1 || !bytes.Equal(got[0].Data(), refused) {
		t.Errorf("a short message: %d messages", len(got))
	}
}
