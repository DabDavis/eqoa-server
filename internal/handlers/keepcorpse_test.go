package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What a corpse holds through a restart (keep.go's KeptCorpse; entity/keepnpc.go
// carries the death). A corpse someone opened keeps what is left on it and is not
// rolled again; one no one opened is rolled at its first opening, as it would have
// been; a loot step's drops stay their characters' own. "The next process" here is
// a second handler and its corpses, over rows that would roll the whole corpse
// again, so a carried corpse and a rolled one differ.

// nextCorpses is the next process's handler for f's corpse, over f's database (making
// one is the slow part of a fixture) and through a file: the same rows, the same death,
// given what was kept when restore. Its session and player are its own; what f's took is
// still in the database, so its character's holdings count on from them.
func nextCorpses(t *testing.T, f *corpseFixture, restore bool) (*corpseFixture, int) {
	t.Helper()
	prev := keepWindows.Swap(f.h)
	b, err := json.Marshal(KeepCorpses())
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptCorpse
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	shop := *f.shopFixture
	player := *f.player
	shop.player = &player
	shop.sess = testSession()
	shop.h = NewNPCWindowHandler(nil, f.inv)
	shop.h.playerOf = func(*session.Session) *entity.Player { return shop.player }
	shop.h.find = f.h.find
	shop.loadLists()
	npc := *f.npc
	g := &corpseFixture{shopFixture: &shop, npc: &npc, lies: true}
	// a death's time through a file, as the World's comes back
	raw, _ := json.Marshal(f.npc.DeathTime)
	if err := json.Unmarshal(raw, &g.npc.DeathTime); err != nil {
		t.Fatal(err)
	}
	g.hook()
	keepWindows.Store(g.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	n := 0
	if restore {
		n = RestoreCorpses(back)
	}
	return g, n
}

func TestACorpseOpenedIsNotRolledAgain(t *testing.T) {
	f := newCorpseFixture(t, 3)
	f.npc.Level = 5 // it carries coins (corpsecoins.go)
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	f.sent()
	if f.heldOf(f.sword) != 1 {
		t.Fatalf("took %d", f.heldOf(f.sword))
	}

	g, n := nextCorpses(t, f, true)
	if n != 1 {
		t.Fatalf("%d corpses back", n)
	}
	tunar := g.player.Tunar
	g.h.OpenCorpse(g.sess, g.npc)
	got := g.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 2 {
		t.Fatalf("%d records left on it, want the 2 that were", n)
	}
	if g.player.Tunar != tunar {
		t.Fatalf("its coins paid twice: %d, was %d", g.player.Tunar, tunar)
	}
	// and it goes on as any corpse: both taken, it goes
	g.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	g.sent()
	g.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	g.sent()
	if g.heldOf(g.sword) != 3 || len(g.cleared) != 1 {
		t.Fatalf("after taking the rest: holds %d, cleared %v", g.heldOf(g.sword), g.cleared)
	}

	// the control: forgotten, its next opener rolls all three again, and the coins
	c, _ := nextCorpses(t, f, false)
	tunar = c.player.Tunar
	c.h.OpenCorpse(c.sess, c.npc)
	if got := c.sent(); lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 3 || c.player.Tunar == tunar {
		t.Fatalf("control: %s, tunar %d was %d", ops(got), c.player.Tunar, tunar)
	}
}

// lastOp is the last message of opcode op among msgs.
func lastOp(t *testing.T, msgs [][]byte, op protocol.GameOpcode) []byte {
	t.Helper()
	for i := len(msgs) - 1; i >= 0; i-- {
		if opOf(msgs[i]) == op {
			return msgs[i]
		}
	}
	t.Fatalf("no 0x%04X in %s", uint16(op), ops(msgs))
	return nil
}

// A corpse emptied with its window still open (its next closing would make it go)
// holds nothing after the restart too.
func TestACorpseEmptiedStaysEmpty(t *testing.T) {
	f := newCorpseFixture(t, 1)
	other := testSession()
	f.h.OpenCorpse(other, f.npc)
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	ls := f.h.loots()
	ls.corpses[f.npc.ServerID].items = nil // another looter took it, and the World has it lying yet
	g, n := nextCorpses(t, f, true)
	if n != 1 {
		t.Fatalf("%d corpses back", n)
	}
	g.h.OpenCorpse(g.sess, g.npc)
	got := g.sent()
	if lootCount(t, got[0]) != 0 {
		t.Fatalf("an emptied corpse holds %d records", lootCount(t, got[0]))
	}
	g.send(protocol.OpcodeClientCloseLoot, nil)
	if len(g.cleared) != 1 {
		t.Fatal("closed empty, it did not go")
	}
}

// A corpse no one opened has nothing to carry: nothing is written, and the first to
// open it after the restart gets its roll and its coins.
func TestACorpseNotOpenedIsRolledAtItsFirstOpening(t *testing.T) {
	f := newCorpseFixture(t, 2)
	f.npc.Level = 5 // it carries coins (corpsecoins.go)
	prev := keepWindows.Swap(f.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	if got := KeepCorpses(); len(got) != 0 {
		t.Fatalf("written %+v", got)
	}
	g, n := nextCorpses(t, f, true)
	if n != 0 {
		t.Fatalf("%d back", n)
	}
	tunar := g.player.Tunar
	g.h.OpenCorpse(g.sess, g.npc)
	got := g.sent()
	if lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 2 || g.player.Tunar == tunar {
		t.Fatalf("%s, tunar %d was %d", ops(got), g.player.Tunar, tunar)
	}
}

// A loot step's drop lies on its character's view from the kill, opened or not, and
// stays there: another character's view does not get it.
func TestAQuestDropComesThrough(t *testing.T) {
	f := newCorpseFixture(t, 1)
	other := uint32(f.charSID + 100)
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.bait))
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, other, f.dropOf(f.bait))

	g, n := nextCorpses(t, f, true)
	if n != 1 {
		t.Fatalf("%d back", n)
	}
	g.player.CharServerID = f.charSID
	g.h.OpenCorpse(g.sess, g.npc)
	got := g.sent()
	if lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 2 {
		t.Fatalf("%d records, want its own 1 (rolled now) and the drop", lootCount(t, lastOp(t, got, protocol.OpcodeLoot)))
	}
	g.send(protocol.OpcodeClientCloseLoot, nil)
	g.sent()

	g.player.CharServerID = other
	g.h.OpenCorpse(g.sess, g.npc)
	if got := g.sent(); lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 2 {
		t.Fatalf("the other's view: %d records", lootCount(t, lastOp(t, got, protocol.OpcodeLoot)))
	}
	g.send(protocol.OpcodeClientCloseLoot, nil)
	g.sent()

	g.player.CharServerID = other + 1
	g.h.OpenCorpse(g.sess, g.npc)
	if got := g.sent(); lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 1 {
		t.Fatalf("a third's view: %d records, want no drop", lootCount(t, lastOp(t, got, protocol.OpcodeLoot)))
	}

	// the control
	c, _ := nextCorpses(t, f, false)
	c.player.CharServerID = f.charSID
	c.h.OpenCorpse(c.sess, c.npc)
	if got := c.sent(); lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 1 {
		t.Fatalf("control: the drop is there with nothing given back")
	}
}

