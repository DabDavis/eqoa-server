package database

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// Stacks sold by the item (auctionunits.go) on the auction rig: a stack of 20
// gems (value 10) in the seller's bank, put up by the item; bidders asking for
// quantities at a most per item. Every step is checked against the rows and
// the house's sums (banks plus escrow is what it was, less the fees), and the
// audit.

// listPer puts count gems of a bank row up by the item.
func (r *auctionRig) listPer(who uint32, itemID, count, minBid, buyout int) (AuctionLot, error) {
	r.t.Helper()
	row, err := NewInventoryStore(r.db).GetCharItem(who, itemID)
	stack, pattern := 1, 0
	if err == nil {
		stack, pattern = row.StackLeft, row.Pattern.PatternID
	}
	lot, _, err := r.db.AuctionList(AuctionListing{Seller: who, SellerName: "Seller", ItemID: itemID,
		PatternID: pattern, Stack: stack, MinBid: minBid, Buyout: buyout, Minutes: 720, Now: r.t0,
		PerItem: true, Count: count})
	return lot, err
}

// bidQ is a bid for qty items at most price each.
func (r *auctionRig) bidQ(who uint32, name string, lot, qty, price int, at time.Time) (AuctionBidResult, error) {
	r.t.Helper()
	return r.db.AuctionBid(AuctionBidRequest{Bidder: who, Name: name, Lot: lot, Amount: price, Qty: qty, Now: at, LiveForm: true})
}

func (r *auctionRig) lot(id int) AuctionLot {
	r.t.Helper()
	l, ok, err := auctionLotTx(r.db, id)
	if err != nil || !ok {
		r.t.Fatalf("lot %d: %v %v", id, ok, err)
	}
	return l
}

func (r *auctionRig) escrowOf(lot int, who uint32) int {
	r.t.Helper()
	b, _, err := auctionBidTx(r.db, lot, who)
	if err != nil {
		r.t.Fatal(err)
	}
	return b.Escrow
}

// gemsIn is how many gems a character's bank holds, all its records.
func (r *auctionRig) gemsIn(who uint32) int {
	r.t.Helper()
	n := 0
	for _, e := range r.rowsOf(who, 1) {
		if e.Pattern.PatternID == r.gem {
			n += e.StackLeft
		}
	}
	return n
}

// The live form puts some of a stack up by the item: that many leave the bank
// row (the rest stays), the fee is each item's times the count; the whole
// rest goes with the row. The disc's form puts the whole record up as one lot
// (a bundle), the fee the lot's, as before.
func TestAuctionStackListedByTheItem(t *testing.T) {
	r := newAuctionRig(t)
	id := r.item(r.seller, r.gem, 20, 1)
	before := r.total()
	lot, err := r.listPer(r.seller, id, 5, 250, 600)
	if err != nil {
		t.Fatal(err)
	}
	if !lot.Multi() || lot.Bundle() || lot.Item.StackLeft != 5 || lot.Fee != 2*5 || lot.MinBid != 250 || lot.Buyout != 600 {
		t.Fatalf("lot %+v", lot)
	}
	if got := r.bank(r.seller); got != 1000-10 {
		t.Errorf("seller's bank %d, want 990 (the fee: 1%% of 250, times 5)", got)
	}
	rows := r.rowsOf(r.seller, 1)
	if len(rows) != 1 || rows[0].ItemID != id || rows[0].StackLeft != 15 {
		t.Errorf("the bank row after: %+v", rows)
	}
	if lot.MinNext() != 250 || lot.CurrentBid() != -1 {
		t.Errorf("a new stack's columns: min %d, bid %d", lot.MinNext(), lot.CurrentBid())
	}
	// more than the row holds, or none, is refused and moves nothing
	if _, err := r.listPer(r.seller, id, 16, 250, 600); err == nil {
		t.Error("16 of 15 were listed")
	} else {
		wantRefusal(t, err, "holds 15")
	}
	if _, err := r.listPer(r.seller, id, 0, 250, 600); err == nil {
		t.Error("0 were listed")
	}
	if r.bank(r.seller) != 990 || r.rowsOf(r.seller, 1)[0].StackLeft != 15 {
		t.Error("a refused listing moved something")
	}
	// the whole rest: the row goes
	rest, err := r.listPer(r.seller, id, 15, 10, 20)
	if err != nil || rest.Item.StackLeft != 15 || len(r.rowsOf(r.seller, 1)) != 0 {
		t.Fatalf("the rest: %+v %v (rows %d)", rest, err, len(r.rowsOf(r.seller, 1)))
	}
	if r.bank(r.seller) != 990-15 {
		t.Errorf("the rest's fee: bank %d", r.bank(r.seller))
	}
	// one by the item is a lot of one, the single lot's
	one, err := r.listPer(r.alt, r.item(r.alt, r.gem, 3, 1), 1, 10, 20)
	if err != nil || one.Multi() || one.Bundle() || one.Item.StackLeft != 1 {
		t.Errorf("one by the item: %+v %v", one, err)
	}
	// planted control: the disc's form, the whole record as one lot
	whole, err := r.list(r.alt, r.item(r.alt, r.gem, 7, 1), 250, 600, 720)
	if err != nil || !whole.Bundle() || whole.Multi() || whole.Item.StackLeft != 7 || whole.Fee != 2 {
		t.Errorf("the whole record: %+v %v", whole, err)
	}
	if got := r.total(); got != before-10-15-1-2 {
		t.Errorf("total %d, want %d less the fees", got, before)
	}
	r.audit()
}

