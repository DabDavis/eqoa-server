package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The auction house's records (handlers/auction.go runs it on the wire).
// As EQOA's: an item is put up out of the seller's bank at a banker, the
// listing fee (one percent of the starting bid, at least one tunar) comes out
// of the bank and is not given back, a bid's tunar comes out of the bidder's
// bank and is held, and at the end the item goes to the winner's bank and the
// tunar to the seller's, or the item back to the seller's bank with no bid
// (Prima's Frontiers guide, "Auctions"; CLIENT's sell form and search window,
// decomp viwndauction.cpp and viwndauctionsearch.cpp). A bid is a PROXY bid,
// as the search window calls it ("Set Proxy Bid", "Submit Proxy Bid", "Acquire
// now for %d Tunar"): the most a bidder will pay. The house holds that much in
// escrow ("In Escrow" beside "Your Bid") while it leads, and the lot's current
// bid is the least that beats every other proxy (one tunar over the next
// highest, never above the leader's own); an outbid leader's escrow goes back
// to its bank at once. A bid at the buyout buys the lot outright.
//
// Every move of an item or of tunar is ONE transaction that checks its source
// still holds what it moves: a listing deletes the bank row only as the
// session's list described it (its owner, the bank, unworn, that item, that
// stack) and takes the fee only from a bank that holds it; a bid takes its
// escrow only from a bank that holds it, and changes the lot only as it was
// read (its leader, proxy, price and count); a settlement closes the lot only
// while it is open, so it runs once, and releases the winner's escrow only
// while it is there; a delivery files the item only once. What the house
// holds, it holds in these tables alone: the item between listing and
// delivery, the tunar between bid and settlement.
//
//	auction_lots     one row a lot: the item (pattern, stack, condition,
//	                 charges), the seller, the starting bid and the buyout,
//	                 the fee paid, when it was put up and ends (Unix seconds,
//	                 the end a whole minute), its state (open, sold, unsold,
//	                 cancelled: a seller whose character is gone), the leader
//	                 and its proxy (held in escrow), the current bid and the
//	                 bids made; once closed, when, whose bank the item goes to
//	                 (holder: the winner, else the seller) and whether it got
//	                 there (an item waits in the house while that bank has no
//	                 room for it)
//	auction_bids     one row a bidder a lot (CLIENT's CHARACTER_BIDS key):
//	                 its proxy ("Your Bid") and what the house holds for it
//	                 now ("In Escrow": the proxy while it leads, else 0)
//	auction_notices  a line a character is to be told (a sale, a win, an
//	                 outbid), written with the event; said at once to a
//	                 player in the world, else at its next login
const auctionSchema = `
	CREATE TABLE IF NOT EXISTS auction_lots (
		lot         INTEGER PRIMARY KEY AUTOINCREMENT,
		seller      INTEGER NOT NULL,
		seller_name TEXT NOT NULL,
		patternid   INTEGER NOT NULL,
		stack       INTEGER NOT NULL,
		remainhp    INTEGER NOT NULL,
		charges     INTEGER NOT NULL,
		min_bid     INTEGER NOT NULL,
		buyout      INTEGER NOT NULL,
		fee         INTEGER NOT NULL,
		listed_at   INTEGER NOT NULL,
		ends_at     INTEGER NOT NULL,
		state       TEXT NOT NULL DEFAULT 'open',
		high_bidder INTEGER NOT NULL DEFAULT 0,
		high_name   TEXT NOT NULL DEFAULT '',
		high_max    INTEGER NOT NULL DEFAULT 0,
		price       INTEGER NOT NULL DEFAULT 0,
		bids        INTEGER NOT NULL DEFAULT 0,
		closed_at   INTEGER NOT NULL DEFAULT 0,
		holder      INTEGER NOT NULL DEFAULT 0,
		delivered   INTEGER NOT NULL DEFAULT 0,
		item_from   INTEGER NOT NULL DEFAULT 0,
		item_to     INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_auction_lots_state ON auction_lots(state, ends_at);
	CREATE INDEX IF NOT EXISTS idx_auction_lots_seller ON auction_lots(seller);
	CREATE INDEX IF NOT EXISTS idx_auction_lots_holder ON auction_lots(holder, delivered);
	CREATE TABLE IF NOT EXISTS auction_bids (
		lot         INTEGER NOT NULL,
		bidder      INTEGER NOT NULL,
		bidder_name TEXT NOT NULL,
		max_bid     INTEGER NOT NULL,
		escrow      INTEGER NOT NULL DEFAULT 0,
		placed_at   INTEGER NOT NULL,
		PRIMARY KEY (lot, bidder)
	);
	CREATE INDEX IF NOT EXISTS idx_auction_bids_bidder ON auction_bids(bidder);
	CREATE TABLE IF NOT EXISTS auction_notices (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid INTEGER NOT NULL,
		line     TEXT NOT NULL,
		made_at  INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_auction_notices_char ON auction_notices(serverid);`

// ensureAuction makes the auction's tables (initSchema).
func (db *SQLiteDB) ensureAuction() error {
	if _, err := db.Exec(auctionSchema); err != nil {
		return fmt.Errorf("auction: %w", err)
	}
	return nil
}

// The house's bounds: CLIENT's sell form (VIWndAuction::DrawSell) unless said.
const (
	AuctionMaxPrice     = 999999999 // the starting bid and the buyout, at most
	AuctionMinMinutes   = 720       // 12 hours (word 1691, "(12-72 hrs)")
	AuctionMaxMinutes   = 4320      // 72 hours
	AuctionMinMinutesGM = 2         // a GM's form counts minutes (InMinutes: the GM channel)
	// AuctionBankSlots is the records a bank list holds (VIBankControl's
	// RecvAdded: AddItem(item, maxStack, 40)): an item is filed into a bank
	// only as that AddItem would keep it.
	AuctionBankSlots = 40
	// AuctionMaxOpenLots is how many lots a character may have open (ours:
	// nothing records SOE's).
	AuctionMaxOpenLots = 20
	// AuctionHistory is how long a closed lot stays in its seller's My
	// Auctions, its bidders' My Bids and a search for closed lots (ours: the
	// windows show a closed lot's "Sold For" and "Time Since Close", and
	// nothing records how long SOE kept one).
	AuctionHistory = 72 * time.Hour
)

