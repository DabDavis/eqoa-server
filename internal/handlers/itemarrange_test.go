package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// The item menu's Arrange (HandleArrangeItem): 0x003D {key held, key of the
// place} moves the held record to the other's list index, the ones between
// sliding along (VIItemList::Op003D, the client's own move on the answer),
// answered 0x003D {from, to}; the order is kept, so the next login's list
// (a MemoryDump's, newInvList over GetCharacterItems) is the same. Both ways
// along the list. The controls: a key no record has and a record onto itself
// answer nothing and move nothing.
func TestArrangeMovesARecordAndKeepsTheOrder(t *testing.T) {
	f := newGearFixture(t)
	var ids []int
	for _, name := range []string{"Apple", "Bone", "Cord", "Dice"} {
		ids = append(ids, f.own(f.pattern(name, -1, 0, 0), -1))
	}
	f.loadList() // keys 0-3 by position
	arrange := func(ka, kb uint32) [][]byte {
		return f.send(protocol.OpcodeArrangeItem, func(w *buffer.Writer) {
			w.WriteUint32(ka)
			w.WriteUint32(kb)
		})
	}
	order := func() []int {
		t.Helper()
		rows, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		var out []int
		for _, r := range rows {
			out = append(out, r.ItemID)
		}
		return out
	}
	same := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]

	// Apple (key 0, index 0) onto Cord's place (key 2, index 2)
	got := arrange(0, 2)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3D, 0x00, 0x00, 0x02}) {
		t.Fatalf("Apple onto Cord sent % x", got)
	}
	l := sessionInvList(f.sess)
	if want := []int{b, c, a, d}; !same(l.itemIDs(), want) || !same(order(), want) {
		t.Fatalf("after Apple onto Cord: the list %v, kept %v, want %v", l.itemIDs(), order(), want)
	}
	// and back along: Dice (key 3, index 3) onto Bone's place (key 1, index 0)
	got = arrange(3, 1)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{0x3D, 0x00, 0x03, 0x00}) {
		t.Fatalf("Dice onto Bone sent % x", got)
	}
	want := []int{d, b, c, a}
	if !same(l.itemIDs(), want) || !same(order(), want) {
		t.Fatalf("after Dice onto Bone: the list %v, kept %v, want %v", l.itemIDs(), order(), want)
	}
	// the next login lists them so
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if got := newInvList(rows).itemIDs(); !same(got, want) {
		t.Errorf("the next login's list %v, want %v", got, want)
	}

	// the controls
	if got := arrange(99, 1); len(got) != 0 {
		t.Errorf("a key no record has answered: % x", got)
	}
	if got := arrange(2, 2); len(got) != 0 {
		t.Errorf("a record onto itself answered: % x", got)
	}
	if !same(l.itemIDs(), want) || !same(order(), want) {
		t.Errorf("the controls moved something: the list %v, kept %v", l.itemIDs(), order())
	}
}
