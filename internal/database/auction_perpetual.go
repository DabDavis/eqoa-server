package database

import (
	"database/sql"
	"time"
)

// The house's standing lots (the owner, 2026-11-14: "lets put all gems,
// alloys etc. for crafting on the auction house perpetually on sale ...
// seller name ilovetesters"; tools/ah_materials_20261114.py puts them up).
// A standing lot is an auction_lots row with perpetual = 1: seller 0 (no
// character: the house itself, under the name its row gives), one item at a
// time, its starting bid and buyout the price of one, open, with no bids. It
// never ends (AuctionSettleDue and AuctionSettle pass it by, and it is never
// an orphan) and never runs out: a bid at its price (or over it: the price is
// what is paid) buys that many, the tunar out of the buyer's bank (it goes
// nowhere: the house is no character) and the items filed into the bank as
// any lot's are (fileItemTx), and the lot is as it was. A bank without room
// for them is refused, nothing taken: nothing waits at the house for a buy.
// No bid record is kept, so a buy shows in no My Bids. The handlers show it
// only with world.perpetual_auctions on (AuctionBidRequest.Perpetual).

// AuctionPerpetualEnds is a standing lot's ends_at: 2100-01-01, past any
// sweep (a client is shown 72 hours left, handlers' writeLot).
const AuctionPerpetualEnds = 4102444800

// AuctionPerpetualMost is the most of an item one buy takes: a stack of it
// (at least one).
func AuctionPerpetualMost(p *ItemPatternRecord) int { return max(1, p.MaxStack) }

// AuctionPurchase is what a buy from a standing lot moved.
type AuctionPurchase struct {
	Qty      int
	Paid     int
	Delivery AuctionDelivery // the items as they reached the bank
}

// buyPerpetualTx is a bid on a standing lot, inside AuctionBid's transaction
// (the lot read and open): qty items at the lot's price each.
func buyPerpetualTx(tx *sql.Tx, req AuctionBidRequest, l AuctionLot, res *AuctionBidResult) error {
	name := l.Item.Pattern.ItemName
	qty, price := max(1, req.Qty), l.Buyout
	if most := AuctionPerpetualMost(&l.Item.Pattern); qty > most {
		return refuseAuction("You may buy at most %d %s at a time.", most, name)
	}
	if req.Amount < price {
		return refuseAuction("%s sells for %d tunar each: bid that to buy it now.", name, price)
	}
	cost := qty * price
	bank, there, err := bankTunarTx(tx, req.Bidder)
	if err != nil {
		return err
	}
	if !there {
		return refuseAuction("You cannot bid on a closed auction.")
	}
	if bank < cost {
		return refuseAuction("Your bank holds %d tunar; %d %s cost %d.", bank, qty, name, cost)
	}
	item := l.Item
	itemID, merged, ok, err := fileItemTx(tx, req.Bidder, &item, qty)
	if err != nil {
		return err
	}
	if !ok {
		return refuseAuction("Your bank has no room for %d %s.", qty, name)
	}
	if err := execOneTx(tx, `UPDATE characters SET bankTunar = bankTunar - ? WHERE serverid = ? AND bankTunar >= ?`,
		cost, req.Bidder, cost); err != nil {
		return err
	}
	d := AuctionDelivery{Lot: l.ID, Holder: req.Bidder, Item: l.Item, Merged: merged}
	d.Item.StackLeft, d.Item.ItemID, d.Item.EquipLoc, d.Item.Location = qty, itemID, -1, -1
	res.Kind, res.Lot, res.Taken, res.Units = BidPerpetual, l, cost, qty
	res.Bought = &AuctionPurchase{Qty: qty, Paid: cost, Delivery: d}
	res.BidderBank, _, err = bankTunarTx(tx, req.Bidder)
	return err
}

// AuctionPerpetualLots is the house's standing lots, by lot.
func (db *SQLiteDB) AuctionPerpetualLots() ([]AuctionLot, error) {
	return auctionLots(db, `WHERE l.perpetual = 1 AND l.state = ? ORDER BY l.lot`, AuctionOpen)
}

// auctionPerpetualNow is a standing lot's end as a client is shown it: the
// longest an auction runs from now.
func auctionPerpetualNow(now time.Time) int64 {
	return now.Unix()/60*60 + AuctionMaxMinutes*60
}

// ShownEnd is a lot's end in Unix seconds as a client is shown it at now: a
// standing lot's is always AuctionMaxMinutes away.
func (l *AuctionLot) ShownEnd(now time.Time) int64 {
	if l.Perpetual {
		return auctionPerpetualNow(now)
	}
	return l.EndsAt
}
