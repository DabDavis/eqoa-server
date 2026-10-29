package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// Stacks over the wire (auction.go, database/auctionunits.go): requests fed
// as the live client's VIBankControl sends them (its count in the same place
// as the disc's constant 1; spec auction-stacks.md section 3) and every reply
// read as its Recv1206 reads it.

// liveForm answers the live forms' offer for a character's session.
func (f *houseFixture) liveForm(c *houseChar, on bool) {
	f.t.Helper()
	v := uint32(0)
	if on {
		v = 1
	}
	payload := build(protocol.OpcodeAuctionForm, 8, func(w *buffer.Writer) { w.WriteUint32(v) })[2:]
	if err := f.house.HandleForm(c.sess, incoming(protocol.OpcodeAuctionForm, payload)); err != nil {
		f.t.Fatal(err)
	}
}

// sellN is Sell with the live form's quantity.
func (f *houseFixture) sellN(c *houseChar, key uint32, count, price, buyout, minutes int) {
	f.request(c, 5, func(w *buffer.Writer) {
		w.WriteUint32(houseBankerID)
		w.WriteUint32(key)
		for _, v := range []int{count, price, buyout, minutes} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
}

// bidN is Send06 with the live form's quantity: the lot, the price per item,
// the count.
func (f *houseFixture) bidN(c *houseChar, lot, price, count int) {
	f.request(c, 6, func(w *buffer.Writer) {
		for _, v := range []int{lot, price, count} {
			w.Write7BitEncodedInt64(int64(v))
		}
	})
}

// columns137 is RecvUpdateLot's lot and its three columns.
func columns137(t *testing.T, msgs []sentMsg) (wireLot, [3]int64) {
	t.Helper()
	d, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 137)
	if !ok {
		t.Fatal("no 137")
	}
	r := &auctionWire{b: d[1:]}
	l := r.lot()
	r.zz() // the page's minutes
	cols := [3]int64{r.zz(), r.zz(), r.zz()}
	if r.bad || r.p != len(d)-1 {
		t.Fatalf("137 read %d of %d", r.p, len(d)-1)
	}
	return l, cols
}

func bidRecord(t *testing.T, msgs []sentMsg, sub byte) wireBid {
	t.Helper()
	d, ok := findSent(msgs, protocol.OpcodeAuctionStuff, sub)
	if !ok {
		t.Fatalf("no %d", sub)
	}
	r := &auctionWire{b: d[1:]}
	b := readBid(r)
	if r.bad || r.p != len(d)-1 {
		t.Fatalf("%d read %d of %d", sub, r.p, len(d)-1)
	}
	return b
}

// A live-form client puts 5 of a stack of 20 up by the item: the bank's
// record keeps 15 (0x1250 takes 5), the fee is each item's times 5, the lot
// is 5 at the prices per item. Controls: a disc-form client's count of 1
// puts its whole record up, as before; a count over 1 is a quantity from any
// client; the live form's 1 is one item.
func TestAuctionStackSellOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Other")
	s, o := f.chars["Seller"], f.chars["Other"]
	f.give(s, f.gem, 20)
	f.liveForm(s, true)
	f.sellN(s, 0, 5, 250, 600, 720)
	msgs := f.drain(s)
	took, ok := findSent(msgs, protocol.OpcodeRemoveBankItem, 0)
	if r := (&auctionWire{b: took}); !ok || r.u8() != 0 || r.zz() != 5 || r.bad {
		t.Fatalf("0x1250 % X", took)
	}
	if n, _ := bankTunarOf(t, msgs); n != 990 || s.player.BankTunar != 990 || f.dbBank(s) != 990 {
		t.Errorf("the fee: sent %d, game %d, kept %d; want 990 (2 each for 5)", n, s.player.BankTunar, f.dbBank(s))
	}
	add, _ := findSent(msgs, protocol.OpcodeAuctionStuff, 130)
	r := &auctionWire{b: add[1:]}
	if l := r.lot(); r.bad || l.qty != 5 || l.min != 250 || l.buyout != 600 || l.item.name != "Rough Gem" {
		t.Errorf("the lot %+v", l)
	}
	if rows := f.bankRows(s); len(rows) != 1 || rows[0].StackLeft != 15 {
		t.Errorf("the bank rows %+v", rows)
	}
	if rec, ok := sessionBankList(s.sess).at(0); !ok || rec.qty != 15 {
		t.Errorf("the list copy %+v", rec)
	}
	if ls := sentLines(msgs); len(ls) != 1 || ls[0] != "5 Rough Gem are up for auction, from 250 tunar each. The listing fee was 10 tunar." {
		t.Errorf("lines %q", ls)
	}
	// the live form's 1 of a stack is one item
	f.sellN(s, 0, 1, 250, 600, 720)
	if msgs := f.drain(s); len(f.bankRows(s)) != 1 || f.bankRows(s)[0].StackLeft != 14 {
		t.Errorf("one by the live form: %+v (%v)", f.bankRows(s), sentLines(msgs))
	}
	// planted control: the disc's form, count 1: the whole record
	f.give(o, f.gem, 7)
	f.sellN(o, 0, 1, 100, 1000, 720)
	msgs = f.drain(o)
	took, _ = findSent(msgs, protocol.OpcodeRemoveBankItem, 0)
	if r := (&auctionWire{b: took}); r.u8() != 0 || r.zz() != 7 {
		t.Errorf("the disc's sale took % X", took)
	}
	if n, _ := bankTunarOf(t, msgs); n != 999 || len(f.bankRows(o)) != 0 {
		t.Errorf("the disc's sale: bank %d, rows %d", n, len(f.bankRows(o)))
	}
	add, _ = findSent(msgs, protocol.OpcodeAuctionStuff, 130)
	if l := (&auctionWire{b: add[1:]}).lot(); l.qty != 7 || l.min != 100 || l.buyout != 1000 {
		t.Errorf("the disc's lot %+v", l)
	}
	// a count over 1 is a quantity whoever sends it
	f.give(o, f.gem, 9)
	f.sellN(o, 0, 3, 10, 20, 720)
	if f.drain(o); len(f.bankRows(o)) != 1 || f.bankRows(o)[0].StackLeft != 6 {
		t.Errorf("3 from the disc's form: %+v", f.bankRows(o))
	}
	// more than the record holds is refused and moves nothing
	f.sellN(o, 0, 7, 10, 20, 720)
	msgs = f.drain(o)
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "holds 6") || !sentClears(msgs) {
		t.Errorf("7 of 6: %q", ls)
	}
	if f.bankRows(o)[0].StackLeft != 6 {
		t.Error("a refused sale took gems")
	}
}

