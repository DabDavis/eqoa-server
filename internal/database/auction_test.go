package database

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// The auction house's records (auction.go) on a fresh database: three
// characters on three accounts and one alt, made-up items. Every test checks
// what moved against what the rows hold, and the house's sums: tunar in banks
// plus tunar held is what it was, less the fees.

type auctionRig struct {
	t                    *testing.T
	db                   *SQLiteDB
	seller, alt, a, b, c uint32
	sword, gem, notrade  int // patterns: a single sword (value 50), gems (stack 20), a No Trade one
	lore, unrentable     int
	t0                   time.Time
}

func newAuctionRig(t *testing.T) *auctionRig {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/auction.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &auctionRig{t: t, db: db, t0: time.Date(2026, 10, 29, 12, 0, 30, 0, time.UTC)}
	store := NewSQLiteCharacterStore(db)
	char := func(name string, acct, bank int) uint32 {
		rec := &CharacterRecord{CharName: name, AccountID: acct, BankTunar: bank}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		return rec.ServerID
	}
	r.seller = char("Seller", 1, 1000)
	r.alt = char("Selleralt", 1, 5000)
	r.a = char("Bidder", 2, 1000)
	r.b = char("Rival", 3, 1000)
	r.c = char("Buyer", 4, 5000)
	r.sword = r.pattern("Fine Sword", 50, 1, 0, 0, 1)
	r.gem = r.pattern("Rough Gem", 10, 20, 0, 0, 1)
	r.notrade = r.pattern("Bound Ring", 10, 1, 1, 0, 1)
	r.lore = r.pattern("Unique Charm", 10, 1, 0, 1, 1)
	r.unrentable = r.pattern("Odd Trinket", 10, 1, 0, 0, 0)
	return r
}

