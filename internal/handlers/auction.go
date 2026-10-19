package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The auction house, minimal and in memory: enough to drive CLIENT's
// windows (VIBankControl.cpp, viwndauction.cpp, viwndauctionsearch.cpp).
// No capture holds an auction session: the formats are CLIENT's [A].
//
//	C>S 0x1205 5  u32 the banker, u32 the bank record's key, zz count,
//	              zz minimum bid, zz buyout, zz minutes              Sell
//	C>S 0x1205 6  zz lot, zz amount, zz count                         Send06 (a bid)
//	C>S 0x1205 7  zz page, zz the lot it starts from, u8 forward,
//	              the query (u8, str, wide, u32, u32, 6 zz, u8)       Search/NextPage/PrevPage
//	C>S 0x1205 8  zz page, zz count, zz lot ids                       RequestLots/RequestBids
//	C>S 0x1205 9  zz lot                                              Send09 (the bidders)
//	S>C 0x1206 129 zz page, zz minutes, the lots, then three zz arrays
//	              (the high bidder, the high bid, the bid count)      RecvResults
//	S>C 0x1206 130 a lot: the seller's (RecvAddLot)
//	S>C 0x1206 132 a bid row: the bidder's (RecvAddBid)
//	S>C 0x1206 135 zz lot, the bidders                                RecvRemoteChars
//
// A lot's item stays in the seller's bank in the database: the house only
// takes it off the session's bank list, so a restart puts it back. Nothing
// is ever sold or settled here.

type auctionLot struct {
	id       int
	sellerID int
	seller   string
	entry    database.CharInventoryEntry
	qty      int
	price    int
	buyout   int
	ends     int // the house's minutes
	highID   int // the high bidder's character, 0 for none
	high     int
	bids     int
	bidders  []auctionBidder
}

type auctionBidder struct {
	charID int
	name   string
	amount int
}

type auctionHouse struct {
	mu   sync.Mutex
	lots []*auctionLot
	next int
	t0   time.Time
}

var house = &auctionHouse{next: 1, t0: time.Now()}

// minutes is the house's clock, which the lots' ends and every page carry.
func (a *auctionHouse) minutes() int {
	return int(time.Since(a.t0) / time.Minute)
}

// writeLot is operator<<(VIMemStream &, const VIAuctionItem &): seven zz
// words (the lot, the stack, hit points, charges, minimum bid, buyout, the
// end), the seller, the item.
func writeLot(w *buffer.Writer, l *auctionLot) {
	for _, v := range []int{l.id, l.qty, l.entry.RemainHP, l.entry.Charges, l.price, l.buyout, l.ends} {
		w.Write7BitEncodedInt64(int64(v))
	}
	w.WriteStringUTF8(l.seller)
	writeItemPattern(w, &l.entry)
}

func writeInts(w *buffer.Writer, v []int) {
	w.Write7BitEncodedInt64(int64(len(v)))
	for _, x := range v {
		w.Write7BitEncodedInt64(int64(x))
	}
}

// resultsPayload is 0x1206 129: a page of lots and their three columns.
func (a *auctionHouse) resultsPayload(page int, lots []*auctionLot) []byte {
	return build(protocol.OpcodeAuctionStuff, 8192, func(w *buffer.Writer) {
		w.WriteByte(129)
		w.Write7BitEncodedInt64(int64(page))
		w.Write7BitEncodedInt64(int64(a.minutes()))
		w.Write7BitEncodedInt64(int64(len(lots)))
		var owner, high, n []int
		for _, l := range lots {
			writeLot(w, l)
			owner = append(owner, l.highID)
			high = append(high, l.high)
			n = append(n, l.bids)
		}
		writeInts(w, owner)
		writeInts(w, high)
		writeInts(w, n)
	})
}

// auctionPageSize is how many lots a page holds (the result menu's rows).
const auctionPageSize = 40

