package handlers

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A merchant's buy-back against a temporary database, each request fed as the
// client sends it and the 0x00B7s read back as CLIENT's VIItemList::Read reads them.

type backFixture struct {
	*shopFixture
	eq *EquipHandler
}

func newBackFixture(t *testing.T) *backFixture {
	t.Helper()
	f := &backFixture{shopFixture: newShopFixture(t)}
	f.bindDestroy()
	return f
}

func (f *backFixture) bindDestroy() {
	f.eq = NewEquipHandler(nil, f.inv, nil, &CharacterHandler{inventoryStore: f.inv})
	f.eq.playerOf = func(*session.Session) *entity.Player { return f.player }
}

// msgs drains what was queued, each message whole (a long 0x00B7 goes in fragments).
func (f *backFixture) msgs() [][]byte {
	var out [][]byte
	var part []byte
	for _, m := range f.sess.Queue().GetPending() {
		part = append(part, m.Data()...)
		if m.Type != protocol.MessageTypeSegmentReliable {
			out = append(out, part)
			part = nil
		}
	}
	return out
}

func (f *backFixture) open() shopBox {
	f.t.Helper()
	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	msgs := f.msgs()
	mustOps(f.t, msgs, protocol.OpcodeMerchantBox)
	return readShopBox(f.t, msgs[0])
}

func (f *backFixture) buyItem(index, count int) [][]byte {
	f.t.Helper()
	f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(index))
		w.Write7BitEncodedInt64(int64(count))
		w.WriteUint32(shopMerchantID)
	})
	return f.msgs()
}

func (f *backFixture) sellItem(key uint32, count int) [][]byte {
	f.t.Helper()
	f.send(protocol.OpcodeMerchantSell, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.Write7BitEncodedInt64(int64(count))
		w.WriteUint32(shopMerchantID)
	})
	return f.msgs()
}

// destroy is the item menu's Destroy, 0x003C {u32 key, u32 how many}.
func (f *backFixture) destroy(key uint32, n int) [][]byte {
	f.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRemoveInvItem)
	msg.SetData(build(protocol.OpcodeRemoveInvItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(uint32(n))
	})[2:])
	if err := f.eq.HandleDestroyItem(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	return f.msgs()
}