// A stack of 5 bought in parts by two bidders: the first asks for 3, the
// second for 2, both win at the starting bid; a third bidder's 1 is outbid
// at once (the winners' prices are higher) and sets the price to one over
// it; at the close each pays that price per item for its units, the rest of
// its escrow back, the seller is paid for all 5 and each bank has its gems.
func TestAuctionStackBoughtInParts(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 5, 1), 5, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()

	res, err := r.bidQ(r.a, "Bidder", lot.ID, 3, 20, r.t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidFirst || res.Units != 3 || res.Taken != 60 || res.BidderBank != 940 || res.Bid.Qty != 3 || res.Bid.Escrow != 60 {
		t.Fatalf("the first bid: %+v", res)
	}
	if l := res.Lot; l.Price != 10 || l.Demand != 3 || l.MinNext() != 10 || l.CurrentBid() != 10 || l.High != r.a {
		t.Errorf("a unit is free: price %d, demand %d, min %d", l.Price, l.Demand, l.MinNext())
	}
	r.audit()

	res, err = r.bidQ(r.b, "Rival", lot.ID, 2, 15, r.t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidTookLead || res.Units != 2 || res.Taken != 30 || len(res.Refunds) != 0 {
		t.Fatalf("the second bid: %+v", res)
	}
	if l := res.Lot; l.Price != 10 || l.Demand != 5 || l.MinNext() != 11 || l.High != r.a {
		t.Errorf("all 5 asked for: price %d, demand %d, min %d, top %d", l.Price, l.Demand, l.MinNext(), l.High)
	}
	r.audit()

	// a bid at the least next bid that does not reach a winner's price: out
	// at once, nothing held, the price one over it
	res, err = r.bidQ(r.c, "Buyer", lot.ID, 1, 12, r.t0.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidOutbid || res.Units != 0 || res.Taken != 0 || res.Bid.Escrow != 0 || res.BidderBank != 5000 {
		t.Fatalf("the third bid: %+v", res)
	}
	if l := res.Lot; l.Price != 13 || l.MinNext() != 14 || l.Demand != 5 {
		t.Errorf("after the third: price %d, min %d", l.Price, l.MinNext())
	}
	if r.escrowOf(lot.ID, r.a) != 60 || r.escrowOf(lot.ID, r.b) != 30 {
		t.Errorf("the winners' escrow moved: %d %d", r.escrowOf(lot.ID, r.a), r.escrowOf(lot.ID, r.b))
	}
	if _, err := r.bidQ(r.c, "Buyer", lot.ID, 1, 13, r.t0); err == nil {
		t.Error("a bid under the least next was taken")
	} else {
		wantRefusal(t, err, "least you may bid")
	}
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	r.audit()

	sw, err := r.db.AuctionSettleDue(time.Unix(lot.EndsAt, 0))
	if err != nil || len(sw.Settled) != 1 {
		t.Fatalf("sweep: %+v %v", sw, err)
	}
	st := sw.Settled[0]
	if !st.Multi || st.Sold != 5 || len(st.Winners) != 2 || len(st.Deliveries) != 2 || st.Lot.State != AuctionSold {
		t.Fatalf("settlement: %+v", st)
	}
	wa, wb := st.Winners[0], st.Winners[1]
	if wa.Bidder != r.a || wa.Units != 3 || wa.Paid != 39 || wa.Refund != 21 || wa.Bank != 961 || wa.Bid.Won != 3 {
		t.Errorf("the first winner %+v", wa)
	}
	if wb.Bidder != r.b || wb.Units != 2 || wb.Paid != 26 || wb.Refund != 4 || wb.Bank != 974 {
		t.Errorf("the second winner %+v", wb)
	}
	if r.bank(r.seller) != 1000-5+65 || st.SellerBank != 1060 {
		t.Errorf("the seller's bank %d / %d, want 1060", r.bank(r.seller), st.SellerBank)
	}
	if r.bank(r.c) != 5000 {
		t.Errorf("the outbid bidder %d", r.bank(r.c))
	}
	if r.gemsIn(r.a) != 3 || r.gemsIn(r.b) != 2 || r.gemsIn(r.seller) != 0 {
		t.Errorf("gems: %d %d, seller %d", r.gemsIn(r.a), r.gemsIn(r.b), r.gemsIn(r.seller))
	}
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	notes, _ := r.db.AuctionNotices(r.a)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "won 3 Rough Gem at 13 tunar each (39 in all)") {
		t.Errorf("the winner's lines %+v", notes)
	}
	notes, _ = r.db.AuctionNotices(r.seller)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "sold all 5 at 13 tunar each, 65 tunar in all") {
		t.Errorf("the seller's lines %+v", notes)
	}
	r.audit()
	// planted control: the sweep again moves nothing
	if sw, err := r.db.AuctionSettleDue(time.Unix(lot.EndsAt, 0).Add(time.Hour)); err != nil || len(sw.Settled) != 0 {
		t.Errorf("a second sweep: %+v %v", sw, err)
	}
	if r.total() != total || r.gemsIn(r.a) != 3 {
		t.Error("a second sweep moved something")
	}
}

