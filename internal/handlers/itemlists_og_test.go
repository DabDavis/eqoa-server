package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// [OG] accounts' lists (entity/itemslots.go; the server owner, 2026-11-18, of
// Vanilla Test: "lets make 100 inventory space in bank and pack for vanilla
// test, but [OG] gets 40/40"): pack_slots and bank_slots 100, nothing
// grandfathered for everyone, og_pack_slots and og_bank_slots 40.

func ogSlots(t *testing.T) {
	t.Helper()
	entity.SetItemSlots(100, 100)
	entity.SetItemSlotsGrandfathered(0, 0)
	entity.SetOGItemSlots(40, 40)
	t.Cleanup(func() {
		entity.SetItemSlots(0, 0)
		entity.SetItemSlotsGrandfathered(0, 0)
		entity.SetOGItemSlots(0, 0)
	})
}

// The sizes: everyone's 100, an [OG] account's 40 grandfathered to 100; with
// og_pack_slots unset an [OG] account's are everyone's; and with the shard's
// own grandfathering (Live's config before: 40, grandfathered 100) an [OG]
// account's are as everyone's.
func TestOGItemSlotsSizes(t *testing.T) {
	ogSlots(t)
	if p, b := entity.ItemSlotsFor(false); p != 100 || b != 100 {
		t.Errorf("everyone's: %d %d", p, b)
	}
	if p, b := entity.ItemSlotsFor(true); p != 40 || b != 40 {
		t.Errorf("an [OG] account's: %d %d", p, b)
	}
	if p, b := entity.ItemSlotsGrandfatheredFor(false); p != 0 || b != 0 {
		t.Errorf("everyone's grandfathered: %d %d", p, b)
	}
	if p, b := entity.ItemSlotsGrandfatheredFor(true); p != 100 || b != 100 {
		t.Errorf("an [OG] account's grandfathered: %d %d", p, b)
	}
	if p, b := entity.ItemListsMostFor(true); p != 100 || b != 100 {
		t.Errorf("an [OG] account's most: %d %d", p, b)
	}
	if p, b := entity.ItemListsMost(); p != 100 || b != 100 || !entity.ItemSlotsOffered() {
		t.Errorf("the offer: %d %d", p, b)
	}

	entity.SetOGItemSlots(0, 0)
	if p, b := entity.ItemSlotsFor(true); p != 100 || b != 100 {
		t.Errorf("unset, an [OG] account's: %d %d", p, b)
	}
	if p, b := entity.ItemSlotsGrandfatheredFor(true); p != 0 || b != 0 {
		t.Errorf("unset, an [OG] account's grandfathered: %d %d", p, b)
	}

	entity.SetItemSlots(40, 40)
	entity.SetItemSlotsGrandfathered(100, 100)
	entity.SetOGItemSlots(40, 40)
	for _, og := range []bool{false, true} {
		if p, b := entity.ItemSlotsFor(og); p != 40 || b != 40 {
			t.Errorf("og %v at 40 grandfathered: %d %d", og, p, b)
		}
		if p, b := entity.ItemSlotsGrandfatheredFor(og); p != 100 || b != 100 {
			t.Errorf("og %v at 40 grandfathered: grandfathered %d %d", og, p, b)
		}
	}
	// og_pack_slots below 40 is 40: a PS2's
	entity.SetOGItemSlots(10, 0)
	if p, _ := entity.OGItemSlots(); p != 40 {
		t.Errorf("og_pack_slots 10: %d", p)
	}
}