func (r *auctionRig) pattern(name string, value, maxStack, trade, lore, rent int) int {
	r.t.Helper()
	res, err := r.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, 0, -1, 0, ?, ?, 0, 0, 0, 0, 1, ?, 100, 0, 32767, 1023, 0, ?, 0, 0, ?, '', 0, 0)`,
		value, trade, rent, maxStack, lore, name)
	if err != nil {
		r.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// item gives a character a row: location 1 the bank, 0 the inventory.
func (r *auctionRig) item(who uint32, pattern, stack, location int) int {
	r.t.Helper()
	id, err := NewInventoryStore(r.db).AddCharItem(who, pattern, stack, 77, 3, location)
	if err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r *auctionRig) bank(who uint32) int {
	r.t.Helper()
	n, ok, err := r.db.BankTunar(who)
	if err != nil || !ok {
		r.t.Fatalf("bank of %d: %v %v", who, ok, err)
	}
	return n
}

// total is every bank's tunar plus what the house holds.
func (r *auctionRig) total() int {
	r.t.Helper()
	var banks, held int
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(bankTunar), 0) FROM characters`).Scan(&banks); err != nil {
		r.t.Fatal(err)
	}
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(escrow), 0) FROM auction_bids`).Scan(&held); err != nil {
		r.t.Fatal(err)
	}
	return banks + held
}

func (r *auctionRig) audit() {
	r.t.Helper()
	problems, err := r.db.AuctionAudit()
	if err != nil {
		r.t.Fatal(err)
	}
	for _, p := range problems {
		r.t.Errorf("audit: %s", p)
	}
}

func (r *auctionRig) rowsOf(who uint32, location int) []CharInventoryEntry {
	r.t.Helper()
	all, err := NewInventoryStore(r.db).GetCharacterItems(who)
	if err != nil {
		r.t.Fatal(err)
	}
	var out []CharInventoryEntry
	for _, e := range all {
		if e.Location == location {
			out = append(out, e)
		}
	}
	return out
}

// list puts the seller's bank row up, as the session's list holds it.
func (r *auctionRig) list(who uint32, itemID, minBid, buyout, minutes int) (AuctionLot, error) {
	r.t.Helper()
	row, err := NewInventoryStore(r.db).GetCharItem(who, itemID)
	stack, pattern := 1, 0
	if err == nil {
		stack, pattern = row.StackLeft, row.Pattern.PatternID
	}
	lot, _, err := r.db.AuctionList(AuctionListing{Seller: who, SellerName: "Seller", ItemID: itemID,
		PatternID: pattern, Stack: stack, MinBid: minBid, Buyout: buyout, Minutes: minutes, Now: r.t0})
	return lot, err
}

func (r *auctionRig) bid(who uint32, name string, lot, amount int, at time.Time) (AuctionBidResult, error) {
	r.t.Helper()
	return r.db.AuctionBid(AuctionBidRequest{Bidder: who, Name: name, Lot: lot, Amount: amount, Now: at})
}

func wantRefusal(t *testing.T, err error, part string) {
	t.Helper()
	line, ok := IsAuctionRefusal(err)
	if !ok {
		t.Fatalf("want a refusal %q, got %v", part, err)
	}
	if !strings.Contains(line, part) {
		t.Fatalf("refusal %q, want %q in it", line, part)
	}
}

// A listing takes the bank row and the fee in one go, and the same row
// cannot be listed twice.
func TestAuctionListTakesTheRowAndTheFee(t *testing.T) {
	r := newAuctionRig(t)
	id := r.item(r.seller, r.sword, 1, 1)
	before := r.total()
	lot, err := r.list(r.seller, id, 250, 900, 720)
	if err != nil {
		t.Fatal(err)
	}
	if lot.State != AuctionOpen || lot.MinBid != 250 || lot.Buyout != 900 || lot.Fee != 2 || lot.Item.Pattern.PatternID != r.sword {
		t.Fatalf("lot %+v", lot)
	}
	// ends at a whole minute, 12 hours on, rounded up
	if lot.EndsAt%60 != 0 || lot.EndsAt < r.t0.Unix()+720*60 || lot.EndsAt >= r.t0.Unix()+720*60+60 {
		t.Errorf("ends %d, listed %d", lot.EndsAt, r.t0.Unix())
	}
	if got := r.bank(r.seller); got != 998 {
		t.Errorf("seller's bank %d, want 998 (the fee is 1%% of 250)", got)
	}
	if rows := r.rowsOf(r.seller, 1); len(rows) != 0 {
		t.Errorf("the bank still holds %d rows", len(rows))
	}
	if got := r.total(); got != before-2 {
		t.Errorf("total %d, want %d less the fee", got, before)
	}
	// planted control: the same row again finds nothing, and nothing moves
	if _, err := r.list(r.seller, id, 250, 900, 720); err == nil {
		t.Fatal("the same row was listed twice")
	}
	if got := r.bank(r.seller); got != 998 {
		t.Errorf("a refused listing took a fee: bank %d", got)
	}
	if lots, _ := r.db.AuctionLotsOf(r.seller, r.t0.Add(-time.Hour), 40); len(lots) != 1 {
		t.Errorf("%d lots, want 1", len(lots))
	}
	r.audit()
}

// What CLIENT's sell form and Prima's guide refuse, the house refuses, and a
// refusal moves nothing.
func TestAuctionListRefusals(t *testing.T) {
	r := newAuctionRig(t)
	ok := r.item(r.seller, r.sword, 1, 1)
	cases := []struct {
		name    string
		item    int
		min, bo int
		minutes int
		want    string
	}{
		{"no trade", r.item(r.seller, r.notrade, 1, 1), 100, 200, 720, "No Trade"},
		{"lore", r.item(r.seller, r.lore, 1, 1), 100, 200, 720, "not auctionable"},
		{"not rentable", r.item(r.seller, r.unrentable, 1, 1), 100, 200, 720, "not auctionable"},
		{"in the inventory", r.item(r.seller, r.sword, 1, 0), 100, 200, 720, "not in your bank"},
		{"another's", r.item(r.a, r.sword, 1, 1), 100, 200, 720, "not in your bank"},
		{"under its value", ok, 49, 200, 720, "at least its value"},
		{"buyout under the bid", ok, 100, 99, 720, "buyout"},
		{"too short", ok, 100, 200, 719, "12 to 72 hours"},
		{"too long", ok, 100, 200, 4321, "12 to 72 hours"},
		{"too dear", ok, 100, AuctionMaxPrice + 1, 720, "more than"},
		{"nothing", ok, 0, 200, 720, "at least 1"},
	}
	bank := r.bank(r.seller)
	for _, c := range cases {
		lot, _, err := r.db.AuctionList(AuctionListing{Seller: r.seller, SellerName: "Seller", ItemID: c.item,
			PatternID: patternOf(t, r, c.item), Stack: 1, MinBid: c.min, Buyout: c.bo, Minutes: c.minutes, Now: r.t0})
		if err == nil {
			t.Errorf("%s: listed as lot %d", c.name, lot.ID)
			continue
		}
		if line, isRefusal := IsAuctionRefusal(err); !isRefusal || !strings.Contains(line, c.want) {
			t.Errorf("%s: %v, want a refusal with %q", c.name, err, c.want)
		}
	}
	// the fee: a bank that cannot pay it
	if _, err := r.db.Exec(`UPDATE characters SET bankTunar = 1 WHERE serverid = ?`, r.seller); err != nil {
		t.Fatal(err)
	}
	_, err := r.list(r.seller, ok, 500, 900, 720)
	wantRefusal(t, err, "listing fee is 5 tunar")
	if r.bank(r.seller) != 1 || len(r.rowsOf(r.seller, 1)) != 4 {
		t.Errorf("a refused fee moved something: bank %d, %d bank rows", r.bank(r.seller), len(r.rowsOf(r.seller, 1)))
	}
	if _, err := r.db.Exec(`UPDATE characters SET bankTunar = ? WHERE serverid = ?`, bank, r.seller); err != nil {
		t.Fatal(err)
	}
	// the session's list says another stack than the row: nothing moves
	g := r.item(r.seller, r.gem, 5, 1)
	if _, _, err := r.db.AuctionList(AuctionListing{Seller: r.seller, ItemID: g, PatternID: r.gem, Stack: 6,
		MinBid: 100, Buyout: 200, Minutes: 720, Now: r.t0}); err == nil || !errors.Is(err, errAuctionChanged) {
		t.Errorf("a stale stack listed: %v", err)
	}
	// a GM's form counts minutes
	if _, _, err := r.db.AuctionList(AuctionListing{Seller: r.seller, ItemID: ok, PatternID: r.sword, Stack: 1,
		MinBid: 100, Buyout: 200, Minutes: 2, GM: true, Now: r.t0}); err != nil {
		t.Errorf("a GM's two minutes: %v", err)
	}
	// a stack goes whole (S2 lets one be put up)
	lot, err := r.list(r.seller, g, 100, 200, 720)
	if err != nil || lot.Item.StackLeft != 5 {
		t.Errorf("a stack: lot %+v, %v", lot.Item, err)
	}
	// the open-lot limit
	for i := 0; i < AuctionMaxOpenLots; i++ {
		id := r.item(r.alt, r.sword, 1, 1)
		if _, _, err := r.db.AuctionList(AuctionListing{Seller: r.alt, ItemID: id, PatternID: r.sword, Stack: 1,
			MinBid: 60, Buyout: 70, Minutes: 720, Now: r.t0}); err != nil {
			if i < AuctionMaxOpenLots {
				t.Fatalf("lot %d: %v", i+1, err)
			}
		}
	}
	extra := r.item(r.alt, r.sword, 1, 1)
	_, _, err = r.db.AuctionList(AuctionListing{Seller: r.alt, ItemID: extra, PatternID: r.sword, Stack: 1,
		MinBid: 60, Buyout: 70, Minutes: 720, Now: r.t0})
	wantRefusal(t, err, "the most you may")
	r.audit()
}

func patternOf(t *testing.T, r *auctionRig, itemID int) int {
	var p int
	if err := r.db.QueryRow(`SELECT patternid FROM charinventory WHERE itemid = ?`, itemID).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Proxy bidding: the lead, a bid under it, a bid over it (the old leader's
// escrow back at once), a raise, the end: the winner pays one over the next
// proxy, the rest of its escrow back, the seller paid, the item in the
// winner's bank. The sums hold at every step.
func TestAuctionProxyBidding(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	at := r.t0.Add(time.Minute)
	step := func(who uint32, name string, amount int, kind AuctionBidKind, price, leader int) AuctionBidResult {
		t.Helper()
		res, err := r.bid(who, name, lot.ID, amount, at)
		if err != nil {
			t.Fatalf("%s bids %d: %v", name, amount, err)
		}
		if res.Kind != kind || res.Lot.Price != price || res.Lot.High != uint32(leader) {
			t.Fatalf("%s bids %d: kind %d price %d leader %d, want %d %d %d",
				name, amount, res.Kind, res.Lot.Price, res.Lot.High, kind, price, leader)
		}
		if got := r.total(); got != total {
			t.Fatalf("%s bids %d: total %d, want %d", name, amount, got, total)
		}
		r.audit()
		return res
	}
	res := step(r.a, "Bidder", 300, BidFirst, 100, int(r.a))
	if r.bank(r.a) != 700 || res.Bid.Escrow != 300 || res.Bid.Max != 300 || res.HadBid {
		t.Errorf("first bid: bank %d, record %+v", r.bank(r.a), res.Bid)
	}
	res = step(r.b, "Rival", 200, BidOutbid, 201, int(r.a))
	if r.bank(r.b) != 1000 || res.Bid.Escrow != 0 || res.Bid.Max != 200 {
		t.Errorf("outbid at once: bank %d, record %+v", r.bank(r.b), res.Bid)
	}
	_, err = r.bid(r.b, "Rival", lot.ID, 201, at)
	wantRefusal(t, err, "least you may bid on Fine Sword is 202")
	res = step(r.b, "Rival", 400, BidTookLead, 301, int(r.b))
	if !res.HadBid || res.Refund == nil || res.Refund.Bidder != r.a || res.Refund.Amount != 300 || res.Refund.Bank != 1000 {
		t.Errorf("took the lead: had %v, refund %+v", res.HadBid, res.Refund)
	}
	if r.bank(r.a) != 1000 || r.bank(r.b) != 600 {
		t.Errorf("banks after the lead changed: %d, %d", r.bank(r.a), r.bank(r.b))
	}
	_, err = r.bid(r.a, "Bidder", lot.ID, 301, at)
	wantRefusal(t, err, "302")
	res = step(r.b, "Rival", 500, BidRaised, 301, int(r.b))
	if r.bank(r.b) != 500 || res.Bid.Escrow != 500 || res.Taken != 100 {
		t.Errorf("raise: bank %d, record %+v, taken %d", r.bank(r.b), res.Bid, res.Taken)
	}
	_, err = r.bid(r.b, "Rival", lot.ID, 450, at)
	wantRefusal(t, err, "already 500")
	// a bid must be in the bank, even one outbid at once
	if _, err := r.db.Exec(`UPDATE characters SET bankTunar = 350 WHERE serverid = ?`, r.a); err != nil {
		t.Fatal(err)
	}
	total -= 650
	_, err = r.bid(r.a, "Bidder", lot.ID, 400, at)
	wantRefusal(t, err, "holds 350")
	step(r.a, "Bidder", 350, BidOutbid, 351, int(r.b))

	// bids after the end are refused, and the sweep settles it
	end := time.Unix(lot.EndsAt, 0)
	_, err = r.bid(r.a, "Bidder", lot.ID, 450, end)
	wantRefusal(t, err, "closed auction")
	sw, err := r.db.AuctionSettleDue(end.Add(-time.Second))
	if err != nil || len(sw.Settled) != 0 {
		t.Fatalf("settled early: %+v %v", sw, err)
	}
	sw, err = r.db.AuctionSettleDue(end)
	if err != nil || len(sw.Settled) != 1 {
		t.Fatalf("sweep: %+v %v", sw, err)
	}
	st := sw.Settled[0]
	if st.Winner != r.b || st.Lot.State != AuctionSold || st.Lot.Price != 351 || st.Refund != 149 {
		t.Errorf("settlement %+v", st)
	}
	if r.bank(r.b) != 649 || r.bank(r.seller) != 999+351 {
		t.Errorf("banks after the end: winner %d (want 649), seller %d (want %d)", r.bank(r.b), r.bank(r.seller), 999+351)
	}
	if got := r.total(); got != total {
		t.Errorf("total after the end %d, want %d", got, total)
	}
	won := r.rowsOf(r.b, 1)
	if len(won) != 1 || won[0].Pattern.PatternID != r.sword || won[0].RemainHP != 77 || won[0].Charges != 3 || st.Delivery == nil {
		t.Errorf("the winner's bank %+v, delivery %+v", won, st.Delivery)
	}
	notes, _ := r.db.AuctionNotices(r.seller)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "sold to Rival for 351") {
		t.Errorf("seller's notices %+v", notes)
	}
	notes, _ = r.db.AuctionNotices(r.b)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "won the auction of Fine Sword for 351") || !strings.Contains(notes[0].Line, "in your bank") {
		t.Errorf("winner's notices %+v", notes)
	}
	notes, _ = r.db.AuctionNotices(r.a)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "outbid") {
		t.Errorf("the outbid one's notices %+v", notes)
	}
	r.audit()
	// planted control: the lot settles once; a sweep again moves nothing
	if sw, err := r.db.AuctionSettleDue(end.Add(time.Hour)); err != nil || len(sw.Settled) != 0 {
		t.Errorf("settled again: %+v %v", sw, err)
	}
	if _, ok, err := r.db.AuctionSettle(lot.ID, end.Add(time.Hour)); ok || err != nil {
		t.Errorf("AuctionSettle on a closed lot: %v %v", ok, err)
	}
	if r.bank(r.b) != 649 || r.bank(r.seller) != 1350 || len(r.rowsOf(r.b, 1)) != 1 {
		t.Errorf("a second settle moved something")
	}
}

// A bid at the buyout buys the lot at once: the leader's escrow back, the
// seller paid the buyout, the item in the buyer's bank. A bid over it buys at
// the buyout.
func TestAuctionBuyout(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	at := r.t0.Add(time.Hour)
	if _, err := r.bid(r.a, "Bidder", lot.ID, 300, at); err != nil {
		t.Fatal(err)
	}
	res, err := r.bid(r.c, "Buyer", lot.ID, 1500, at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != BidBoughtOut || res.Settled == nil || res.Lot.State != AuctionSold || res.Lot.Price != 1000 || res.Taken != 1000 {
		t.Fatalf("buyout %+v", res)
	}
	if res.Refund == nil || res.Refund.Bidder != r.a || r.bank(r.a) != 1000 {
		t.Errorf("the leader's escrow: %+v, bank %d", res.Refund, r.bank(r.a))
	}
	if r.bank(r.c) != 4000 || r.bank(r.seller) != 999+1000 || len(r.rowsOf(r.c, 1)) != 1 {
		t.Errorf("banks: buyer %d, seller %d; buyer's bank rows %d", r.bank(r.c), r.bank(r.seller), len(r.rowsOf(r.c, 1)))
	}
	if res.Lot.EndsAt > at.Unix() {
		t.Errorf("a bought lot still ends at %d", res.Lot.EndsAt)
	}
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	// it takes no more bids
	_, err = r.bid(r.b, "Rival", lot.ID, 1000, at)
	wantRefusal(t, err, "closed")
	r.audit()
}

// A seller cannot bid on its own lot, nor buy it, nor can another character
// of its account.
func TestAuctionOwnLotRefused(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []uint32{r.seller, r.alt} {
		for _, amount := range []int{100, 1000} {
			_, err := r.bid(who, "Self", lot.ID, amount, r.t0)
			wantRefusal(t, err, "your own auctions")
		}
	}
	if r.bank(r.alt) != 5000 || r.bank(r.seller) != 999 {
		t.Errorf("a refused bid moved tunar")
	}
	// planted control: another account's character may
	if _, err := r.bid(r.a, "Bidder", lot.ID, 100, r.t0); err != nil {
		t.Errorf("another account: %v", err)
	}
}

// Unsold, the item goes back to the seller's bank; a bank with no room keeps
// it waiting at the house, and it is filed once room is made. A stack goes
// onto the bank's stack of the same item when the whole fits, else waits.
func TestAuctionDeliveryWaitsForRoom(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	var filler []int
	for i := 0; i < AuctionBankSlots; i++ {
		filler = append(filler, r.item(r.seller, r.sword, 1, 1))
	}
	end := time.Unix(lot.EndsAt, 0)
	sw, err := r.db.AuctionSettleDue(end)
	if err != nil || len(sw.Settled) != 1 {
		t.Fatal(sw, err)
	}
	st := sw.Settled[0]
	if st.Lot.State != AuctionUnsold || st.Delivery != nil || st.Lot.Delivered || st.Lot.Holder != r.seller {
		t.Fatalf("unsold into a full bank: %+v", st)
	}
	if n, _ := r.db.AuctionWaiting(r.seller); n != 1 {
		t.Errorf("%d waiting, want 1", n)
	}
	notes, _ := r.db.AuctionNotices(r.seller)
	if len(notes) != 1 || !strings.Contains(notes[0].Line, "no bids") || !strings.Contains(notes[0].Line, "waits") {
		t.Errorf("notices %+v", notes)
	}
	if got, left, err := r.db.AuctionDeliverPending(r.seller, end); err != nil || len(got) != 0 || left != 1 {
		t.Errorf("delivered into a full bank: %v %d %v", got, left, err)
	}
	if err := NewInventoryStore(r.db).DeleteCharItem(r.seller, filler[0]); err != nil {
		t.Fatal(err)
	}
	got, left, err := r.db.AuctionDeliverPending(r.seller, end)
	if err != nil || len(got) != 1 || left != 0 || got[0].Item.Pattern.PatternID != r.sword || got[0].Merged {
		t.Fatalf("delivered %+v, %d left, %v", got, left, err)
	}
	if n := len(r.rowsOf(r.seller, 1)); n != AuctionBankSlots {
		t.Errorf("%d bank rows, want %d", n, AuctionBankSlots)
	}
	// planted control: once delivered, again files nothing
	if got, left, err := r.db.AuctionDeliverPending(r.seller, end); err != nil || len(got) != 0 || left != 0 {
		t.Errorf("delivered twice: %v %d %v", got, left, err)
	}

	// stacks: onto the bank's stack of the same item when it fits
	g := r.item(r.a, r.gem, 8, 1)
	glot, _, err := r.db.AuctionList(AuctionListing{Seller: r.a, SellerName: "Bidder", ItemID: g, PatternID: r.gem, Stack: 8,
		MinBid: 10, Buyout: 100, Minutes: 720, Now: r.t0})
	if err != nil {
		t.Fatal(err)
	}
	have := r.item(r.b, r.gem, 12, 1)
	res, err := r.bid(r.b, "Rival", glot.ID, 100, r.t0)
	if err != nil || res.Settled == nil || res.Settled.Delivery == nil || !res.Settled.Delivery.Merged {
		t.Fatalf("gems bought: %+v %v", res, err)
	}
	rows := r.rowsOf(r.b, 1)
	if len(rows) != 1 || rows[0].ItemID != have || rows[0].StackLeft != 20 {
		t.Errorf("the stack after: %+v", rows)
	}
	// a stack that would overflow waits
	g2 := r.item(r.a, r.gem, 1, 1)
	glot2, _, err := r.db.AuctionList(AuctionListing{Seller: r.a, SellerName: "Bidder", ItemID: g2, PatternID: r.gem, Stack: 1,
		MinBid: 10, Buyout: 100, Minutes: 720, Now: r.t0})
	if err != nil {
		t.Fatal(err)
	}
	res, err = r.bid(r.b, "Rival", glot2.ID, 100, r.t0)
	if err != nil || res.Settled == nil || res.Settled.Delivery != nil {
		t.Fatalf("a full stack took more: %+v %v", res, err)
	}
	if rows := r.rowsOf(r.b, 1); len(rows) != 1 || rows[0].StackLeft != 20 {
		t.Errorf("the full stack changed: %+v", rows)
	}
	r.audit()
}

// What ended while the server was down settles when it is back.
func TestAuctionRestartSettlesWhatEnded(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/restart.db")
	if err != nil {
		t.Fatal(err)
	}
	r := &auctionRig{t: t, db: db, t0: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	store := NewSQLiteCharacterStore(db)
	for i, name := range []string{"Seller", "Bidder"} {
		rec := &CharacterRecord{CharName: name, AccountID: i + 1, BankTunar: 1000}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			r.seller = rec.ServerID
		} else {
			r.a = rec.ServerID
		}
	}
	r.sword = r.pattern("Fine Sword", 50, 1, 0, 0, 1)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", lot.ID, 250, r.t0); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// two days later, a new process
	db, err = NewSQLite(dir + "/restart.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r.db = db
	later := r.t0.Add(48 * time.Hour)
	sw, err := db.AuctionSettleDue(later)
	if err != nil || len(sw.Settled) != 1 || sw.Settled[0].Winner != r.a {
		t.Fatalf("after the restart: %+v %v", sw, err)
	}
	if r.bank(r.a) != 900 || r.bank(r.seller) != 999+100 || len(r.rowsOf(r.a, 1)) != 1 {
		t.Errorf("banks %d %d, the winner's rows %d", r.bank(r.a), r.bank(r.seller), len(r.rowsOf(r.a, 1)))
	}
	r.audit()
}

// Many settles of one lot at once: one settles it, the seller is paid once
// and the winner gets one item.
func TestAuctionConcurrentSettleOnce(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", lot.ID, 400, r.t0); err != nil {
		t.Fatal(err)
	}
	end := time.Unix(lot.EndsAt, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	settled := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var ok bool
			var err error
			if i%2 == 0 {
				_, ok, err = r.db.AuctionSettle(lot.ID, end)
			} else {
				var sw AuctionSweep
				sw, err = r.db.AuctionSettleDue(end)
				ok = len(sw.Settled) > 0
			}
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				settled++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if settled != 1 {
		t.Errorf("settled %d times", settled)
	}
	if r.bank(r.seller) != 999+100 || r.bank(r.a) != 900 || len(r.rowsOf(r.a, 1)) != 1 {
		t.Errorf("after: seller %d, winner %d, winner's rows %d", r.bank(r.seller), r.bank(r.a), len(r.rowsOf(r.a, 1)))
	}
	r.audit()
}

// Two buyers at the buyout at once: one buys it, the other is refused and
// keeps its tunar; one row listed by two requests at once is one lot.
func TestAuctionConcurrentDoubleBuyAndDoubleSell(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	total := r.total()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, who := range []uint32{r.a, r.c} {
		wg.Add(1)
		go func(i int, who uint32) {
			defer wg.Done()
			_, errs[i] = r.bid(who, "Racer", lot.ID, 1000, r.t0)
		}(i, who)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		if err == nil {
			won++
		} else if _, ok := IsAuctionRefusal(err); !ok {
			t.Errorf("not a refusal: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d buyers won (%v)", won, errs)
	}
	items := len(r.rowsOf(r.a, 1)) + len(r.rowsOf(r.c, 1))
	if items != 1 || r.bank(r.seller) != 999+1000 || r.bank(r.a)+r.bank(r.c) != 6000-1000 || r.total() != total {
		t.Errorf("items %d, seller %d, buyers %d+%d, total %d want %d", items, r.bank(r.seller), r.bank(r.a), r.bank(r.c), r.total(), total)
	}

	// one row, two listings at once
	id := r.item(r.seller, r.sword, 1, 1)
	errs = make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = r.db.AuctionList(AuctionListing{Seller: r.seller, SellerName: "Seller", ItemID: id,
				PatternID: r.sword, Stack: 1, MinBid: 100, Buyout: 200, Minutes: 720, Now: r.t0})
		}(i)
	}
	wg.Wait()
	listed := 0
	for _, err := range errs {
		if err == nil {
			listed++
		}
	}
	var lots int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM auction_lots WHERE item_from = ?`, id).Scan(&lots); err != nil {
		t.Fatal(err)
	}
	if listed != 1 || lots != 1 {
		t.Errorf("one row listed %d times (%d lots)", listed, lots)
	}
	if r.bank(r.seller) != 999+1000-1 {
		t.Errorf("seller's bank %d: the fee was taken %d times", r.bank(r.seller), (999 + 1000 - r.bank(r.seller)))
	}
	r.audit()
}