// The states of a lot.
const (
	AuctionOpen      = "open"
	AuctionSold      = "sold"
	AuctionUnsold    = "unsold"
	AuctionCancelled = "cancelled" // its seller's character is gone
)

// AuctionFee is the listing fee: one percent of the starting bid, at least
// one tunar (DrawSell's "Listing fee", VIMAX(1, m_price / 100); Prima: "It
// will only cost you one percent of the item's minimum listing price").
func AuctionFee(minBid int) int {
	return max(1, minBid/100)
}

// AuctionLot is one row of auction_lots, the item with its pattern.
type AuctionLot struct {
	ID         int
	Seller     uint32
	SellerName string
	// Item is the pattern, and the stack, condition and charges as put up
	// (ItemID: the charinventory row it was).
	Item      CharInventoryEntry
	MinBid    int // the starting bid (m_p2F4, "Min. Price")
	Buyout    int // m_p2F8, "Max. Price": "Acquire now for %d Tunar"
	Fee       int
	ListedAt  int64
	EndsAt    int64 // Unix seconds, a whole minute
	State     string
	High      uint32 // the leading bidder, 0 none
	HighName  string
	HighMax   int // its proxy, held in escrow
	Price     int // the current bid (with a leader)
	Bids      int
	ClosedAt  int64
	Holder    uint32 // once closed: whose bank the item goes to
	Delivered bool
	ItemTo    int // the charinventory row it became
}

// Running reports a lot still taking bids: open and its time not up.
func (l *AuctionLot) Running(now time.Time) bool {
	return l.State == AuctionOpen && now.Unix() < l.EndsAt
}

// CurrentBid is the lot's current bid, -1 with none (the page's second
// column: DrawSummary's "no bids").
func (l *AuctionLot) CurrentBid() int {
	if l.High == 0 {
		return -1
	}
	return l.Price
}

// MinNext is the least a new bid may be (the page's third column, word 1710
// "Min. Bid"): the starting bid, or one over the current bid.
func (l *AuctionLot) MinNext() int {
	if l.High == 0 {
		return l.MinBid
	}
	return l.Price + 1
}

// AuctionBid is one row of auction_bids.
type AuctionBid struct {
	Lot      int
	Bidder   uint32
	Name     string
	Max      int // its proxy ("Your Bid")
	Escrow   int // held for it now ("In Escrow")
	PlacedAt int64
}

// AuctionNotice is a line a character is to be told.
type AuctionNotice struct {
	ID   int
	Line string
}

// AuctionRefusal is a request the house turns down, with what the player is
// told.
type AuctionRefusal struct{ Line string }

func (r *AuctionRefusal) Error() string { return r.Line }

func refuseAuction(format string, args ...any) error {
	return &AuctionRefusal{Line: fmt.Sprintf(format, args...)}
}

// errAuctionChanged is a conditional write that found its row changed under
// it: the transaction is rolled back and nothing moves.
var errAuctionChanged = errors.New("auction: a row changed under the transaction")

// IsAuctionRefusal reports a refusal and its line.
func IsAuctionRefusal(err error) (string, bool) {
	var r *AuctionRefusal
	if errors.As(err, &r) {
		return r.Line, true
	}
	return "", false
}

// sqlRunner is what both the database and a transaction run.
type sqlRunner interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// auctionTx runs fn in one transaction, rolled back on any error.
func (db *SQLiteDB) auctionTx(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// execOneTx runs a write that must change exactly one row.
func execOneTx(tx sqlRunner, query string, args ...any) error {
	res, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w (%d rows, want 1): %s", errAuctionChanged, n, strings.Join(strings.Fields(query), " "))
	}
	return nil
}

const auctionLotColumns = `l.lot, l.seller, l.seller_name, l.stack, l.remainhp, l.charges, l.item_from,
	l.min_bid, l.buyout, l.fee, l.listed_at, l.ends_at, l.state, l.high_bidder, l.high_name,
	l.high_max, l.price, l.bids, l.closed_at, l.holder, l.delivered, l.item_to, ` + itemPatternColumns

func scanAuctionLot(scan func(...any) error) (AuctionLot, error) {
	var l AuctionLot
	var delivered int
	err := scan(append([]any{
		&l.ID, &l.Seller, &l.SellerName, &l.Item.StackLeft, &l.Item.RemainHP, &l.Item.Charges, &l.Item.ItemID,
		&l.MinBid, &l.Buyout, &l.Fee, &l.ListedAt, &l.EndsAt, &l.State, &l.High, &l.HighName,
		&l.HighMax, &l.Price, &l.Bids, &l.ClosedAt, &l.Holder, &delivered, &l.ItemTo,
	}, patternDest(&l.Item.Pattern)...)...)
	l.Delivered = delivered != 0
	l.Item.EquipLoc, l.Item.Location = -1, -1
	return l, err
}