// What the World does not have dead of the same death (its row gone, or made another
// creature by a data step; or a corpse decayed in the hold) is passed over.
func TestACorpseWhoseCreatureIsNotDeadIsPassedOver(t *testing.T) {
	f := newCorpseFixture(t, 2)
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()

	g, _ := nextCorpses(t, f, false)
	prev := keepWindows.Swap(f.h)
	kept := KeepCorpses()
	keepWindows.Store(g.h)
	t.Cleanup(func() { keepWindows.Store(prev) })

	g.lies = false
	if n := RestoreCorpses(kept); n != 0 {
		t.Fatalf("no corpse lies, %d back", n)
	}
	g.lies = true
	g.npc.DeathTime = g.npc.DeathTime.Add(time.Second)
	if n := RestoreCorpses(kept); n != 0 {
		t.Fatalf("another death's corpse, %d back", n)
	}
	if c := g.h.loots().corpses[g.npc.ServerID]; c != nil {
		t.Fatalf("kept %+v", c)
	}
	g.npc.DeathTime = g.npc.DeathTime.Add(-time.Second)
	if n := RestoreCorpses(kept); n != 1 {
		t.Fatalf("its corpse, %d back", n)
	}
}

// A server with no window handler (a test's) keeps none, and takes none.
func TestNoWindowsNoCorpses(t *testing.T) {
	prev := keepWindows.Swap(nil)
	t.Cleanup(func() { keepWindows.Store(prev) })
	if got := KeepCorpses(); got != nil {
		t.Fatalf("%+v", got)
	}
	if n := RestoreCorpses([]KeptCorpse{{ID: 1, Opened: true}}); n != 0 {
		t.Fatalf("%d back", n)
	}
}