// Everyone's character at 95 takes up to 100; an [OG] account's at 30 stops at
// 40 (pack and bank, its lists told 40, its login's gifts 40); an [OG] one at
// 95 keeps all 95, sent whole, and takes no new record while a stack still
// tops up.
func TestOGItemSlotsLists(t *testing.T) {
	ogSlots(t)

	// everyone's: 100
	f := newListsWorld(t)
	f.own(f.sword, 95, database.LocPack)
	sess := f.session(100, 100)
	if p, b := listCaps(sess); p != 100 || b != 100 {
		t.Fatalf("everyone's caps: %d %d", p, b)
	}
	dump, _ := f.dump(sess, true)
	dumpedLists(t, dump, 95, 0)
	l := sessionInvList(sess)
	n := 0
	for takesOne(l, f.sword) {
		n++
	}
	if n != 5 {
		t.Errorf("everyone's took %d, want 5", n)
	}
	if r := sessionListSizes(sess); r.packOf() != 100 || r.bankOf() != 100 {
		t.Errorf("everyone's login room: %+v", r)
	}

	// an [OG] account's at 30: 40
	g := newListsWorld(t)
	g.own(g.sword, 30, database.LocPack)
	g.own(g.gem, 12, database.LocBank)
	og := g.session(100, 100)
	og.SetOG(true)
	dump, _ = g.dump(og, true)
	dumpedLists(t, dump, 30, 12)
	l, b := sessionInvList(og), sessionBankList(og)
	if l.capacity() != 40 || b.capacity() != 40 {
		t.Fatalf("[OG] lists of %d and %d, want 40", l.capacity(), b.capacity())
	}
	n = 0
	for takesOne(l, g.sword) {
		n++
	}
	if n != 10 {
		t.Errorf("the [OG] pack took %d, want 10", n)
	}
	n = 0
	for takesOne(b, g.sword) {
		n++
	}
	if n != 28 {
		t.Errorf("the [OG] bank took %d, want 28", n)
	}
	if r := sessionListSizes(og); r.packOf() != 40 || r.bankOf() != 40 {
		t.Errorf("the [OG] login room: %+v", r)
	}
	if bankCapOf(og) != 40 {
		t.Errorf("the [OG] bank's room: %d", bankCapOf(og))
	}

	// an [OG] account's at 95: keeps 95, nothing new, a stack tops up
	h := newListsWorld(t)
	h.own(h.sword, 94, database.LocPack)
	h.own(h.gem, 1, database.LocPack)
	og2 := h.session(100, 100)
	og2.SetOG(true)
	dump, _ = h.dump(og2, true)
	dumpedLists(t, dump, 95, 0)
	if rows := h.rowsAt(); len(rows[database.LocPackKept]) != 0 {
		t.Fatalf("%d kept", len(rows[database.LocPackKept]))
	}
	l = sessionInvList(og2)
	if l.capacity() != 95 || takesOne(l, h.sword) || l.wouldAdd(h.sword, 1, 1) != 0 || packCapOf(og2) > 95 {
		t.Errorf("an [OG] pack of 95 took a 96th (cap %d)", l.capacity())
	}
	if l.wouldAdd(h.gem, 5, 20) != 5 {
		t.Error("an [OG] stack did not top up")
	}
}

// A keep restart's resume holds a list to the session's limits now
// (listGrandfathered): everyone's not grandfathered, an [OG] account's at 40.
func TestOGItemSlotsListGrandfathered(t *testing.T) {
	ogSlots(t)
	for _, bank := range []bool{false, true} {
		if on, _ := listGrandfathered(bank, false); on {
			t.Errorf("bank %v: everyone's grandfathered", bank)
		}
		if on, lim := listGrandfathered(bank, true); !on || lim != 40 {
			t.Errorf("bank %v: an [OG] account's: %v %d", bank, on, lim)
		}
	}
	l := &invList{cap: 100}
	grandfatherList(l, false, 2, true)
	if l.most != 40 || l.keptN != 2 {
		t.Errorf("an [OG] pack: most %d kept %d", l.most, l.keptN)
	}
	l = &invList{cap: 100}
	grandfatherList(l, false, 2, false)
	if l.most != 0 {
		t.Errorf("everyone's pack: most %d", l.most)
	}
}

// readOG gives the session its account's choice before a character's entry
// fills its lists (the gifts and a transfer at SelectedChar, by its [OG]
// sizes), with no player yet; off where world.og_accounts is unset.
func TestReadOGBeforeTheGifts(t *testing.T) {
	ogSlots(t)
	db, store := newSafeDB(t)
	accs := database.NewSQLiteAccountStore(db)
	acc, err := accs.Create("ogpack", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := accs.SetAccountOG(acc.ID, true); err != nil {
		t.Fatal(err)
	}
	h := &CharacterHandler{store: store}
	sess := entitySlotsSession()
	sess.SetAccountID(acc.ID)
	h.readOG(sess)
	if sess.OG() {
		t.Fatal("og_accounts unset: the session is [OG]")
	}
	ogSwitch(t)
	h.readOG(sess)
	if !sess.OG() {
		t.Fatal("og_accounts set: the session is not [OG]")
	}
	if r := sessionListSizes(sess); r.packOf() != 40 || r.bankOf() != 40 {
		t.Errorf("the [OG] login room: %+v", r)
	}
}