// A partly filled bid sets the price to its own (it is the highest left
// short); a bidder's new bid replaces its own, asking for more at a higher
// price, and the other winner is left with one of its three at the close.
func TestAuctionStackPartialFill(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 5, 1), 5, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 3, 20, r.t0); err != nil {
		t.Fatal(err)
	}
	res, err := r.bidQ(r.b, "Rival", lot.ID, 4, 15, r.t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Units != 2 || res.Bid.Escrow != 60 || res.Lot.Price != 15 || res.Lot.MinNext() != 16 || res.Lot.Demand != 7 {
		t.Fatalf("a partly filled bid: units %d, escrow %d, price %d, min %d", res.Units, res.Bid.Escrow, res.Lot.Price, res.Lot.MinNext())
	}
	r.audit()
	// fewer than it asked before is refused; the same bid again is refused
	if _, err := r.bidQ(r.b, "Rival", lot.ID, 3, 30, r.t0); err == nil {
		t.Error("a new bid asked for fewer")
	} else {
		wantRefusal(t, err, "may not ask for fewer")
	}
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 3, 20, r.t0); err == nil {
		t.Error("the same bid again was taken")
	}
	res, err = r.bidQ(r.b, "Rival", lot.ID, 4, 21, r.t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidRaised || res.Units != 4 || res.Taken != 84-60 || res.Lot.Price != 20 || res.Lot.High != r.b {
		t.Fatalf("the raise: %+v", res)
	}
	if r.escrowOf(lot.ID, r.a) != 60 {
		t.Errorf("the other winner's escrow %d", r.escrowOf(lot.ID, r.a))
	}
	r.audit()
	st, ok, err := r.db.AuctionSettle(lot.ID, time.Unix(lot.EndsAt, 0))
	if err != nil || !ok || len(st.Winners) != 2 {
		t.Fatalf("settle: %+v %v %v", st, ok, err)
	}
	if w := st.Winners[0]; w.Bidder != r.b || w.Units != 4 || w.Paid != 80 || w.Refund != 4 {
		t.Errorf("the raiser %+v", w)
	}
	if w := st.Winners[1]; w.Bidder != r.a || w.Units != 1 || w.Paid != 20 || w.Refund != 40 {
		t.Errorf("the one left short %+v", w)
	}
	if r.bank(r.seller) != 1000-5+100 || r.gemsIn(r.a) != 1 || r.gemsIn(r.b) != 4 || r.total() != total {
		t.Errorf("seller %d, gems %d %d, total %d/%d", r.bank(r.seller), r.gemsIn(r.a), r.gemsIn(r.b), r.total(), total)
	}
	r.audit()
}

