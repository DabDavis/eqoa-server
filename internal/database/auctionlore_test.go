package database

import (
	"strings"
	"testing"
	"time"
)

// Lore items at the house (auctionlore.go): listed only from a client that
// sells one, one at a time; no bid or buy by a character that has one, has
// one waiting or leads another lot of it; a delivery held while its holder
// has one.

func (r *auctionRig) listLore(who uint32, itemID, minBid, buyout int, loreOK bool) (AuctionLot, error) {
	r.t.Helper()
	row, err := NewInventoryStore(r.db).GetCharItem(who, itemID)
	if err != nil {
		r.t.Fatal(err)
	}
	lot, _, err := r.db.AuctionList(AuctionListing{Seller: who, SellerName: "Seller", ItemID: itemID,
		PatternID: row.Pattern.PatternID, Stack: row.StackLeft, MinBid: minBid, Buyout: buyout, Minutes: 720,
		Now: r.t0, LoreOK: loreOK})
	return lot, err
}

func TestAuctionLoreListing(t *testing.T) {
	r := newAuctionRig(t)
	charm := r.item(r.seller, r.lore, 1, 1)
	// a PS2's sale (no live form): CanSell's word 1727, nothing moved
	_, err := r.listLore(r.seller, charm, 100, 200, false)
	wantRefusal(t, err, "not auctionable")
	if r.bank(r.seller) != 1000 || len(r.rowsOf(r.seller, 1)) != 1 {
		t.Fatalf("a refused Lore sale moved something")
	}
	// vanilla's: listed
	lot, err := r.listLore(r.seller, charm, 100, 200, true)
	if err != nil {
		t.Fatal(err)
	}
	if lot.Item.Pattern.PatternID != r.lore || lot.Item.StackLeft != 1 || len(r.rowsOf(r.seller, 1)) != 0 {
		t.Fatalf("lore lot %+v, %d bank rows left", lot, len(r.rowsOf(r.seller, 1)))
	}
	// a stack of a stackable Lore item: one at a time, by either form
	tokens := r.pattern("Unique Token", 10, 5, 0, 1, 1)
	stack := r.item(r.seller, tokens, 3, 1)
	_, err = r.listLore(r.seller, stack, 100, 200, true)
	wantRefusal(t, err, "one at a time")
	_, _, err = r.db.AuctionList(AuctionListing{Seller: r.seller, SellerName: "Seller", ItemID: stack,
		PatternID: tokens, Stack: 3, MinBid: 100, Buyout: 200, Minutes: 720, Now: r.t0, LoreOK: true,
		PerItem: true, Count: 2})
	wantRefusal(t, err, "one at a time")
	one, _, err := r.db.AuctionList(AuctionListing{Seller: r.seller, SellerName: "Seller", ItemID: stack,
		PatternID: tokens, Stack: 3, MinBid: 100, Buyout: 200, Minutes: 720, Now: r.t0, LoreOK: true,
		PerItem: true, Count: 1})
	if err != nil || one.Item.StackLeft != 1 || one.Multi() {
		t.Fatalf("one of a Lore stack: %+v %v", one, err)
	}
	r.audit()
}

