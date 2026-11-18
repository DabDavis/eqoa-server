package database

import "testing"

// A Hardcore character never shares the account bank (accountbank.go's
// hcBankSplit): Seller and Selleralt share account 1; Selleralt made Hardcore,
// with world.hardcore on, its bank is its own rows and its own bankTunar, and
// the account's bank is Seller's alone. The controls: world.hardcore off, the
// account bank is both characters' as before.
func TestHardcoreBankIsItsOwn(t *testing.T) {
	r := newAuctionRig(t)
	SetAccountBank(true, 0)
	t.Cleanup(func() { SetAccountBank(false, 0) })
	on := true
	SetHardcoreBankCheck(func() bool { return on })
	t.Cleanup(func() { SetHardcoreBankCheck(nil) })
	if _, err := r.db.Exec(`INSERT INTO accountbank (accountid, tunar) VALUES (1, 6000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE characters SET hardcore = 1, bankTunar = 40 WHERE serverid = ?`, r.alt); err != nil {
		t.Fatal(err)
	}
	r.item(r.seller, r.sword, 1, LocBank)
	r.item(r.alt, r.gem, 3, LocBank)
	if n, _ := BankRowCount(r.db, r.seller); n != 1 {
		t.Errorf("Seller's (the account's) bank holds %d rows, want its own 1", n)
	}
	if n, _ := BankRowCount(r.db, r.alt); n != 1 {
		t.Errorf("the Hardcore Selleralt's bank holds %d rows, want its own 1", n)
	}
	if r.bank(r.seller) != 6000 || r.bank(r.alt) != 40 {
		t.Errorf("tunar: account %d, Hardcore %d; want 6000 and 40", r.bank(r.seller), r.bank(r.alt))
	}
	inv := NewInventoryStore(r.db)
	if after, ok, err := inv.AddBankTunarOf(r.alt, 10, false); err != nil || !ok || after != 50 {
		t.Errorf("a Hardcore deposit: %d %v %v, want 50", after, ok, err)
	}
	if r.bank(r.seller) != 6000 {
		t.Errorf("the Hardcore deposit reached the account: %d", r.bank(r.seller))
	}
	items, err := inv.GetBankItems(r.alt)
	if err != nil || len(items) != 1 || items[0].Pattern.PatternID != r.gem {
		t.Errorf("the Hardcore bank's items %+v %v", items, err)
	}

	on = false // the control
	if n, _ := BankRowCount(r.db, r.alt); n != 2 {
		t.Errorf("world.hardcore off: the account bank holds %d rows for Selleralt, want 2", n)
	}
	if r.bank(r.alt) != 6000 {
		t.Errorf("world.hardcore off: Selleralt's bank tunar %d, want the account's 6000", r.bank(r.alt))
	}
}