// Buy all now: the whole stack at the buyout per item takes it at once, every
// other bid's escrow back; the buyout for fewer than all is refused with the
// live client's word 1858; over the buyout is the buyout.
func TestAuctionStackBuyAll(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 5, 1), 5, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 3, 20, r.t0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.c, "Buyer", lot.ID, 4, 50, r.t0); err == nil {
		t.Fatal("the buyout for 4 of 5 was taken")
	} else {
		wantRefusal(t, err, "You can only bid the buy-it-now price if you bid on all 5 items in the stack.")
	}
	res, err := r.bidQ(r.c, "Buyer", lot.ID, 5, 70, r.t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidBoughtOut || res.Settled == nil || len(res.Refunds) != 1 || res.Refunds[0].Bidder != r.a || res.Refunds[0].Amount != 60 {
		t.Fatalf("buy all: %+v", res)
	}
	st := res.Settled
	if st.Lot.State != AuctionSold || st.Lot.Price != 50 || len(st.Winners) != 1 || st.Winners[0].Paid != 250 || st.Winners[0].Refund != 0 {
		t.Errorf("settled: %+v", st)
	}
	if r.bank(r.c) != 5000-250 || r.bank(r.a) != 1000 || r.bank(r.seller) != 1000-5+250 || r.gemsIn(r.c) != 5 {
		t.Errorf("banks: buyer %d, outbid %d, seller %d; gems %d", r.bank(r.c), r.bank(r.a), r.bank(r.seller), r.gemsIn(r.c))
	}
	notes, _ := r.db.AuctionNotices(r.a)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "bought outright; your 60 tunar") {
		t.Errorf("the outbid bidder's lines %+v", notes)
	}
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	if _, err := r.bidQ(r.b, "Rival", lot.ID, 1, 20, r.t0.Add(2*time.Minute)); err == nil {
		t.Error("a bid on a lot bought out was taken")
	}
	r.audit()
}

// What the live client guarantees is checked here too: no more than the
// stack, a bank that holds the bid, the seller's account never; and a stack
// no one bid on goes back to its seller whole, a winner's bank with no room
// keeping its units at the house until it has room.
func TestAuctionStackRefusalsAndRoom(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 6, 1), 4, 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 5, 20, r.t0); err == nil {
		t.Error("5 of 4")
	} else {
		wantRefusal(t, err, "only 4")
	}
	if _, err := r.bidQ(r.alt, "Selleralt", lot.ID, 1, 20, r.t0); err == nil {
		t.Error("the seller's account bid")
	}
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 4, 300, r.t0); err == nil {
		t.Error("a bid of 1200 from a bank of 1000")
	} else {
		wantRefusal(t, err, "bank holds 1000")
	}
	if r.bank(r.a) != 1000 {
		t.Errorf("a refused bid took %d", 1000-r.bank(r.a))
	}
	// the winner's bank is full of other things
	for i := 0; i < AuctionBankSlots; i++ {
		r.item(r.b, r.pattern("Trinket", 1, 1, 0, 0, 1), 1, 1)
	}
	if _, err := r.bidQ(r.b, "Rival", lot.ID, 2, 30, r.t0); err != nil {
		t.Fatal(err)
	}
	st, ok, err := r.db.AuctionSettle(lot.ID, time.Unix(lot.EndsAt, 0))
	if err != nil || !ok || st.Sold != 2 || len(st.Deliveries) != 1 || st.Deliveries[0].Holder != r.seller || st.Deliveries[0].Item.StackLeft != 2 {
		t.Fatalf("settle: %+v %v %v", st, ok, err)
	}
	// the unsold 2 merged onto the seller's 2 left in its bank
	if r.gemsIn(r.seller) != 4 || len(r.rowsOf(r.seller, 1)) != 1 || !st.Deliveries[0].Merged {
		t.Errorf("the seller's gems %d in %d rows", r.gemsIn(r.seller), len(r.rowsOf(r.seller, 1)))
	}
	if n, _ := r.db.AuctionWaiting(r.b); n != 1 {
		t.Errorf("waiting for the winner: %d", n)
	}
	if r.bank(r.b) != 1000-20 {
		t.Errorf("the winner paid %d (2 at the starting bid 10)", 1000-r.bank(r.b))
	}
	got, left, err := r.db.AuctionDeliverPending(r.b, r.t0)
	if err != nil || len(got) != 0 || left != 1 {
		t.Errorf("no room yet: %v %d %v", got, left, err)
	}
	if err := NewInventoryStore(r.db).DeleteCharItem(r.b, r.rowsOf(r.b, 1)[0].ItemID); err != nil {
		t.Fatal(err)
	}
	got, left, err = r.db.AuctionDeliverPending(r.b, r.t0)
	if err != nil || len(got) != 1 || left != 0 || got[0].Item.StackLeft != 2 || r.gemsIn(r.b) != 2 {
		t.Errorf("with room: %+v %d %v (gems %d)", got, left, err, r.gemsIn(r.b))
	}
	notes, _ := r.db.AuctionNotices(r.b)
	if len(notes) != 2 || !strings.Contains(notes[0].Line, "wait at the auction house") || !strings.Contains(notes[1].Line, "Rough Gem (2) from the auction house") {
		t.Errorf("the winner's lines %+v", notes)
	}
	// planted control: delivered once
	if got, _, _ := r.db.AuctionDeliverPending(r.b, r.t0); len(got) != 0 || r.gemsIn(r.b) != 2 {
		t.Error("a share was delivered twice")
	}
	r.audit()
}