// The stack of 5 bought in parts over the wire: each bid's record (132) is
// its quantity at its price per item with qty x price in escrow, the lot's
// columns are the price every winner pays and the least next bid, a bid that
// reaches no unit holds nothing; the history shows the winners at the price
// with their quantities; at the close each winner's bank gets its gems
// (0x1251) and its change (0x1253), the seller its pay.
func TestAuctionStackBoughtInPartsOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder", "Rival", "Buyer")
	s, a, b, c := f.chars["Seller"], f.chars["Bidder"], f.chars["Rival"], f.chars["Buyer"]
	for _, x := range []*houseChar{s, a, b} {
		f.liveForm(x, true)
	}
	f.give(s, f.gem, 5)
	f.sellN(s, 0, 5, 10, 50, 720)
	f.drain(s)

	f.bidN(a, 1, 20, 3)
	msgs := f.drain(a)
	if rb := bidRecord(t, msgs, 132); rb.char != int64(a.sid) || rb.max != 20 || rb.x0c != 3 || rb.escrow != 60 {
		t.Errorf("the first record %+v", rb)
	}
	if n, _ := bankTunarOf(t, msgs); n != 940 || !sentClears(msgs) {
		t.Errorf("the first bid's bank %d", n)
	}
	if _, cols := columns137(t, msgs); cols[0] != int64(a.sid) || cols[1] != 10 || cols[2] != 10 {
		t.Errorf("a unit free: columns %v, want leader, 10, 10", cols)
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "wins them now at 10 tunar each. 60 tunar is held") {
		t.Errorf("the first bid's line %q", ls)
	}
	f.bidN(b, 1, 15, 2)
	msgs = f.drain(b)
	if rb := bidRecord(t, msgs, 132); rb.max != 15 || rb.x0c != 2 || rb.escrow != 30 {
		t.Errorf("the second record %+v", rb)
	}
	if _, cols := columns137(t, msgs); cols[1] != 10 || cols[2] != 11 {
		t.Errorf("all asked for: columns %v, want 10, 11", cols)
	}
	if _, ok := findSent(f.drain(s), protocol.OpcodeAuctionStuff, 137); !ok {
		t.Error("the seller was not shown the lot")
	}
	// a disc-form client's bid: one, outbid at once
	f.bidN(c, 1, 12, 1)
	msgs = f.drain(c)
	if rb := bidRecord(t, msgs, 132); rb.escrow != 0 || rb.max != 12 || rb.x0c != 1 {
		t.Errorf("the third record %+v", rb)
	}
	if _, ok := bankTunarOf(t, msgs); ok {
		t.Error("a bid that won nothing moved tunar")
	}
	if _, cols := columns137(t, msgs); cols[1] != 13 || cols[2] != 14 {
		t.Errorf("after the third: columns %v, want 13, 14", cols)
	}
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "Higher bids take all 5 Rough Gem") {
		t.Errorf("the third's line %q", ls)
	}
	// the history: the winners at the price they pay, with their quantities
	f.request(a, 9, func(w *buffer.Writer) { w.Write7BitEncodedInt64(1) })
	d, ok := findSent(f.drain(a), protocol.OpcodeAuctionStuff, 135)
	if !ok {
		t.Fatal("no 135")
	}
	r := &auctionWire{b: d[1:]}
	r.zz()
	type row struct {
		name     string
		bid, qty int64
	}
	var rows []row
	for n := r.zz(); n > 0; n-- {
		r.zz()
		r.zz()
		r.zz()
		x := row{name: r.str8()}
		x.bid, x.qty = r.zz(), r.zz()
		rows = append(rows, x)
	}
	if r.bad || r.p != len(d)-1 || len(rows) != 3 || rows[0] != (row{"Bidder", 13, 3}) || rows[1] != (row{"Rival", 13, 2}) || rows[2] != (row{"Buyer", 12, 1}) {
		t.Errorf("the history %+v", rows)
	}

	f.now = f.now.Add(13 * time.Hour)
	f.house.Sweep()
	for _, x := range []struct {
		c          *houseChar
		gems, bank int64
	}{{a, 3, 961}, {b, 2, 974}} {
		msgs := f.drain(x.c)
		add, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0)
		if !ok {
			t.Errorf("%s: no 0x1251", x.c.name)
			continue
		}
		if stack := (&auctionWire{b: add}).zz(); stack != x.gems {
			t.Errorf("%s was filed %d gems, want %d", x.c.name, stack, x.gems)
		}
		if n, _ := bankTunarOf(t, msgs); n != x.bank || int64(x.c.player.BankTunar) != x.bank {
			t.Errorf("%s's bank %d, want %d", x.c.name, n, x.bank)
		}
		if rb := bidRecord(t, msgs, 134); rb.escrow != 0 {
			t.Errorf("%s still holds %d", x.c.name, rb.escrow)
		}
		if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "at 13 tunar each") {
			t.Errorf("%s's lines %q", x.c.name, ls)
		}
		if rows := f.bankRows(x.c); len(rows) != 1 || int64(rows[0].StackLeft) != x.gems {
			t.Errorf("%s's rows %+v", x.c.name, rows)
		}
	}
	sm := f.drain(s)
	if n, _ := bankTunarOf(t, sm); n != 1060 {
		t.Errorf("the seller's bank %d, want 1060", n)
	}
	if ls := sentLines(sm); len(ls) != 1 || !strings.Contains(ls[0], "sold all 5 at 13 tunar each") {
		t.Errorf("the seller's lines %q", ls)
	}
	if got := f.drain(c); len(got) != 0 {
		t.Errorf("the outbid bidder was sent %d messages at the close", len(got))
	}
	if problems, _ := f.db.AuctionAudit(); len(problems) != 0 {
		t.Errorf("audit: %v", problems)
	}
}