// auctionLots runs a query of whole lots (where: after FROM ... JOIN).
func auctionLots(q sqlRunner, where string, args ...any) ([]AuctionLot, error) {
	rows, err := q.Query(`SELECT `+auctionLotColumns+`
		FROM auction_lots l JOIN itempattern p ON p.patternid = l.patternid `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuctionLot
	for rows.Next() {
		l, err := scanAuctionLot(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func auctionLotTx(q sqlRunner, lot int) (AuctionLot, bool, error) {
	lots, err := auctionLots(q, `WHERE l.lot = ?`, lot)
	if err != nil || len(lots) == 0 {
		return AuctionLot{}, false, err
	}
	return lots[0], true, nil
}

func auctionBidTx(q sqlRunner, lot int, bidder uint32) (AuctionBid, bool, error) {
	b := AuctionBid{Lot: lot, Bidder: bidder}
	err := q.QueryRow(`SELECT bidder_name, max_bid, escrow, placed_at FROM auction_bids WHERE lot = ? AND bidder = ?`,
		lot, bidder).Scan(&b.Name, &b.Max, &b.Escrow, &b.PlacedAt)
	if err == sql.ErrNoRows {
		return b, false, nil
	}
	return b, err == nil, err
}

// bankTunarTx is a character's bank tunar, and whether the character is
// there at all.
func bankTunarTx(q sqlRunner, serverID uint32) (int, bool, error) {
	var bank int
	err := q.QueryRow(`SELECT bankTunar FROM characters WHERE serverid = ?`, serverID).Scan(&bank)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return bank, err == nil, err
}

// BankTunar is a character's bank tunar as kept, and whether it is there.
func (db *SQLiteDB) BankTunar(serverID uint32) (int, bool, error) {
	return bankTunarTx(db, serverID)
}

func noticeTx(tx sqlRunner, serverID uint32, now time.Time, format string, args ...any) error {
	if serverID == 0 {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO auction_notices (serverid, line, made_at) VALUES (?, ?, ?)`,
		serverID, fmt.Sprintf(format, args...), now.Unix())
	return err
}

// creditBankTx puts tunar in a character's bank; a character that is gone
// takes nothing (false).
func creditBankTx(tx sqlRunner, serverID uint32, n int) (bool, error) {
	if n == 0 {
		return true, nil
	}
	res, err := tx.Exec(`UPDATE characters SET bankTunar = bankTunar + ? WHERE serverid = ?`, n, serverID)
	if err != nil {
		return false, err
	}
	k, err := res.RowsAffected()
	return k == 1, err
}

// AuctionListing is a bank record put up.
type AuctionListing struct {
	Seller     uint32
	SellerName string
	// ItemID, PatternID and Stack are the bank record as the session's list
	// holds it: the row must still be exactly that.
	ItemID    int
	PatternID int
	Stack     int
	MinBid    int
	Buyout    int
	Minutes   int
	GM        bool // a GM's form counts minutes: 2 at least
	Now       time.Time
}

// AuctionList puts a bank record up for auction: the listing fee out of the
// seller's bank and the row out of it into the house, in one transaction. It
// returns the lot and the seller's bank tunar after.
func (db *SQLiteDB) AuctionList(req AuctionListing) (AuctionLot, int, error) {
	least := AuctionMinMinutes
	if req.GM {
		least = AuctionMinMinutesGM
	}
	switch {
	case req.MinBid < 1:
		return AuctionLot{}, 0, refuseAuction("The starting bid must be at least 1 tunar.")
	case req.MinBid > AuctionMaxPrice || req.Buyout > AuctionMaxPrice:
		return AuctionLot{}, 0, refuseAuction("No price may be more than %d tunar.", AuctionMaxPrice)
	case req.Buyout < req.MinBid:
		return AuctionLot{}, 0, refuseAuction("The buyout must be at least the starting bid.")
	case req.Minutes < least || req.Minutes > AuctionMaxMinutes:
		if req.GM {
			return AuctionLot{}, 0, refuseAuction("An auction lasts %d to %d minutes.", least, AuctionMaxMinutes)
		}
		return AuctionLot{}, 0, refuseAuction("An auction lasts 12 to 72 hours.")
	}
	var lot AuctionLot
	var bank int
	err := db.auctionTx(func(tx *sql.Tx) error {
		row := tx.QueryRow(`SELECT c.itemid, c.stackleft, c.remainHP, c.remaincharge, c.equiploc, c.location, c.listnumber,
			`+itemPatternColumns+`
			FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.itemid = ? AND c.serverid = ?`, req.ItemID, req.Seller)
		e, err := scanItemRow(row.Scan)
		if err == sql.ErrNoRows {
			return refuseAuction("That item is not in your bank.")
		}
		if err != nil {
			return err
		}
		p := &e.Pattern
		switch {
		case e.Location != 1 || e.EquipLoc != -1:
			return refuseAuction("That item is not in your bank.")
		case p.PatternID != req.PatternID || e.StackLeft != req.Stack:
			// the session's list and the row disagree: nothing moves
			return fmt.Errorf("%w: item %d is %d of pattern %d, the list says %d of %d",
				errAuctionChanged, e.ItemID, e.StackLeft, p.PatternID, req.Stack, req.PatternID)
		case e.StackLeft < 1 || e.StackLeft > max(1, p.MaxStack):
			return fmt.Errorf("%w: item %d holds %d, its pattern stacks to %d", errAuctionChanged, e.ItemID, e.StackLeft, p.MaxStack)
		case p.Trade != 0:
			return refuseAuction("No Trade items cannot be put up for auction.")
		case p.Lore != 0 || p.Rent == 0:
			// CLIENT's CanSell, word 1727
			return refuseAuction("Item is not auctionable")
		case req.MinBid < p.ItemCost:
			// DrawSell clamps the starting bid up to the item's value
			return refuseAuction("The starting bid for %s must be at least its value, %d tunar.", p.ItemName, p.ItemCost)
		}
		var open int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM auction_lots WHERE seller = ? AND state = ?`,
			req.Seller, AuctionOpen).Scan(&open); err != nil {
			return err
		}
		if open >= AuctionMaxOpenLots {
			return refuseAuction("You have %d auctions open, the most you may.", open)
		}
		fee := AuctionFee(req.MinBid)
		res, err := tx.Exec(`UPDATE characters SET bankTunar = bankTunar - ? WHERE serverid = ? AND bankTunar >= ?`,
			fee, req.Seller, fee)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			have, _, _ := bankTunarTx(tx, req.Seller)
			return refuseAuction("The listing fee is %d tunar, and your bank holds %d.", fee, have)
		}
		if err := execOneTx(tx, `DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND location = 1
			AND equiploc = -1 AND stackleft = ? AND patternid = ?`,
			e.ItemID, req.Seller, e.StackLeft, p.PatternID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, req.Seller, e.ItemID); err != nil {
			return err
		}
		now := req.Now.Unix()
		ends := (now + int64(req.Minutes)*60 + 59) / 60 * 60
		r, err := tx.Exec(`INSERT INTO auction_lots (seller, seller_name, patternid, stack, remainhp, charges,
			min_bid, buyout, fee, listed_at, ends_at, item_from) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			req.Seller, req.SellerName, p.PatternID, e.StackLeft, e.RemainHP, e.Charges,
			req.MinBid, req.Buyout, fee, now, ends, e.ItemID)
		if err != nil {
			return err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		l, ok, err := auctionLotTx(tx, int(id))
		if err != nil || !ok {
			return fmt.Errorf("auction: lot %d not read back: %v", id, err)
		}
		lot = l
		bank, _, err = bankTunarTx(tx, req.Seller)
		return err
	})
	return lot, bank, err
}

// AuctionBidRequest is a bid (Send06): a proxy, the most the bidder will pay.
type AuctionBidRequest struct {
	Bidder uint32
	Name   string
	Lot    int
	Amount int
	Now    time.Time
}

// AuctionBidKind is what a bid did.
type AuctionBidKind int

const (
	BidFirst     AuctionBidKind = iota // the lot's first: it leads at the starting bid
	BidRaised                          // the leader raised its own proxy
	BidTookLead                        // over the leader's proxy: it leads, the old leader's escrow went back
	BidOutbid                          // at or under the leader's proxy: outbid at once, nothing held
	BidBoughtOut                       // at the buyout: the lot is the bidder's, settled at once
)

// AuctionRefund is an outbid leader's escrow returned to its bank.
type AuctionRefund struct {
	Bidder uint32
	Amount int
	Bank   int  // its bank tunar after
	Gone   bool // its character is gone: nothing was returned
	Bid    AuctionBid
}

// AuctionBidResult is what a bid moved.
type AuctionBidResult struct {
	Kind       AuctionBidKind
	Lot        AuctionLot // after
	Bid        AuctionBid // the bidder's record after
	HadBid     bool       // the bidder had a record on the lot before
	Taken      int        // the tunar taken from the bidder's bank
	BidderBank int
	Refund     *AuctionRefund     // BidTookLead, BidBoughtOut: the old leader's escrow
	Settled    *AuctionSettlement // BidBoughtOut
}

// AuctionBid places a bid, in one transaction: the escrow out of the
// bidder's bank, the old leader's back to its own, the lot changed only as it
// was read, and the lot settled at once by a bid at its buyout.
func (db *SQLiteDB) AuctionBid(req AuctionBidRequest) (AuctionBidResult, error) {
	var res AuctionBidResult
	if req.Amount < 1 {
		return res, refuseAuction("A bid must be at least 1 tunar.")
	}
	err := db.auctionTx(func(tx *sql.Tx) error {
		l, ok, err := auctionLotTx(tx, req.Lot)
		if err != nil {
			return err
		}
		if !ok || !l.Running(req.Now) {
			return refuseAuction("You cannot bid on a closed auction.") // word 1776
		}
		if l.Seller == req.Bidder {
			return refuseAuction("You cannot bid on your own auctions.") // word 1775
		}
		var sellerAcct, bidderAcct int
		if err := tx.QueryRow(`SELECT accountid FROM characters WHERE serverid = ?`, l.Seller).Scan(&sellerAcct); err == sql.ErrNoRows {
			return refuseAuction("You cannot bid on a closed auction.")
		} else if err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT accountid FROM characters WHERE serverid = ?`, req.Bidder).Scan(&bidderAcct); err != nil {
			return err
		}
		if sellerAcct == bidderAcct {
			// an account's characters are one seller: no bidding up one's
			// own lot with another of them
			return refuseAuction("You cannot bid on your own auctions.")
		}
		prior, had, err := auctionBidTx(tx, l.ID, req.Bidder)
		if err != nil {
			return err
		}
		held := 0 // what the house holds for this bidder on this lot
		if l.High == req.Bidder {
			held = l.HighMax
		}
		if prior.Escrow != held {
			return fmt.Errorf("%w: lot %d holds %d for character %d, its leader's proxy says %d",
				errAuctionChanged, l.ID, prior.Escrow, req.Bidder, held)
		}
		amount, kind := req.Amount, BidFirst
		var need int
		switch {
		case amount >= l.Buyout:
			amount, kind = l.Buyout, BidBoughtOut
			need = amount - held
		case l.High == req.Bidder:
			if amount <= l.HighMax {
				return refuseAuction("Your proxy bid on %s is already %d tunar.", l.Item.Pattern.ItemName, l.HighMax)
			}
			kind, need = BidRaised, amount-l.HighMax
		case amount < l.MinNext():
			return refuseAuction("The least you may bid on %s is %d tunar.", l.Item.Pattern.ItemName, l.MinNext())
		case l.High == 0:
			kind, need = BidFirst, amount
		case amount > l.HighMax:
			kind, need = BidTookLead, amount
		default:
			kind = BidOutbid
		}
		bank, _, err := bankTunarTx(tx, req.Bidder)
		if err != nil {
			return err
		}
		// a bid must be in the bank (Prima: "You must have the tunar in the
		// bank for either of these options"), even one outbid at once
		if hold := max(need, amount-held); bank < hold {
			return refuseAuction("Your bank holds %d tunar; that bid needs %d.", bank, hold)
		}
		if need > 0 {
			if err := execOneTx(tx, `UPDATE characters SET bankTunar = bankTunar - ? WHERE serverid = ? AND bankTunar >= ?`,
				need, req.Bidder, need); err != nil {
				return err
			}
		}
		now := req.Now.Unix()
		if l.High != 0 && l.High != req.Bidder && (kind == BidTookLead || kind == BidBoughtOut) {
			rf, err := refundLeaderTx(tx, &l, req.Now)
			if err != nil {
				return err
			}
			if kind == BidBoughtOut {
				err = noticeTx(tx, l.High, req.Now, "%s was bought outright; your %d tunar is back in your bank.",
					l.Item.Pattern.ItemName, l.HighMax)
			} else {
				err = noticeTx(tx, l.High, req.Now, "You have been outbid on %s; your %d tunar is back in your bank.",
					l.Item.Pattern.ItemName, l.HighMax)
			}
			if err != nil {
				return err
			}
			res.Refund = rf
		}
		high, highName, highMax, price := l.High, l.HighName, l.HighMax, l.Price
		switch kind {
		case BidFirst:
			high, highName, highMax, price = req.Bidder, req.Name, amount, l.MinBid
		case BidRaised:
			highMax = amount
		case BidTookLead:
			high, highName, highMax, price = req.Bidder, req.Name, amount, min(amount, l.HighMax+1)
		case BidOutbid:
			price = min(l.HighMax, amount+1)
		case BidBoughtOut:
			high, highName, highMax, price = req.Bidder, req.Name, amount, amount
		}
		if err := execOneTx(tx, `UPDATE auction_lots SET high_bidder = ?, high_name = ?, high_max = ?, price = ?, bids = bids + 1
			WHERE lot = ? AND state = ? AND high_bidder = ? AND high_max = ? AND price = ? AND bids = ?`,
			high, highName, highMax, price, l.ID, AuctionOpen, l.High, l.HighMax, l.Price, l.Bids); err != nil {
			return err
		}
		escrow := 0
		if high == req.Bidder {
			escrow = highMax
		}
		if _, err := tx.Exec(`INSERT INTO auction_bids (lot, bidder, bidder_name, max_bid, escrow, placed_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(lot, bidder) DO UPDATE SET bidder_name = excluded.bidder_name, max_bid = excluded.max_bid,
				escrow = excluded.escrow, placed_at = excluded.placed_at`,
			l.ID, req.Bidder, req.Name, amount, escrow, now); err != nil {
			return err
		}
		after, _, err := auctionLotTx(tx, l.ID)
		if err != nil {
			return err
		}
		if kind == BidBoughtOut {
			st, err := settleTx(tx, after, req.Now)
			if err != nil {
				return err
			}
			res.Settled = &st
			after = st.Lot
		}
		res.Kind, res.Lot, res.HadBid, res.Taken = kind, after, had, need
		if res.Bid, _, err = auctionBidTx(tx, l.ID, req.Bidder); err != nil {
			return err
		}
		res.BidderBank, _, err = bankTunarTx(tx, req.Bidder)
		return err
	})
	return res, err
}

// refundLeaderTx gives a lot's leader its escrow back, exactly once: only
// while the house holds it.
func refundLeaderTx(tx sqlRunner, l *AuctionLot, now time.Time) (*AuctionRefund, error) {
	if err := execOneTx(tx, `UPDATE auction_bids SET escrow = 0 WHERE lot = ? AND bidder = ? AND escrow = ?`,
		l.ID, l.High, l.HighMax); err != nil {
		return nil, err
	}
	ok, err := creditBankTx(tx, l.High, l.HighMax)
	if err != nil {
		return nil, err
	}
	rf := &AuctionRefund{Bidder: l.High, Amount: l.HighMax, Gone: !ok}
	if rf.Bank, _, err = bankTunarTx(tx, l.High); err != nil {
		return nil, err
	}
	if rf.Bid, _, err = auctionBidTx(tx, l.ID, l.High); err != nil {
		return nil, err
	}
	return rf, nil
}

// AuctionSettlement is what closing a lot moved.
type AuctionSettlement struct {
	Lot        AuctionLot // after
	Winner     uint32     // 0: unsold
	WinnerBank int
	Refund     int // the winner's escrow over the price, back to its bank
	WinnerBid  AuctionBid
	SellerBank int
	SellerGone bool
	Delivery   *AuctionDelivery // the item reached its holder's bank
}

// AuctionSettle closes one lot whose time is up (or any open lot, now
// ending it), in one transaction; a lot already closed is not settled again
// (ok false).
func (db *SQLiteDB) AuctionSettle(lot int, now time.Time) (AuctionSettlement, bool, error) {
	var st AuctionSettlement
	settled := false
	err := db.auctionTx(func(tx *sql.Tx) error {
		l, ok, err := auctionLotTx(tx, lot)
		if err != nil || !ok || l.State != AuctionOpen {
			return err
		}
		if st, err = settleTx(tx, l, now); err != nil {
			return err
		}
		settled = true
		return nil
	})
	return st, settled, err
}

// settleTx closes an open lot: the winner's escrow pays the seller the price
// and the rest goes back to the winner's bank, the item to the winner's bank
// (or, unsold, back to the seller's) when there is room, and the lines for
// both. The state's change is conditional on the lot still being open, so a
// lot settles once.
func settleTx(tx sqlRunner, l AuctionLot, now time.Time) (AuctionSettlement, error) {
	st := AuctionSettlement{}
	winner := l.High
	if winner != 0 {
		if _, there, err := bankTunarTx(tx, winner); err != nil {
			return st, err
		} else if !there {
			// the winner's character is gone, and its escrow with it
			if _, err := tx.Exec(`UPDATE auction_bids SET escrow = 0 WHERE lot = ? AND bidder = ?`, l.ID, winner); err != nil {
				return st, err
			}
			winner = 0
		}
	}
	state, holder := AuctionUnsold, l.Seller
	if winner != 0 {
		state, holder = AuctionSold, winner
	}
	ends := l.EndsAt
	if t := now.Unix() / 60 * 60; t < ends {
		ends = t // bought out: the time since close counts from now
	}
	if err := execOneTx(tx, `UPDATE auction_lots SET state = ?, closed_at = ?, holder = ?, ends_at = ?
		WHERE lot = ? AND state = ?`, state, now.Unix(), holder, ends, l.ID, AuctionOpen); err != nil {
		return st, err
	}
	name := l.Item.Pattern.ItemName
	if winner != 0 {
		if err := execOneTx(tx, `UPDATE auction_bids SET escrow = 0 WHERE lot = ? AND bidder = ? AND escrow = ?`,
			l.ID, winner, l.HighMax); err != nil {
			return st, err
		}
		st.Winner, st.Refund = winner, l.HighMax-l.Price
		if _, err := creditBankTx(tx, winner, st.Refund); err != nil {
			return st, err
		}
		ok, err := creditBankTx(tx, l.Seller, l.Price)
		if err != nil {
			return st, err
		}
		st.SellerGone = !ok
		if err := noticeTx(tx, l.Seller, now, "Your auction of %s sold to %s for %d tunar; the tunar is in your bank.",
			name, l.HighName, l.Price); err != nil {
			return st, err
		}
	} else if _, there, err := bankTunarTx(tx, l.Seller); err != nil {
		return st, err
	} else {
		st.SellerGone = !there
	}
	// nothing else of the lot's may still be held
	var stray sql.NullInt64
	if err := tx.QueryRow(`SELECT SUM(escrow) FROM auction_bids WHERE lot = ?`, l.ID).Scan(&stray); err != nil {
		return st, err
	}
	if stray.Int64 != 0 {
		return st, fmt.Errorf("%w: lot %d still holds %d tunar of other bidders", errAuctionChanged, l.ID, stray.Int64)
	}
	after, _, err := auctionLotTx(tx, l.ID)
	if err != nil {
		return st, err
	}
	d, err := deliverTx(tx, &after, now)
	if err != nil {
		return st, err
	}
	st.Delivery = d
	where := "It is in your bank."
	if d == nil && !after.Delivered {
		where = "It waits at the auction house for room in your bank."
	}
	if winner != 0 {
		if err := noticeTx(tx, winner, now, "You won the auction of %s for %d tunar. %s", name, l.Price, where); err != nil {
			return st, err
		}
	} else if !st.SellerGone {
		if err := noticeTx(tx, l.Seller, now, "Your auction of %s ended with no bids. %s", name, where); err != nil {
			return st, err
		}
	}
	if st.Lot, _, err = auctionLotTx(tx, l.ID); err != nil {
		return st, err
	}
	if winner != 0 {
		if st.WinnerBank, _, err = bankTunarTx(tx, winner); err != nil {
			return st, err
		}
		if st.WinnerBid, _, err = auctionBidTx(tx, l.ID, winner); err != nil {
			return st, err
		}
	}
	if !st.SellerGone {
		st.SellerBank, _, err = bankTunarTx(tx, l.Seller)
	}
	return st, err
}

// AuctionDelivery is a lot's item filed into its holder's bank.
type AuctionDelivery struct {
	Lot    int
	Holder uint32
	// Item is the record as it arrives (its pattern, condition and
	// charges; StackLeft how many came, ItemID the row they are in).
	Item   CharInventoryEntry
	Merged bool // onto a stack of the same item already there
}

// deliverTx files a closed lot's item into its holder's bank, as CLIENT's
// AddItem(item, maxStack, 40) would keep it: a stackable item onto the first
// record of the same item when the whole stack fits there, else a new record
// while the bank holds fewer than 40. It returns nil when the item must wait
// (no room), and marks a lot whose holder is gone delivered with nothing to
// deliver to. The lot is marked delivered only while it is not, so an item
// is filed once.
func deliverTx(tx sqlRunner, l *AuctionLot, now time.Time) (*AuctionDelivery, error) {
	if l.State == AuctionOpen || l.Delivered {
		return nil, nil
	}
	if l.Holder != 0 {
		if _, there, err := bankTunarTx(tx, l.Holder); err != nil {
			return nil, err
		} else if !there {
			l.Holder = 0
		}
	}
	if l.Holder == 0 {
		// nobody to take it: the item goes with the character that is gone
		if err := execOneTx(tx, `UPDATE auction_lots SET delivered = 1 WHERE lot = ? AND delivered = 0`, l.ID); err != nil {
			return nil, err
		}
		l.Delivered = true
		return nil, nil
	}
	rows, err := tx.Query(`SELECT itemid, patternid, stackleft FROM charinventory
		WHERE serverid = ? AND location = 1 ORDER BY listnumber, itemid`, l.Holder)
	if err != nil {
		return nil, err
	}
	type bankRow struct{ itemID, pattern, stack int }
	var bank []bankRow
	for rows.Next() {
		var r bankRow
		if err := rows.Scan(&r.itemID, &r.pattern, &r.stack); err != nil {
			rows.Close()
			return nil, err
		}
		bank = append(bank, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	p := &l.Item.Pattern
	n := l.Item.StackLeft
	d := &AuctionDelivery{Lot: l.ID, Holder: l.Holder, Item: l.Item}
	d.Item.StackLeft, d.Item.EquipLoc, d.Item.Location = n, -1, -1
	onto := -1
	if p.MaxStack >= 2 {
		for i := range bank {
			if bank[i].pattern == p.PatternID {
				onto = i
				break
			}
		}
	}
	switch {
	case onto >= 0:
		if bank[onto].stack+n > p.MaxStack {
			return nil, nil
		}
		if err := execOneTx(tx, `UPDATE charinventory SET stackleft = stackleft + ?
			WHERE itemid = ? AND serverid = ? AND location = 1 AND stackleft = ?`,
			n, bank[onto].itemID, l.Holder, bank[onto].stack); err != nil {
			return nil, err
		}
		d.Item.ItemID, d.Merged = bank[onto].itemID, true
	case len(bank) >= AuctionBankSlots:
		return nil, nil
	default:
		r, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, ?, ?, ?, ?, -1, 1,
				(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 1))`,
			l.Holder, n, l.Item.RemainHP, l.Item.Charges, p.PatternID, l.Holder)
		if err != nil {
			return nil, err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return nil, err
		}
		d.Item.ItemID = int(id)
	}
	if err := execOneTx(tx, `UPDATE auction_lots SET delivered = 1, item_to = ? WHERE lot = ? AND delivered = 0`,
		d.Item.ItemID, l.ID); err != nil {
		return nil, err
	}
	l.Delivered, l.ItemTo = true, d.Item.ItemID
	return d, nil
}

// AuctionDeliverPending files every closed lot's item that waits for its
// holder's bank, each in its own transaction, and returns what arrived and
// how many still wait.
func (db *SQLiteDB) AuctionDeliverPending(holder uint32, now time.Time) ([]AuctionDelivery, int, error) {
	waiting, err := auctionLots(db, `WHERE l.holder = ? AND l.delivered = 0 AND l.state != ? ORDER BY l.lot`, holder, AuctionOpen)
	if err != nil {
		return nil, 0, err
	}
	var out []AuctionDelivery
	left := 0
	for _, w := range waiting {
		var d *AuctionDelivery
		err := db.auctionTx(func(tx *sql.Tx) error {
			l, ok, err := auctionLotTx(tx, w.ID)
			if err != nil || !ok || l.Delivered || l.Holder != holder {
				return err
			}
			d, err = deliverTx(tx, &l, now)
			if err == nil && d != nil {
				err = noticeTx(tx, holder, now, "%s from the auction house is in your bank.", l.Item.Pattern.ItemName)
			}
			return err
		})
		if err != nil {
			return out, left, err
		}
		if d != nil {
			out = append(out, *d)
		} else {
			left++
		}
	}
	return out, left, nil
}

// AuctionWaiting is how many closed lots' items wait for room in a
// character's bank.
func (db *SQLiteDB) AuctionWaiting(holder uint32) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM auction_lots WHERE holder = ? AND delivered = 0 AND state != ?`,
		holder, AuctionOpen).Scan(&n)
	return n, err
}

// AuctionSweep is what one pass of the house moved.
type AuctionSweep struct {
	Settled   []AuctionSettlement
	Refunds   []AuctionRefund // escrow returned from lots cancelled
	Cancelled []AuctionLot
}

// AuctionSettleDue closes every lot whose time is up (a restart settles what
// ended while the server was down), each in its own transaction, after the
// orphans: an open lot whose seller's character is gone is cancelled (its
// leader's escrow back, the item gone with the seller's other things), a lead
// whose bidder's character is gone is dropped (its escrow gone with it), and
// a closed lot's item whose holder is gone is marked delivered.
func (db *SQLiteDB) AuctionSettleDue(now time.Time) (AuctionSweep, error) {
	var sw AuctionSweep
	orphans, err := auctionLots(db, `WHERE l.state = ? AND NOT EXISTS
		(SELECT 1 FROM characters c WHERE c.serverid = l.seller) ORDER BY l.lot`, AuctionOpen)
	if err != nil {
		return sw, err
	}
	for _, o := range orphans {
		var rf *AuctionRefund
		var after AuctionLot
		err := db.auctionTx(func(tx *sql.Tx) error {
			l, ok, err := auctionLotTx(tx, o.ID)
			if err != nil || !ok || l.State != AuctionOpen {
				return err
			}
			if l.High != 0 {
				if rf, err = refundLeaderTx(tx, &l, now); err != nil {
					return err
				}
				if err := noticeTx(tx, l.High, now, "The auction of %s was withdrawn; your %d tunar is back in your bank.",
					l.Item.Pattern.ItemName, l.HighMax); err != nil {
					return err
				}
			}
			if err := execOneTx(tx, `UPDATE auction_lots SET state = ?, closed_at = ?, holder = 0, delivered = 1
				WHERE lot = ? AND state = ?`, AuctionCancelled, now.Unix(), l.ID, AuctionOpen); err != nil {
				return err
			}
			after, _, err = auctionLotTx(tx, l.ID)
			return err
		})
		if err != nil {
			return sw, err
		}
		if after.ID != 0 {
			sw.Cancelled = append(sw.Cancelled, after)
		}
		if rf != nil {
			sw.Refunds = append(sw.Refunds, *rf)
		}
	}
	// a lead whose character is gone: the lot goes on as if it had no bids,
	// and what was held for it went with the character (looked for first: a
	// sweep that finds nothing writes nothing)
	var gone int
	if err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM auction_lots l WHERE l.state = ? AND l.high_bidder != 0 AND NOT EXISTS
			(SELECT 1 FROM characters c WHERE c.serverid = l.high_bidder)) +
		(SELECT COUNT(*) FROM auction_bids b WHERE b.escrow != 0 AND NOT EXISTS
			(SELECT 1 FROM characters c WHERE c.serverid = b.bidder)) +
		(SELECT COUNT(*) FROM auction_lots l WHERE l.state != ? AND l.delivered = 0 AND NOT EXISTS
			(SELECT 1 FROM characters c WHERE c.serverid = l.holder))`, AuctionOpen, AuctionOpen).Scan(&gone); err != nil {
		return sw, err
	}
	if gone > 0 {
		if err := db.auctionTx(func(tx *sql.Tx) error {
			if _, err := tx.Exec(`UPDATE auction_lots SET high_bidder = 0, high_name = '', high_max = 0, price = 0
				WHERE state = ? AND high_bidder != 0 AND NOT EXISTS
				(SELECT 1 FROM characters c WHERE c.serverid = auction_lots.high_bidder)`, AuctionOpen); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE auction_bids SET escrow = 0 WHERE escrow != 0 AND NOT EXISTS
				(SELECT 1 FROM characters c WHERE c.serverid = auction_bids.bidder)`); err != nil {
				return err
			}
			_, err := tx.Exec(`UPDATE auction_lots SET delivered = 1 WHERE state != ? AND delivered = 0 AND NOT EXISTS
				(SELECT 1 FROM characters c WHERE c.serverid = auction_lots.holder)`, AuctionOpen)
			return err
		}); err != nil {
			return sw, err
		}
	}
	due, err := auctionLots(db, `WHERE l.state = ? AND l.ends_at <= ? ORDER BY l.ends_at, l.lot`, AuctionOpen, now.Unix())
	if err != nil {
		return sw, err
	}
	for _, l := range due {
		st, ok, err := db.AuctionSettle(l.ID, now)
		if err != nil {
			return sw, err
		}
		if ok {
			sw.Settled = append(sw.Settled, st)
		}
	}
	return sw, nil
}