func TestAuctionLoreBidRefusedWhenHeld(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listLore(r.seller, r.item(r.seller, r.lore, 1, 1), 100, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	at := r.t0.Add(time.Hour)
	store := NewInventoryStore(r.db)
	for _, where := range []string{"pack", "bank", "worn"} {
		var id int
		switch where {
		case "pack":
			id = r.item(r.a, r.lore, 1, 0)
		case "bank":
			id = r.item(r.a, r.lore, 1, 1)
		case "worn":
			id = r.item(r.a, r.lore, 1, 0)
			if _, err := r.db.Exec(`UPDATE charinventory SET equiploc = 5 WHERE itemid = ?`, id); err != nil {
				t.Fatal(err)
			}
		}
		bank := r.bank(r.a)
		_, err := r.bid(r.a, "Bidder", lot.ID, 100, at)
		wantRefusal(t, err, "You already have one of these")
		_, err = r.bid(r.a, "Bidder", lot.ID, 1000, at) // the buyout too
		wantRefusal(t, err, "You already have one of these")
		if r.bank(r.a) != bank || r.lot(lot.ID).High != 0 || r.lot(lot.ID).State != AuctionOpen {
			t.Fatalf("%s: a refused bid moved something", where)
		}
		if err := store.DeleteCharItem(r.a, id); err != nil {
			t.Fatal(err)
		}
	}
	// one on a gram coming to it counts too
	if _, err := r.db.Exec(`INSERT INTO mail_attachments (gramid, tunar, patternid, stack, remainhp, charges, itemname,
		from_id, from_name, to_id, to_name, sent) VALUES (1, 0, ?, 1, 100, 0, 'Unique Charm', ?, 'Rival', ?, 'Bidder', 0)`,
		r.lore, r.b, r.a); err != nil {
		t.Fatal(err)
	}
	_, err = r.bid(r.a, "Bidder", lot.ID, 100, at)
	wantRefusal(t, err, "You already have one of these")
	if _, err := r.db.Exec(`DELETE FROM mail_attachments`); err != nil {
		t.Fatal(err)
	}
	// holding none, it bids; a second lot of the same is refused while it leads
	if _, err := r.bid(r.a, "Bidder", lot.ID, 200, at); err != nil {
		t.Fatal(err)
	}
	other, err := r.listLore(r.b, r.item(r.b, r.lore, 1, 1), 100, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.bid(r.a, "Bidder", other.ID, 1000, at)
	wantRefusal(t, err, "lead the bidding on another")
	// raising its own bid on the lot it leads is not
	if res, err := r.bid(r.a, "Bidder", lot.ID, 300, at); err != nil || res.Kind != BidRaised {
		t.Fatalf("raise: %+v %v", res, err)
	}
	// outbid there, it may bid on the other
	if _, err := r.bid(r.c, "Buyer", lot.ID, 500, at); err != nil {
		t.Fatal(err)
	}
	if _, err := r.bid(r.a, "Bidder", other.ID, 150, at); err != nil {
		t.Fatal(err)
	}
	// a won lot waiting at the house (no room) counts as had
	if _, err := r.db.Exec(`UPDATE auction_lots SET state = ?, holder = ?, delivered = 0 WHERE lot = ?`,
		AuctionSold, r.c, other.ID); err != nil {
		t.Fatal(err)
	}
	_, err = r.bid(r.c, "Buyer", lot.ID, 900, at)
	wantRefusal(t, err, "waiting at the auction house")
}

func TestAuctionLorePerpetualBuy(t *testing.T) {
	r := newAuctionRig(t)
	lot := r.standing(r.lore, 100)
	_, err := r.buy(r.c, lot, 100, 2, true)
	wantRefusal(t, err, "you can hold only one")
	if _, err := r.buy(r.c, lot, 100, 1, true); err != nil {
		t.Fatal(err)
	}
	bank := r.bank(r.c)
	_, err = r.buy(r.c, lot, 100, 1, true)
	wantRefusal(t, err, "You already have one of these")
	if r.bank(r.c) != bank || len(r.rowsOf(r.c, 1)) != 1 {
		t.Fatalf("a refused buy moved something: bank %d, %d rows", r.bank(r.c), len(r.rowsOf(r.c, 1)))
	}
}

// One that comes to the winner by another way after its bid: the item waits
// at the house, said so, until the winner has none; the same for an unsold
// one back to a seller that has one again.
func TestAuctionLoreDeliveryHeld(t *testing.T) {
	r := newAuctionRig(t)
	lot, err := r.listLore(r.seller, r.item(r.seller, r.lore, 1, 1), 100, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	at := r.t0.Add(time.Hour)
	if _, err := r.bid(r.a, "Bidder", lot.ID, 200, at); err != nil {
		t.Fatal(err)
	}
	looted := r.item(r.a, r.lore, 1, 0) // a drop, after the bid
	end := time.Unix(lot.EndsAt, 0).Add(time.Minute)
	st, ok, err := r.db.AuctionSettle(lot.ID, end)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if st.Delivery != nil || st.Lot.Delivered || st.Lot.Holder != r.a || len(r.rowsOf(r.a, 1)) != 0 {
		t.Fatalf("delivered past Lore: %+v", st)
	}
	notes, err := r.db.AuctionNotices(r.a)
	if err != nil || len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Line, "until you no longer have one") {
		t.Fatalf("winner's line: %+v %v", notes, err)
	}
	if names, err := r.db.AuctionLoreWaiting(r.a); err != nil || len(names) != 1 || names[0] != "Unique Charm" {
		t.Fatalf("lore waiting: %v %v", names, err)
	}
	if got, left, err := r.db.AuctionDeliverPending(r.a, end); err != nil || len(got) != 0 || left != 1 {
		t.Fatalf("pending while held: %v %d %v", got, left, err)
	}
	if err := NewInventoryStore(r.db).DeleteCharItem(r.a, looted); err != nil {
		t.Fatal(err)
	}
	if names, err := r.db.AuctionLoreWaiting(r.a); err != nil || len(names) != 0 {
		t.Fatalf("lore waiting after: %v %v", names, err)
	}
	if got, left, err := r.db.AuctionDeliverPending(r.a, end); err != nil || len(got) != 1 || left != 0 {
		t.Fatalf("pending once none is held: %v %d %v", got, left, err)
	}
	if rows := r.rowsOf(r.a, 1); len(rows) != 1 || rows[0].Pattern.PatternID != r.lore {
		t.Fatalf("winner's bank %+v", rows)
	}

	// unsold, back to a seller that has one again
	back, err := r.listLore(r.b, r.item(r.b, r.lore, 1, 1), 100, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	r.item(r.b, r.lore, 1, 0)
	if st, ok, err := r.db.AuctionSettle(back.ID, time.Unix(back.EndsAt, 0).Add(time.Minute)); err != nil || !ok || st.Delivery != nil || st.Lot.Delivered {
		t.Fatalf("unsold Lore back past one held: %+v %v %v", st, ok, err)
	}
	notes, err = r.db.AuctionNotices(r.b)
	if err != nil || len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Line, "until you no longer have one") {
		t.Fatalf("seller's line: %+v %v", notes, err)
	}
	r.audit()
}