func (f *backFixture) entries() []database.BuyBackEntry {
	f.t.Helper()
	e, err := f.inv.BuyBacks(f.charSID)
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

// tableRows counts the character's rows of the table itself, dead ones too.
func (f *backFixture) tableRows() int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM charbuyback WHERE serverid = ?`, f.charSID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// aged lets the repeat guard's time pass.
func (f *backFixture) aged() {
	if w := sessionMerchantWindow(f.sess); w != nil {
		w.mu.Lock()
		w.servedAt = w.servedAt.Add(-2 * buyBackRepeat)
		w.mu.Unlock()
	}
}

type shopRow struct {
	qty, hp, charges, equip, quick int64
	key                            uint32
	item                           wireItem
}

type shopBox struct {
	id                uint32
	sellRate, buyRate int64
	counter           uint32
	rows              []shopRow
	greeting          string
}

func readNetItem(r *auctionWire) shopRow {
	row := shopRow{qty: r.zz(), hp: r.zz(), charges: r.zz(), equip: r.zz(), quick: r.zz(), key: r.u32()}
	row.item = r.item()
	return row
}

// readShopBox is the dispatcher's 0x00B7: the id, the two rates, VIItemList::Read,
// ReadString. CLIENT's ItemInternal holds 40 and its Read checks nothing, so any
// more is a failure here.
func readShopBox(t *testing.T, msg []byte) shopBox {
	t.Helper()
	if opOf(msg) != protocol.OpcodeMerchantBox {
		t.Fatalf("not a 0x00B7: % X", msg[:2])
	}
	r := &auctionWire{b: msg[2:]}
	b := shopBox{id: r.u32(), sellRate: r.zz(), buyRate: r.zz()}
	n := int(r.zz())
	if n > invListCap {
		t.Fatalf("0x00B7 lists %d records: CLIENT's list holds %d and Read does not look", n, invListCap)
	}
	b.counter = r.u32()
	for i := 0; i < n; i++ {
		b.rows = append(b.rows, readNetItem(r))
		rec := b.rows[i].item
		if len([]rune(rec.name)) > itemNameMax || len([]rune(rec.desc)) > itemDescMax {
			t.Errorf("record %d: %d-character name or %d-character description: ReadString keeps it EMPTY", i, len(rec.name), len(rec.desc))
		}
	}
	b.greeting = r.wide()
	if r.bad || r.p != len(r.b) {
		t.Fatalf("0x00B7 did not read to its end (bad %v, %d of %d)", r.bad, r.p, len(r.b))
	}
	if len([]rune(b.greeting)) > greetingMax {
		t.Errorf("the greeting is %d characters: ReadString keeps it EMPTY", len(b.greeting))
	}
	return b
}

// readAdded is 0x003B's one record.
func readAdded(t *testing.T, msg []byte) shopRow {
	t.Helper()
	if opOf(msg) != protocol.OpcodeAddInvItem {
		t.Fatalf("not a 0x003B: % X", msg[:2])
	}
	r := &auctionWire{b: msg[2:]}
	row := readNetItem(r)
	if r.bad || r.p != len(r.b) {
		t.Fatalf("0x003B did not read to its end")
	}
	return row
}

func shopNames(box shopBox) []string {
	var out []string
	for _, r := range box.rows {
		out = append(out, r.item.name)
	}
	return out
}

func sameNames(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
}

// Sold with the window up: the list again first, its new line last and marked, then the answer
// CLIENT waits on; the line bought: the list again without it, then the item itself as it is, then the
// purse where it was. The controls: no line before the sale, and the same greeting after.
func TestBuyBackSoldThenBoughtBack(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	f.catalog(f.sword, 1, 0)
	axe := f.pattern("Axe", 200, 1, 100, 12)
	f.own(axe, 1, 100, 0)
	f.loadLists()
	f.player.Tunar = 50

	box := f.open()
	sameNames(t, "before the sale", shopNames(box), "Fishing Bait", "Rusty Sword")
	if box.greeting != merchantGreeting {
		t.Errorf("the greeting with nothing to buy back: %q", box.greeting)
	}

	msgs := f.sellItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	if f.player.Tunar != 250 {
		t.Fatalf("after the sale the purse holds %d", f.player.Tunar)
	}
	box = readShopBox(t, msgs[0])
	sameNames(t, "after the sale", shopNames(box), "Fishing Bait", "Rusty Sword", "Buy back: Axe")
	line := box.rows[2]
	if line.item.value != 200 || line.item.maxStack != 1 || line.qty != 1 || line.hp != 100 || line.key != 2 || box.counter != 3 {
		t.Errorf("the line: %+v, counter %d", line, box.counter)
	}
	if !strings.Contains(line.item.desc, "You sold this for 200 tunar") {
		t.Errorf("the line's description: %q", line.item.desc)
	}
	if !strings.Contains(box.greeting, buyBackAtTheEnd) {
		t.Errorf("the greeting does not say where it is: %q", box.greeting)
	}

	msgs = f.buyItem(2, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	box = readShopBox(t, msgs[0])
	sameNames(t, "after the buy-back", shopNames(box), "Fishing Bait", "Rusty Sword")
	if box.greeting != merchantGreeting {
		t.Errorf("the greeting after the last line went: %q", box.greeting)
	}
	got := readAdded(t, msgs[1])
	if got.item.name != "Axe" || got.item.value != 200 || got.item.pattern != int64(axe) || got.qty != 1 || got.hp != 100 || got.equip != -1 {
		t.Errorf("0x003B: %+v", got)
	}
	if stored, _ := f.storedTunar(); f.player.Tunar != 50 || stored != 50 {
		t.Errorf("the purse holds %d, stored %d, want 50", f.player.Tunar, stored)
	}
	rows := f.rows()
	if len(rows) != 1 || rows[0].Pattern.PatternID != axe || rows[0].RemainHP != 100 || rows[0].Location != 0 || rows[0].EquipLoc != -1 {
		t.Fatalf("the rows: %+v", rows)
	}
	l := sessionInvList(f.sess)
	if ids := l.itemIDs(); len(ids) != 1 || ids[0] != rows[0].ItemID || l.index(got.key) != 0 {
		t.Errorf("the mirror: %v, key %d at %d", ids, got.key, l.index(got.key))
	}
	if len(f.entries()) != 0 || f.tableRows() != 0 {
		t.Errorf("the entry is still kept: %+v", f.entries())
	}
}

// With no window noted (a client that opened none, a restart's), a sale sends no list: nothing there
// to hold in step. The entry is made all the same.
func TestBuyBackSaleWithNoWindowSendsNoList(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	mustOps(t, f.sellItem(0, 1), protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	if e := f.entries(); len(e) != 1 || e[0].How != database.BuyBackSold {
		t.Fatalf("the entries: %+v", e)
	}
	sameNames(t, "opened after", shopNames(f.open()), "Buy back: Rusty Sword")
}

// Destroyed, at what a merchant would have paid at the time; a worn one comes back in the pack.
func TestBuyBackDestroyedThenBoughtBack(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	axe := f.pattern("Axe", 200, 1, 100, 12)
	id := f.own(axe, 1, 50, 0) // half worn: a quarter off
	if err := f.inv.SetEquipLoc(f.charSID, id, 16); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	f.player.Tunar = 1000

	f.destroy(0, 1)
	if len(f.rows()) != 0 || f.player.Tunar != 1000 {
		t.Fatalf("after the Destroy: %+v, %d", f.rows(), f.player.Tunar)
	}

	box := f.open()
	sameNames(t, "the list", shopNames(box), "Fishing Bait", "Buy back: Axe")
	line := box.rows[1]
	if want := int64(sellPrice(200, 50, 100)); line.item.value != want || want != 150 {
		t.Errorf("the line's price %d, want %d", line.item.value, want)
	}
	if line.hp != 50 || !strings.Contains(line.item.desc, "You destroyed this; 150 tunar brings it back.") || !strings.Contains(line.item.desc, "Condition 50 of 100") {
		t.Errorf("the line: %+v", line)
	}

	msgs := f.buyItem(1, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if f.player.Tunar != 850 {
		t.Errorf("the purse holds %d, want 850", f.player.Tunar)
	}
	rows := f.rows()
	if len(rows) != 1 || rows[0].RemainHP != 50 || rows[0].EquipLoc != -1 || rows[0].Location != 0 {
		t.Fatalf("the rows: %+v", rows)
	}
}

// Five are kept and a sixth pushes out the oldest, sold or destroyed alike; the list ends with the
// newest, the one a press of Up from the first row lands on (VIWndList::Scroll wraps).
func TestBuyBackSixthPushesOutTheOldest(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	for _, n := range []string{"A", "B", "C", "D", "E", "F"} {
		f.own(f.pattern(n, 10, 1, 0, -1), 1, 0, 0)
	}
	f.loadLists()
	for key := uint32(0); key < 5; key++ {
		if key%2 == 0 {
			mustOps(t, f.sellItem(key, 1), protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
		} else {
			f.destroy(key, 1)
		}
	}
	sameNames(t, "five", shopNames(f.open()), "Fishing Bait",
		"Buy back: A", "Buy back: B", "Buy back: C", "Buy back: D", "Buy back: E")
	f.destroy(5, 1)
	sameNames(t, "the sixth", shopNames(f.open()), "Fishing Bait",
		"Buy back: B", "Buy back: C", "Buy back: D", "Buy back: E", "Buy back: F")
	if f.tableRows() != database.BuyBackKept {
		t.Errorf("%d rows kept", f.tableRows())
	}
	// and the list is right: the last row is the newest
	if msgs := f.buyItem(5, 1); readAdded(t, msgs[1]).item.name != "F" {
		t.Errorf("the last row gave %+v", readAdded(t, msgs[1]))
	}
}

// A stack sold in one action is one entry, comes back whole whatever the count asks, and joins the
// stack held when all of it fits there. The record's own figures come with 0x003B; the line's say the whole
// price and a stack of 1, so VIWndQuantity shows that price and sends 1.
func TestBuyBackStackComesBackWhole(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.bait, 20, 100, 0)
	f.loadLists()
	f.player.Tunar = 0

	f.sellItem(0, 15)
	if f.player.Tunar != 15*38 {
		t.Fatalf("the sale paid %d", f.player.Tunar)
	}
	box := f.open()
	line := box.rows[0]
	if line.item.name != "Buy back: 15 Fishing Bait" || line.item.value != 15*38 || line.item.maxStack != 1 || line.qty != 15 {
		t.Fatalf("the line: %+v", line)
	}
	if !strings.Contains(line.item.desc, "You sold these 15 for 570 tunar; the same brings them all back.") {
		t.Errorf("the line's description: %q", line.item.desc)
	}

	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	got := readAdded(t, msgs[1])
	if got.qty != 15 || got.item.maxStack != 20 || got.item.value != 38 || got.key != 0 {
		t.Errorf("0x003B: %+v", got)
	}
	rows := f.rows()
	if len(rows) != 1 || rows[0].StackLeft != 20 || f.player.Tunar != 0 {
		t.Fatalf("the stack: %+v, the purse %d", rows, f.player.Tunar)
	}
	if rec, _ := sessionInvList(f.sess).at(0); rec.qty != 20 {
		t.Errorf("the mirror holds %d", rec.qty)
	}

	// destroyed, and a count that is not the stack's
	f.destroy(0, 7)
	f.player.Tunar = 7 * 38
	f.open()
	msgs = f.buyItem(0, 99)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if rows := f.rows(); readAdded(t, msgs[1]).qty != 7 || rows[0].StackLeft != 20 || f.player.Tunar != 0 {
		t.Errorf("count 99: %+v, the stack %+v, the purse %d", readAdded(t, msgs[1]), rows, f.player.Tunar)
	}

	// a count of 0 asks for nothing
	f.destroy(0, 7)
	f.player.Tunar = 7 * 38
	f.open()
	mustOps(t, f.buyItem(0, 0), protocol.OpcodeCancelPending)
	if len(f.entries()) != 1 {
		t.Errorf("count 0 took the entry")
	}
}

// All or nothing: a stack that does not fit on the record CLIENT's AddItem would file it on is
// refused whole, where a ware's part would come. The control: with room it comes.
func TestBuyBackStackThatDoesNotFitIsRefusedWhole(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.bait, 20, 100, 0)
	f.loadLists()
	f.player.Tunar = 10000
	f.sellItem(0, 15) // 5 left
	f.catalog(f.bait, 1, 0)
	f.open() // the ware at 0, the line at 1
	mustOps(t, f.buyItem(0, 6), protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if rows := f.rows(); rows[0].StackLeft != 11 {
		t.Fatalf("the stack after the ware: %+v", rows)
	}
	purse := f.player.Tunar
	msgs := f.buyItem(1, 1) // 11 + 15 > 20
	mustOps(t, msgs, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if rows := f.rows(); rows[0].StackLeft != 11 || f.player.Tunar != purse || len(f.entries()) != 1 {
		t.Fatalf("refused, but changed: %+v, %d, %+v", rows, f.player.Tunar, f.entries())
	}
	f.destroy(0, 6) // the entry for these is the newest now: 5 left
	f.open()        // the ware at 0, the 15 at 1, the 6 at 2
	mustOps(t, f.buyItem(1, 1), protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if rows := f.rows(); rows[0].StackLeft != 20 {
		t.Errorf("the stack with room: %+v", rows)
	}
}

// Exactly what went: condition (a broken item too, which catalogRecord would show new) and charges
// (0 on a charged record too, which giveItem would fill).
func TestBuyBackRestoresConditionAndCharges(t *testing.T) {
	f := newBackFixture(t)
	flask := f.pattern("Flask", 100, 1, 50, -1)
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, source, charges) VALUES (?, 'Self', 'test', 4)`, flask); err != nil {
		t.Fatal(err)
	}
	f.player.Tunar = 100000

	round := func(hp, charges int, how string) shopRow {
		t.Helper()
		id, err := f.inv.AddCharItem(f.charSID, flask, 1, hp, charges, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`UPDATE charinventory SET remaincharge = ?, remainHP = ? WHERE itemid = ?`, charges, hp, id); err != nil {
			t.Fatal(err)
		}
		f.loadLists()
		if how == "sold" {
			f.sellItem(0, 1)
		} else {
			f.destroy(0, 1)
		}
		box := f.open()
		line := box.rows[len(box.rows)-1]
		msgs := f.buyItem(len(box.rows)-1, 1)
		got := readAdded(t, msgs[1])
		if got.hp != int64(hp) || got.charges != int64(charges) {
			t.Errorf("%s at %d/%d: 0x003B holds %d/%d", how, hp, charges, got.hp, got.charges)
		}
		rows := f.rows()
		if len(rows) != 1 || rows[0].RemainHP != hp || rows[0].Charges != charges {
			t.Errorf("%s at %d/%d: the row %+v", how, hp, charges, rows)
		}
		if _, err := f.db.Exec(`DELETE FROM charinventory WHERE serverid = ?`, f.charSID); err != nil {
			t.Fatal(err)
		}
		return line
	}
	for _, how := range []string{"sold", "destroyed"} {
		if line := round(37, 3, how); line.hp != 37 || line.charges != 3 {
			t.Errorf("the %s line shows %+v", how, line)
		}
		if line := round(0, 2, how); line.hp != 0 || line.charges != 2 {
			t.Errorf("the broken %s line shows %+v", how, line)
		}
		round(50, 0, how)
	}
	// the controls: what the other paths would have done with them
	if rec := catalogRecord(database.CharInventoryEntry{Pattern: database.ItemPatternRecord{MaxHP: 50}}); rec.RemainHP != 50 {
		t.Errorf("catalogRecord shows a broken item at %d: this test no longer tells anything", rec.RemainHP)
	}
	if n := f.inv.ItemCharges(flask); n != 4 {
		t.Errorf("a new one holds %d charges", n)
	}
}

