package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// A take the pack has no room for is answered CLIENT's own "no room" line and
// leaves the window as it is (petitions 1026, 1030: Arcana's loot window
// "flashes instead of throwing the full inventory message": it was closed and
// sent again, unchanged, with no word); a take-all that takes nothing the
// same. The control: with a place free, the take goes in and the window is
// sent again.
func TestATakeWithNoRoomSaysSoAndLeavesTheWindow(t *testing.T) {
	f := newCorpseFixture(t, 1)
	for i := 0; i < 40; i++ {
		f.own(f.sword, 1, 100, 0)
	}
	f.loadLists()
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()

	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	got := f.sent()
	mustOps(t, got, protocol.OpcodeClientMessage)
	f.send(protocol.OpcodeLootTakeAll, nil)
	got = f.sent()
	mustOps(t, got, protocol.OpcodeLootTakeAll, protocol.OpcodeClientMessage)
	if n := len(f.rows()); n != 40 {
		t.Fatalf("%d rows, want the 40 it held", n)
	}

	// a place free: taken, the window sent again (and emptied, closed)
	if _, err := f.db.Exec(`DELETE FROM charinventory WHERE itemid = (SELECT max(itemid) FROM charinventory)`); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	got = f.sent()
	if len(got) < 2 || opOf(got[0]) != protocol.OpcodeAddInvItem || opOf(got[len(got)-1]) != protocol.OpcodeClientCloseLoot {
		t.Fatalf("with room: %s, want the item added and the window closed (emptied)", ops(got))
	}
}