// A bidder's character deleted with its bid winning units: its escrow goes
// with it and the stack is priced again without it; a seller's deleted: every
// bid has its escrow back and the lot is withdrawn.
func TestAuctionStackDeletedCharacters(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 5, 1), 3, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 2, 30, r.t0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.b, "Rival", lot.ID, 2, 20, r.t0); err != nil {
		t.Fatal(err)
	}
	if l := r.lot(lot.ID); l.Price != 20 || l.Demand != 4 {
		t.Fatalf("before: price %d, demand %d", l.Price, l.Demand)
	}
	store := NewSQLiteCharacterStore(r.db)
	if err := store.Delete(r.a); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.AuctionSettleDue(r.t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if l := r.lot(lot.ID); l.Price != 10 || l.Demand != 2 || l.High != r.b || l.HighMax != 40 || l.MinNext() != 10 {
		t.Errorf("without the deleted bidder: %+v", l)
	}
	r.audit()
	other, err := r.listPer(r.alt, r.item(r.alt, r.gem, 4, 1), 4, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.c, "Buyer", other.ID, 2, 30, r.t0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.bidQ(r.b, "Rival", other.ID, 1, 25, r.t0); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(r.alt); err != nil {
		t.Fatal(err)
	}
	sw, err := r.db.AuctionSettleDue(r.t0.Add(2 * time.Minute))
	if err != nil || len(sw.Cancelled) != 1 || len(sw.Refunds) != 2 {
		t.Fatalf("sweep: %+v %v", sw, err)
	}
	if r.bank(r.c) != 5000 || r.bank(r.b) != 1000-40 {
		t.Errorf("after the withdrawal: %d, %d", r.bank(r.c), r.bank(r.b))
	}
	r.audit()
}

// Twenty bids racing on one stack: every one either moved what it should or
// nothing, and the house's sums and audit hold after.
func TestAuctionStackConcurrentBids(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listPer(r.seller, r.item(r.seller, r.gem, 10, 1), 10, 10, 100000)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := []uint32{r.a, r.b, r.c}[i%3]
			r.bidQ(who, "Racer", lot.ID, 1+i%4, 15+i*7%40, r.t0)
		}(i)
	}
	wg.Wait()
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	r.audit()
	st, ok, err := r.db.AuctionSettle(lot.ID, time.Unix(lot.EndsAt, 0))
	if err != nil || !ok {
		t.Fatalf("settle: %v %v", ok, err)
	}
	sold := 0
	for _, w := range st.Winners {
		sold += w.Units
	}
	if sold != st.Sold || r.gemsIn(r.a)+r.gemsIn(r.b)+r.gemsIn(r.c)+r.gemsIn(r.seller) != 10 || r.total() != total {
		t.Errorf("sold %d (%d), gems %d, total %d/%d", sold, st.Sold,
			r.gemsIn(r.a)+r.gemsIn(r.b)+r.gemsIn(r.c)+r.gemsIn(r.seller), r.total(), total)
	}
	r.audit()
}

