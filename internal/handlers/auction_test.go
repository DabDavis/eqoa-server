package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
)

// A results page as RecvResults reads it: the sub-op, the page, the
// minutes, the lots, then three arrays as long as the lots; a lot with no
// bid carries -1 as its high bid.
func TestAuctionResultsPage(t *testing.T) {
	h := &auctionHouse{next: 1}
	lot := &auctionLot{id: 7, seller: "Seller", qty: 1, price: 100, buyout: 300, ends: 720,
		entry: database.CharInventoryEntry{Pattern: database.ItemPatternRecord{ItemName: "Sword"}}}
	p := h.resultsPayload(3, []*auctionLot{lot})
	r := buffer.NewReader(p[2:])
	if sub, _ := r.ReadByte(); sub != 129 {
		t.Fatalf("sub-op %d", sub)
	}
	page, _ := r.Read7BitEncodedInt64()
	r.Read7BitEncodedInt64() // the minutes
	n, _ := r.Read7BitEncodedInt64()
	if page != 3 || n != 1 {
		t.Fatalf("page %d, %d lots", page, n)
	}
	var words []int64
	for i := 0; i < 7; i++ {
		v, _ := r.Read7BitEncodedInt64()
		words = append(words, v)
	}
	if words[0] != 7 || words[4] != 100 || words[5] != 300 || words[6] != 720 {
		t.Errorf("the lot's words %v", words)
	}
	// the seller
	if n, _ := r.ReadUint32(); n != 6 {
		t.Errorf("seller length %d", n)
	} else if b, _ := r.ReadBytes(6); string(b) != "Seller" {
		t.Errorf("seller %q", b)
	}
	// negative control: one bid makes the high bid the amount, not -1
	lot.bids, lot.high = 1, 150
	q := h.resultsPayload(3, []*auctionLot{lot})
	if len(q) == len(p) && string(q) == string(p) {
		t.Error("a bid changed nothing")
	}
}
