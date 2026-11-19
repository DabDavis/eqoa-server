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

// A shared drop (a season's token) is the kill group's, not one character's
// (2026-11-16: "lootable by anyone with kill credit, not a specific
// player. This is crushing multiboxers"): two credited characters' tokens are
// one stack on the corpse, listed after its own items and before a character's
// own drop; one character takes the whole stack, the other sees it gone; one
// outside the kill group still may not open the corpse; and the corpse goes
// with the last of its items, the shared and the own drops taken.
func TestASharedDropIsTheKillGroups(t *testing.T) {
	r := newRightsRig(t, 1)
	first := r.player.CharServerID
	second := first + 100 // the same player's other window
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: r.npc.DeathTime,
		Players: []entity.KillPlayer{{ServerID: first, Name: "First"}, {ServerID: second, Name: "Second"}}})
	r.h.AddSharedDrop(r.npc.ServerID, r.npc.DeathTime, r.dropOf(r.bait)) // the first's roll
	r.h.AddSharedDrop(r.npc.ServerID, r.npc.DeathTime, r.dropOf(r.bait)) // the second's
	r.h.AddQuestDrop(r.npc.ServerID, r.npc.DeathTime, first, r.dropOf(r.sword))

	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider's opening: 0x0085 %d, want %d", got, lootAnotherOwner)
	}

	// the second character takes both tokens: they are the second record, one stack
	r.player.CharServerID = second
	r.h.OpenCorpse(r.sess, r.npc)
	if n := lootCount(t, r.sent()[0]); n != 2 {
		t.Fatalf("the second character sees %d records, want the corpse's 1 and the tokens", n)
	}
	r.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(2) })
	r.sent()
	if r.heldOf(r.bait) != 2 {
		t.Fatalf("the second character holds %d tokens, want both", r.heldOf(r.bait))
	}
	r.h.HandleLootClose(r.sess, nil)
	r.sent()

	// the first sees no token left, and its own drop is still its own, after
	// the corpse's item
	r.player.CharServerID = first
	r.sess.SetInventoryList(newInvList(nil))
	r.h.OpenCorpse(r.sess, r.npc)
	if n := lootCount(t, r.sent()[0]); n != 2 {
		t.Fatalf("the first character sees %d records, want the corpse's 1 and its own drop", n)
	}
	r.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(1) })
	r.sent()
	if r.heldOf(r.bait) != 0 || r.heldOf(r.sword) != 1 {
		t.Fatalf("the first character holds bait %d sword %d, want its own drop alone", r.heldOf(r.bait), r.heldOf(r.sword))
	}
	if len(r.cleared) != 0 {
		t.Fatal("the corpse went with its own item on it")
	}
	r.send(protocol.OpcodeLootTakeAll, nil)
	r.sent()
	if len(r.cleared) != 1 {
		t.Fatalf("everything taken, the corpse did not go: cleared %v", r.cleared)
	}
}

// The control: a shared drop left on the corpse keeps it, and the take of a
// character's own drop after a shared stack takes that one (the positions:
// the corpse's items, the shared drops, its own).
func TestASharedDropKeepsTheCorpse(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.h.AddSharedDrop(f.npc.ServerID, f.npc.DeathTime, f.dropOf(f.bait))
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.sword))
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, sharedDrops, f.dropOf(f.sword)) // no character: refused
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 2 {
		t.Fatalf("the looter sees %d records, want the token and its own drop", n)
	}
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(1) })
	got := f.sent()
	if f.heldOf(f.sword) != 1 || f.heldOf(f.bait) != 0 {
		t.Fatalf("the second record taken: sword %d bait %d, want its own drop", f.heldOf(f.sword), f.heldOf(f.bait))
	}
	if lootCount(t, lastOp(t, got, protocol.OpcodeLoot)) != 1 {
		t.Fatalf("after its own drop: %s, want the token left", ops(got))
	}
	f.send(protocol.OpcodeClientCloseLoot, nil)
	f.sent()
	if len(f.cleared) != 0 {
		t.Fatal("the corpse went with a token on it")
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

// A drop ahead of its step (questpreloot.go) keeps no corpse (Live petition
// 5118, a player: ant soldiers by Klick lay on, empty to him, holding Ant
// Chitin dropped ahead for his other characters): the corpse's own item
// taken, the corpse goes with another character's drop ahead still on it.
func TestADropAheadKeepsNoCorpse(t *testing.T) {
	f := newCorpseFixture(t, 1)
	other := uint32(f.charSID + 100)
	f.h.AddQuestDropAhead(f.npc.ServerID, f.npc.DeathTime, other, f.dropOf(f.bait))
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 1 {
		t.Fatalf("the looter sees %d records, want its corpse's 1 alone", n)
	}
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	f.sent()
	if len(f.cleared) != 1 {
		t.Fatalf("emptied but for another's drop ahead, the corpse did not go: cleared %v", f.cleared)
	}
}

// The control: the other character's drop ahead lies on its view while the
// corpse does, and an on-step drop beside it still keeps the corpse.
func TestADropAheadIsTakenWhileTheCorpseLies(t *testing.T) {
	f := newCorpseFixture(t, 1)
	other := uint32(f.charSID + 100)
	f.h.AddQuestDrop(f.npc.ServerID, f.npc.DeathTime, other, f.dropOf(f.bait))
	f.h.AddQuestDropAhead(f.npc.ServerID, f.npc.DeathTime, f.charSID, f.dropOf(f.bait))
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 2 {
		t.Fatalf("the looter sees %d records, want its corpse's 1 and its drop ahead", n)
	}
	f.send(protocol.OpcodeLootTakeAll, nil)
	f.sent()
	if f.heldOf(f.bait) != 1 {
		t.Fatalf("the drop ahead not taken: bait %d", f.heldOf(f.bait))
	}
	if len(f.cleared) != 0 {
		t.Fatal("the corpse went with another character's on-step drop on it")
	}
}