// A stack a disc client put up whole, bid on by a client of the live form: it
// bids for all of it at a price per item, which is the whole lot's price
// times the stack; for fewer it is refused. The disc client's own bid (one,
// the whole lot's price) is as before.
func TestAuctionBundleBidByTheItem(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.gem, 7, 1), 100, 1000, 720)
	if err != nil || !lot.Bundle() {
		t.Fatalf("the bundle: %+v %v", lot, err)
	}
	if _, err := r.bidQ(r.a, "Bidder", lot.ID, 1, 20, r.t0); err == nil {
		t.Error("one of a bundle was taken from the live form")
	} else {
		wantRefusal(t, err, "whole stack of 7")
	}
	res, err := r.bidQ(r.a, "Bidder", lot.ID, 7, 20, r.t0)
	if err != nil || res.Kind != BidFirst || res.Bid.Max != 140 || res.Taken != 140 || res.Lot.Price != 100 {
		t.Fatalf("7 at 20 each: %+v %v", res, err)
	}
	// the disc's own: one, the whole lot's price
	res, err = r.bid(r.b, "Rival", lot.ID, 200, r.t0)
	if err != nil || res.Kind != BidTookLead || res.Lot.Price != 141 {
		t.Fatalf("the disc's bid: %+v %v", res, err)
	}
	// and the disc's with a count over one is refused
	if _, err := r.db.AuctionBid(AuctionBidRequest{Bidder: r.c, Name: "Buyer", Lot: lot.ID, Amount: 300, Qty: 2, Now: r.t0}); err == nil {
		t.Error("a count of 2 on a lot sold as one")
	}
	r.audit()
}

// standUnits, the rule alone: with one item it is the single lot's proxy rule
// (one over the next highest, never over the leader's own); a free unit is
// the starting bid; ties go to the earlier bid.
func TestStandUnitsRule(t *testing.T) {
	bid := func(who uint32, qty, max, seq int) AuctionBid {
		return AuctionBid{Bidder: who, Qty: qty, Max: max, Seq: seq}
	}
	for _, c := range []struct {
		name    string
		in, out []AuctionBid
		n, min  int
		price   int
		units   map[uint32]int
		top     uint32
		demand  int
	}{
		{"one alone", []AuctionBid{bid(1, 1, 300, 1)}, nil, 1, 100, 100, map[uint32]int{1: 1}, 1, 1},
		{"one over the next", []AuctionBid{bid(1, 1, 300, 1), bid(2, 1, 200, 2)}, nil, 1, 100, 201, map[uint32]int{1: 1, 2: 0}, 1, 1},
		{"a tie: the earlier", []AuctionBid{bid(2, 1, 300, 2), bid(1, 1, 300, 1)}, nil, 1, 100, 300, map[uint32]int{1: 1, 2: 0}, 1, 1},
		{"an outbid one still counts", []AuctionBid{bid(1, 1, 300, 1)}, []AuctionBid{bid(2, 1, 250, 2)}, 1, 100, 251, map[uint32]int{1: 1}, 1, 1},
		{"a free unit", []AuctionBid{bid(1, 2, 30, 1)}, nil, 3, 10, 10, map[uint32]int{1: 2}, 1, 2},
		{"a partly filled bid", []AuctionBid{bid(1, 3, 20, 1), bid(2, 4, 15, 2)}, nil, 5, 10, 15, map[uint32]int{1: 3, 2: 2}, 1, 7},
		{"exactly filled", []AuctionBid{bid(1, 3, 20, 1), bid(2, 2, 15, 2)}, nil, 5, 10, 10, map[uint32]int{1: 3, 2: 2}, 1, 5},
		{"none", nil, nil, 5, 10, 0, map[uint32]int{}, 0, 0},
	} {
		st := standUnits(c.in, c.out, c.n, c.min)
		if st.price != c.price || st.top.Bidder != c.top || st.demand != c.demand {
			t.Errorf("%s: price %d top %d demand %d, want %d %d %d", c.name, st.price, st.top.Bidder, st.demand, c.price, c.top, c.demand)
		}
		for who, u := range c.units {
			if st.units[who] != u {
				t.Errorf("%s: %d has %d units, want %d", c.name, who, st.units[who], u)
			}
		}
	}
}
