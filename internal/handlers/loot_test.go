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
	f.hook()
	return f
}

// hook stands the World's corpse hooks in on the handler.
func (f *corpseFixture) hook() {
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

// A corpse that decays closes the window open on it, and is forgotten. (A
// second looter is refused while it is open: lootrights.go.)
func TestADecayedCorpseClosesItsWindows(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()

	f.h.CorpsesGone([]entity.EntityID{f.npc.ServerID})
	mustOps(t, f.sent(), protocol.OpcodeClientCloseLoot)
	ls := f.h.loots()
	if len(ls.open) != 0 || len(ls.since) != 0 || ls.corpses[f.npc.ServerID] != nil {
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
// id) opens it, as the reference server answers it (F3 20261026-042112: a creature shown
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

// A corpse's coins go to its first opener at once, as the reference server's did (a
// level 2 Orc Pawn's 2: 0x0052 with the purse, "You receive 2 tunar from the
// corpse.", then its items), kept; opened again it holds only its items. The
// roll is the live game's 1 + 0..level/4 (work board 658); a pet's corpse
// carries none.
func TestACorpsesTunarGoesToItsFirstOpener(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.npc.Level = 9
	f.h.coinRoll = func(n int) int {
		if n != 3 {
			t.Fatalf("rolled over %d, want 0..2 (level 9 / 4) above 1", n)
		}
		return 2
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
}

// Work board 658: an ordinary corpse holds 1 + 0..level/4 (a level 17's 1-5, a
// level 40's 1-11, a level 60's 1-16, levels 1-3 exactly 1); an animal's none;
// a named creature's its npccoins row's roll, an animal's too; a pet's none, a
// row or not. The negative control is the old rule's range: a level 40 never
// reaches 40.
func TestCorpseCoinsAreTheLiveGames(t *testing.T) {
	span := func(level int, named coinRange, wild bool) (lo, hi int) {
		lo, hi = 1<<30, -1
		for i := 0; i < 400; i++ {
			r := i
			n := corpseCoins(&entity.NPC{Level: level}, func(k int) int { return r % k }, named, wild)
			lo, hi = min(lo, n), max(hi, n)
		}
		return lo, hi
	}
	for _, c := range []struct{ level, lo, hi int }{{1, 1, 1}, {3, 1, 1}, {5, 1, 2}, {15, 1, 4}, {17, 1, 5}, {40, 1, 11}, {60, 1, 16}} {
		if lo, hi := span(c.level, coinRange{}, false); lo != c.lo || hi != c.hi {
			t.Errorf("level %d: %d-%d, want %d-%d", c.level, lo, hi, c.lo, c.hi)
		}
	}
	if lo, hi := span(40, coinRange{}, true); lo != 0 || hi != 0 {
		t.Errorf("a level 40 animal's corpse held %d-%d, want none", lo, hi)
	}
	if lo, hi := span(10, coinRange{214, 214, true}, false); lo != 214 || hi != 214 {
		t.Errorf("a 214..214 row gave %d-%d", lo, hi)
	}
	for _, r := range []func(int) int{func(int) int { return 0 }, func(k int) int { return k - 1 }} {
		n := corpseCoins(&entity.NPC{Level: 60}, r, coinRange{24388, 46040, true}, true)
		if n != 24388 && n != 46040 {
			t.Errorf("a named animal's row's ends gave %d", n)
		}
	}
	if n := corpseCoins(&entity.NPC{Level: 40, Owner: 1009}, nil, coinRange{214, 214, true}, false); n != 0 {
		t.Fatalf("a pet's corpse carried %d tunar", n)
	}
	if n := corpseCoins(&entity.NPC{Level: 40}, nil, coinRange{}, false); n < 1 || n > 11 {
		t.Fatalf("a level 40's corpse carried %d tunar, want 1 to 11", n)
	}
}

// The npccoins row is read at the corpse's first roll, by the name as npcclass
// keys it; the World's wildlife test keeps an animal's corpse empty.
func TestACorpseReadsItsNPCCoinsRow(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.npc.Name, f.npc.Level = "Lil'snapa", 12
	if _, err := f.db.Exec(`INSERT INTO npccoins (npc_name, coins_min, coins_max, source) VALUES ('lil''snapa', 214, 214, 'video-audit')`); err != nil {
		t.Fatal(err)
	}
	if n := f.h.corpseCoinsOf(f.npc, nil); n != 214 {
		t.Fatalf("Lil'snapa's corpse held %d, want 214", n)
	}
	f.h.isWild = func(*entity.NPC) bool { return true }
	rat := &entity.NPC{Name: "giant rat", Level: 20}
	if n := f.h.corpseCoinsOf(rat, nil); n != 0 {
		t.Fatalf("a rat's corpse held %d tunar", n)
	}
	f.h.isWild = func(*entity.NPC) bool { return false }
	orc := &entity.NPC{Name: "orc pawn", Level: 20}
	if n := f.h.corpseCoinsOf(orc, nil); n < 1 || n > 6 {
		t.Fatalf("an orc's corpse held %d tunar, want 1-6", n)
	}
}

// Petition 2590: a corpse opened is refreshed (the World's RefreshCorpse: it lies on from
// the open), its death's; one refused (another's kill, gone) is not.
func TestACorpseOpenedIsRefreshed(t *testing.T) {
	f := newCorpseFixture(t, 1)
	var refreshed []time.Time
	f.h.refreshCorpse = func(id entity.EntityID, died time.Time) bool {
		if id == f.npc.ServerID {
			refreshed = append(refreshed, died)
		}
		return true
	}
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	if len(refreshed) != 1 || !refreshed[0].Equal(f.npc.DeathTime) {
		t.Fatalf("opened: refreshed %v", refreshed)
	}
	f.send(protocol.OpcodeClientCloseLoot, nil)
	f.sent()
	f.lies = false
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	if len(refreshed) != 1 {
		t.Fatalf("a gone corpse refreshed: %v", refreshed)
	}
}

// A take is answered as the game answers it, 0x0019 {slot}: the window is left
// open as it is, its selection where it was (Heirloom petition 2616: the close
// and resend reopened it, and VIWndLoot::Open put the cursor back on the top
// row at every take). The last record taken, the window closes as before.
func TestATakeIsAnsweredInPlace(t *testing.T) {
	f := newCorpseFixture(t, 3)
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	take := func(key, n uint32) [][]byte {
		f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(key); w.WriteUint32(n) })
		return f.sent()
	}
	for want := 1; want <= 2; want++ {
		got := take(1, 1)
		if len(got) == 0 || opOf(got[0]) != protocol.OpcodeClientLoot {
			t.Fatalf("take %d: sent %s, want 0x0019 first", want, ops(got))
		}
		for _, m := range got {
			if op := opOf(m); op == protocol.OpcodeClientCloseLoot || op == protocol.OpcodeLoot || op == protocol.OpcodeAddInvItem {
				t.Fatalf("take %d: the window closed and was sent again: %s", want, ops(got))
			}
		}
		if got[0][2] != 1 {
			t.Fatalf("take %d: 0x0019 for slot %d, want 1", want, got[0][2])
		}
		if n := len(f.rows()); n != want {
			t.Fatalf("take %d: %d rows in the pack, want %d", want, n, want)
		}
	}
	// the last one: taken, and the window closes (the corpse goes)
	got := take(0, 1)
	if opOf(got[0]) != protocol.OpcodeAddInvItem || opOf(got[len(got)-1]) != protocol.OpcodeClientCloseLoot {
		t.Fatalf("emptied: sent %s, want 0x003B .. 0x0016", ops(got))
	}
	if len(f.cleared) != 1 || len(f.rows()) != 3 {
		t.Fatalf("emptied: cleared %v, %d rows", f.cleared, len(f.rows()))
	}
}

// A take the client's Msg0019 would not mirror is the close and resend: part of
// a stack (the client moves the whole record).
func TestAPartTakeIsResent(t *testing.T) {
	f := newCorpseFixture(t, 1)
	bait := f.bait
	if _, err := f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
		VALUES (?, 5, 100, ?, 1)`, corpseNPCID, bait); err != nil {
		t.Fatal(err)
	}
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(2) })
	got := f.sent()
	if opOf(got[0]) != protocol.OpcodeAddInvItem || opOf(got[len(got)-2]) != protocol.OpcodeClientCloseLoot ||
		opOf(got[len(got)-1]) != protocol.OpcodeLoot {
		t.Fatalf("2 of a stack of 5: sent %s, want 0x003B .. 0x0016 0x0018", ops(got))
	}
	// the whole stack, as CLIENT's LootOne asks: in place
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(3) })
	if got = f.sent(); opOf(got[0]) != protocol.OpcodeClientLoot {
		t.Fatalf("the rest of the stack: sent %s, want 0x0019", ops(got))
	}
}