// Never a profit: what a sale pays, the line costs, and it then sells for the same again; a destroyed
// one costs what a merchant would have paid. An item worth nothing comes for nothing, with an empty
// purse. A Rose merchants do not buy, so a destroyed one costs nothing, not its ten million.
func TestBuyBackPrice(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	axe := f.pattern("Axe", 333, 1, 100, 12)
	f.player.Tunar = 1000

	for _, hp := range []int{100, 50, 7, 0} {
		id := f.own(axe, 1, hp, 0)
		f.loadLists()
		start := f.player.Tunar
		f.open()
		f.sellItem(0, 1)
		paid := f.player.Tunar - start
		if want := sellPrice(333, hp, 100); paid != want {
			t.Errorf("condition %d paid %d, want %d", hp, paid, want)
		}
		msgs := f.buyItem(1, 1)
		mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
		if f.player.Tunar != start {
			t.Errorf("condition %d: the purse went %d -> %d through a sale and its buy-back", hp, start, f.player.Tunar)
		}
		// sold again it fetches the same, and destroyed instead it costs the same
		f.aged()
		key := readAdded(t, msgs[1]).key
		f.sellItem(key, 1)
		if got := f.player.Tunar - start; got != paid {
			t.Errorf("condition %d: sold again for %d, first %d", hp, got, paid)
		}
		box := f.open()
		if got := box.rows[1].item.value; got != int64(paid) {
			t.Errorf("condition %d: the line costs %d, the sale paid %d", hp, got, paid)
		}
		f.buyItem(1, 1)
		f.loadLists()
		f.destroy(0, 1)
		box = f.open()
		if got := box.rows[1].item.value; got != int64(paid) {
			t.Errorf("condition %d: destroyed, the line costs %d, a sale paid %d", hp, got, paid)
		}
		if merchantPrice(int(box.rows[1].item.value)) != paid {
			t.Errorf("GetMerchantPrice shows %d, the server charges %d", merchantPrice(int(box.rows[1].item.value)), paid)
		}
		f.buyItem(1, 1)
		if _, err := f.db.Exec(`DELETE FROM charinventory WHERE itemid >= ?`, id); err != nil {
			t.Fatal(err)
		}
		f.player.Tunar = 1000
		f.aged()
	}

	free := f.pattern("Heirloom", 0, 1, 80, 12)
	f.own(free, 1, 80, 0)
	f.loadLists()
	f.player.Tunar = 0
	f.sellItem(0, 1)
	if line := f.open().rows[1]; line.item.value != 0 || !strings.HasPrefix(line.item.desc, "You sold this for nothing; it comes back for nothing.") {
		t.Errorf("the line worth nothing: %+v", line)
	}
	if msgs := f.buyItem(1, 1); len(msgs) != 3 || readAdded(t, msgs[1]).item.name != "Heirloom" || f.player.Tunar != 0 {
		t.Errorf("worth nothing, with nothing: %s, purse %d", ops(msgs), f.player.Tunar)
	}
}

