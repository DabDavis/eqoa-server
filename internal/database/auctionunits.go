package database

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// Stacks sold by the item: SOE's live client's auction (feature bit 0x08;
// ~/.cache/f3pass24/livediff/specs/auction-stacks.md). A lot put up by the
// item with two or more (AuctionLot.Multi) sells in units: a bid is a proxy
// for a quantity at a most per item, and the highest bids per item take units
// down the ranking until the stack runs out. How SOE's server settled a stack
// is not recorded; this is the inferred rule the spec names, made a proxy
// auction the way the single lot's is:
//
//   - the ranking: the higher price per item first, at one price the earlier
//     bid (the lot's bid count when placed);
//   - the price: one for every winner, the least that beats every bid left
//     without a unit it asked for (one over the highest of them), never under
//     the starting bid nor over the lowest winner's own; while the winners
//     ask for fewer than the stack a unit is free and the price is the
//     starting bid. With one item it is the single lot's rule exactly: the
//     leader pays one over the next highest proxy;
//   - the escrow: a bid that wins any unit holds its whole quantity times its
//     price in escrow ("In Escrow", qty x price); a bid pushed out of every
//     unit has it back in its bank at once, as an outbid leader does;
//   - the least next bid (the page's third column, C): the starting bid while
//     a unit is free, else one over the price;
//   - a new bid from a bidder replaces its record: never fewer items than it
//     asked before (the live form's own floor), at no less than C;
//   - the buyout: a bid for the whole stack at the buyout per item takes it
//     at once ("Buy all now"); a bid at the buyout for fewer is refused with
//     the live client's word 1858;
//   - at the close the winners pay the price per item for the units they
//     hold, the rest of their escrow back; the seller is paid; each winner's
//     units and the unsold rest (the seller's) are filed into the banks as
//     shares (auction_shares), each waiting at the house while its bank has
//     no room, as a whole lot's item does.

// AuctionWinner is a bid's take when a stack sold by the item closes.
type AuctionWinner struct {
	Bidder uint32
	Units  int
	Paid   int // Units x the price per item
	Refund int // the rest of its escrow, back in its bank
	Bank   int // its bank tunar after
	Gone   bool
	Bid    AuctionBid // its record after
}

// unitStand is how a stack sold by the item stands with its bids: each bid's
// units down the ranking, the price per item every winner pays, the items the
// winners ask for, the first of them and what they hold.
type unitStand struct {
	units  map[uint32]int
	price  int
	demand int
	top    AuctionBid // Bidder 0: no winner
	held   int        // each winner's qty x max
}

// rankBids orders bids as units go to them: the highest price per item
// first, at one price the earlier.
func rankBids(bids []AuctionBid) {
	sort.SliceStable(bids, func(i, j int) bool {
		if bids[i].Max != bids[j].Max {
			return bids[i].Max > bids[j].Max
		}
		return bids[i].Seq < bids[j].Seq
	})
}

// standUnits gives a stack of n to the competing bids (in: those holding
// escrow, and a new one; ranked here) and prices it. out are the bids already
// pushed out: their prices still stand against the winners'.
func standUnits(in, out []AuctionBid, n, minBid int) unitStand {
	rankBids(in)
	st := unitStand{units: map[uint32]int{}}
	left := n
	lost := -1 // the highest price per item a bid was left short at
	lowest := 0
	for _, b := range in {
		u := min(b.Qty, left)
		left -= u
		st.units[b.Bidder] = u
		if u < b.Qty {
			lost = max(lost, b.Max)
		}
		if u == 0 {
			continue
		}
		if st.top.Bidder == 0 {
			st.top = b
		}
		st.demand += b.Qty
		st.held += b.Qty * b.Max
		if lowest == 0 || b.Max < lowest {
			lowest = b.Max
		}
	}
	for _, b := range out {
		lost = max(lost, b.Max)
	}
	switch {
	case st.top.Bidder == 0:
		st.price = 0
	case st.demand < n:
		st.price = minBid
	default:
		st.price = min(lowest, max(minBid, lost+1))
	}
	return st
}