// Many bidders at once on one lot: whatever order they land in, the sums hold
// and the house holds exactly its leader's proxy.
func TestAuctionConcurrentBids(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 100000, 720)
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
			r.bid(who, "Racer", lot.ID, 150+i*37%400, r.t0)
		}(i)
	}
	wg.Wait()
	if r.total() != total {
		t.Errorf("total %d, want %d", r.total(), total)
	}
	r.audit()
	l, _, _ := auctionLotTx(r.db, lot.ID)
	if l.High == 0 || l.Price > l.HighMax || l.Price < l.MinBid {
		t.Errorf("after the race: %+v", l)
	}
}

// A seller's character deleted with a lot open: the lot is withdrawn and its
// leader's escrow goes back. A leader deleted: the lead is dropped.
func TestAuctionDeletedCharacters(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", lot.ID, 300, r.t0); err != nil {
		t.Fatal(err)
	}
	other, err := r.list(r.alt, r.item(r.alt, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.b, "Rival", other.ID, 300, r.t0); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteCharacterStore(r.db)
	if err := store.Delete(r.seller); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(r.b); err != nil {
		t.Fatal(err)
	}
	sw, err := r.db.AuctionSettleDue(r.t0.Add(time.Minute))
	if err != nil || len(sw.Cancelled) != 1 || len(sw.Refunds) != 1 || sw.Refunds[0].Bidder != r.a {
		t.Fatalf("sweep: %+v %v", sw, err)
	}
	if r.bank(r.a) != 1000 {
		t.Errorf("the leader of a withdrawn lot has %d", r.bank(r.a))
	}
	l, _, _ := auctionLotTx(r.db, other.ID)
	if l.State != AuctionOpen || l.High != 0 || l.MinNext() != 100 {
		t.Errorf("the deleted leader's lot: %+v", l)
	}
	r.audit()
	// it settles unsold, back to its seller
	st, ok, err := r.db.AuctionSettle(other.ID, time.Unix(other.EndsAt, 0))
	if err != nil || !ok || st.Lot.State != AuctionUnsold || st.Delivery == nil || st.Delivery.Holder != r.alt {
		t.Errorf("after: %+v %v %v", st, ok, err)
	}
}