// What a line's description begins with says what happens in each case; "nothing brings it back" read
// as if it could not come back, on a PS2's screen.
func TestBuyBackWords(t *testing.T) {
	for _, c := range []struct {
		how          string
		n, price, hp int
		want         string
	}{
		{database.BuyBackSold, 1, 75, 100, "You sold this for 75 tunar; the same brings it back."},
		{database.BuyBackSold, 15, 570, 100, "You sold these 15 for 570 tunar; the same brings them all back."},
		{database.BuyBackSold, 1, 0, 100, "You sold this for nothing; it comes back for nothing."},
		{database.BuyBackSold, 3, 0, 100, "You sold these 3 for nothing; they all come back for nothing."},
		{database.BuyBackDestroyed, 1, 75, 100, "You destroyed this; 75 tunar brings it back."},
		{database.BuyBackDestroyed, 7, 266, 100, "You destroyed these 7; 266 tunar brings them all back."},
		{database.BuyBackDestroyed, 1, 0, 100, "You destroyed this; it comes back for nothing."},
		{database.BuyBackDestroyed, 2, 0, 100, "You destroyed these 2; they all come back for nothing."},
		{database.BuyBackSold, 1, 75, 25, "You sold this for 75 tunar; the same brings it back. Condition 25 of 100."},
		{database.BuyBackDestroyed, 1, 0, 0, "You destroyed this; it comes back for nothing. Condition 0 of 100."},
	} {
		e := database.BuyBackEntry{How: c.how, Price: c.price}
		e.Item.StackLeft, e.Item.RemainHP, e.Item.Pattern.MaxHP = c.n, c.hp, 100
		if got := buyBackWords(&e); got != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}

func TestBuyBackRoseDestroyedCostsNothing(t *testing.T) {
	f := newBackFixture(t)
	if _, err := f.db.Exec(`INSERT INTO itempattern
		(patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 10000000, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Rose of Rehabilitation', '', 0, 0)`,
		database.RoseOfRehabilitation); err != nil {
		t.Fatal(err)
	}
	f.own(database.RoseOfRehabilitation, 1, 0, 0)
	f.loadLists()
	f.open()
	// the control: merchants refuse it, and that makes no entry
	mustOps(t, f.sellItem(0, 1), protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if len(f.entries()) != 0 {
		t.Fatalf("a refused sale made an entry")
	}
	f.destroy(0, 1)
	if line := f.open().rows[0]; line.item.value != 0 {
		t.Errorf("a destroyed Rose costs %d", line.item.value)
	}
}

// No room in the pack: refused, nothing moved, and said, since 0x00D7 shows nothing. Made room, it comes.
func TestBuyBackNoRoom(t *testing.T) {
	f := newBackFixture(t)
	for i := 0; i < invListCap; i++ {
		f.own(f.sword, 1, 100, 0)
	}
	f.loadLists()
	f.player.Tunar = 1000
	f.destroy(0, 1)
	f.own(f.sword, 1, 100, 0) // the pack full again, with a row the list has not
	f.loadLists()
	f.open()
	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if !bytes.Contains(msgs[0], utf16le(buyBackNoRoom)) {
		t.Errorf("the refusal does not say why: % X", msgs[0])
	}
	if len(f.rows()) != invListCap || f.player.Tunar != 1000 || len(f.entries()) != 1 {
		t.Fatalf("refused, but changed: %d rows, %d tunar, %d entries", len(f.rows()), f.player.Tunar, len(f.entries()))
	}
	if _, err := f.db.Exec(`DELETE FROM charinventory WHERE itemid = ?`, f.rows()[0].ItemID); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	mustOps(t, f.buyItem(0, 1), protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if len(f.rows()) != invListCap || f.player.Tunar != 900 {
		t.Errorf("with room: %d rows, %d tunar", len(f.rows()), f.player.Tunar)
	}
}

// A full pack still takes a stack that joins a record it holds (AddItem's rule, a ware's too), and
// no more than the stack's most: the control refuses what would pass it.
func TestBuyBackStackJoinsAFullPack(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.bait, 15, 100, 0)
	for i := 1; i < invListCap; i++ {
		f.own(f.sword, 1, 100, 0)
	}
	f.loadLists()
	f.player.Tunar = 10000
	f.sellItem(0, 5) // 10 left
	f.open()
	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if got := readAdded(t, msgs[1]); got.qty != 5 || got.key != 0 {
		t.Errorf("0x003B: %+v", got)
	}
	if rows := f.rows(); len(rows) != invListCap || rows[0].StackLeft != 15 {
		t.Fatalf("%d rows, the stack %d", len(rows), rows[0].StackLeft)
	}
	f.player.Tunar = 10000
	f.sellItem(0, 5)
	f.aged()
	f.open()
	if _, err := f.db.Exec(`UPDATE charinventory SET stackleft = 16 WHERE itemid = ?`, f.rows()[0].ItemID); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	mustOps(t, f.buyItem(0, 1), protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
}

// Too little tunar by one: refused and said; with exactly the price it comes and the purse is empty.
func TestBuyBackTooLittleTunar(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.player.Tunar = 0
	f.sellItem(0, 1)
	f.open()
	f.player.Tunar = 99
	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if !bytes.Contains(msgs[0], utf16le(buyBackNoTunar)) {
		t.Errorf("the refusal does not say why: % X", msgs[0])
	}
	if len(f.rows()) != 0 || f.player.Tunar != 99 || len(f.entries()) != 1 {
		t.Fatalf("refused, but changed: %+v, %d, %+v", f.rows(), f.player.Tunar, f.entries())
	}
	f.player.Tunar = 100
	mustOps(t, f.buyItem(0, 1), protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if len(f.rows()) != 1 || f.player.Tunar != 0 {
		t.Errorf("with the price: %+v, %d", f.rows(), f.player.Tunar)
	}
}

// The request sent twice. A copy of the client's own resending never reaches a handler (the
// control below, the listener's TakeReliableOnce). Past that: the second finds its entry gone when it
// was the last line and nothing where it was; with a line after it, that line has moved into its place, and
// the second must not buy that either, until the time a player would need has passed.
func TestBuyBackRetransmit(t *testing.T) {
	f := newBackFixture(t)
	for _, n := range []string{"A", "B", "C"} {
		f.own(f.pattern(n, 10, 1, 0, -1), 1, 0, 0)
	}
	f.loadLists()
	f.player.Tunar = 1000
	for key := uint32(0); key < 3; key++ {
		f.sellItem(key, 1)
	}
	purse := f.player.Tunar
	f.open() // A, B, C at 0, 1, 2

	msgs := f.buyItem(1, 1) // B
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if got := readAdded(t, msgs[1]); got.item.name != "B" {
		t.Fatalf("the first gave %+v", got)
	}
	sameNames(t, "the list after", shopNames(readShopBox(t, msgs[0])), "Buy back: A", "Buy back: C")

	mustOps(t, f.buyItem(1, 1), protocol.OpcodeCancelPending)
	if rows := f.rows(); len(rows) != 1 || f.player.Tunar != purse-10 || len(f.entries()) != 2 {
		t.Fatalf("the copy changed something: %+v, %d, %d entries", rows, f.player.Tunar, len(f.entries()))
	}

	// a player's own request for what is now at that index, after the time it takes
	f.aged()
	msgs = f.buyItem(1, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if got := readAdded(t, msgs[1]); got.item.name != "C" {
		t.Errorf("the next gave %+v", got)
	}

	// the last line: nothing is there for a copy to find, at any time
	f.aged()
	mustOps(t, f.buyItem(1, 1), protocol.OpcodeCancelPending)
	msgs = f.buyItem(0, 1)
	if got := readAdded(t, msgs[1]); got.item.name != "A" {
		t.Errorf("the first line gave %+v", got)
	}
	f.aged()
	mustOps(t, f.buyItem(0, 1), protocol.OpcodeCancelPending)
	if len(f.rows()) != 3 || f.player.Tunar != purse-30 || f.tableRows() != 0 {
		t.Errorf("at the end: %d rows, %d tunar, %d entries", len(f.rows()), f.player.Tunar, f.tableRows())
	}
}

// The entry gone from under a list that still shows it (a GM's edit, the window's list old): refused
// and said, the list sent again, and the purse and pack as they were.
func TestBuyBackLineWhoseEntryWent(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.sellItem(0, 1)
	f.open()
	purse := f.player.Tunar
	if _, err := f.db.Exec(`DELETE FROM charbuyback WHERE serverid = ?`, f.charSID); err != nil {
		t.Fatal(err)
	}
	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if len(readShopBox(t, msgs[0]).rows) != 0 || len(f.rows()) != 0 || f.player.Tunar != purse {
		t.Errorf("refused, but: %+v, %d", f.rows(), f.player.Tunar)
	}
}

// Over a reload: logged out, the server restarted, a new session: the entries are there and come back.
func TestBuyBackSurvivesAReload(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	axe := f.pattern("Axe", 200, 1, 100, 12)
	f.own(axe, 1, 40, 0)
	f.own(f.bait, 9, 100, 0)
	f.loadLists()
	f.sellItem(0, 1)
	f.destroy(1, 9)
	purse := f.player.Tunar

	var seq int
	var name, path string
	if err := f.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	f.db.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.db, f.inv = db, database.NewInventoryStore(db)
	f.sess = testSession()
	f.h = NewNPCWindowHandler(nil, f.inv)
	f.h.playerOf = func(*session.Session) *entity.Player { return f.player }
	f.h.find = func(_ *session.Session, id uint32) (entity.Entity, bool) {
		n, ok := f.npcs[id]
		return n, ok
	}
	f.bindDestroy()
	f.loadLists()

	box := f.open()
	sameNames(t, "after the reload", shopNames(box), "Fishing Bait", "Buy back: Axe", "Buy back: 9 Fishing Bait")
	if box.rows[1].hp != 40 {
		t.Errorf("the axe's condition: %+v", box.rows[1])
	}
	msgs := f.buyItem(1, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
	if got := readAdded(t, msgs[1]); got.item.name != "Axe" || got.hp != 40 {
		t.Errorf("0x003B: %+v", got)
	}
	if f.player.Tunar != purse-sellPrice(200, 40, 100) {
		t.Errorf("the purse holds %d", f.player.Tunar)
	}
}

// The risky part. A ware's index means what it always did with lines after it: every ware bought at
// its index is that ware at its price, the wares' records are byte for byte what they were with no
// lines (the control), the lines' presence moves nothing, and a ware added mid-window (a GM's) does
// not turn the list's first line into a ware: the client's indexes are those of the list it holds.
func TestBuyBackLeavesEveryWareWhereItWas(t *testing.T) {
	f := newBackFixture(t)
	shield := f.pattern("Shield", 70, 1, 100, 21)
	f.catalog(f.bait, 1, 0)
	f.catalog(f.sword, 1, 0)
	f.catalog(shield, 1, 0)
	f.player.Tunar = 100000
	f.loadLists()

	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	plain := f.msgs()[0]
	if n := len(readShopBox(t, plain).rows); n != 3 {
		t.Fatalf("%d wares", n)
	}

	for _, n := range []string{"A", "B"} {
		id := f.pattern(n, 10, 1, 0, -1)
		f.own(id, 1, 0, 0)
	}
	f.loadLists()
	f.destroy(0, 1)
	f.destroy(1, 1)

	f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
	withLines := f.msgs()[0]
	box := readShopBox(t, withLines)
	sameNames(t, "the list", shopNames(box), "Fishing Bait", "Rusty Sword", "Shield", "Buy back: A", "Buy back: B")

	// id, rates, then the count and counter differ: the records from there are the same bytes
	lead := len(plain) - len(buildGreeting(merchantGreeting))
	const header = 2 + 4 + 2 + 2 + 1 + 4 // op, id, two rates, count (one byte), counter
	if !bytes.Equal(plain[header:lead], withLines[header:lead]) {
		t.Errorf("the wares' records changed with lines after them")
	}

	wantValue := map[int]int{0: 38, 1: 100, 2: 70}
	wantName := map[int]string{0: "Fishing Bait", 1: "Rusty Sword", 2: "Shield"}
	for i := 0; i < 3; i++ {
		start := f.player.Tunar
		msgs := f.buyItem(i, 1)
		mustOps(t, msgs, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar) // no list: nothing about it changed
		got := readAdded(t, msgs[0])
		if got.item.name != wantName[i] || start-f.player.Tunar != wantValue[i] {
			t.Errorf("ware %d gave %q for %d", i, got.item.name, start-f.player.Tunar)
		}
	}
	if len(f.entries()) != 2 {
		t.Errorf("buying wares touched the entries: %+v", f.entries())
	}

	// the same after a sale has sent the list again
	key := uint32(2) // Fishing Bait's record, the first bought
	msgs := f.sellItem(key, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	sameNames(t, "after a sale", shopNames(readShopBox(t, msgs[0])), "Fishing Bait", "Rusty Sword", "Shield",
		"Buy back: A", "Buy back: B", "Buy back: Fishing Bait")
	got := readAdded(t, f.buyItem(1, 1)[0])
	if got.item.name != "Rusty Sword" {
		t.Errorf("ware 1 after a sale gave %q", got.item.name)
	}

	// a ware added under an open window: the client's index 3 is still the first line it holds
	f.catalog(f.bait, 1, 0)
	if msgs := f.buyItem(3, 1); opOf(msgs[0]) != protocol.OpcodeMerchantBox || readAdded(t, msgs[1]).item.name != "A" {
		t.Errorf("index 3 under a catalog that grew: %s", ops(msgs))
	}
}

func buildGreeting(s string) []byte {
	return build(protocol.OpcodeMerchantBox, 16+2*len(s), func(w *buffer.Writer) { w.WriteStringUnicode(s) })[2:]
}

// With no note of the list (a restart's: the window is not kept, the entries are), a ware's index still
// names that ware and a line's names nothing: refused, nothing taken.
func TestBuyBackAfterARestartOfTheWindow(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.player.Tunar = 1000
	f.sellItem(0, 1)
	f.open()
	f.sess.SetMerchantWindow(nil)
	mustOps(t, f.buyItem(1, 1), protocol.OpcodeCancelPending)
	if len(f.entries()) != 1 {
		t.Errorf("the entry went")
	}
	got := readAdded(t, f.buyItem(0, 1)[0])
	if got.item.name != "Fishing Bait" {
		t.Errorf("ware 0 gave %q", got.item.name)
	}
}

// Under another merchant's id the index is that merchant's, as ever: no line of the window on the
// first answers it.
func TestBuyBackIsTheWindowsMerchants(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	const other = 0x00100BBC
	f.npcs[other] = &entity.NPC{Name: "Other", NPCID: 99, NPCType: 0x81}
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.sellItem(0, 1)
	f.open()
	f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(1)
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(other)
	})
	mustOps(t, f.msgs(), protocol.OpcodeCancelPending)
	if len(f.entries()) != 1 {
		t.Errorf("the entry went")
	}
}

// The list never passes CLIENT's 40, and the wares keep their places: lines fill what room there is,
// newest kept, and the greeting says what it can. 45 wares were cut to 40 before this too.
func TestBuyBackListNeverPassesForty(t *testing.T) {
	for _, c := range []struct {
		wares, wantWares int
		want             []string
		greeting         string
	}{
		{33, 33, []string{"A", "B", "C", "D", "E"}, buyBackAtTheEnd},
		{35, 35, []string{"A", "B", "C", "D", "E"}, buyBackAtTheEnd},
		{36, 36, []string{"B", "C", "D", "E"}, buyBackSomeAtEnd},
		{38, 38, []string{"D", "E"}, buyBackSomeAtEnd},
		{39, 39, []string{"E"}, buyBackSomeAtEnd},
		{40, 40, nil, buyBackNoShelf},
		{45, 40, nil, buyBackNoShelf},
	} {
		t.Run(fmt.Sprint(c.wares), func(t *testing.T) {
			f := newBackFixture(t)
			f.catalog(f.sword, c.wares, 0)
			for _, n := range []string{"A", "B", "C", "D", "E"} {
				f.own(f.pattern(n, 10, 1, 0, -1), 1, 0, 0)
			}
			f.loadLists()
			for key := uint32(0); key < 5; key++ {
				f.destroy(key, 1)
			}
			box := f.open()
			if len(box.rows) != c.wantWares+len(c.want) {
				t.Fatalf("%d records", len(box.rows))
			}
			var lines []string
			for _, r := range box.rows[c.wantWares:] {
				lines = append(lines, strings.TrimPrefix(r.item.name, buyBackMark))
			}
			sameNames(t, "the lines", lines, c.want...)
			if !strings.Contains(box.greeting, c.greeting) {
				t.Errorf("the greeting: %q", box.greeting)
			}
			// every line is the one it says, on the list the client holds each time (an
			// older entry comes into view as a newer goes, where the room is short)
			f.player.Tunar = 1000
			for len(box.rows) > c.wantWares {
				f.aged()
				last := len(box.rows) - 1
				msgs := f.buyItem(last, 1)
				mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
				if got, want := readAdded(t, msgs[1]).item.name, strings.TrimPrefix(box.rows[last].item.name, buyBackMark); got != want {
					t.Errorf("index %d gave %q, want %q", last, got, want)
				}
				box = readShopBox(t, msgs[0])
			}
			if len(f.entries()) != 5-len(c.want) && c.wantWares < 36 {
				t.Errorf("%d entries left", len(f.entries()))
			}
			if c.wantWares+len(c.want) == 40 && len(c.want) == 0 {
				f.player.Tunar = 1000
				mustOps(t, f.buyItem(40, 1), protocol.OpcodeCancelPending)
				if got := readAdded(t, f.buyItem(39, 1)[0]); got.item.name != "Rusty Sword" {
					t.Errorf("the 40th ware gave %q", got.item.name)
				}
			}
		})
	}
	// every greeting fits m_merchantName with either window's own
	for _, base := range []string{merchantGreeting, genieShopGreets} {
		for _, more := range []string{buyBackAtTheEnd, buyBackSomeAtEnd, buyBackNoShelf} {
			if n := len([]rune(base + " " + more)); n > greetingMax {
				t.Errorf("%q + %q is %d characters", base, more, n)
			}
		}
	}
	l := merchantList{held: 1}
	if s := l.greetingWith(strings.Repeat("x", 100)); s != buyBackNoShelf {
		t.Errorf("a greeting too long for both keeps what is said of the lines: %q", s)
	}
}

// An entry whose record has been deleted drops quietly, the others keeping their order and their
// indexes; so does one whose line is up when its record goes (refused, the list sent again).
func TestBuyBackRecordDeleted(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	var ids []int
	for _, n := range []string{"A", "B", "C"} {
		id := f.pattern(n, 10, 1, 0, -1)
		ids = append(ids, id)
		f.own(id, 1, 0, 0)
	}
	f.loadLists()
	for key := uint32(0); key < 3; key++ {
		f.destroy(key, 1)
	}
	if _, err := f.db.Exec(`DELETE FROM itempattern WHERE patternid = ?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	box := f.open()
	sameNames(t, "the list", shopNames(box), "Fishing Bait", "Buy back: A", "Buy back: C")
	if f.tableRows() != 2 {
		t.Errorf("not dropped: %d rows", f.tableRows())
	}
	if got := readAdded(t, f.buyItem(2, 1)[1]); got.item.name != "C" {
		t.Errorf("index 2 gave %q", got.item.name)
	}

	if _, err := f.db.Exec(`DELETE FROM itempattern WHERE patternid = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	f.aged()
	msgs := f.buyItem(1, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	sameNames(t, "the list again", shopNames(readShopBox(t, msgs[0])), "Fishing Bait")
	if f.tableRows() != 0 {
		t.Errorf("%d rows left", f.tableRows())
	}
}

// No Rent never goes in: a record of rent 0 (what a spell conjures) or named in norentvanish (what
// vanishes at a logout), sold or destroyed. One that becomes No Rent after it went stops being offered,
// and a logout's vanishing leaves nothing to buy. The controls: the same record with rent is kept, and
// NO TRADE goes in like anything else.
func TestBuyBackNoRent(t *testing.T) {
	f := newBackFixture(t)
	conjured := f.pattern("Conjured", 10, 1, 0, -1)
	vanishing := f.pattern("Vanishing", 10, 1, 0, -1)
	later := f.pattern("Later", 10, 1, 0, -1)
	bound := f.pattern("Bound", 10, 1, 0, -1)
	if _, err := f.db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid = ?`, conjured); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO norentvanish (patternid, note) VALUES (?, 'test')`, vanishing); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE itempattern SET trade = 1 WHERE patternid = ?`, bound); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{conjured, vanishing, conjured, vanishing, later, bound} {
		f.own(id, 1, 0, 0)
	}
	f.loadLists()
	f.player.Tunar = 1000
	mustOps(t, f.sellItem(0, 1), protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	mustOps(t, f.sellItem(1, 1), protocol.OpcodeRemoveInvItem, protocol.OpcodePlayerTunar)
	f.destroy(2, 1)
	f.destroy(3, 1)
	if f.tableRows() != 0 {
		t.Fatalf("No Rent went in: %+v", f.entries())
	}
	f.sellItem(4, 1)
	f.destroy(5, 1)
	sameNames(t, "kept", shopNames(f.open()), "Buy back: Later", "Buy back: Bound")

	if _, err := f.db.Exec(`INSERT INTO norentvanish (patternid, note) VALUES (?, 'test')`, later); err != nil {
		t.Fatal(err)
	}
	sameNames(t, "once it vanishes", shopNames(f.open()), "Buy back: Bound")
	if f.tableRows() != 1 {
		t.Errorf("%d rows left", f.tableRows())
	}
	if _, err := f.db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid = ?`, bound); err != nil {
		t.Fatal(err)
	}
	if box := f.open(); len(box.rows) != 0 || box.greeting != merchantGreeting {
		t.Errorf("once it is No Rent: %q, %q", shopNames(box), box.greeting)
	}
}

// Only a sale and a Destroy make an entry: the bank's moves make none (the control: the sale after them
// does). keepBuyBack has those two callers and no other, so a corpse, a trade, the auction and a quest,
// which all take items by their own paths, cannot.
func TestBuyBackOnlyFromASaleOrADestroy(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	bank := func(fromBank byte, key uint32) {
		f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) {
			w.WriteUint32(shopBankerID)
			w.WriteByte(fromBank)
			w.WriteUint32(key)
			w.Write7BitEncodedInt64(1)
		})
		f.msgs()
	}
	bank(0, 0)
	bank(1, 0)
	if f.tableRows() != 0 {
		t.Errorf("the bank made an entry")
	}
	key := sessionInvList(f.sess).nextKey() - 1
	f.sellItem(key, 1)
	if f.tableRows() != 1 {
		t.Errorf("the sale made none")
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`\bkeepBuyBack\(`)
	callers := map[string]int{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "buyback.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(call.FindAll(src, -1)); n > 0 {
			callers[name] = n
		}
	}
	if len(callers) != 2 || callers["merchant.go"] != 1 || callers["destroy.go"] != 1 {
		t.Errorf("keepBuyBack's callers: %v", callers)
	}
}

// Another character's entries are not offered, and a line cannot reach one.
func TestBuyBackIsTheCharactersOwn(t *testing.T) {
	f := newBackFixture(t)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.sellItem(0, 1)
	f.open() // the line noted for this character

	rec := &database.CharacterRecord{CharName: "Other", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(f.db).Create(rec); err != nil {
		t.Fatal(err)
	}
	first := f.player
	f.player = &entity.Player{ServerID: 1002, CharServerID: rec.ServerID, Name: "Other", Tunar: 1000}
	f.loadListsFor(rec.ServerID)
	msgs := f.buyItem(0, 1)
	mustOps(t, msgs, protocol.OpcodeMerchantBox, protocol.OpcodeColoredChat, protocol.OpcodeCancelPending)
	if len(readShopBox(t, msgs[0]).rows) != 0 || f.player.Tunar != 1000 {
		t.Errorf("another character's line: %s, %d", ops(msgs), f.player.Tunar)
	}
	if box := f.open(); len(box.rows) != 0 {
		t.Errorf("another character is offered %q", shopNames(box))
	}
	f.player = first
	f.loadLists()
	if box := f.open(); len(box.rows) != 1 {
		t.Errorf("its own has %q", shopNames(box))
	}
}

func (f *backFixture) loadListsFor(serverID uint32) {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(serverID)
	if err != nil {
		f.t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvList(rows))
}

// A long name and a long description: cut to what CLIENT keeps (one longer comes out empty), the mark
// and the sentence first, so they are what stays. The record's own comes back whole with 0x003B.
func TestBuyBackRecordFitsTheClient(t *testing.T) {
	f := newBackFixture(t)
	long := strings.Repeat("n", 60)
	desc := strings.Repeat("d", 250)
	id := f.pattern(long, 10, 5, 0, -1)
	if _, err := f.db.Exec(`UPDATE itempattern SET itemdesc = ? WHERE patternid = ?`, desc, id); err != nil {
		t.Fatal(err)
	}
	f.own(id, 5, 0, 0)
	f.loadLists()
	f.destroy(0, 5)
	f.player.Tunar = 1000
	box := f.open() // readShopBox fails on either limit
	line := box.rows[0]
	if !strings.HasPrefix(line.item.name, buyBackMark+"5 n") || len([]rune(line.item.name)) != itemNameMax {
		t.Errorf("the name: %q", line.item.name)
	}
	if !strings.HasPrefix(line.item.desc, "You destroyed these 5;") || len([]rune(line.item.desc)) != itemDescMax {
		t.Errorf("the description: %q", line.item.desc)
	}
	msgs := f.buyItem(0, 1)
	if got := readAdded(t, msgs[1]); got.item.name != long || got.item.desc != desc || got.item.maxStack != 5 {
		t.Errorf("0x003B: %+v", got)
	}
}

// The window a genie opened is a merchant's like any other: its id comes back with each request, and a
// sale or a buy-back under it sends the list again under it with its own greeting.
func TestBuyBackKeepsTheWindowsOwnIDAndGreeting(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.bait, 1, 0)
	f.own(f.sword, 1, 100, 0)
	f.loadLists()
	f.h.openMerchant(f.sess, f.player, f.npcs[shopMerchantID], shopMerchantID, genieShopGreets)
	if box := readShopBox(t, f.msgs()[0]); box.greeting != genieShopGreets {
		t.Fatalf("the greeting: %q", box.greeting)
	}
	msgs := f.sellItem(0, 1)
	box := readShopBox(t, msgs[0])
	if box.id != shopMerchantID || box.greeting != genieShopGreets+" "+buyBackAtTheEnd {
		t.Errorf("after the sale: %#x %q", box.id, box.greeting)
	}
	msgs = f.buyItem(1, 1)
	if box := readShopBox(t, msgs[0]); box.greeting != genieShopGreets {
		t.Errorf("after the buy-back: %q", box.greeting)
	}
}

// keepBuyBack's window note: the repeat guard goes with the note when the list is sent again.
func TestBuyBackRepeatGuardOutlivesTheList(t *testing.T) {
	w := &merchantWindow{}
	now := time.Now()
	if w.repeated(3, now) {
		t.Error("nothing served yet")
	}
	w.serve(3, now)
	if !w.repeated(3, now.Add(buyBackRepeat-time.Millisecond)) || w.repeated(4, now) || w.repeated(3, now.Add(buyBackRepeat)) {
		t.Error("the guard's window")
	}
}