// A disc-form client on a stack: its "Acquire now" (the buyout for its one)
// is refused with the live client's word 1858, its one below the buyout
// takes a unit; buy all from the live form takes the stack and the one who
// held a unit has its escrow back.
func TestAuctionStackDiscClientAndBuyAll(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Disc", "Live")
	s, d, l := f.chars["Seller"], f.chars["Disc"], f.chars["Live"]
	f.liveForm(s, true)
	f.liveForm(l, true)
	f.give(s, f.gem, 5)
	f.sellN(s, 0, 5, 10, 50, 720)
	f.drain(s)
	f.bid(d, 1, 50) // the disc's Send06: lot, amount, 1
	msgs := f.drain(d)
	if ls := sentLines(msgs); len(ls) != 1 || ls[0] != "You can only bid the buy-it-now price if you bid on all 5 items in the stack." {
		t.Errorf("the disc's buyout: %q", ls)
	}
	if m, ok := findSent(msgs, protocol.OpcodeErrorMessage, 0); !ok || len(m) == 0 || !sentClears(msgs) {
		t.Error("the refusal is not red, or leaves the wait")
	}
	f.bid(d, 1, 30)
	msgs = f.drain(d)
	if rb := bidRecord(t, msgs, 132); rb.x0c != 1 || rb.escrow != 30 {
		t.Errorf("the disc's one: %+v", rb)
	}
	f.bidN(l, 1, 50, 5)
	lm := f.drain(l)
	if add, ok := findSent(lm, protocol.OpcodeAddBankItem, 0); !ok || (&auctionWire{b: add}).zz() != 5 {
		t.Errorf("buy all: no 5 gems filed (%v)", lm)
	}
	if n, _ := bankTunarOf(t, lm); n != 750 {
		t.Errorf("buy all's bank %d", n)
	}
	dm := f.drain(d)
	if n, ok := bankTunarOf(t, dm); !ok || n != 1000 {
		t.Errorf("the disc bidder's escrow back: %d (%v)", n, ok)
	}
	if ls := sentLines(dm); len(ls) != 1 || !strings.Contains(ls[0], "bought outright; your 30 tunar") {
		t.Errorf("the disc bidder's lines %q", ls)
	}
	if n, _ := bankTunarOf(t, f.drain(s)); n != 995+250 {
		t.Errorf("the seller's bank %d", n)
	}
}