// splitBids is a lot's records but one bidder's: those holding escrow and
// those not, and that bidder's own.
func splitBids(all []AuctionBid, bidder uint32) (in, out []AuctionBid, own AuctionBid, had bool) {
	for _, b := range all {
		switch {
		case b.Bidder == bidder:
			own, had = b, true
		case b.Escrow > 0:
			in = append(in, b)
		default:
			out = append(out, b)
		}
	}
	return in, out, own, had
}

// refundUnitBidTx gives a bid its escrow back, exactly once, and says why.
func refundUnitBidTx(tx sqlRunner, l *AuctionLot, b AuctionBid, now time.Time, why string) (AuctionRefund, error) {
	rf := AuctionRefund{Bidder: b.Bidder, Amount: b.Escrow}
	if err := execOneTx(tx, `UPDATE auction_bids SET escrow = 0 WHERE lot = ? AND bidder = ? AND escrow = ?`,
		l.ID, b.Bidder, b.Escrow); err != nil {
		return rf, err
	}
	ok, err := creditBankTx(tx, b.Bidder, b.Escrow)
	if err != nil {
		return rf, err
	}
	rf.Gone = !ok
	if rf.Bank, _, err = bankTunarTx(tx, b.Bidder); err != nil {
		return rf, err
	}
	if rf.Bid, _, err = auctionBidTx(tx, l.ID, b.Bidder); err != nil {
		return rf, err
	}
	if !rf.Gone {
		err = noticeTx(tx, b.Bidder, now, why, l.Item.Pattern.ItemName, b.Escrow)
	}
	return rf, err
}

