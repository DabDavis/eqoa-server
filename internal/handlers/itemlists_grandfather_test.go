package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Grandfathered lists (entity/itemslots.go; the server owner, 2026-11-18, of
// Vanilla Test: "limit bank and inventory slots back to 40 each", those past
// it keeping what they hold): pack_slots and bank_slots 40, grandfathered at
// the old 100.

func grandfather(t *testing.T) {
	t.Helper()
	entity.SetItemSlots(40, 40)
	entity.SetItemSlotsGrandfathered(100, 100)
	t.Cleanup(func() {
		entity.SetItemSlots(0, 0)
		entity.SetItemSlotsGrandfathered(0, 0)
	})
}

// sword is one more unworn single sword offered to a list: whether it took it.
func takesOne(l *invList, pattern int) bool {
	_, took := l.add(990000, pattern, 1, 1)
	return took == 1
}

// A character at 95 keeps all 95, sent whole to vanilla in a list of 95; it
// takes no new record (loot, a purchase, a give, a craft, a trade: add,
// wouldAdd, roomFor); one dropped is gone (94, still full), and its next dump
// is a list of 94. A stack still tops up. The control: the same character
// with the limit at 100 and nothing grandfathered takes up to 100.
func TestItemListsGrandfatheredKeepsWhatItHolds(t *testing.T) {
	f := newListsWorld(t)
	grandfather(t)
	ids := f.own(f.sword, 94, database.LocPack)
	f.own(f.gem, 1, database.LocPack) // a stack of 1 of 20: the 95th record

	sess := f.session(100, 100)
	if p, b := listCaps(sess); p != 100 || b != 100 {
		t.Fatalf("a vanilla client offered the grandfathered 100: %d %d", p, b)
	}
	dump, _ := f.dump(sess, true)
	dumpedLists(t, dump, 95, 0)
	if rows := f.rowsAt(); len(rows[database.LocPackKept]) != 0 {
		t.Fatalf("%d kept from a client that holds them", len(rows[database.LocPackKept]))
	}
	l := sessionInvList(sess)
	if l.capacity() != 95 || len(l.recs) != 95 {
		t.Fatalf("the list: cap %d, %d records", l.capacity(), len(l.recs))
	}
	if takesOne(l, f.sword) || l.wouldAdd(f.sword, 1, 1) != 0 ||
		len(l.roomFor(nil, []invGive{{patternID: f.sword, n: 1, maxStack: 1}})) != 1 || packCapOf(sess) > 95 {
		t.Error("a pack of 95 took a 96th")
	}
	if l.wouldAdd(f.gem, 5, 20) != 5 {
		t.Error("a stack held did not top up")
	}

	// drops one: 94, and still nothing new
	if !l.took(0, 1) {
		t.Fatal("took")
	}
	if err := f.inv.DeleteCharItem(f.sid, ids[0]); err != nil {
		t.Fatal(err)
	}
	if takesOne(l, f.sword) || l.wouldAdd(f.sword, 1, 1) != 0 {
		t.Error("the slot freed above 40 was filled again")
	}
	// even a trade's swap: one given for one taken is a new record at 93
	if short := l.roomFor([]invTake{{itemID: ids[1], n: 1}}, []invGive{{patternID: f.sword, n: 1, maxStack: 1}}); len(short) != 1 {
		t.Error("a swap filled a freed slot above 40")
	}
	// the next dump (a login, after a restart): a list of 94
	sess2 := f.session(100, 100)
	dump, _ = f.dump(sess2, true)
	dumpedLists(t, dump, 94, 0)
	if l := sessionInvList(sess2); l == nil || l.capacity() != 94 || takesOne(l, f.sword) {
		t.Errorf("the next login's list: %+v", l)
	}

	// the control: not grandfathered, the server's 100
	g := newListsWorld(t)
	entity.SetItemSlots(100, 100)
	entity.SetItemSlotsGrandfathered(0, 0)
	g.own(g.sword, 95, database.LocPack)
	s := g.session(100, 100)
	g.dump(s, true)
	l = sessionInvList(s)
	n := 0
	for takesOne(l, g.sword) {
		n++
	}
	if n != 5 {
		t.Errorf("the control took %d, want 5", n)
	}
}

// A character at 30 stops at 40 (its list told 40, not 100), and a bank the
// same; the control is the server's 100 with nothing grandfathered.
func TestItemListsGrandfatheredUnderTheLimitStopsAtIt(t *testing.T) {
	f := newListsWorld(t)
	grandfather(t)
	f.own(f.sword, 30, database.LocPack)
	f.own(f.gem, 12, database.LocBank)
	sess := f.session(100, 100)
	dump, _ := f.dump(sess, true)
	dumpedLists(t, dump, 30, 12)
	l, b := sessionInvList(sess), sessionBankList(sess)
	if l.capacity() != 40 || b.capacity() != 40 {
		t.Fatalf("lists of %d and %d, want 40", l.capacity(), b.capacity())
	}
	n := 0
	for takesOne(l, f.sword) {
		n++
	}
	if n != 10 {
		t.Errorf("the pack took %d, want 10", n)
	}
	if short := l.roomFor(nil, []invGive{{patternID: f.sword, n: 1, maxStack: 1}}); len(short) != 1 {
		t.Error("roomFor past 40")
	}
	n = 0
	for takesOne(b, f.sword) {
		n++
	}
	if n != 28 {
		t.Errorf("the bank took %d, want 28", n)
	}
	// the auction files into the bank as its room says
	if bankCapOf(sess) != 40 {
		t.Errorf("the bank's room for the auction: %d", bankCapOf(sess))
	}
	// the login's gifts and a transfer: 40
	if r := sessionListSizes(sess); r.packOf() != 40 || r.bankOf() != 40 {
		t.Errorf("the login's room: %+v", r)
	}
}

// A PS2 is sent 40 as always and the rest kept; a grandfathered character
// there takes nothing new (the kept count), and back on vanilla is sent all
// it holds. The control: a PS2's 40 under the limit takes up to 40.
func TestItemListsGrandfatheredOnAPS2(t *testing.T) {
	f := newListsWorld(t)
	grandfather(t)
	ids := f.own(f.sword, 95, database.LocPack)
	ps2 := f.session(0, 0)
	if p, b := listCaps(ps2); p != 40 || b != 40 {
		t.Fatalf("a PS2: %d %d", p, b)
	}
	dump, _ := f.dump(ps2, true)
	dumpedLists(t, dump, 40, 0)
	rows := f.rowsAt()
	if len(rows[database.LocPack]) != 40 || len(rows[database.LocPackKept]) != 55 {
		t.Fatalf("pack %d kept %d", len(rows[database.LocPack]), len(rows[database.LocPackKept]))
	}
	l := sessionInvList(ps2)
	if l.capacity() != 40 || l.keptN != 55 {
		t.Fatalf("the PS2's list: cap %d kept %d", l.capacity(), l.keptN)
	}
	l.took(0, 1)
	if err := f.inv.DeleteCharItem(f.sid, ids[0]); err != nil {
		t.Fatal(err)
	}
	if takesOne(l, f.sword) {
		t.Error("a PS2 with 94 held filled a freed slot")
	}
	// back on vanilla: all 94
	dump, _ = f.dump(f.session(100, 100), true)
	dumpedLists(t, dump, 94, 0)
	if rows := f.rowsAt(); len(rows[database.LocPackKept]) != 0 {
		t.Errorf("%d still kept", len(rows[database.LocPackKept]))
	}

	// the control
	g := newListsWorld(t)
	g.own(g.sword, 30, database.LocPack)
	s := g.session(0, 0)
	g.dump(s, true)
	n := 0
	for takesOne(sessionInvList(s), g.sword) {
		n++
	}
	if n != 10 {
		t.Errorf("a PS2 at 30 took %d, want 10", n)
	}
}

// Unset (Heirloom), or not above the limit: off, every size as without it.
func TestItemListsGrandfatherOff(t *testing.T) {
	t.Cleanup(func() {
		entity.SetItemSlots(0, 0)
		entity.SetItemSlotsGrandfathered(0, 0)
	})
	for _, g := range [][2]int{{0, 0}, {100, 100}, {60, 60}} {
		entity.SetItemSlots(100, 100)
		entity.SetItemSlotsGrandfathered(g[0], g[1])
		if p, b := entity.ItemSlotsGrandfathered(); p != 0 || b != 0 {
			t.Errorf("%v: grandfathered %d %d", g, p, b)
		}
		if p, b := entity.ItemListsMost(); p != 100 || b != 100 {
			t.Errorf("%v: offered %d %d", g, p, b)
		}
		l := newInvListCap(nil, 100)
		grandfatherList(l, false, 3)
		if l.most != 0 || l.roomBound() != 100 {
			t.Errorf("%v: a list grandfathered: %d", g, l.most)
		}
		rows := []database.ItemListRow{{Location: database.LocPack}}
		if p, b := characterListSizes(rows, 100, 100); p != 100 || b != 100 {
			t.Errorf("%v: sizes %d %d", g, p, b)
		}
	}
	// a server of 40 with nothing grandfathered offers nothing
	entity.SetItemSlots(40, 40)
	entity.SetItemSlotsGrandfathered(0, 0)
	if entity.ItemSlotsOffered() {
		t.Error("a server of 40 offered")
	}
	entity.SetItemSlotsGrandfathered(100, 0)
	if !entity.ItemSlotsOffered() {
		t.Error("a grandfathered pack not offered")
	}
	if p, b := entity.ItemListsMost(); p != 100 || b != 40 {
		t.Errorf("offered %d %d", p, b)
	}
}

// A keep restart carries a grandfathered list's limit and kept count.
func TestKeptListCarriesGrandfather(t *testing.T) {
	l := newInvListCap(nil, 95)
	l.most, l.keptN = 40, 7
	back := keptList(l).list()
	if back.most != 40 || back.keptN != 7 || back.capacity() != 95 || back.roomBound() != 33 {
		t.Errorf("carried: %+v", back)
	}
	if c := l.clone(); c.roomBound() != 33 {
		t.Errorf("a clone: %d", c.roomBound())
	}
}