// A stack a disc client put up whole, seen and bid on by a live-form client:
// its prices each item's share, rounded up, its columns too, and it bids for
// the whole stack; the disc client still sees the lot as it put it up.
func TestAuctionBundleShownByTheItem(t *testing.T) {
	f := newHouseFixture(t, "Disc", "Live")
	d, l := f.chars["Disc"], f.chars["Live"]
	f.liveForm(l, true)
	f.give(d, f.gem, 7)
	f.sell(d, 0, 100, 1000, 720)
	f.drain(d)
	search := func(c *houseChar) wirePage {
		t.Helper()
		f.request(c, 8, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(4)
			w.Write7BitEncodedInt64(1)
			w.Write7BitEncodedInt64(1)
		})
		p, ok := findSent(f.drain(c), protocol.OpcodeAuctionStuff, 129)
		if !ok {
			t.Fatal("no page")
		}
		return readPage(t, p)
	}
	if p := search(l); len(p.lots) != 1 || p.lots[0].qty != 7 || p.lots[0].min != 15 || p.lots[0].buyout != 143 ||
		p.cols[1][0] != -1 || p.cols[2][0] != 15 {
		t.Errorf("the live form's page: %+v, columns %v", p.lots, p.cols)
	}
	if p := search(d); p.lots[0].qty != 7 || p.lots[0].min != 100 || p.lots[0].buyout != 1000 || p.cols[2][0] != 100 {
		t.Errorf("the disc's page: %+v, columns %v", p.lots, p.cols)
	}
	f.bidN(l, 1, 15, 1)
	if ls := sentLines(f.drain(l)); len(ls) != 1 || !strings.Contains(ls[0], "whole stack of 7") {
		t.Errorf("one of a bundle: %q", ls)
	}
	f.bidN(l, 1, 15, 7)
	msgs := f.drain(l)
	if rb := bidRecord(t, msgs, 132); rb.max != 15 || rb.x0c != 7 || rb.escrow != 105 {
		t.Errorf("the live form's record %+v", rb)
	}
	if lot, cols := columns137(t, msgs); lot.min != 15 || cols[1] != 15 || cols[2] != 15 {
		t.Errorf("the live form's lot %+v, columns %v (100 whole is 15 each; the least next 101 is 15 each)", lot, cols)
	}
	if lot, cols := columns137(t, f.drain(d)); lot.min != 100 || cols[1] != 100 || cols[2] != 101 {
		t.Errorf("the disc's lot %+v, columns %v", lot, cols)
	}
}

