package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Lore items at the house over the wire (database/auctionlore.go): a PS2's
// sale (the disc's form) refused as CLIENT's CanSell refuses it, vanilla's
// (the live form, 0x7E0B) taken; a bid by a character that has one refused
// with the line; a delivery held while the winner has one, said at the bank.
func TestAuctionLoreOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Bidder")
	s, a := f.chars["Seller"], f.chars["Bidder"]
	charm := f.pattern("Unique Charm", 10, 1)
	if _, err := f.db.Exec(`UPDATE itempattern SET lore = 1 WHERE patternid = ?`, charm); err != nil {
		t.Fatal(err)
	}
	f.give(s, charm, 1)
	// the disc's form: refused, nothing moved
	f.sell(s, 0, 100, 900, 720)
	msgs := f.drain(s)
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "not auctionable") || !sentClears(msgs) {
		t.Fatalf("a PS2's Lore sale: lines %q", ls)
	}
	if len(f.bankRows(s)) != 1 {
		t.Fatal("a refused Lore sale took the item")
	}
	// vanilla's live form: taken
	setForm := func(sess *session.Session) {
		f.house.formMu.Lock()
		f.house.form[sess] = true
		f.house.formMu.Unlock()
	}
	setForm(s.sess)
	setForm(a.sess)
	f.sell(s, 0, 100, 900, 720)
	msgs = f.drain(s)
	if _, ok := findSent(msgs, protocol.OpcodeAuctionStuff, 130); !ok || len(f.bankRows(s)) != 0 {
		t.Fatalf("vanilla's Lore sale not listed: lines %q", sentLines(msgs))
	}
	// a bidder that has one in its pack: refused with the line
	held, err := f.inv.AddCharItem(a.sid, charm, 1, 90, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.load(a)
	f.bid(a, 1, 900)
	msgs = f.drain(a)
	if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "You already have one of these") || !sentClears(msgs) {
		t.Fatalf("a bid by a holder: lines %q", ls)
	}
	if f.dbBank(a) != 1000 {
		t.Fatalf("a refused bid took %d", 1000-f.dbBank(a))
	}
	// without it, the buyout; one comes by another way before it is filed:
	// the item waits, and the bank says why
	if err := f.inv.DeleteCharItem(a.sid, held); err != nil {
		t.Fatal(err)
	}
	f.load(a)
	f.bid(a, 1, 100)
	f.drain(a)
	if _, err := f.inv.AddCharItem(a.sid, charm, 1, 90, 0, 0); err != nil {
		t.Fatal(err)
	}
	f.load(a)
	st, ok, err := f.db.AuctionSettle(1, f.now)
	if err != nil || !ok || st.Delivery != nil {
		t.Fatalf("settled past Lore: %+v %v %v", st, ok, err)
	}
	f.house.mu.Lock()
	bankMu.Lock()
	f.house.deliverLocked(a.sess, a.player)
	bankMu.Unlock()
	f.house.mu.Unlock()
	msgs = f.drain(a)
	if _, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0); ok {
		t.Fatal("a Lore item filed to one that has one")
	}
	ls := sentLines(msgs)
	if len(ls) != 1 || !strings.Contains(ls[0], "holds Unique Charm for you until you no longer have one") {
		t.Fatalf("waiting lines %q", ls)
	}
	if n, _ := f.db.AuctionWaiting(a.sid); n != 1 {
		t.Fatalf("%d waiting", n)
	}
}