// HandleAuction is 0x1205.
func (h *NPCWindowHandler) HandleAuction(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil {
		return nil
	}
	player := h.playerOf(sess)
	if player == nil {
		return nil
	}
	zz := func() int {
		v, _ := r.Read7BitEncodedInt64()
		return int(v)
	}
	switch sub {
	case 5:
		id, _ := r.ReadUint32()
		key, _ := r.ReadUint32()
		_, price, buyout, minutes := zz(), zz(), zz(), zz()
		if !h.auctionSell(sess, player, id, key, price, buyout, minutes) {
			refuse(sess)
		}
	case 7:
		page, from := zz(), zz()
		fwd, _ := r.ReadByte()
		house.mu.Lock()
		var lots []*auctionLot
		start := 0
		for i, l := range house.lots {
			if l.id == from {
				if fwd != 0 {
					start = i + 1
				} else {
					start = i - auctionPageSize
				}
			}
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < len(house.lots) && len(lots) < auctionPageSize; i++ {
			lots = append(lots, house.lots[i])
		}
		payload := house.resultsPayload(page, lots)
		house.mu.Unlock()
		queueReliable(sess, protocol.OpcodeAuctionStuff, payload)
		log.Printf("Auction: %s searched, page %d, %d lots", player.Name, page, len(lots))
	case 8:
		page, n := zz(), zz()
		want := map[int]bool{}
		for i := 0; i < n && i < 1000; i++ {
			want[zz()] = true
		}
		house.mu.Lock()
		var lots []*auctionLot
		for _, l := range house.lots {
			if want[l.id] {
				lots = append(lots, l)
			}
		}
		payload := house.resultsPayload(page, lots)
		house.mu.Unlock()
		queueReliable(sess, protocol.OpcodeAuctionStuff, payload)
	case 6:
		lot, amount := zz(), zz()
		house.mu.Lock()
		var l *auctionLot
		for _, x := range house.lots {
			if x.id == lot {
				l = x
			}
		}
		ok := l != nil && amount >= l.price && amount > l.high && amount <= player.Tunar
		if ok {
			l.high, l.highID = amount, int(player.CharServerID)
			l.bids++
			l.bidders = append(l.bidders, auctionBidder{int(player.CharServerID), player.Name, amount})
		}
		house.mu.Unlock()
		if !ok {
			refuse(sess)
			return nil
		}
		queueReliable(sess, protocol.OpcodeAuctionStuff, build(protocol.OpcodeAuctionStuff, 64, func(w *buffer.Writer) {
			w.WriteByte(132)
			for _, v := range []int{int(player.CharServerID), lot, amount, 0, amount} {
				w.Write7BitEncodedInt64(int64(v))
			}
			w.WriteByte(0)
		}))
		log.Printf("Auction: %s bids %d on lot %d", player.Name, amount, lot)
	case 9:
		lot := zz()
		house.mu.Lock()
		var bidders []auctionBidder
		for _, x := range house.lots {
			if x.id == lot {
				bidders = append(bidders, x.bidders...)
			}
		}
		house.mu.Unlock()
		queueReliable(sess, protocol.OpcodeAuctionStuff, build(protocol.OpcodeAuctionStuff, 2048, func(w *buffer.Writer) {
			w.WriteByte(135)
			w.Write7BitEncodedInt64(int64(lot))
			w.Write7BitEncodedInt64(int64(len(bidders)))
			for _, b := range bidders {
				w.Write7BitEncodedInt64(int64(b.charID))
				w.Write7BitEncodedInt64(int64(lot))
				w.Write7BitEncodedInt64(0)
				w.WriteStringUTF8(b.name)
				w.Write7BitEncodedInt64(int64(b.amount))
				w.Write7BitEncodedInt64(0)
			}
		}))
	default:
		log.Printf("Auction: %s sent sub-op %d, not handled", player.Name, sub)
		refuse(sess)
	}
	return nil
}

// auctionSell puts a bank record up: off the session's bank list (0x1250),
// a new lot to the seller (0x1206 130).
func (h *NPCWindowHandler) auctionSell(sess *session.Session, player *entity.Player, id, key uint32, price, buyout, minutes int) bool {
	_, _, ok := h.windowNPC(sess, id, npcFlagBank, "AuctionSell")
	bank := sessionBankList(sess)
	if !ok || h.inv == nil || bank == nil {
		return false
	}
	i := bank.index(key)
	rec, found := bank.at(i)
	if !found {
		return false
	}
	row, err := h.inv.GetCharItem(player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("AuctionSell: %s: %v", player.Name, err)
		return false
	}
	if !bank.took(i, rec.qty) {
		return false
	}
	house.mu.Lock()
	l := &auctionLot{id: house.next, sellerID: int(player.CharServerID), seller: player.Name, entry: wireRecord(row), qty: rec.qty,
		price: price, buyout: buyout, ends: house.minutes() + minutes}
	house.next++
	house.lots = append(house.lots, l)
	payload := build(protocol.OpcodeAuctionStuff, 2048, func(w *buffer.Writer) {
		w.WriteByte(130)
		writeLot(w, l)
	})
	house.mu.Unlock()
	queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, rec.qty))
	queueReliable(sess, protocol.OpcodeAuctionStuff, payload)
	log.Printf("Auction: %s put up %q as lot %d (%d / %d, %d minutes)", player.Name, row.Pattern.ItemName, l.id, price, buyout, minutes)
	return true
}