// The offer's answer is the session's: 1 the live forms, 0 the disc's again;
// a short answer changes nothing.
func TestAuctionFormAnswer(t *testing.T) {
	f := newHouseFixture(t, "Seller")
	s := f.chars["Seller"]
	if f.house.liveForm(s.sess) {
		t.Fatal("a session starts with the live forms")
	}
	f.liveForm(s, true)
	if !f.house.liveForm(s.sess) {
		t.Fatal("answered 1: not the live forms")
	}
	f.house.HandleForm(s.sess, incoming(protocol.OpcodeAuctionForm, []byte{0, 0}))
	if !f.house.liveForm(s.sess) {
		t.Fatal("a short answer changed it")
	}
	f.liveForm(s, false)
	if f.house.liveForm(s.sess) {
		t.Fatal("answered 0: still the live forms")
	}
	if (*AuctionHouse)(nil).liveForm(s.sess) {
		t.Fatal("no house has the live forms")
	}
}

// The MemoryDump's bid records keep the disc's 24-byte record on a stack too
// (five zz and a byte: no live x18), its quantity in x0C, read to the end.
func TestAuctionStackDumpKeepsTheDiscRecord(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	f.liveForm(s, true)
	f.liveForm(a, true)
	f.give(s, f.gem, 9)
	f.sellN(s, 0, 4, 10, 50, 720)
	f.bidN(a, 1, 20, 3)
	buf := make([]byte, 8192)
	w := buffer.NewWriter(buf)
	f.house.WriteDumpListsFor(w, a.sess, a.sid)
	r := &auctionWire{b: buf[:w.Position()]}
	if n := r.zz(); n != 0 {
		t.Fatalf("the bidder's lots %d", n)
	}
	if n := r.zz(); n != 1 {
		t.Fatalf("the bidder's bids %d", n)
	}
	if b := readBid(r); b.lot != 1 || b.max != 20 || b.x0c != 3 || b.escrow != 60 {
		t.Errorf("the bid %+v", b)
	}
	if r.bad || r.p != w.Position() {
		t.Errorf("read %d of %d", r.p, w.Position())
	}
	// a live-form client's bid on a stack put up whole, in a later dump: by
	// the item, as its 132 showed it
	f.give(s, f.gem, 7)
	f.liveForm(s, false)
	f.sellN(s, uint32(sessionBankList(s.sess).nextKey()-1), 1, 100, 1000, 720)
	f.bidN(a, 2, 15, 7)
	w = buffer.NewWriter(buf)
	f.house.WriteDumpListsFor(w, a.sess, a.sid)
	r = &auctionWire{b: buf[:w.Position()]}
	r.zz()
	if n := r.zz(); n != 2 {
		t.Fatalf("the bidder's bids %d", n)
	}
	if b := readBid(r); b.lot != 2 || b.max != 15 || b.x0c != 7 || b.escrow != 105 {
		t.Errorf("the bundle bid in the dump %+v", b)
	}
}
