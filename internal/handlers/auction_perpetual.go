package handlers

import (
	"fmt"
	"log"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The house's standing lots on the wire (database/auction_perpetual.go): a
// lot like any other in a search, its seller the name its row gives
// ("ilovetesters"), one item at its price, 72 hours left whenever it is
// looked at. "Acquire now" (a bid at the price, for a count on the live form)
// buys: the tunar out of the bank (0x1253), the items into it (0x1251), the
// lot sent again as it was (137) and a line. Shown and sold only with
// world.perpetual_auctions on; off, they are left out of every search and
// refused as closed.

var perpetualAuctionsOn atomic.Bool

// SetPerpetualAuctions turns the house's standing lots on or off
// (world.perpetual_auctions).
func SetPerpetualAuctions(on bool) { perpetualAuctionsOn.Store(on) }

// PerpetualAuctionsOn reports world.perpetual_auctions.
func PerpetualAuctionsOn() bool { return perpetualAuctionsOn.Load() }

// boughtLocked tells a buyer what its buy from a standing lot moved. With the
// house's lock and bankMu held.
func (a *AuctionHouse) boughtLocked(sess *session.Session, player *entity.Player, res *database.AuctionBidResult, v lotView) {
	b, l := res.Bought, &res.Lot
	player.BankTunar = res.BidderBank
	queueReliable(sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(res.BidderBank))
	a.fileLocked(sess, b.Delivery)
	queueReliable(sess, protocol.OpcodeAuctionStuff, updateLotPayload(l, a.pageMinutes(sess), v))
	name := l.Item.Pattern.ItemName
	if b.Qty == 1 {
		auctionLine(sess, fmt.Sprintf("You bought %s from %s for %d tunar. It is in your bank.", name, l.SellerName, b.Paid))
	} else {
		auctionLine(sess, fmt.Sprintf("You bought %d %s from %s at %d tunar each, %d tunar in all. They are in your bank.",
			b.Qty, name, l.SellerName, l.Buyout, b.Paid))
	}
	log.Printf("Auction: %s bought %d %q from standing lot %d for %d (bank %d)",
		player.Name, b.Qty, name, l.ID, b.Paid, res.BidderBank)
}