// bidUnitsTx is a bid on a stack sold by the item, inside AuctionBid's
// transaction (the lot read, running, not the bidder's own): qty items at
// most req.Amount each.
func bidUnitsTx(tx *sql.Tx, req AuctionBidRequest, l AuctionLot, qty int, res *AuctionBidResult) error {
	name, n := l.Item.Pattern.ItemName, l.Item.StackLeft
	all, err := auctionBids(tx, `WHERE b.lot = ?`, l.ID)
	if err != nil {
		return err
	}
	in, out, prior, had := splitBids(all, req.Bidder)
	sum := prior.Escrow
	for _, b := range in {
		sum += b.Escrow
	}
	if sum != l.HighMax {
		return fmt.Errorf("%w: lot %d says it holds %d, its bids hold %d", errAuctionChanged, l.ID, l.HighMax, sum)
	}
	price := req.Amount
	switch {
	case qty > n:
		return refuseAuction("There are only %d %s in that lot.", n, name)
	case had && qty < prior.Qty:
		return refuseAuction("Your bid on %s is for %d; a new one may not ask for fewer.", name, prior.Qty)
	}
	buyout := price >= l.Buyout
	if buyout {
		price = l.Buyout
		if qty != n {
			// the live client's own check, word 1858
			return refuseAuction("You can only bid the buy-it-now price if you bid on all %d items in the stack.", n)
		}
	} else if floor := l.MinNext(); price < floor {
		return refuseAuction("The least you may bid on %s is %d tunar each.", name, floor)
	} else if had && prior.Escrow > 0 && qty == prior.Qty && price == prior.Max {
		return refuseAuction("Your bid on %s is already %d at %d tunar each.", name, qty, price)
	}
	bank, _, err := bankTunarTx(tx, req.Bidder)
	if err != nil {
		return err
	}
	// a bid must be in the bank (Prima), even one outbid at once
	if hold := qty*price - prior.Escrow; bank < hold {
		return refuseAuction("Your bank holds %d tunar; that bid needs %d.", bank, hold)
	}
	mine := AuctionBid{Lot: l.ID, Bidder: req.Bidder, Name: req.Name, Max: price, Qty: qty, Seq: l.Bids + 1}
	var st unitStand
	if buyout {
		for _, b := range in {
			rf, err := refundUnitBidTx(tx, &l, b, req.Now, "%s was bought outright; your %d tunar is back in your bank.")
			if err != nil {
				return err
			}
			res.Refunds = append(res.Refunds, rf)
		}
		st = unitStand{units: map[uint32]int{req.Bidder: n}, price: l.Buyout, demand: n, top: mine, held: n * l.Buyout}
	} else {
		st = standUnits(append(in, mine), out, n, l.MinBid)
		for _, b := range in {
			if st.units[b.Bidder] > 0 {
				continue
			}
			rf, err := refundUnitBidTx(tx, &l, b, req.Now, "You have been outbid on %s; your %d tunar is back in your bank.")
			if err != nil {
				return err
			}
			res.Refunds = append(res.Refunds, rf)
		}
	}
	won := st.units[req.Bidder]
	escrow := 0
	if won > 0 {
		escrow = qty * price
	}
	switch delta := escrow - prior.Escrow; {
	case delta > 0:
		if err := execOneTx(tx, `UPDATE characters SET bankTunar = bankTunar - ? WHERE serverid = ? AND bankTunar >= ?`,
			delta, req.Bidder, delta); err != nil {
			return err
		}
		res.Taken = delta
	case delta < 0:
		if _, err := creditBankTx(tx, req.Bidder, -delta); err != nil {
			return err
		}
		res.Returned = -delta
	}
	if _, err := tx.Exec(`INSERT INTO auction_bids (lot, bidder, bidder_name, max_bid, escrow, placed_at, qty, seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(lot, bidder) DO UPDATE SET bidder_name = excluded.bidder_name, max_bid = excluded.max_bid,
			escrow = excluded.escrow, placed_at = excluded.placed_at, qty = excluded.qty, seq = excluded.seq`,
		l.ID, req.Bidder, req.Name, price, escrow, req.Now.Unix(), qty, mine.Seq); err != nil {
		return err
	}
	if err := execOneTx(tx, `UPDATE auction_lots SET price = ?, high_bidder = ?, high_name = ?, high_max = ?, demand = ?,
		bids = bids + 1 WHERE lot = ? AND state = ? AND bids = ? AND high_max = ?`,
		st.price, st.top.Bidder, st.top.Name, st.held, st.demand, l.ID, AuctionOpen, l.Bids, l.HighMax); err != nil {
		return err
	}
	after, _, err := auctionLotTx(tx, l.ID)
	if err != nil {
		return err
	}
	switch {
	case buyout:
		res.Kind = BidBoughtOut
	case won == 0:
		res.Kind = BidOutbid
	case had && prior.Escrow > 0:
		res.Kind = BidRaised
	case len(in) == 0:
		res.Kind = BidFirst
	default:
		res.Kind = BidTookLead
	}
	if buyout {
		s, err := settleTx(tx, after, req.Now)
		if err != nil {
			return err
		}
		res.Settled = &s
		after = s.Lot
	}
	res.Lot, res.HadBid, res.Units = after, had, won
	if res.Bid, _, err = auctionBidTx(tx, l.ID, req.Bidder); err != nil {
		return err
	}
	res.BidderBank, _, err = bankTunarTx(tx, req.Bidder)
	return err
}

