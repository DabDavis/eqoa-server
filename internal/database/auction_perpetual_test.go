package database

import (
	"reflect"
	"testing"
	"time"
)

// standing puts up a standing lot as tools/ah_materials_20261114.py does.
func (r *auctionRig) standing(pattern, price int) int {
	r.t.Helper()
	res, err := r.db.Exec(`INSERT INTO auction_lots (seller, seller_name, patternid, stack, remainhp, charges,
		min_bid, buyout, fee, listed_at, ends_at, per_item, perpetual)
		VALUES (0, 'ilovetesters', ?, 1, 100, 0, ?, ?, 0, ?, ?, 1, 1)`,
		pattern, price, price, r.t0.Unix(), AuctionPerpetualEnds)
	if err != nil {
		r.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func (r *auctionRig) buy(who uint32, lot, amount, qty int, on bool) (AuctionBidResult, error) {
	r.t.Helper()
	return r.db.AuctionBid(AuctionBidRequest{Bidder: who, Name: "Buyer", Lot: lot, Amount: amount, Qty: qty,
		Now: r.t0.Add(time.Minute), Perpetual: on})
}

// A buy from a standing lot charges the buyer's bank, files the items into
// it, and leaves the lot as it was, buy after buy; nothing is held, no bid
// is recorded and nobody is paid.
func TestAuctionPerpetualBuyLeavesTheLot(t *testing.T) {
	r := newAuctionRig(t)
	lot := r.standing(r.gem, 100)
	before, _ := r.db.AuctionLotsByID([]int{lot})
	total := r.total()

	res, err := r.buy(r.c, lot, 100, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidPerpetual || res.Bought == nil || res.Bought.Paid != 100 || res.BidderBank != 4900 {
		t.Fatalf("first buy: %+v", res)
	}
	// a stack: price per item x count, onto the stack already there
	res, err = r.buy(r.c, lot, 100, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Bought.Paid != 500 || !res.Bought.Delivery.Merged || r.bank(r.c) != 4400 {
		t.Fatalf("stack buy: %+v, bank %d", res.Bought, r.bank(r.c))
	}
	rows := r.rowsOf(r.c, 1)
	if len(rows) != 1 || rows[0].Pattern.PatternID != r.gem || rows[0].StackLeft != 6 {
		t.Fatalf("bank rows: %+v", rows)
	}
	if got := r.total(); got != total-600 {
		t.Fatalf("tunar: %d, want %d (the 600 paid go nowhere)", got, total-600)
	}
	after, _ := r.db.AuctionLotsByID([]int{lot})
	if len(after) != 1 || !reflect.DeepEqual(after[0], before[0]) {
		t.Fatalf("the lot changed:\n%+v\n%+v", before, after)
	}
	var bids int
	r.db.QueryRow(`SELECT COUNT(*) FROM auction_bids`).Scan(&bids)
	if bids != 0 {
		t.Fatalf("%d bid records", bids)
	}
	r.audit()
}

// What a standing lot refuses: under its price, over a stack, more than the
// bank holds, a bank with no room, and every bid while the gate is off. A
// refusal moves nothing.
func TestAuctionPerpetualRefusals(t *testing.T) {
	r := newAuctionRig(t)
	lot := r.standing(r.gem, 100)
	sword := r.standing(r.sword, 500)
	_, err := r.buy(r.c, lot, 99, 1, true)
	wantRefusal(t, err, "sells for 100 tunar each")
	_, err = r.buy(r.c, lot, 100, 21, true)
	wantRefusal(t, err, "at most 20")
	_, err = r.buy(r.a, sword, 500, 1, true) // bank 1000: twice is too much
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.buy(r.a, sword, 500, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.buy(r.a, sword, 500, 1, true)
	wantRefusal(t, err, "Your bank holds 0 tunar")
	_, err = r.buy(r.c, lot, 100, 1, false)
	wantRefusal(t, err, "closed auction")
	// over the price pays the price
	res, err := r.buy(r.c, lot, 1000, 1, true)
	if err != nil || res.Bought.Paid != 100 {
		t.Fatalf("over the price: %+v %v", res.Bought, err)
	}
	// a full bank: refused, nothing taken
	for i := 0; i < AuctionBankSlots; i++ {
		r.item(r.b, r.sword, 1, 1)
	}
	_, err = r.buy(r.b, lot, 100, 1, true)
	wantRefusal(t, err, "no room")
	if r.bank(r.b) != 1000 {
		t.Fatalf("a refused buy took tunar: %d", r.bank(r.b))
	}
	r.audit()
}

// The sweep never closes nor cancels a standing lot (its seller is no
// character, its end far off), searches show it, and a settle passes it by.
func TestAuctionPerpetualNeverExpires(t *testing.T) {
	r := newAuctionRig(t)
	lot := r.standing(r.gem, 100)
	later := time.Unix(AuctionPerpetualEnds+3600, 0) // even past its stored end
	sw, err := r.db.AuctionSettleDue(later)
	if err != nil {
		t.Fatal(err)
	}
	if len(sw.Settled)+len(sw.Cancelled) != 0 {
		t.Fatalf("swept: %+v", sw)
	}
	if _, ok, err := r.db.AuctionSettle(lot, later); ok || err != nil {
		t.Fatalf("settled: %v %v", ok, err)
	}
	open, err := r.db.AuctionOpenLots(later)
	if err != nil || len(open) != 1 || open[0].ID != lot || !open[0].Perpetual || !open[0].Running(later) {
		t.Fatalf("open lots: %+v %v", open, err)
	}
	closed, err := r.db.AuctionClosedLots(later, later.Add(-AuctionHistory))
	if err != nil || len(closed) != 0 {
		t.Fatalf("closed lots: %+v %v", closed, err)
	}
	if res, err := r.buy(r.c, lot, 100, 1, true); err != nil || res.Bought == nil {
		t.Fatalf("buy after the sweep: %v", err)
	}
	if end := open[0].ShownEnd(r.t0); end != r.t0.Unix()/60*60+AuctionMaxMinutes*60 {
		t.Fatalf("shown end %d", end)
	}
}