// AuctionOpenLots is every lot taking bids, newest first.
func (db *SQLiteDB) AuctionOpenLots(now time.Time) ([]AuctionLot, error) {
	return auctionLots(db, `WHERE l.state = ? AND l.ends_at > ? ORDER BY l.lot DESC`, AuctionOpen, now.Unix())
}

// AuctionClosedLots is every lot sold or unsold since then (or whose time is
// up), newest first: a search's "Status: Closed".
func (db *SQLiteDB) AuctionClosedLots(now time.Time, since time.Time) ([]AuctionLot, error) {
	return auctionLots(db, `WHERE (l.state = ? AND l.ends_at <= ?) OR (l.state IN (?, ?) AND l.closed_at >= ?)
		ORDER BY l.lot DESC`, AuctionOpen, now.Unix(), AuctionSold, AuctionUnsold, since.Unix())
}

// AuctionLotsByID is the lots of those ids, in no order.
func (db *SQLiteDB) AuctionLotsByID(ids []int) ([]AuctionLot, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return auctionLots(db, `WHERE l.lot IN (`+marks+`)`, args...)
}

// AuctionLotsOf is a seller's lots open or closed since then, newest first,
// at most limit.
func (db *SQLiteDB) AuctionLotsOf(seller uint32, since time.Time, limit int) ([]AuctionLot, error) {
	return auctionLots(db, `WHERE l.seller = ? AND (l.state = ? OR l.closed_at >= ?) ORDER BY l.lot DESC LIMIT ?`,
		seller, AuctionOpen, since.Unix(), limit)
}