// settleUnitsTx closes an open stack sold by the item (settleTx's for a
// Multi lot): the winners down the ranking pay the lot's price per item for
// their units, the rest of each escrow back; the seller is paid; the units
// and the unsold rest are shares, filed now where there is room. The state's
// change is conditional on the lot still being open, so it settles once.
func settleUnitsTx(tx sqlRunner, l AuctionLot, now time.Time) (AuctionSettlement, error) {
	st := AuctionSettlement{Multi: true}
	name, n := l.Item.Pattern.ItemName, l.Item.StackLeft
	bids, err := auctionBids(tx, `WHERE b.lot = ? AND b.escrow > 0`, l.ID)
	if err != nil {
		return st, err
	}
	var in []AuctionBid
	for _, b := range bids {
		if _, there, err := bankTunarTx(tx, b.Bidder); err != nil {
			return st, err
		} else if !there {
			// the bidder's character is gone, and its escrow with it
			if err := execOneTx(tx, `UPDATE auction_bids SET escrow = 0 WHERE lot = ? AND bidder = ? AND escrow = ?`,
				l.ID, b.Bidder, b.Escrow); err != nil {
				return st, err
			}
			continue
		}
		in = append(in, b)
	}
	rankBids(in)
	left, total := n, 0
	for _, b := range in {
		u := min(b.Qty, left)
		left -= u
		paid := u * l.Price
		if paid > b.Escrow {
			return st, fmt.Errorf("%w: lot %d: %d at %d each is more than the %d held for character %d",
				errAuctionChanged, l.ID, u, l.Price, b.Escrow, b.Bidder)
		}
		if err := execOneTx(tx, `UPDATE auction_bids SET escrow = 0, won = ? WHERE lot = ? AND bidder = ? AND escrow = ?`,
			u, l.ID, b.Bidder, b.Escrow); err != nil {
			return st, err
		}
		if _, err := creditBankTx(tx, b.Bidder, b.Escrow-paid); err != nil {
			return st, err
		}
		if u == 0 {
			// a bid left with no unit holds nothing (it cannot: units only
			// came free since it was priced): its escrow back
			if err := noticeTx(tx, b.Bidder, now, "You have been outbid on %s; your %d tunar is back in your bank.", name, b.Escrow); err != nil {
				return st, err
			}
			continue
		}
		total += paid
		st.Winners = append(st.Winners, AuctionWinner{Bidder: b.Bidder, Units: u, Paid: paid, Refund: b.Escrow - paid})
	}
	st.Sold = n - left
	state, top, topName := AuctionUnsold, uint32(0), ""
	if len(st.Winners) > 0 {
		state, top = AuctionSold, st.Winners[0].Bidder
		topName = in[0].Name
	}
	ends := l.EndsAt
	if t := now.Unix() / 60 * 60; t < ends {
		ends = t // bought out: the time since close counts from now
	}
	// the lot carries nothing to a bank itself: its shares do
	if err := execOneTx(tx, `UPDATE auction_lots SET state = ?, closed_at = ?, holder = 0, delivered = 1, ends_at = ?,
		high_bidder = ?, high_name = ?, high_max = 0 WHERE lot = ? AND state = ?`,
		state, now.Unix(), ends, top, topName, l.ID, AuctionOpen); err != nil {
		return st, err
	}
	if total > 0 {
		ok, err := creditBankTx(tx, l.Seller, total)
		if err != nil {
			return st, err
		}
		st.SellerGone = !ok
	} else if _, there, err := bankTunarTx(tx, l.Seller); err != nil {
		return st, err
	} else {
		st.SellerGone = !there
	}
	var stray sql.NullInt64
	if err := tx.QueryRow(`SELECT SUM(escrow) FROM auction_bids WHERE lot = ?`, l.ID).Scan(&stray); err != nil {
		return st, err
	}
	if stray.Int64 != 0 {
		return st, fmt.Errorf("%w: lot %d still holds %d tunar of other bidders", errAuctionChanged, l.ID, stray.Int64)
	}
	waits := map[uint32]bool{}
	share := func(holder uint32, qty, price int) error {
		r, err := tx.Exec(`INSERT INTO auction_shares (lot, holder, qty, price) VALUES (?, ?, ?, ?)`, l.ID, holder, qty, price)
		if err != nil {
			return err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		d, err := deliverShareTx(tx, int(id), holder, qty, &l)
		if err != nil {
			return err
		}
		if d != nil {
			st.Deliveries = append(st.Deliveries, *d)
		} else {
			waits[holder] = true
		}
		return nil
	}
	for _, w := range st.Winners {
		if err := share(w.Bidder, w.Units, l.Price); err != nil {
			return st, err
		}
	}
	if left > 0 && !st.SellerGone {
		if err := share(l.Seller, left, 0); err != nil {
			return st, err
		}
	}
	where := func(holder uint32, one bool) string {
		switch {
		case waits[holder] && one:
			return "It waits at the auction house for room in your bank."
		case waits[holder]:
			return "They wait at the auction house for room in your bank."
		case one:
			return "It is in your bank."
		}
		return "They are in your bank."
	}
	for i := range st.Winners {
		w := &st.Winners[i]
		if err := noticeTx(tx, w.Bidder, now, "You won %d %s at %d tunar each (%d in all). %s",
			w.Units, name, l.Price, w.Paid, where(w.Bidder, w.Units == 1)); err != nil {
			return st, err
		}
		if w.Bank, _, err = bankTunarTx(tx, w.Bidder); err != nil {
			return st, err
		}
		if w.Bid, _, err = auctionBidTx(tx, l.ID, w.Bidder); err != nil {
			return st, err
		}
	}
	if !st.SellerGone {
		var line string
		switch {
		case st.Sold == 0:
			line = fmt.Sprintf("Your auction of %d %s ended with no bids. %s", n, name, where(l.Seller, n == 1))
		case left == 0:
			line = fmt.Sprintf("Your auction of %d %s sold all %d at %d tunar each, %d tunar in all; the tunar is in your bank.",
				n, name, n, l.Price, total)
		default:
			line = fmt.Sprintf("Your auction of %d %s sold %d at %d tunar each, %d tunar in all; the tunar is in your bank. The %d unsold: %s",
				n, name, st.Sold, l.Price, total, left, where(l.Seller, left == 1))
		}
		if err := noticeTx(tx, l.Seller, now, "%s", line); err != nil {
			return st, err
		}
		if st.SellerBank, _, err = bankTunarTx(tx, l.Seller); err != nil {
			return st, err
		}
	}
	st.Lot, _, err = auctionLotTx(tx, l.ID)
	return st, err
}

// deliverShareTx files a share's items into its holder's bank as CLIENT's
// AddItem would keep them (fileItemTx), marking it delivered only while it is
// not; a holder that is gone takes nothing (marked delivered). nil: it waits.
func deliverShareTx(tx sqlRunner, id int, holder uint32, qty int, l *AuctionLot) (*AuctionDelivery, error) {
	if _, there, err := bankTunarTx(tx, holder); err != nil {
		return nil, err
	} else if !there {
		return nil, execOneTx(tx, `UPDATE auction_shares SET delivered = 1 WHERE id = ? AND delivered = 0`, id)
	}
	itemID, merged, ok, err := fileItemTx(tx, holder, &l.Item, qty)
	if err != nil || !ok {
		return nil, err
	}
	if err := execOneTx(tx, `UPDATE auction_shares SET delivered = 1, item_to = ? WHERE id = ? AND delivered = 0`,
		itemID, id); err != nil {
		return nil, err
	}
	d := &AuctionDelivery{Lot: l.ID, Holder: holder, Item: l.Item, Merged: merged}
	d.Item.StackLeft, d.Item.ItemID, d.Item.EquipLoc, d.Item.Location = qty, itemID, -1, -1
	return d, nil
}

// restandUnitsTx prices an open stack sold by the item again after bids left
// it (their characters gone, their escrow with them): units come free, the
// winners' order and price are made again, and a bid left with no unit (it
// cannot be: units only came free) has its escrow back.
func restandUnitsTx(tx sqlRunner, l AuctionLot, now time.Time) ([]AuctionRefund, error) {
	all, err := auctionBids(tx, `WHERE b.lot = ?`, l.ID)
	if err != nil {
		return nil, err
	}
	in, out, _, _ := splitBids(all, 0)
	st := standUnits(in, out, l.Item.StackLeft, l.MinBid)
	var refunds []AuctionRefund
	for _, b := range in {
		if st.units[b.Bidder] > 0 {
			continue
		}
		rf, err := refundUnitBidTx(tx, &l, b, now, "You have been outbid on %s; your %d tunar is back in your bank.")
		if err != nil {
			return refunds, err
		}
		refunds = append(refunds, rf)
	}
	return refunds, execOneTx(tx, `UPDATE auction_lots SET price = ?, high_bidder = ?, high_name = ?, high_max = ?, demand = ?
		WHERE lot = ? AND state = ?`, st.price, st.top.Bidder, st.top.Name, st.held, st.demand, l.ID, AuctionOpen)
}
