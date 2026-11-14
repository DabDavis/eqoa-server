package database

import (
	"testing"
	"time"
)

// The account bank's records (accountbank.go) on the auction rig: Seller and
// Selleralt share account 1, their banks merged as the tool merges them
// (accountbank 6000, their own columns left). With the account bank on, a row
// one of them banked is the other's to list, the fee and a bid's escrow come
// out of the account's tunar, a lot won is filed into the account's bank by
// its 2000; off (the control), none of it.
func TestAccountBankAuction(t *testing.T) {
	r := newAuctionRig(t)
	SetAccountBank(true, 0)
	t.Cleanup(func() { SetAccountBank(false, 0) })
	if _, err := r.db.Exec(`INSERT INTO accountbank (accountid, tunar) VALUES (1, 6000)`); err != nil {
		t.Fatal(err)
	}
	if r.bank(r.seller) != 6000 || r.bank(r.alt) != 6000 {
		t.Fatalf("the account's banks %d / %d, want 6000 each", r.bank(r.seller), r.bank(r.alt))
	}
	sword := r.item(r.seller, r.sword, 1, LocBank)
	lot, err := r.list(r.alt, sword, 250, 900, 720)
	if err != nil {
		t.Fatalf("Selleralt lists Seller's banked sword: %v", err)
	}
	if r.bank(r.seller) != 5998 {
		t.Errorf("after the fee the account holds %d, want 5998", r.bank(r.seller))
	}
	// Bidder (account 2) wins it; the escrow from its own column (no merge for
	// account 2: its accountbank row is made with 0, so it bids with nothing)
	if _, err := r.bid(r.a, "Bidder", lot.ID, 300, r.t0); err == nil {
		t.Error("account 2's unmerged bank paid a bid")
	}
	if _, err := r.db.Exec(`UPDATE accountbank SET tunar = 1000 WHERE accountid = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO accountbank (accountid, tunar) VALUES (2, 1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", lot.ID, 300, r.t0); err != nil {
		t.Fatal(err)
	}
	sw, err := r.db.AuctionSettleDue(time.Unix(lot.EndsAt, 0))
	if err != nil || len(sw.Settled) != 1 || sw.Settled[0].Delivery == nil {
		t.Fatalf("settled %+v %v", sw, err)
	}
	if got := r.bank(r.alt); got != 5998+250 {
		t.Errorf("the seller's account holds %d, want %d", got, 5998+250)
	}
	// a gem lot filed into account 1's bank counts both characters' rows
	if BankSlotsOf(r.seller) != 2000 {
		t.Errorf("an account bank's holder files into %d", BankSlotsOf(r.seller))
	}
	n, err := BankRowCount(r.db, r.alt)
	if err != nil || n != 0 {
		t.Errorf("account 1's bank holds %d after the sale (%v)", n, err)
	}
	r.item(r.alt, r.gem, 3, LocBank)
	if n, _ := BankRowCount(r.db, r.seller); n != 1 {
		t.Errorf("Seller counts %d of the account's bank rows, want Selleralt's 1", n)
	}
}

// Off, the same rows are each character's own: Selleralt cannot list
// Seller's banked sword, and the fee comes from its own column.
func TestAccountBankAuctionOff(t *testing.T) {
	r := newAuctionRig(t)
	sword := r.item(r.seller, r.sword, 1, LocBank)
	if _, err := r.list(r.alt, sword, 250, 900, 720); err == nil {
		t.Fatal("Selleralt listed Seller's banked sword with the account bank off")
	}
	if r.bank(r.seller) != 1000 || r.bank(r.alt) != 5000 {
		t.Errorf("banks %d / %d", r.bank(r.seller), r.bank(r.alt))
	}
	if BankSlotsOf(r.seller) != AuctionBankSlots {
		t.Errorf("files into %d", BankSlotsOf(r.seller))
	}
}
