package database

// auctionlore.go — Lore items at the auction house (Heirloom petition 3245,
// the owner's call, 2026-11-26).
//
// CLIENT's VIWndAuction::CanSell refuses a Lore item (word 1727), so a PS2
// never puts one up; vanilla sells one from its newest build. The house keeps
// the Lore rule itself, so no character comes to hold two of one:
//
//   - a listing: a Lore item only from a client that sells one (the live form,
//     handlers' 0x7E0B: AuctionListing.LoreOK), and one at a time, so a Lore
//     lot is always a single item;
//   - a bid or a buy (a standing lot's too): refused while the bidder holds one
//     anywhere a Lore item counts (pack, worn, kept, its bank or its account's,
//     on a gram coming to it), has one waiting at the house, or leads another
//     lot of it, and for more than one;
//   - a delivery (a winner's, or an unsold item back to its seller): held at
//     the house while its holder holds one (it may have come by another way
//     since the bid), filed at a later bank visit or login once it does not.

import "fmt"

// loreHeldTx is whether a character holds one of an item anywhere a Lore item
// counts (LoreHeldOrComing's rows: its pack, worn, kept, its bank or its
// account's) or has one coming on a gram.
func loreHeldTx(q sqlRunner, holder uint32, patternID int) (bool, error) {
	cond, args := heldCond("", holder)
	var n int
	if err := q.QueryRow(`SELECT
		(SELECT COUNT(*) FROM charinventory WHERE patternid = ? AND (serverid = ? OR `+cond+`)) +
		(SELECT COUNT(*) FROM mail_attachments WHERE to_id = ? AND patternid = ?)`,
		append(append([]any{patternID, holder}, args...), holder, patternID)...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// loreAtHouseTx is whether a character has one of an item waiting at the
// house (a closed lot or a share not yet delivered to it), and whether it
// leads (holds escrow on) an open lot of it other than lot.
func loreAtHouseTx(q sqlRunner, holder uint32, patternID, lot int) (waiting, leading bool, err error) {
	var w, l int
	if err := q.QueryRow(`SELECT
		(SELECT COUNT(*) FROM auction_lots WHERE holder = ? AND delivered = 0 AND state != ? AND patternid = ?) +
		(SELECT COUNT(*) FROM auction_shares s JOIN auction_lots l ON l.lot = s.lot
			WHERE s.holder = ? AND s.delivered = 0 AND l.patternid = ?),
		(SELECT COUNT(*) FROM auction_bids b JOIN auction_lots l ON l.lot = b.lot
			WHERE b.bidder = ? AND b.escrow > 0 AND l.state = ? AND l.patternid = ? AND l.lot != ?)`,
		holder, AuctionOpen, patternID, holder, patternID, holder, AuctionOpen, patternID, lot).Scan(&w, &l); err != nil {
		return false, false, err
	}
	return w > 0, l > 0, nil
}

// loreBidTx refuses a bid on (or a buy from) a Lore lot that would leave its
// bidder with two: one held, one waiting at the house, another lot of it led,
// or more than one asked for.
func loreBidTx(q sqlRunner, bidder uint32, l *AuctionLot, qty int) error {
	p := &l.Item.Pattern
	if p.Lore == 0 {
		return nil
	}
	if qty > 1 {
		return refuseAuction("%s is Lore: you can hold only one.", p.ItemName)
	}
	held, err := loreHeldTx(q, bidder, p.PatternID)
	if err != nil {
		return err
	}
	waiting, leading, err := loreAtHouseTx(q, bidder, p.PatternID, l.ID)
	if err != nil {
		return err
	}
	switch {
	case held:
		return refuseAuction(loreHeldLine, p.ItemName)
	case waiting:
		return refuseAuction("You already have one of these waiting at the auction house: %s is Lore, and you can hold only one.", p.ItemName)
	case leading:
		return refuseAuction("You already lead the bidding on another %s: it is Lore, and you can hold only one.", p.ItemName)
	}
	return nil
}

// loreHeldLine is a refused bid's line when the bidder holds one already.
const loreHeldLine = "You already have one of these: %s is Lore, and you can hold only one."

// loreHoldsTx is whether a Lore lot's item must wait at the house for holder:
// it holds one already.
func loreHoldsTx(q sqlRunner, holder uint32, l *AuctionLot) (bool, error) {
	if l.Item.Pattern.Lore == 0 || holder == 0 {
		return false, nil
	}
	return loreHeldTx(q, holder, l.Item.Pattern.PatternID)
}

// loreWaitLine is where a Lore item held at the house is, said with its
// settlement.
func loreWaitLine(name string) string {
	return fmt.Sprintf("It waits at the auction house until you no longer have one: %s is Lore.", name)
}

// AuctionLoreWaiting is the names of the Lore items waiting at the house for a
// character because it holds one already (a delivery held, not one waiting
// for room).
func (db *SQLiteDB) AuctionLoreWaiting(holder uint32) ([]string, error) {
	waiting, err := auctionLots(db, `WHERE l.holder = ? AND l.delivered = 0 AND l.state != ? AND p.lore != 0 ORDER BY l.lot`,
		holder, AuctionOpen)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := range waiting {
		held, err := loreHoldsTx(db, holder, &waiting[i])
		if err != nil {
			return out, err
		}
		if held {
			out = append(out, waiting[i].Item.Pattern.ItemName)
		}
	}
	return out, nil
}
