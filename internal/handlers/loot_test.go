package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// A corpse's window against the World's corpse rules (entity/corpse.go,
// F3 20261025-044229): one with nothing on it opens as the game's "Empty",
// emptied and closed it goes (ClearCorpse), one left with items does not,
// one gone answers "That entity is not lootable", and a corpse that decays
// closes the windows open on it.

const corpseNPCID = 21 // its npcs.id: its npcitem rows are its loot

// corpseFixture is the shop fixture with a dead NPC and the World's corpse
// hooks stood in for: whether it lies, and the clears asked for.
type corpseFixture struct {
	*shopFixture
	npc     *entity.NPC
	lies    bool
	cleared []entity.EntityID
}

func newCorpseFixture(t *testing.T, items int) *corpseFixture {
	f := &corpseFixture{shopFixture: newShopFixture(t), lies: true}
	f.npc = &entity.NPC{ServerID: 5000, Name: "beetle", NPCID: corpseNPCID, Dead: true, DeathTime: time.Now()}
	for i := 0; i < items; i++ {
		if _, err := f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
			VALUES (?, 1, 100, ?, ?)`, corpseNPCID, f.sword, i); err != nil {
			t.Fatal(err)
		}
	}
	f.h.corpseLies = func(id entity.EntityID) (time.Time, bool) {
		return f.npc.DeathTime, f.lies && id == f.npc.ServerID
	}
	f.h.clearCorpse = func(id entity.EntityID, died time.Time) bool {
		if !f.lies || !died.Equal(f.npc.DeathTime) {
			return false
		}
		f.lies = false
		f.cleared = append(f.cleared, id)
		return true
	}
	return f
}

// lootCount is a 0x0018's record count (its zigzag after the opcode).
func lootCount(t *testing.T, m []byte) int64 {
	t.Helper()
	n, err := buffer.NewReader(m[2:]).Read7BitEncodedInt64()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnEmptyCorpseOpensAndGoesWhenClosed(t *testing.T) {
	f := newCorpseFixture(t, 0)
	if !f.h.OpenCorpse(f.sess, f.npc) {
		t.Fatal("a corpse was not taken as one")
	}
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 0 {
		t.Fatalf("an empty corpse sent %d records", n)
	}
	if len(f.cleared) != 0 {
		t.Fatal("cleared before it was closed")
	}
	f.send(protocol.OpcodeClientCloseLoot, nil)
	mustOps(t, f.sent(), protocol.OpcodeClientCloseLoot)
	if len(f.cleared) != 1 || f.cleared[0] != f.npc.ServerID {
		t.Fatalf("closed empty, cleared %v; want the corpse", f.cleared)
	}

	// gone: not opened again, "That entity is not lootable"
	f.h.OpenCorpse(f.sess, f.npc)
	got = f.sent()
	mustOps(t, got, protocol.OpcodeLootOptions)
	if got[0][2] != lootNotLootable {
		t.Fatalf("a gone corpse answered 0x0085 %d", got[0][2])
	}
}

// Closed with something left on it, a corpse stays for the next looter, and
// it goes once a take leaves it empty (the window closes then too).
func TestACorpseWithItemsGoesOnlyEmptied(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 1 {
		t.Fatalf("%d records, want 1", n)
	}
	f.send(protocol.OpcodeClientCloseLoot, nil)
	f.sent()
	if len(f.cleared) != 0 {
		t.Fatal("closed with an item on it, the corpse went")
	}

	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	got = f.sent()
	if len(f.cleared) != 1 {
		t.Fatalf("emptied by a take (%s), cleared %v", ops(got), f.cleared)
	}
	if opOf(got[len(got)-1]) != protocol.OpcodeClientCloseLoot {
		t.Fatalf("emptied, the window was not closed: %s", ops(got))
	}
}

// A corpse that decays closes every window open on it, and is forgotten.
func TestADecayedCorpseClosesItsWindows(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.h.OpenCorpse(f.sess, f.npc)
	other := testSession()
	f.h.OpenCorpse(other, f.npc)
	f.sent()
	other.Queue().GetPending()

	f.h.CorpsesGone([]entity.EntityID{f.npc.ServerID})
	mustOps(t, f.sent(), protocol.OpcodeClientCloseLoot)
	var others []byte
	for _, m := range other.Queue().GetPending() {
		others = append(others, m.Data()[:2]...)
	}
	if len(others) != 2 || opOf(others) != protocol.OpcodeClientCloseLoot {
		t.Fatalf("the other looter was sent % x", others)
	}
	ls := f.h.loots()
	if len(ls.open) != 0 || ls.corpses[f.npc.ServerID] != nil {
		t.Fatalf("a decayed corpse is remembered: %d open, record %v", len(ls.open), ls.corpses[f.npc.ServerID])
	}
}

// A pet's corpse is no one's to loot (entity/pet.go): "That entity is not
// lootable", no window. The control: the same corpse unowned opens.
func TestAPetsCorpseIsNotLootable(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.npc.Owner = 1001
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLootOptions)
	if got[0][2] != lootNotLootable {
		t.Fatalf("a pet's corpse answered 0x0085 %d", got[0][2])
	}
	f.npc.Owner = 0
	f.h.OpenCorpse(f.sess, f.npc)
	mustOps(t, f.sent(), protocol.OpcodeLoot)
}

// The attack button on a corpse (CLIENT's ToggleAutoAttack, 0x0015 with its
// id) opens it, as Sandstorm answers it (F3 20261026-042112: a creature shown
// as attackable could not be looted at all); a creature alive is no corpse
// (0x0085 2), and one out of reach is refused in red, nothing opened.
func TestTheAttackButtonOpensACorpse(t *testing.T) {
	f := newCorpseFixture(t, 1)
	const id = 77
	f.npcs[id] = f.npc
	lootIt := func() { f.send(protocol.OpcodeLootCorpse, func(w *buffer.Writer) { w.WriteUint32(id) }) }

	lootIt()
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 1 {
		t.Fatalf("%d records, want the corpse's 1", n)
	}

	f.send(protocol.OpcodeClientCloseLoot, nil)
	f.sent()
	f.npc.Dead = false
	lootIt()
	got = f.sent()
	mustOps(t, got, protocol.OpcodeLootOptions)
	if got[0][2] != lootNotLootable {
		t.Fatalf("a live creature answered 0x0085 %d", got[0][2])
	}

	f.npc.Dead = true
	f.player.X = entity.InteractRange + 1
	lootIt()
	got = f.sent()
	mustOps(t, got, protocol.OpcodeColoredChat)
	f.player.X = entity.InteractRange - 1
	lootIt()
	mustOps(t, f.sent(), protocol.OpcodeLoot)
}

// A corpse's coins go to its first opener at once, as Sandstorm's did (a
// level 2 Orc Pawn's 2: 0x0052 with the purse, "You receive 2 tunar from the
// corpse.", then its items), kept; opened again it holds only its items. The
// roll is the creature's level to twice it; a pet's corpse carries none.
func TestACorpsesTunarGoesToItsFirstOpener(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.npc.Level = 2
	f.h.coinRoll = func(n int) int {
		if n != 3 {
			t.Fatalf("rolled over %d, want 0..2 above the level", n)
		}
		return 1
	}
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodePlayerTunar, protocol.OpcodeClientMessage, protocol.OpcodeLoot)
	if purse, _ := buffer.NewReader(got[0][2:]).Read7BitEncodedInt64(); purse != 565 || f.player.Tunar != 565 {
		t.Fatalf("purse %d (player %d), want 562 + 3", purse, f.player.Tunar)
	}
	if tunar, _ := f.storedTunar(); tunar != 565 {
		t.Fatalf("kept %d tunar, want 565", tunar)
	}
	if line, _ := readWideLine(got[1][2:]); line != "You receive 3 tunar from the corpse." {
		t.Fatalf("said %q", line)
	}

	f.send(protocol.OpcodeClientCloseLoot, nil)
	f.sent()
	f.h.OpenCorpse(f.sess, f.npc)
	mustOps(t, f.sent(), protocol.OpcodeLoot) // the coins gone with the first

	if n := corpseCoins(&entity.NPC{Level: 40, Owner: 1009}, nil); n != 0 {
		t.Fatalf("a pet's corpse carried %d tunar", n)
	}
	if n := corpseCoins(&entity.NPC{Level: 40}, nil); n < 40 || n > 80 {
		t.Fatalf("a level 40's corpse carried %d tunar, want 40 to 80", n)
	}
}