// AuctionBidsOf is a bidder's records on lots open or closed since then,
// newest first, at most limit.
func (db *SQLiteDB) AuctionBidsOf(bidder uint32, since time.Time, limit int) ([]AuctionBid, error) {
	return auctionBids(db, `JOIN auction_lots l ON l.lot = b.lot
		WHERE b.bidder = ? AND (l.state = ? OR l.closed_at >= ?) ORDER BY b.lot DESC LIMIT ?`,
		bidder, AuctionOpen, since.Unix(), limit)
}

// AuctionBidders is every bidder's record on a lot, the highest proxy first.
func (db *SQLiteDB) AuctionBidders(lot int) ([]AuctionBid, error) {
	return auctionBids(db, `WHERE b.lot = ? ORDER BY b.max_bid DESC, b.placed_at`, lot)
}

func auctionBids(q sqlRunner, where string, args ...any) ([]AuctionBid, error) {
	rows, err := q.Query(`SELECT b.lot, b.bidder, b.bidder_name, b.max_bid, b.escrow, b.placed_at
		FROM auction_bids b `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuctionBid
	for rows.Next() {
		var b AuctionBid
		if err := rows.Scan(&b.Lot, &b.Bidder, &b.Name, &b.Max, &b.Escrow, &b.PlacedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AuctionNotices is what a character is yet to be told, oldest first.
func (db *SQLiteDB) AuctionNotices(serverID uint32) ([]AuctionNotice, error) {
	rows, err := db.Query(`SELECT id, line FROM auction_notices WHERE serverid = ? ORDER BY id`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuctionNotice
	for rows.Next() {
		var n AuctionNotice
		if err := rows.Scan(&n.ID, &n.Line); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AuctionNoticesDone forgets notices told.
func (db *SQLiteDB) AuctionNoticesDone(ids []int) error {
	for _, id := range ids {
		if _, err := db.Exec(`DELETE FROM auction_notices WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// AuctionAudit checks what the house holds against what its rows say: an
// open lot holds exactly its leader's proxy, in the leader's record alone,
// its price within the proxy and not under the starting bid; a closed lot
// holds nothing. It returns every problem found (none: nil).
func (db *SQLiteDB) AuctionAudit() ([]string, error) {
	var problems []string
	rows, err := db.Query(`SELECT l.lot, l.state, l.high_bidder, l.high_max, l.price, l.min_bid,
		COALESCE(SUM(b.escrow), 0), COALESCE(SUM(CASE WHEN b.bidder = l.high_bidder THEN b.escrow ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN b.escrow < 0 THEN 1 ELSE 0 END), 0)
		FROM auction_lots l LEFT JOIN auction_bids b ON b.lot = l.lot GROUP BY l.lot ORDER BY l.lot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var lot, high, highMax, price, minBid, held, leaderHeld, negative int
		var state string
		if err := rows.Scan(&lot, &state, &high, &highMax, &price, &minBid, &held, &leaderHeld, &negative); err != nil {
			return nil, err
		}
		switch {
		case negative > 0:
			problems = append(problems, fmt.Sprintf("lot %d: a negative escrow", lot))
		case state != AuctionOpen && held != 0:
			problems = append(problems, fmt.Sprintf("lot %d is %s and still holds %d", lot, state, held))
		case state == AuctionOpen && high == 0 && held != 0:
			problems = append(problems, fmt.Sprintf("lot %d has no leader and holds %d", lot, held))
		case state == AuctionOpen && high != 0 && (held != highMax || leaderHeld != highMax):
			problems = append(problems, fmt.Sprintf("lot %d holds %d (%d its leader's), its leader's proxy is %d", lot, held, leaderHeld, highMax))
		case state == AuctionOpen && high != 0 && (price > highMax || price < minBid):
			problems = append(problems, fmt.Sprintf("lot %d's price %d is outside %d..%d", lot, price, minBid, highMax))
		}
	}
	return problems, rows.Err()
}