// A write made from a stale read (what a second process, or a tool beside
// the server, would make) is refused by its own condition: a settlement of a
// lot already closed, a delivery of an item already filed. Nothing moves.
func TestAuctionStaleWritesRefused(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", lot.ID, 400, r.t0); err != nil {
		t.Fatal(err)
	}
	stale, _, _ := auctionLotTx(r.db, lot.ID) // open, as a second reader saw it
	end := time.Unix(lot.EndsAt, 0)
	if _, ok, err := r.db.AuctionSettle(lot.ID, end); !ok || err != nil {
		t.Fatalf("settle: %v %v", ok, err)
	}
	seller, winner, rows := r.bank(r.seller), r.bank(r.a), len(r.rowsOf(r.a, 1))
	err = r.db.auctionTx(func(tx *sql.Tx) error {
		_, err := settleTx(tx, stale, end)
		return err
	})
	if !errors.Is(err, errAuctionChanged) {
		t.Errorf("a stale settle: %v", err)
	}
	closed, _, _ := auctionLotTx(r.db, lot.ID)
	closed.Delivered = false // as a reader before the delivery saw it
	err = r.db.auctionTx(func(tx *sql.Tx) error {
		_, err := deliverTx(tx, &closed, end)
		return err
	})
	if !errors.Is(err, errAuctionChanged) {
		t.Errorf("a stale delivery: %v", err)
	}
	if r.bank(r.seller) != seller || r.bank(r.a) != winner || len(r.rowsOf(r.a, 1)) != rows {
		t.Errorf("a stale write moved something: seller %d->%d, winner %d->%d, rows %d->%d",
			seller, r.bank(r.seller), winner, r.bank(r.a), rows, len(r.rowsOf(r.a, 1)))
	}
	// unsold: nothing is held, so the state's own condition is the guard
	unsold, err := r.list(r.seller, r.item(r.seller, r.sword, 1, 1), 100, 1000, 720)
	if err != nil {
		t.Fatal(err)
	}
	stale, _, _ = auctionLotTx(r.db, unsold.ID)
	if _, ok, err := r.db.AuctionSettle(unsold.ID, end); !ok || err != nil {
		t.Fatalf("settle: %v %v", ok, err)
	}
	notes, _ := r.db.AuctionNotices(r.seller)
	err = r.db.auctionTx(func(tx *sql.Tx) error {
		_, err := settleTx(tx, stale, end)
		return err
	})
	if !errors.Is(err, errAuctionChanged) {
		t.Errorf("a stale settle of an unsold lot: %v", err)
	}
	if again, _ := r.db.AuctionNotices(r.seller); len(again) != len(notes) {
		t.Errorf("a stale settle told the seller again: %d notices, were %d", len(again), len(notes))
	}
	r.audit()
}
