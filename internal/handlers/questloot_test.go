package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// dropOf is a corpse record of a pattern, as a loot step drops one.
func (f *corpseFixture) dropOf(pattern int) database.CharInventoryEntry {
	f.t.Helper()
	pat, err := database.NewItemPatternStore(f.db).GetItemPattern(pattern)
	if err != nil || pat == nil {
		f.t.Fatalf("pattern %d: %v", pattern, err)
	}
	return database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat}
}

// A loot step's drop lies on its own character's view of the corpse, after
// the corpse's items: listed and taken there, never in another's view; the
// corpse goes only when its own items and every drop are gone.
func TestAQuestDropIsItsLootersOwn(t *testing.T) {
	f := newCorpseFixture(t, 1)
	other := uint32(f.charSID + 100)
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.bait))
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, other, f.dropOf(f.bait))

	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 2 {
		t.Fatalf("the looter sees %d records, want its corpse's 1 and its own drop", n)
	}

	// the drop is the second record: taken into the inventory
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(1) })
	f.sent()
	if f.heldOf(f.bait) != 1 || f.heldOf(f.sword) != 0 {
		t.Fatalf("after taking the drop: bait %d sword %d, want 1 0", f.heldOf(f.bait), f.heldOf(f.sword))
	}
	// the corpse's own item, now the only record: the window empties, but
	// the other character's drop keeps the corpse
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	got = f.sent()
	if opOf(got[len(got)-1]) != protocol.OpcodeClientCloseLoot {
		t.Fatalf("emptied for the looter, its window was not closed: %s", ops(got))
	}
	if len(f.cleared) != 0 {
		t.Fatal("the corpse went with another character's drop on it")
	}

	// the control: the other character takes its drop, and the corpse goes
	f.player.CharServerID = other
	f.sess.SetInventoryList(newInvList(nil))
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 1 {
		t.Fatalf("the other character sees %d records, want its drop alone", n)
	}
	f.send(protocol.OpcodeLootTakeAll, nil)
	f.sent()
	if len(f.cleared) != 1 {
		t.Fatalf("every drop taken, the corpse did not go: cleared %v", f.cleared)
	}
}

// A drop of another death (the creature dead again) is not on this corpse,
// and a corpse forgotten forgets its drops.
func TestAQuestDropIsOneDeaths(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime.Add(-60e9), f.charSID, f.dropOf(f.bait))
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 0 {
		t.Fatalf("an earlier death's drop shows: %d records", n)
	}
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.bait))
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.bait))
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	if n := lootCount(t, got[0]); n != 1 {
		t.Fatalf("two drops of one item are %d records, want one stack", n)
	}
	f.h.CorpsesGone([]entity.EntityID{f.npc.ServerID})
	if f.h.loots().drops[f.npc.ServerID] != nil {
		t.Fatal("a gone corpse's drops are remembered")
	}
}

// heldOf is how many of a pattern the fixture's character holds.
func (f *corpseFixture) heldOf(pattern int) int {
	f.t.Helper()
	var n int
	f.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`,
		f.player.CharServerID, pattern).Scan(&n)
	return n
}
